package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

const landmarkBuilderName = "canonical-ir-landmarks"
const landmarkBuilderVersion = "v1"

// PrecomputedRecord is a compact, generation-bound projection record. Handles
// are encoded only at the MCP boundary; this layer retains canonical IDs.
type PrecomputedRecord struct {
	ID, CatalogRevision, Kind, Repository, Service, Label string
	Summary                                               map[string]any
	EvidenceIDs, ClaimIDs, DependencyIDs                  []string
	SourceGenerations                                     []string
	BuilderName, BuilderVersion                           string
	Confidence                                            float64
	Coverage                                              map[string]any
	RecomputationReason                                   string
}

type LandmarkFilter struct {
	Repository, Service string
	Kinds               []string
	MinimumConfidence   float64
	Limit, Offset       int
}

// Landmarks returns only the active, catalog-aligned landmark projection.
func (s *Store) Landmarks(ctx context.Context, f LandmarkFilter) ([]PrecomputedRecord, bool, error) {
	if err := s.RequireProjection(ctx, "landmarks"); err != nil {
		return nil, false, err
	}
	r, err := s.ActiveCatalogRevision(ctx)
	if err != nil {
		return nil, false, err
	}
	q := `SELECT p.record_id,p.catalog_revision_id,p.record_kind,p.repository_id,p.service,p.label,p.summary_json,p.evidence_ids,p.claim_ids,p.dependency_ids,p.source_generations,p.builder_name,p.builder_version,p.confidence,p.coverage_json,p.recomputation_reason FROM precomputed_records p JOIN active_projection_builds a ON a.projection_build_id=p.projection_build_id WHERE a.projection_kind='landmarks' AND p.catalog_revision_id=?`
	args := []any{r.ID}
	if f.Repository != "" {
		q += ` AND p.repository_id=?`
		args = append(args, f.Repository)
	}
	if f.Service != "" {
		q += ` AND p.service=?`
		args = append(args, f.Service)
	}
	if f.MinimumConfidence > 0 {
		q += ` AND p.confidence>=?`
		args = append(args, f.MinimumConfidence)
	}
	if len(f.Kinds) > 0 {
		q += ` AND p.record_kind IN (` + placeholders(len(f.Kinds)) + `)`
		for _, kind := range f.Kinds {
			args = append(args, kind)
		}
	}
	q += ` ORDER BY p.record_kind,p.repository_id,p.service,p.label,p.record_id LIMIT ? OFFSET ?`
	args = append(args, f.Limit+1, f.Offset)
	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, false, err
	}
	defer rows.Close()
	out := []PrecomputedRecord{}
	for rows.Next() {
		var x PrecomputedRecord
		var summary, evidence, claims, deps, generations, coverage string
		if err = rows.Scan(&x.ID, &x.CatalogRevision, &x.Kind, &x.Repository, &x.Service, &x.Label, &summary, &evidence, &claims, &deps, &generations, &x.BuilderName, &x.BuilderVersion, &x.Confidence, &coverage, &x.RecomputationReason); err != nil {
			return nil, false, err
		}
		if json.Unmarshal([]byte(summary), &x.Summary) != nil || json.Unmarshal([]byte(evidence), &x.EvidenceIDs) != nil || json.Unmarshal([]byte(claims), &x.ClaimIDs) != nil || json.Unmarshal([]byte(deps), &x.DependencyIDs) != nil || json.Unmarshal([]byte(generations), &x.SourceGenerations) != nil || json.Unmarshal([]byte(coverage), &x.Coverage) != nil {
			return nil, false, fmt.Errorf("invalid landmark projection record")
		}
		out = append(out, x)
	}
	if err = rows.Err(); err != nil {
		return nil, false, err
	}
	truncated := len(out) > f.Limit
	if truncated {
		out = out[:f.Limit]
	}
	return out, truncated, nil
}

// LandmarkCandidates supplies planner anchors only when their labels match the
// fixed query text. The returned entity/evidence is still canonical and is
// expanded normally by the caller.
func (s *Store) LandmarkCandidates(ctx context.Context, text string, f QueryFilter) ([]Candidate, error) {
	terms := strings.Fields(strings.ToLower(text))
	if len(terms) == 0 {
		return nil, nil
	}
	records, _, err := s.Landmarks(ctx, LandmarkFilter{Repository: f.Repository, MinimumConfidence: f.MinimumConfidence, Limit: 100})
	if err != nil {
		return nil, err
	}
	var out []Candidate
	for _, record := range records {
		label := strings.ToLower(record.Label)
		matched := false
		for _, term := range terms {
			if strings.Contains(label, term) {
				matched = true
				break
			}
		}
		if !matched {
			continue
		}
		for _, evidence := range record.EvidenceIDs {
			row := s.db.QueryRowContext(ctx, `SELECT e.entity_id,e.generation_id,e.repo_id,e.kind,e.label,e.path,e.language,e.evidence_id,v.source_id,v.path,v.file_sha256,v.excerpt,v.start_byte,v.end_byte,v.start_line,v.start_column,v.end_line,v.end_column,COALESCE((SELECT MAX(c.confidence) FROM claims c WHERE c.subject_id=e.entity_id OR c.object_id=e.entity_id),1) FROM entities e JOIN evidence v ON v.evidence_id=e.evidence_id WHERE e.evidence_id=? AND e.generation_id IN (SELECT generation_id FROM catalog_revision_members WHERE catalog_revision_id=(SELECT catalog_revision_id FROM active_catalog_revision WHERE singleton=1)) ORDER BY e.kind,e.entity_id LIMIT 1`, evidence)
			var c Candidate
			if err := scanLandmarkCandidate(row, &c); err == nil {
				c.MatchType = "landmark"
				out = append(out, c)
				break
			}
		}
	}
	return out, nil
}

func scanLandmarkCandidate(row interface{ Scan(...any) error }, c *Candidate) error {
	err := row.Scan(&c.Entity.ID, &c.Entity.GenerationID, &c.Entity.RepoID, &c.Entity.Kind, &c.Entity.Label, &c.Entity.Path, &c.Entity.Language, &c.Entity.EvidenceID, &c.Evidence.SourceID, &c.Evidence.Path, &c.Evidence.SHA256, &c.Evidence.Excerpt, &c.Evidence.Span.StartByte, &c.Evidence.Span.EndByte, &c.Evidence.Span.StartLine, &c.Evidence.Span.StartColumn, &c.Evidence.Span.EndLine, &c.Evidence.Span.EndColumn, &c.Confidence)
	c.Evidence.GenerationID = c.Entity.GenerationID
	c.Evidence.RepoID = c.Entity.RepoID
	c.Evidence.ID = c.Entity.EvidenceID
	return err
}

type landmarkSeed struct {
	kind, repo, service, label, generation string
	evidence, claims, dependencies         []string
	confidence                             float64
	summary                                map[string]any
}

func landmarkID(s landmarkSeed) string {
	return "lm-" + digest(s.kind, s.repo, s.service, s.label, strings.Join(s.dependencies, ","))
}

func uniqueSorted(values []string) []string {
	seen := map[string]bool{}
	for _, value := range values {
		if value != "" {
			seen[value] = true
		}
	}
	out := make([]string, 0, len(seen))
	for value := range seen {
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}

func landmarkRecord(seed landmarkSeed, reason string) PrecomputedRecord {
	seed.evidence, seed.claims, seed.dependencies = uniqueSorted(seed.evidence), uniqueSorted(seed.claims), uniqueSorted(seed.dependencies)
	return PrecomputedRecord{ID: landmarkID(seed), Kind: seed.kind, Repository: seed.repo, Service: seed.service, Label: seed.label,
		Summary: seed.summary, EvidenceIDs: seed.evidence, ClaimIDs: seed.claims, DependencyIDs: seed.dependencies,
		SourceGenerations: uniqueSorted([]string{seed.generation}), BuilderName: landmarkBuilderName, BuilderVersion: landmarkBuilderVersion,
		Confidence: seed.confidence, Coverage: map[string]any{"complete": true, "derivation": "canonical_ir_only"}, RecomputationReason: reason}
}

func addSeed(out *[]PrecomputedRecord, seed landmarkSeed, reason string) {
	// A record without an evidence anchor is not a fact and must never be exposed.
	if len(seed.evidence) == 0 {
		return
	}
	*out = append(*out, landmarkRecord(seed, reason))
}

// buildLandmarks reads only persisted canonical IR / catalog records. Its
// selection rules are deliberately fixed and inspectable.
func buildLandmarks(ctx context.Context, tx *sql.Tx, buildID, revision, reason string) (string, error) {
	changed, priorBuild, err := landmarkChangedRepositories(ctx, tx, revision)
	if err != nil {
		return "", err
	}
	var out []PrecomputedRecord
	rows, err := tx.QueryContext(ctx, `SELECT m.repo_id,m.generation_id,MIN(e.evidence_id),COUNT(f.source_id)
 FROM catalog_revision_members m JOIN source_files f ON f.generation_id=m.generation_id
 JOIN entities e ON e.generation_id=m.generation_id AND e.kind='file' AND e.path=f.path
 WHERE m.catalog_revision_id=? GROUP BY m.repo_id,m.generation_id ORDER BY m.repo_id`, revision)
	if err != nil {
		return "", err
	}
	for rows.Next() {
		var repo, gen, evidence string
		var files int
		if err = rows.Scan(&repo, &gen, &evidence, &files); err != nil {
			rows.Close()
			return "", err
		}
		addSeed(&out, landmarkSeed{kind: "repository_summary", repo: repo, label: repo, generation: gen, evidence: []string{evidence}, dependencies: []string{"repository:" + repo}, confidence: 1, summary: map[string]any{"repository": repo, "file_count": files}}, reason)
		addSeed(&out, landmarkSeed{kind: "architecture_landmark", repo: repo, label: repo + " repository boundary", generation: gen, evidence: []string{evidence}, dependencies: []string{"repository:" + repo}, confidence: 1, summary: map[string]any{"landmark": "repository_boundary", "repository": repo}}, reason)
		// Catalog ownership is optional. Absence is an explicit unknown fact with
		// a repository evidence anchor, never an organisational inference.
		var owner string
		_ = tx.QueryRowContext(ctx, `SELECT owner FROM approved_ownership WHERE repository_id=? AND coordinate='repository'`, repo).Scan(&owner)
		ownership := map[string]any{"state": "unknown", "coordinate": "repository"}
		if owner != "" {
			ownership = map[string]any{"state": "confirmed", "coordinate": "repository", "owner": owner}
		}
		addSeed(&out, landmarkSeed{kind: "ownership_fact", repo: repo, label: repo + " ownership", generation: gen, evidence: []string{evidence}, dependencies: []string{"repository:" + repo, "ownership:" + owner}, confidence: 1, summary: ownership}, reason)
	}
	if err = rows.Err(); err != nil {
		rows.Close()
		return "", err
	}
	rows.Close()

	// Source-side endpoints and message producers are service anchors; they do
	// not claim a runtime service boundary beyond the extracted source symbol.
	rows, err = tx.QueryContext(ctx, `SELECT s.repo_id,c.generation_id,s.label,c.predicate,c.claim_id,c.evidence_id
 FROM claims c JOIN entities s ON s.entity_id=c.subject_id JOIN catalog_revision_members m ON m.generation_id=c.generation_id
 WHERE m.catalog_revision_id=? AND c.predicate IN ('DECLARES_EFFECTIVE_ENDPOINT','DECLARES_UI_ROUTE','PUBLISHES_EVENT','PRODUCES_TOPIC','SENDS_QUEUE')
 ORDER BY s.repo_id,s.label,c.predicate,c.claim_id`, revision)
	if err != nil {
		return "", err
	}
	for rows.Next() {
		var repo, gen, label, predicate, claim, evidence string
		if err = rows.Scan(&repo, &gen, &label, &predicate, &claim, &evidence); err != nil {
			rows.Close()
			return "", err
		}
		addSeed(&out, landmarkSeed{kind: "service_summary", repo: repo, service: label, label: label, generation: gen, evidence: []string{evidence}, claims: []string{claim}, dependencies: []string{claim}, confidence: .9, summary: map[string]any{"anchor_predicate": predicate, "source_symbol": label}}, reason)
	}
	if err = rows.Err(); err != nil {
		rows.Close()
		return "", err
	}
	rows.Close()

	// Important paths are selected only by named V1 roles, never by source text.
	rows, err = tx.QueryContext(ctx, `SELECT f.repo_id,f.generation_id,f.path,e.evidence_id,f.classification
 FROM source_files f JOIN catalog_revision_members m ON m.generation_id=f.generation_id
 JOIN entities e ON e.generation_id=f.generation_id AND e.kind='file' AND e.path=f.path
 WHERE m.catalog_revision_id=? ORDER BY f.repo_id,f.path,e.evidence_id`, revision)
	if err != nil {
		return "", err
	}
	for rows.Next() {
		var repo, gen, path, evidence, classification string
		if err = rows.Scan(&repo, &gen, &path, &evidence, &classification); err != nil {
			rows.Close()
			return "", err
		}
		role := importantPathRole(path, classification)
		if role != "" {
			addSeed(&out, landmarkSeed{kind: "important_path", repo: repo, label: path, generation: gen, evidence: []string{evidence}, dependencies: []string{"path:" + path}, confidence: .85, summary: map[string]any{"path": path, "selection_rule": role}}, reason)
		}
	}
	if err = rows.Err(); err != nil {
		rows.Close()
		return "", err
	}
	rows.Close()

	rows, err = tx.QueryContext(ctx, `SELECT s.repo_id,c.generation_id,s.path,s.label,o.label,c.predicate,c.claim_id,c.evidence_id,c.confidence
 FROM claims c JOIN entities s ON s.entity_id=c.subject_id JOIN entities o ON o.entity_id=c.object_id
 JOIN catalog_revision_members m ON m.generation_id=c.generation_id WHERE m.catalog_revision_id=?
 AND c.predicate IN ('DEPENDENCY_DECLARATION','DECLARES_MODULE','DECLARES_EFFECTIVE_ENDPOINT','DECLARES_ENDPOINT','DECLARES_UI_ROUTE','PUBLISHES_EVENT','CONSUMES_EVENT','PRODUCES_TOPIC','CONSUMES_TOPIC','SENDS_QUEUE','LISTENS_QUEUE','DEFINES_CONFIGURATION','REFERENCES_CONFIGURATION','BINDS_CONFIGURATION')
 ORDER BY s.repo_id,c.predicate,s.label,o.label,c.claim_id`, revision)
	if err != nil {
		return "", err
	}
	for rows.Next() {
		var repo, gen, path, subject, object, predicate, claim, evidence string
		var confidence float64
		if err = rows.Scan(&repo, &gen, &path, &subject, &object, &predicate, &claim, &evidence, &confidence); err != nil {
			rows.Close()
			return "", err
		}
		kind := "contract"
		if predicate == "DEPENDENCY_DECLARATION" || predicate == "DECLARES_MODULE" {
			kind = "dependency"
		}
		if strings.Contains(predicate, "CONFIGURATION") {
			kind = "boundary"
		}
		label := predicate + ": " + object
		addSeed(&out, landmarkSeed{kind: kind, repo: repo, service: subject, label: label, generation: gen, evidence: []string{evidence}, claims: []string{claim}, dependencies: []string{claim}, confidence: confidence, summary: map[string]any{"predicate": predicate, "subject": subject, "object": object, "path": path}}, reason)
		if role := claimPathRole(predicate); role != "" && path != "" {
			addSeed(&out, landmarkSeed{kind: "important_path", repo: repo, label: path, generation: gen, evidence: []string{evidence}, claims: []string{claim}, dependencies: []string{claim}, confidence: confidence, summary: map[string]any{"path": path, "selection_rule": role}}, reason)
		}
	}
	if err = rows.Err(); err != nil {
		rows.Close()
		return "", err
	}
	rows.Close()

	rows, err = tx.QueryContext(ctx, `SELECT f.repo_id,f.generation_id,f.path,e.evidence_id FROM source_files f JOIN catalog_revision_members m ON m.generation_id=f.generation_id JOIN entities e ON e.generation_id=f.generation_id AND e.kind='file' AND e.path=f.path WHERE m.catalog_revision_id=? AND (f.path LIKE '%_test.go' OR f.path LIKE '%.test.ts' OR f.path LIKE '%.spec.ts' OR f.path LIKE '%/test/%') ORDER BY f.repo_id,f.path`, revision)
	if err != nil {
		return "", err
	}
	for rows.Next() {
		var repo, gen, path, evidence string
		if err = rows.Scan(&repo, &gen, &path, &evidence); err != nil {
			rows.Close()
			return "", err
		}
		addSeed(&out, landmarkSeed{kind: "test_anchor", repo: repo, label: path, generation: gen, evidence: []string{evidence}, dependencies: []string{"path:" + path}, confidence: .9, summary: map[string]any{"path": path, "selection_rule": "test_path"}}, reason)
	}
	if err = rows.Err(); err != nil {
		rows.Close()
		return "", err
	}
	rows.Close()

	// Cross claims are the only source for estate topology. They retain both
	// direct source anchors and assert no undocumented runtime relationship.
	rows, err = tx.QueryContext(ctx, `SELECT x.cross_claim_id,s.repo_id,o.repo_id,x.predicate,x.subject_evidence_id,x.object_evidence_id,x.confidence FROM cross_claims x JOIN entities s ON s.entity_id=x.subject_id JOIN entities o ON o.entity_id=x.object_id WHERE x.catalog_revision_id=? ORDER BY s.repo_id,o.repo_id,x.predicate,x.cross_claim_id`, revision)
	if err != nil {
		return "", err
	}
	for rows.Next() {
		var claim, from, to, predicate, left, right string
		var confidence float64
		if err = rows.Scan(&claim, &from, &to, &predicate, &left, &right, &confidence); err != nil {
			rows.Close()
			return "", err
		}
		addSeed(&out, landmarkSeed{kind: "estate_topology_fact", repo: from, label: from + " " + predicate + " " + to, evidence: []string{left, right}, claims: []string{claim}, dependencies: []string{claim}, confidence: confidence, summary: map[string]any{"from_repository": from, "to_repository": to, "predicate": predicate}}, reason)
	}
	if err = rows.Err(); err != nil {
		rows.Close()
		return "", err
	}
	rows.Close()

	if priorBuild != "" {
		// Unchanged members retain their exact prior records. The record remains
		// tied to the newly active catalog revision but still names the same
		// immutable repository generation and evidence dependencies.
		kept := out[:0]
		for _, r := range out {
			if changed[r.Repository] || r.Kind == "estate_topology_fact" {
				kept = append(kept, r)
			}
		}
		out = kept
		if err := copyUnchangedLandmarks(ctx, tx, buildID, revision, priorBuild, changed); err != nil {
			return "", err
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	for _, r := range out {
		if err = insertLandmark(ctx, tx, buildID, revision, r); err != nil {
			return "", err
		}
	}
	var total int
	if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM precomputed_records WHERE projection_build_id=?`, buildID).Scan(&total); err != nil {
		return "", err
	}
	return fmt.Sprintf(`{"records":%d,"recomputed":%d,"retained":%d}`, total, len(out), total-len(out)), nil
}

// landmarkChangedRepositories compares the target catalog to the prior active
// catalog before activation. It is entirely IR/catalog based and deliberately
// treats bootstrap as a full recomputation.
func landmarkChangedRepositories(ctx context.Context, tx *sql.Tx, revision string) (map[string]bool, string, error) {
	changed := map[string]bool{}
	var priorRevision, priorBuild string
	err := tx.QueryRowContext(ctx, `SELECT catalog_revision_id FROM active_catalog_revision WHERE singleton=1`).Scan(&priorRevision)
	if err == sql.ErrNoRows {
		rows, e := tx.QueryContext(ctx, `SELECT repo_id FROM catalog_revision_members WHERE catalog_revision_id=?`, revision)
		if e != nil {
			return nil, "", e
		}
		defer rows.Close()
		for rows.Next() {
			var repo string
			if e = rows.Scan(&repo); e != nil {
				return nil, "", e
			}
			changed[repo] = true
		}
		return changed, "", rows.Err()
	}
	if err != nil {
		return nil, "", err
	}
	_ = tx.QueryRowContext(ctx, `SELECT projection_build_id FROM active_projection_builds WHERE projection_kind='landmarks'`).Scan(&priorBuild)
	rows, err := tx.QueryContext(ctx, `SELECT n.repo_id,n.generation_id,COALESCE(o.generation_id,'') FROM catalog_revision_members n LEFT JOIN catalog_revision_members o ON o.catalog_revision_id=? AND o.repo_id=n.repo_id WHERE n.catalog_revision_id=?`, priorRevision, revision)
	if err != nil {
		return nil, "", err
	}
	defer rows.Close()
	for rows.Next() {
		var repo, next, old string
		if err = rows.Scan(&repo, &next, &old); err != nil {
			return nil, "", err
		}
		if next != old {
			changed[repo] = true
		}
	}
	return changed, priorBuild, rows.Err()
}

func copyUnchangedLandmarks(ctx context.Context, tx *sql.Tx, buildID, revision, priorBuild string, changed map[string]bool) error {
	rows, err := tx.QueryContext(ctx, `SELECT record_id,record_kind,repository_id,service,label,summary_json,evidence_ids,claim_ids,dependency_ids,source_generations,builder_name,builder_version,confidence,coverage_json FROM precomputed_records WHERE projection_build_id=? AND record_kind<>'estate_topology_fact' ORDER BY record_id`, priorBuild)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var id, kind, repo, service, label, summary, evidence, claims, deps, generations, builder, builderVersion, coverage string
		var confidence float64
		if err = rows.Scan(&id, &kind, &repo, &service, &label, &summary, &evidence, &claims, &deps, &generations, &builder, &builderVersion, &confidence, &coverage); err != nil {
			return err
		}
		if changed[repo] {
			continue
		}
		if _, err = tx.ExecContext(ctx, `INSERT INTO precomputed_records VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`, id, buildID, revision, kind, repo, service, label, summary, evidence, claims, deps, generations, builder, builderVersion, confidence, coverage, "retained_unchanged"); err != nil {
			return err
		}
	}
	return rows.Err()
}

func importantPathRole(path, classification string) string {
	lower := strings.ToLower(path)
	switch {
	case classification == "configuration":
		return "configuration"
	case strings.HasSuffix(lower, "package.json") || strings.HasSuffix(lower, "build.gradle") || strings.HasSuffix(lower, "pom.xml") || strings.HasSuffix(lower, "settings.gradle"):
		return "manifest"
	case strings.Contains(lower, "/test/") || strings.HasSuffix(lower, "_test.go") || strings.Contains(lower, ".test.") || strings.Contains(lower, ".spec."):
		return "test"
	}
	return ""
}
func claimPathRole(predicate string) string {
	if strings.Contains(predicate, "ENDPOINT") || predicate == "DECLARES_UI_ROUTE" {
		return "route_or_controller"
	}
	if strings.Contains(predicate, "EVENT") || strings.Contains(predicate, "TOPIC") || strings.Contains(predicate, "QUEUE") {
		return "listener_or_event"
	}
	if strings.Contains(predicate, "CONFIGURATION") {
		return "configuration"
	}
	if predicate == "DEPENDENCY_DECLARATION" || predicate == "DECLARES_MODULE" {
		return "manifest"
	}
	return ""
}
func insertLandmark(ctx context.Context, tx *sql.Tx, buildID, revision string, r PrecomputedRecord) error {
	summary, _ := json.Marshal(r.Summary)
	evidence, _ := json.Marshal(r.EvidenceIDs)
	claims, _ := json.Marshal(r.ClaimIDs)
	dependencies, _ := json.Marshal(r.DependencyIDs)
	generations, _ := json.Marshal(r.SourceGenerations)
	coverage, _ := json.Marshal(r.Coverage)
	_, err := tx.ExecContext(ctx, `INSERT INTO precomputed_records VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`, r.ID, buildID, revision, r.Kind, r.Repository, r.Service, r.Label, string(summary), string(evidence), string(claims), string(dependencies), string(generations), r.BuilderName, r.BuilderVersion, r.Confidence, string(coverage), r.RecomputationReason)
	return err
}
