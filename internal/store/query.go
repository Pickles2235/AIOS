package store

import (
	"context"
	"database/sql"
	"fmt"
	"sort"
	"strings"
)

// QueryFilter deliberately exposes only canonical attributes.  Version is an
// assertion against an active generation ID or its indexed Git commit.
type QueryFilter struct {
	Repository, Version                                          string
	Kinds, Languages, Classifications, Fields, RelationshipTypes []string
	MinimumConfidence                                            float64
}

// ExactCandidates is the lookup-projection route for symbol and path identity
// resolution. It intentionally does not consult FTS, so a lexical outage does
// not prevent an authoritative exact answer.
func (s *Store) ExactCandidates(ctx context.Context, query string, filter QueryFilter) ([]Candidate, error) {
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	if err := assertActiveVersionUsing(ctx, tx, filter); err != nil {
		return nil, err
	}
	if err := requireProjectionUsing(ctx, tx, "lookup"); err != nil {
		return nil, err
	}
	where, args := canonicalWhere(filter)
	q := `SELECT e.entity_id,e.generation_id,e.repo_id,e.kind,e.label,e.path,e.language,e.evidence_id,v.source_id,v.path,v.file_sha256,v.excerpt,v.start_byte,v.end_byte,v.start_line,v.start_column,v.end_line,v.end_column,COALESCE((SELECT MAX(c.confidence) FROM claims c WHERE c.subject_id=e.entity_id OR c.object_id=e.entity_id),1)
		FROM projection_lookup_records p JOIN active_projection_builds a ON a.projection_build_id=p.projection_build_id JOIN entities e ON e.entity_id=p.entity_id JOIN evidence v ON v.evidence_id=p.evidence_id
		WHERE a.projection_kind='lookup' AND (lower(e.label)=lower(?) OR lower(e.path)=lower(?))` + where + ` ORDER BY e.repo_id,e.path,e.entity_id`
	rows, err := tx.QueryContext(ctx, q, append([]any{query, query}, args...)...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Candidate
	for rows.Next() {
		c, err := scanCandidate(rows)
		if err != nil {
			return nil, err
		}
		if strings.EqualFold(c.Entity.Label, query) {
			c.MatchType, c.Field = "exact_identifier", "symbol"
		} else {
			c.MatchType, c.Field = "exact_path", "path"
		}
		if c.Confidence >= filter.MinimumConfidence {
			out = append(out, c)
		}
	}
	return out, rows.Err()
}

// LexicalCandidates exposes only the FTS/BM25 portion of SearchCandidates.
func (s *Store) LexicalCandidates(ctx context.Context, query string, filter QueryFilter) ([]Candidate, error) {
	all, err := s.SearchCandidates(ctx, query, filter)
	if err != nil {
		return nil, err
	}
	out := make([]Candidate, 0, len(all))
	for _, c := range all {
		if c.MatchType == "lexical" {
			out = append(out, c)
		}
	}
	return out, nil
}

type Candidate struct {
	Entity           Entity
	Evidence         Evidence
	Derivation       string
	Confidence       float64
	MatchType, Field string
	BM25             float64
}

// CandidateReference is a cache-safe candidate work product. It deliberately
// excludes excerpts and source content; HydrateCandidateReferences reads those
// again from active canonical IR.
type CandidateReference struct {
	EntityID, EvidenceID, MatchType, Field string
	Confidence, BM25                       float64
}

func CandidateReferences(in []Candidate) []CandidateReference {
	out := make([]CandidateReference, 0, len(in))
	for _, c := range in {
		out = append(out, CandidateReference{c.Entity.ID, c.Evidence.ID, c.MatchType, c.Field, c.Confidence, c.BM25})
	}
	return out
}

func (s *Store) HydrateCandidateReferences(ctx context.Context, refs []CandidateReference) ([]Candidate, error) {
	out := make([]Candidate, 0, len(refs))
	for _, ref := range refs {
		rows, err := s.db.QueryContext(ctx, `SELECT e.entity_id,e.generation_id,e.repo_id,e.kind,e.label,e.path,e.language,e.evidence_id,v.source_id,v.path,v.file_sha256,v.excerpt,v.start_byte,v.end_byte,v.start_line,v.start_column,v.end_line,v.end_column,COALESCE((SELECT MAX(c.confidence) FROM claims c WHERE c.subject_id=e.entity_id OR c.object_id=e.entity_id),1) FROM entities e JOIN active_generations a ON a.generation_id=e.generation_id JOIN evidence v ON v.evidence_id=e.evidence_id WHERE e.entity_id=? AND e.evidence_id=?`, ref.EntityID, ref.EvidenceID)
		if err != nil {
			return nil, err
		}
		if !rows.Next() {
			rows.Close()
			return nil, fmt.Errorf("cached canonical handle is stale")
		}
		c, err := scanCandidate(rows)
		rows.Close()
		if err != nil {
			return nil, err
		}
		c.MatchType, c.Field, c.Confidence, c.BM25 = ref.MatchType, ref.Field, ref.Confidence, ref.BM25
		out = append(out, c)
	}
	return out, nil
}

var SearchFields = map[string]bool{"source": true, "documentation": true, "path": true, "symbol": true, "strings": true, "logs_errors": true, "configuration": true}

func (s *Store) SearchCandidates(ctx context.Context, query string, filter QueryFilter) ([]Candidate, error) {
	if filter.MinimumConfidence < 0 || filter.MinimumConfidence > 1 {
		return nil, fmt.Errorf("minimum confidence must be between 0 and 1")
	}
	fields := filter.Fields
	if len(fields) == 0 {
		fields = []string{"source", "documentation", "path", "symbol", "strings", "logs_errors", "configuration"}
	}
	for _, field := range fields {
		if !SearchFields[field] {
			return nil, fmt.Errorf("unsupported search field %q", field)
		}
	}
	if err := s.assertActiveVersion(ctx, filter); err != nil {
		return nil, err
	}
	if err := s.requireProjection(ctx, "lexical"); err != nil {
		return nil, err
	}
	where, args := canonicalWhere(filter)
	seen := map[string]Candidate{}
	add := func(c Candidate) {
		key := c.Entity.ID + ":" + c.MatchType + ":" + c.Field
		if old, ok := seen[key]; !ok || c.BM25 < old.BM25 {
			seen[key] = c
		}
	}

	// Exact candidates are intentionally separate from FTS and carry no opaque score.
	exact := `SELECT e.entity_id,e.generation_id,e.repo_id,e.kind,e.label,e.path,e.language,e.evidence_id,v.source_id,v.path,v.file_sha256,v.excerpt,v.start_byte,v.end_byte,v.start_line,v.start_column,v.end_line,v.end_column,COALESCE((SELECT MAX(c.confidence) FROM claims c WHERE c.subject_id=e.entity_id OR c.object_id=e.entity_id),1)
		FROM entities e JOIN active_generations a ON a.generation_id=e.generation_id JOIN evidence v ON v.evidence_id=e.evidence_id WHERE `
	for _, spec := range []struct{ condition, typ, field string }{
		{"lower(e.label)=lower(?)", "exact_identifier", "symbol"},
		{"lower(e.path)=lower(?)", "exact_path", "path"},
	} {
		rows, err := s.db.QueryContext(ctx, exact+spec.condition+where+` ORDER BY e.repo_id,e.path,e.entity_id`, append([]any{query}, args...)...)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			c, err := scanCandidate(rows)
			if err != nil {
				rows.Close()
				return nil, err
			}
			c.MatchType, c.Field = spec.typ, spec.field
			if c.Confidence >= filter.MinimumConfidence {
				add(c)
			}
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return nil, err
		}
		rows.Close()
	}
	// Literal content, protocol and config names are exact substring matches.
	rows, err := s.db.QueryContext(ctx, `SELECT e.entity_id,e.generation_id,e.repo_id,e.kind,e.label,e.path,e.language,e.evidence_id,v.source_id,v.path,v.file_sha256,v.excerpt,v.start_byte,v.end_byte,v.start_line,v.start_column,v.end_line,v.end_column,COALESCE((SELECT MAX(c.confidence) FROM claims c WHERE c.subject_id=e.entity_id OR c.object_id=e.entity_id),1)
		FROM entities e JOIN active_generations a ON a.generation_id=e.generation_id JOIN evidence v ON v.evidence_id=e.evidence_id JOIN source_files f ON f.source_id=v.source_id
		WHERE instr(lower(f.content),lower(?))>0`+where+` ORDER BY e.repo_id,e.path,e.entity_id`, append([]any{query}, args...)...)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		c, err := scanCandidate(rows)
		if err != nil {
			rows.Close()
			return nil, err
		}
		c.MatchType, c.Field = "exact_literal", "strings"
		if strings.HasPrefix(c.Entity.Kind, "configuration") {
			c.MatchType, c.Field = "exact_protocol_config", "configuration"
		}
		if c.Confidence >= filter.MinimumConfidence {
			add(c)
		}
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	rows.Close()

	match := ftsPhrase(query)
	columns := make([]string, 0, len(fields))
	for _, field := range fields {
		columns = append(columns, field+":"+match)
	}
	rows, err = s.db.QueryContext(ctx, `SELECT e.entity_id,e.generation_id,e.repo_id,e.kind,e.label,e.path,e.language,e.evidence_id,v.source_id,v.path,v.file_sha256,v.excerpt,v.start_byte,v.end_byte,v.start_line,v.start_column,v.end_line,v.end_column,COALESCE((SELECT MAX(c.confidence) FROM claims c WHERE c.subject_id=e.entity_id OR c.object_id=e.entity_id),1),bm25(search_fts)
		FROM search_fts JOIN entities e ON e.entity_id=search_fts.entity_id JOIN active_generations a ON a.generation_id=e.generation_id JOIN evidence v ON v.evidence_id=e.evidence_id
		WHERE search_fts MATCH ?`+where+` ORDER BY bm25(search_fts),e.repo_id,e.path,e.entity_id`, append([]any{strings.Join(columns, " OR ")}, args...)...)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		c, err := scanCandidateWithBM25(rows)
		if err != nil {
			rows.Close()
			return nil, err
		}
		c.MatchType, c.Field = "lexical", fields[0]
		if c.Confidence >= filter.MinimumConfidence {
			add(c)
		}
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	rows.Close()
	out := make([]Candidate, 0, len(seen))
	for _, c := range seen {
		out = append(out, c)
	}
	sort.Slice(out, func(i, j int) bool {
		left, right := candidateClass(out[i]), candidateClass(out[j])
		if left != right {
			return left < right
		}
		if out[i].MatchType == "lexical" && out[i].BM25 != out[j].BM25 {
			return out[i].BM25 < out[j].BM25
		}
		if out[i].Confidence != out[j].Confidence {
			return out[i].Confidence > out[j].Confidence
		}
		return out[i].Entity.RepoID+"\x00"+out[i].Entity.Path+"\x00"+fmt.Sprint(out[i].Evidence.Span.StartByte)+"\x00"+out[i].Entity.ID < out[j].Entity.RepoID+"\x00"+out[j].Entity.Path+"\x00"+fmt.Sprint(out[j].Evidence.Span.StartByte)+"\x00"+out[j].Entity.ID
	})
	return out, nil
}

func candidateClass(c Candidate) int {
	switch c.MatchType {
	case "exact_identifier":
		if strings.HasPrefix(c.Entity.Kind, "symbol:") {
			return 0
		}
		return 1
	case "exact_path":
		return 2
	case "exact_protocol_config":
		return 3
	case "exact_literal":
		return 4
	case "lexical":
		return 5
	default:
		return 6
	}
}

// StructuralCandidates resolves source-backed relationship endpoints. It is a
// fixed canonical query over active claims, not a caller-provided graph query.
func (s *Store) StructuralCandidates(ctx context.Context, query string, filter QueryFilter) ([]Candidate, error) {
	if err := s.assertActiveVersion(ctx, filter); err != nil {
		return nil, err
	}
	if err := s.requireProjection(ctx, "graph"); err != nil {
		return nil, err
	}
	where, args := canonicalWhere(filter)
	rows, err := s.db.QueryContext(ctx, `SELECT e.entity_id,e.generation_id,e.repo_id,e.kind,e.label,e.path,e.language,e.evidence_id,v.source_id,v.path,v.file_sha256,v.excerpt,v.start_byte,v.end_byte,v.start_line,v.start_column,v.end_line,v.end_column,COALESCE((SELECT MAX(c2.confidence) FROM claims c2 WHERE c2.subject_id=e.entity_id OR c2.object_id=e.entity_id),1)
		FROM entities e JOIN active_generations a ON a.generation_id=e.generation_id JOIN evidence v ON v.evidence_id=e.evidence_id
		WHERE EXISTS (SELECT 1 FROM claims c JOIN entities target ON target.entity_id=CASE WHEN c.subject_id=e.entity_id THEN c.object_id ELSE c.subject_id END WHERE (c.subject_id=e.entity_id OR c.object_id=e.entity_id) AND lower(target.label)=lower(?))`+where+` ORDER BY e.repo_id,e.path,e.entity_id`, append([]any{query}, args...)...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Candidate
	for rows.Next() {
		c, err := scanCandidate(rows)
		if err != nil {
			return nil, err
		}
		if c.Confidence >= filter.MinimumConfidence {
			c.MatchType, c.Field = "structural", "claim"
			out = append(out, c)
		}
	}
	return out, rows.Err()
}

// GraphCandidates expands only canonical claim adjacency from supplied seed
// entities. The recursive CTE has caller-supplied-but-validated depth and
// result caps; it cannot express arbitrary traversal or SQL.
func (s *Store) GraphCandidates(ctx context.Context, seeds []string, filter QueryFilter, depth, limit int) ([]Candidate, error) {
	if len(seeds) == 0 || depth < 1 || depth > 4 || limit < 1 || limit > 500 {
		return nil, nil
	}
	if err := s.assertActiveVersion(ctx, filter); err != nil {
		return nil, err
	}
	if err := s.requireProjection(ctx, "graph"); err != nil {
		return nil, err
	}
	if len(seeds) > 500 {
		seeds = seeds[:500]
	}
	where, args := canonicalWhere(filter)
	values := placeholders(len(seeds))
	query := `WITH RECURSIVE walk(entity_id,depth) AS (
		SELECT entity_id,0 FROM entities WHERE entity_id IN (` + values + `)
		UNION
		SELECT CASE WHEN c.subject_id=w.entity_id THEN c.object_id ELSE c.subject_id END,w.depth+1
		FROM walk w JOIN claims c ON c.subject_id=w.entity_id OR c.object_id=w.entity_id
		WHERE w.depth < ?`
	if len(filter.RelationshipTypes) > 0 {
		query += ` AND upper(c.predicate) IN (` + placeholders(len(filter.RelationshipTypes)) + `)`
	}
	query += `
	)
	SELECT e.entity_id,e.generation_id,e.repo_id,e.kind,e.label,e.path,e.language,e.evidence_id,v.source_id,v.path,v.file_sha256,v.excerpt,v.start_byte,v.end_byte,v.start_line,v.start_column,v.end_line,v.end_column,COALESCE((SELECT MAX(c2.confidence) FROM claims c2 WHERE c2.subject_id=e.entity_id OR c2.object_id=e.entity_id),1)
	FROM entities e JOIN active_generations a ON a.generation_id=e.generation_id JOIN evidence v ON v.evidence_id=e.evidence_id
	JOIN (SELECT entity_id,MIN(depth) depth FROM walk GROUP BY entity_id) w ON w.entity_id=e.entity_id
	WHERE w.depth > 0` + where + ` ORDER BY w.depth,e.repo_id,e.path,e.entity_id LIMIT ?`
	params := make([]any, 0, len(seeds)+len(args)+2+len(filter.RelationshipTypes))
	for _, seed := range seeds {
		params = append(params, seed)
	}
	params = append(params, depth)
	for _, typ := range filter.RelationshipTypes {
		params = append(params, strings.ToUpper(typ))
	}
	params = append(params, args...)
	params = append(params, limit)
	rows, err := s.db.QueryContext(ctx, query, params...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Candidate
	for rows.Next() {
		c, err := scanCandidate(rows)
		if err != nil {
			return nil, err
		}
		c.MatchType, c.Field = "graph", "claim"
		if c.Confidence >= filter.MinimumConfidence {
			out = append(out, c)
		}
	}
	return out, rows.Err()
}

func canonicalWhere(f QueryFilter) (string, []any) {
	q, args := "", []any{}
	if f.Repository != "" {
		q += " AND e.repo_id=?"
		args = append(args, f.Repository)
	}
	for _, x := range []struct {
		column string
		values []string
	}{{"e.kind", f.Kinds}, {"e.language", f.Languages}, {"sf.classification", f.Classifications}} {
		if len(x.values) > 0 {
			q += " AND lower(" + x.column + ") IN (" + strings.TrimRight(strings.Repeat("?,", len(x.values)), ",") + ")"
			for _, v := range x.values {
				args = append(args, strings.ToLower(v))
			}
		}
	}
	// source classification is joined only when needed; aliases are supplied by callers.
	q = strings.ReplaceAll(q, "sf.classification", "(SELECT classification FROM source_files WHERE source_id=v.source_id)")
	return q, args
}

func (s *Store) assertActiveVersion(ctx context.Context, f QueryFilter) error {
	return assertActiveVersionUsing(ctx, s.db, f)
}

func assertActiveVersionUsing(ctx context.Context, reader projectionReader, f QueryFilter) error {
	if f.Version == "" {
		return nil
	}
	q := `SELECT count(*) FROM generations g JOIN active_generations a ON a.generation_id=g.generation_id WHERE (g.generation_id=? OR g.git_commit=?)`
	args := []any{f.Version, f.Version}
	if f.Repository != "" {
		q += " AND g.repo_id=?"
		args = append(args, f.Repository)
	}
	var n int
	if err := reader.QueryRowContext(ctx, q, args...).Scan(&n); err != nil {
		return err
	}
	if n == 0 {
		return fmt.Errorf("version is not an active generation")
	}
	return nil
}

// requireProjection prevents a derived index from silently serving a catalog
// revision other than the IR revision from which it was built.
func (s *Store) RequireProjection(ctx context.Context, kind string) error {
	return s.requireProjection(ctx, kind)
}

func (s *Store) requireProjection(ctx context.Context, kind string) error {
	return requireProjectionUsing(ctx, s.db, kind)
}

func requireProjectionUsing(ctx context.Context, reader projectionReader, kind string) error {
	var revision string
	if err := reader.QueryRowContext(ctx, `SELECT catalog_revision_id FROM active_catalog_revision`).Scan(&revision); err != nil {
		return fmt.Errorf("%s projection unavailable: no active IR catalog", kind)
	}
	var build, fingerprint, state, builder, schema string
	err := reader.QueryRowContext(ctx, `SELECT p.projection_build_id,p.source_ir_fingerprint,p.state,p.builder_version,p.projection_schema_version FROM active_projection_builds a JOIN projection_builds p ON p.projection_build_id=a.projection_build_id WHERE a.projection_kind=? AND p.catalog_revision_id=?`, kind, revision).Scan(&build, &fingerprint, &state, &builder, &schema)
	if err != nil || state != "ready" || (kind == "lookup" && (builder != projectionBuilderVersion(kind) || schema != "v1")) {
		return fmt.Errorf("%s projection unavailable for active IR catalog", kind)
	}
	var current string
	if err = reader.QueryRowContext(ctx, `SELECT COALESCE(group_concat(generation_id, ','),'') FROM (SELECT generation_id FROM catalog_revision_members WHERE catalog_revision_id=? ORDER BY generation_id)`, revision).Scan(&current); err != nil || current != fingerprint {
		return fmt.Errorf("%s projection is stale for active IR catalog", kind)
	}
	if kind == "lookup" {
		return validateLookupCompleteness(ctx, reader, build, revision)
	}
	return nil
}

func ftsPhrase(q string) string { return `"` + strings.ReplaceAll(q, `"`, "") + `"` }
func scanCandidate(rows interface{ Scan(...any) error }) (Candidate, error) {
	var c Candidate
	err := rows.Scan(&c.Entity.ID, &c.Entity.GenerationID, &c.Entity.RepoID, &c.Entity.Kind, &c.Entity.Label, &c.Entity.Path, &c.Entity.Language, &c.Entity.EvidenceID, &c.Evidence.SourceID, &c.Evidence.Path, &c.Evidence.SHA256, &c.Evidence.Excerpt, &c.Evidence.Span.StartByte, &c.Evidence.Span.EndByte, &c.Evidence.Span.StartLine, &c.Evidence.Span.StartColumn, &c.Evidence.Span.EndLine, &c.Evidence.Span.EndColumn, &c.Confidence)
	c.Evidence.GenerationID = c.Entity.GenerationID
	c.Evidence.RepoID = c.Entity.RepoID
	c.Evidence.ID = c.Entity.EvidenceID
	return c, err
}
func scanCandidateWithBM25(rows interface{ Scan(...any) error }) (Candidate, error) {
	var c Candidate
	err := rows.Scan(&c.Entity.ID, &c.Entity.GenerationID, &c.Entity.RepoID, &c.Entity.Kind, &c.Entity.Label, &c.Entity.Path, &c.Entity.Language, &c.Entity.EvidenceID, &c.Evidence.SourceID, &c.Evidence.Path, &c.Evidence.SHA256, &c.Evidence.Excerpt, &c.Evidence.Span.StartByte, &c.Evidence.Span.EndByte, &c.Evidence.Span.StartLine, &c.Evidence.Span.StartColumn, &c.Evidence.Span.EndLine, &c.Evidence.Span.EndColumn, &c.Confidence, &c.BM25)
	c.Evidence.GenerationID, c.Evidence.RepoID, c.Evidence.ID = c.Entity.GenerationID, c.Entity.RepoID, c.Entity.EvidenceID
	return c, err
}
