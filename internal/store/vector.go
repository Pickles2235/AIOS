package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	vectorpkg "github.com/AdamNi-7080/AIOS/internal/vector"
)

const vectorSchemaVersion = "vector-v1"

type VectorOptions struct {
	Enabled    bool
	Dimensions int
	Embedder   vectorpkg.Embedder
	DataDir    string
}

func (o VectorOptions) normalized() (VectorOptions, error) {
	if !o.Enabled {
		return o, nil
	}
	if o.Embedder == nil {
		o.Embedder = vectorpkg.NewBundled(o.DataDir)
	}
	if o.Embedder.Dimensions() < 8 {
		return o, fmt.Errorf("vector dimensions must be at least 8")
	}
	return o, nil
}

// RebuildVectorProjection reads only persisted active canonical IR. A failure
// is recorded as a failed derived build and does not affect canonical reads.
func (s *Store) RebuildVectorProjection(ctx context.Context, options VectorOptions, reason string) error {
	if s.readOnly {
		return fmt.Errorf("store is read-only")
	}
	o, err := options.normalized()
	if err != nil {
		return err
	}
	r, err := s.ActiveCatalogRevision(ctx)
	if err != nil {
		return fmt.Errorf("cannot rebuild vector projection: no active IR catalog")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	fingerprint, err := catalogFingerprint(ctx, tx, r.ID)
	if err != nil {
		return err
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	identity, dimensions := "disabled", 0
	if o.Enabled {
		identity, dimensions = o.Embedder.Identity(), o.Embedder.Dimensions()
	}
	input := digest(vectorSchemaVersion, fingerprint, identity, fmt.Sprint(dimensions))
	buildID := "pb-" + digest("vector", r.ID, input, now)
	if _, err = tx.ExecContext(ctx, `INSERT INTO projection_builds(projection_build_id,projection_kind,projection_schema_version,catalog_revision_id,source_ir_fingerprint,builder_name,builder_version,input_fingerprint,projection_fingerprint,created_at,activated_at,state,record_counts,validation_summary,rebuild_reason) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`, buildID, "vector", vectorSchemaVersion, r.ID, fingerprint, "local-vector-builder", identity, input, "", now, nil, "staged", `{}`, `{"valid":false}`, reason); err != nil {
		return err
	}
	if !o.Enabled {
		_, err = tx.ExecContext(ctx, `UPDATE projection_builds SET state='disabled',validation_summary='{"valid":true,"enabled":false}' WHERE projection_build_id=?`, buildID)
		if err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, `DELETE FROM active_projection_builds WHERE projection_kind='vector'`)
		if err != nil {
			return err
		}
		return tx.Commit()
	}
	rows, err := tx.QueryContext(ctx, `SELECT e.entity_id,e.canonical_entity_id,e.generation_id,e.evidence_id,v.file_sha256,v.start_byte,v.end_byte,e.label,e.kind,e.identity,v.excerpt FROM entities e JOIN evidence v ON v.evidence_id=e.evidence_id WHERE e.generation_id IN (SELECT generation_id FROM catalog_revision_members WHERE catalog_revision_id=?) ORDER BY e.repo_id,e.path,e.entity_id`, r.ID)
	if err != nil {
		return err
	}
	defer rows.Close()
	indexed, reused := 0, 0
	for rows.Next() {
		var entity, canonical, generation, evidence, hash, label, kind, entityIdentity, excerpt string
		var start, end int
		if err = rows.Scan(&entity, &canonical, &generation, &evidence, &hash, &start, &end, &label, &kind, &entityIdentity, &excerpt); err != nil {
			return err
		}
		// llama-embedding treats literal newlines as separate prompts. Collapse
		// canonical fields into one deterministic prompt so every record produces
		// exactly one embedding vector.
		text := strings.Join(strings.Fields(label+" "+kind+" "+entityIdentity+" "+excerpt), " ")
		content := digest(canonical, hash, fmt.Sprint(start), fmt.Sprint(end), text)
		var embeddingID string
		err = tx.QueryRowContext(ctx, `SELECT embedding_id FROM vector_embedding_cache WHERE model_identity=? AND dimensions=? AND input_fingerprint=?`, identity, dimensions, content).Scan(&embeddingID)
		if err == sql.ErrNoRows {
			v, e := vectorpkg.Embed(ctx, o.Embedder, text)
			if e != nil {
				return e
			}
			if len(v) != dimensions {
				return fmt.Errorf("embedder returned %d dimensions, expected %d", len(v), dimensions)
			}
			payload, _ := json.Marshal(v)
			embeddingID = "ve-" + digest(identity, fmt.Sprint(dimensions), content)
			if _, err = tx.ExecContext(ctx, `INSERT INTO vector_embedding_cache(embedding_id,model_identity,dimensions,input_fingerprint,vector_json,created_at) VALUES(?,?,?,?,?,?)`, embeddingID, identity, dimensions, content, string(payload), now); err != nil {
				return err
			}
		} else if err != nil {
			return err
		} else {
			reused++
		}
		recordID := "vr-" + digest(canonical, hash, fmt.Sprint(start), fmt.Sprint(end), identity, vectorSchemaVersion)
		if _, err = tx.ExecContext(ctx, `INSERT INTO projection_vector_records VALUES(?,?,?,?,?,?,?)`, buildID, recordID, generation, entity, evidence, embeddingID, content); err != nil {
			return err
		}
		indexed++
	}
	if err = rows.Err(); err != nil {
		return err
	}
	counts := fmt.Sprintf(`{"records":%d,"reused":%d,"dimensions":%d}`, indexed, reused, dimensions)
	projection := digest("vector", vectorSchemaVersion, input, counts)
	validation := fmt.Sprintf(`{"valid":true,"enabled":true,"model_identity":%q,"dimensions":%d}`, identity, dimensions)
	if _, err = tx.ExecContext(ctx, `UPDATE projection_builds SET projection_fingerprint=?,activated_at=?,state='ready',record_counts=?,validation_summary=? WHERE projection_build_id=?`, projection, now, counts, validation, buildID); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO active_projection_builds(projection_kind,projection_build_id) VALUES('vector',?) ON CONFLICT(projection_kind) DO UPDATE SET projection_build_id=excluded.projection_build_id`, buildID); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) ResetVectorProjection(ctx context.Context) error {
	if s.readOnly {
		return fmt.Errorf("store is read-only")
	}
	_, err := s.db.ExecContext(ctx, `DELETE FROM active_projection_builds WHERE projection_kind='vector'`)
	return err
}

// VectorCandidates always returns canonical records. Scores are ranking aids.
func (s *Store) VectorCandidates(ctx context.Context, query string, filter QueryFilter, embedder vectorpkg.Embedder, limit int) ([]Candidate, []float64, error) {
	if err := s.assertActiveVersion(ctx, filter); err != nil {
		return nil, nil, err
	}
	if err := s.requireProjection(ctx, "vector"); err != nil {
		return nil, nil, err
	}
	qv, err := vectorpkg.Embed(ctx, embedder, query)
	if err != nil {
		return nil, nil, err
	}
	where, args := canonicalWhere(filter)
	rows, err := s.db.QueryContext(ctx, `SELECT e.entity_id,e.generation_id,e.repo_id,e.kind,e.label,e.path,e.language,e.evidence_id,v.source_id,v.path,v.file_sha256,v.excerpt,v.start_byte,v.end_byte,v.start_line,v.start_column,v.end_line,v.end_column,COALESCE((SELECT MAX(c.confidence) FROM claims c WHERE c.subject_id=e.entity_id OR c.object_id=e.entity_id),1),x.vector_json FROM projection_vector_records p JOIN active_projection_builds a ON a.projection_build_id=p.projection_build_id JOIN vector_embedding_cache x ON x.embedding_id=p.embedding_id JOIN entities e ON e.entity_id=p.entity_id JOIN evidence v ON v.evidence_id=p.evidence_id WHERE a.projection_kind='vector'`+where+` ORDER BY e.repo_id,e.path,e.entity_id`, args...)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()
	type scored struct {
		c     Candidate
		score float64
	}
	var all []scored
	for rows.Next() {
		var raw string
		c, e := scanCandidateWithVector(rows, &raw)
		if e != nil {
			return nil, nil, e
		}
		var v []float64
		if json.Unmarshal([]byte(raw), &v) != nil {
			return nil, nil, fmt.Errorf("invalid vector payload")
		}
		c.MatchType, c.Field = "vector", "semantic"
		if c.Confidence >= filter.MinimumConfidence {
			all = append(all, scored{c, vectorpkg.Cosine(qv, v)})
		}
	}
	if err = rows.Err(); err != nil {
		return nil, nil, err
	}
	sort.Slice(all, func(i, j int) bool {
		if all[i].score != all[j].score {
			return all[i].score > all[j].score
		}
		return all[i].c.Entity.ID < all[j].c.Entity.ID
	})
	if len(all) > limit {
		all = all[:limit]
	}
	out := make([]Candidate, len(all))
	scores := make([]float64, len(all))
	for i := range all {
		out[i], scores[i] = all[i].c, all[i].score
	}
	return out, scores, nil
}
func scanCandidateWithVector(rows interface{ Scan(...any) error }, raw *string) (Candidate, error) {
	var c Candidate
	err := rows.Scan(&c.Entity.ID, &c.Entity.GenerationID, &c.Entity.RepoID, &c.Entity.Kind, &c.Entity.Label, &c.Entity.Path, &c.Entity.Language, &c.Entity.EvidenceID, &c.Evidence.SourceID, &c.Evidence.Path, &c.Evidence.SHA256, &c.Evidence.Excerpt, &c.Evidence.Span.StartByte, &c.Evidence.Span.EndByte, &c.Evidence.Span.StartLine, &c.Evidence.Span.StartColumn, &c.Evidence.Span.EndLine, &c.Evidence.Span.EndColumn, &c.Confidence, raw)
	c.Evidence.GenerationID, c.Evidence.RepoID, c.Evidence.ID = c.Entity.GenerationID, c.Entity.RepoID, c.Entity.EvidenceID
	return c, err
}
