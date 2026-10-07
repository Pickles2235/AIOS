// Package knowledge exposes bounded, read-only views of canonical V1 data.
// It deliberately reads only the derived store; repository checkouts are never
// opened by this package.
package knowledge

import (
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
	"unicode"

	"github.com/AdamNi-7080/AIOS/internal/catalog"
	"github.com/AdamNi-7080/AIOS/internal/model"
	"github.com/AdamNi-7080/AIOS/internal/planner"
	"github.com/AdamNi-7080/AIOS/internal/store"
)

type Service struct {
	cfg catalog.Config
	db  *store.Store
}

func New(cfg catalog.Config, db *store.Store) *Service { return &Service{cfg: cfg, db: db} }

type Handle struct{ Type, Generation, ID string }

func Encode(typ, generation, id string) string {
	b, _ := json.Marshal(Handle{typ, generation, id})
	return "kb." + typ + "1." + base64.RawURLEncoding.EncodeToString(b)
}
func Decode(value string) (Handle, error) {
	var h Handle
	p := strings.SplitN(value, ".", 3)
	if len(p) != 3 || p[0] != "kb" {
		return h, fmt.Errorf("invalid handle")
	}
	b, e := base64.RawURLEncoding.DecodeString(p[2])
	if e != nil || json.Unmarshal(b, &h) != nil || h.Type+"1" != p[1] {
		return h, fmt.Errorf("invalid handle")
	}
	return h, nil
}

type Status struct {
	ActiveCatalog string                   `json:"active_catalog_revision,omitempty"`
	Repositories  []RepositoryStatus       `json:"repositories"`
	Projection    string                   `json:"projection_state"`
	Projections   []store.ProjectionStatus `json:"projections,omitempty"`
}
type RepositoryStatus struct {
	ID          string `json:"id"`
	Generation  string `json:"generation"`
	Revision    string `json:"revision"`
	ContentHash string `json:"content_hash"`
	Active      bool   `json:"active"`
}
type Span = model.Span
type Entity struct {
	Handle        string  `json:"handle"`
	Kind          string  `json:"kind"`
	Label         string  `json:"label"`
	Identity      string  `json:"identity"`
	Extractor     string  `json:"extractor"`
	Repository    string  `json:"repository"`
	Path          string  `json:"path"`
	Generation    string  `json:"generation"`
	Evidence      string  `json:"evidence"`
	Confidence    float64 `json:"confidence"`
	EvidenceCount int     `json:"evidence_count"`
	Span          Span    `json:"span"`
}
type Claim struct {
	Handle     string  `json:"handle"`
	Subject    string  `json:"subject"`
	Predicate  string  `json:"predicate"`
	Object     string  `json:"object"`
	Evidence   string  `json:"evidence"`
	Derivation string  `json:"derivation"`
	Extractor  string  `json:"extractor"`
	Confidence float64 `json:"confidence"`
	Generation string  `json:"generation"`
}
type Projection struct {
	Repository    string         `json:"repository"`
	Generation    string         `json:"generation"`
	Nodes         []Entity       `json:"nodes"`
	Edges         []Claim        `json:"edges"`
	Truncated     bool           `json:"truncated"`
	NextCursor    string         `json:"next_cursor,omitempty"`
	AppliedLimits map[string]int `json:"applied_limits"`
}
type Excerpt struct {
	GitCommit   string   `json:"git_commit"`
	WorkingTree bool     `json:"working_tree"`
	SHA256      string   `json:"sha256"`
	Evidence    string   `json:"evidence"`
	Source      string   `json:"source"`
	Repository  string   `json:"repository"`
	Path        string   `json:"path"`
	Revision    string   `json:"revision"`
	Generation  string   `json:"generation"`
	Span        Span     `json:"span"`
	Lines       []string `json:"lines"`
	StartLine   int      `json:"start_line"`
	EndLine     int      `json:"end_line"`
	Truncated   bool     `json:"truncated"`
}
type Query struct {
	Text              string  `json:"text"`
	Repository        string  `json:"repository"`
	Capability        string  `json:"capability,omitempty"`
	Limit             int     `json:"limit"`
	MinimumConfidence float64 `json:"minimum_confidence"`
}
type TraceEvent struct {
	Kind   string `json:"kind"`
	Detail string `json:"detail"`
	Handle string `json:"handle"`
	Rank   int    `json:"rank,omitempty"`
}
type QueryResult struct {
	Coverage      *store.CoverageBasis `json:"coverage,omitempty"`
	Status        string               `json:"status"`
	Entities      []Entity             `json:"entities"`
	Trace         []TraceEvent         `json:"trace"`
	Truncated     bool                 `json:"truncated"`
	AppliedLimits map[string]int       `json:"applied_limits"`
}

func (s *Service) Status(ctx context.Context) (Status, error) {
	d, err := s.db.Diagnostics(ctx, "")
	if err != nil {
		return Status{}, err
	}
	out := Status{ActiveCatalog: d.ActiveCatalog, Projection: "ready", Projections: d.Projections, Repositories: []RepositoryStatus{}}
	for _, configured := range s.cfg.SourceRepositories() {
		r := RepositoryStatus{ID: configured.ID}
		g, e := s.db.ActiveGeneration(ctx, configured.ID)
		if e == nil {
			r.Active = true
			r.Generation = g.ID
			r.ContentHash = g.ContentHash
			_ = s.db.QueryRowCanonical(ctx, "SELECT git_commit FROM generations WHERE generation_id=?", g.ID).Scan(&r.Revision)
		}
		out.Repositories = append(out.Repositories, r)
	}
	if len(d.Snapshots) == 0 {
		out.Projection = "no_active_generation"
		return out, nil
	}
	required := map[string]bool{"lookup": false, "lexical": false, "graph": false, "path": false, "ui": false, "cache": false}
	for _, p := range d.Projections {
		if _, ok := required[p.Kind]; ok && p.State == "ready" {
			required[p.Kind] = true
		}
	}
	for _, ready := range required {
		if !ready {
			out.Projection = "unavailable"
			break
		}
	}
	return out, nil
}

func (s *Service) active(ctx context.Context, h Handle) error {
	g, e := s.db.ActiveGeneration(ctx, repoForGeneration(ctx, s.db, h.Generation))
	if e != nil || g.ID != h.Generation {
		return fmt.Errorf("handle is stale or unknown")
	}
	return nil
}
func repoForGeneration(ctx context.Context, db *store.Store, generation string) string {
	var repo string
	_ = db.QueryRowCanonical(ctx, "SELECT repo_id FROM generations WHERE generation_id=?", generation).Scan(&repo)
	return repo
}
func (s *Service) Entity(ctx context.Context, handle string) (Entity, error) {
	h, e := Decode(handle)
	if e != nil || h.Type != "e" {
		return Entity{}, fmt.Errorf("entity handle required")
	}
	if e = s.active(ctx, h); e != nil {
		return Entity{}, e
	}
	return s.entity(ctx, h.Generation, h.ID)
}
func (s *Service) entity(ctx context.Context, generation, id string) (Entity, error) {
	var x Entity
	var eid, ev string
	e := s.db.QueryRowCanonical(ctx, `SELECT e.entity_id,e.kind,e.label,e.identity,e.extractor,e.repo_id,v.path,e.generation_id,e.evidence_id,COALESCE((SELECT MAX(c.confidence) FROM claims c WHERE c.subject_id=e.entity_id OR c.object_id=e.entity_id),1),v.start_byte,v.end_byte,v.start_line,v.start_column,v.end_line,v.end_column,(SELECT count(*) FROM claims c WHERE c.subject_id=e.entity_id OR c.object_id=e.entity_id) FROM entities e JOIN evidence v ON v.evidence_id=e.evidence_id WHERE e.generation_id=? AND e.entity_id=?`, generation, id).Scan(&eid, &x.Kind, &x.Label, &x.Identity, &x.Extractor, &x.Repository, &x.Path, &x.Generation, &ev, &x.Confidence, &x.Span.StartByte, &x.Span.EndByte, &x.Span.StartLine, &x.Span.StartColumn, &x.Span.EndLine, &x.Span.EndColumn, &x.EvidenceCount)
	if e != nil {
		return x, e
	}
	x.Handle = Encode("e", generation, eid)
	x.Evidence = Encode("v", generation, ev)
	return x, nil
}
func (s *Service) claim(ctx context.Context, generation, id string) (Claim, error) {
	var x Claim
	var cid, sid, oid, eid string
	e := s.db.QueryRowCanonical(ctx, "SELECT claim_id,subject_id,predicate,object_id,evidence_id,extractor,derivation,confidence,generation_id FROM claims WHERE generation_id=? AND claim_id=?", generation, id).Scan(&cid, &sid, &x.Predicate, &oid, &eid, &x.Extractor, &x.Derivation, &x.Confidence, &x.Generation)
	if e != nil {
		return x, e
	}
	x.Handle = Encode("c", generation, cid)
	x.Subject = Encode("e", generation, sid)
	x.Object = Encode("e", generation, oid)
	x.Evidence = Encode("v", generation, eid)
	return x, nil
}

func (s *Service) Projection(ctx context.Context, repo, cursor string, limit int) (Projection, error) {
	if limit < 1 || limit > 100 {
		return Projection{}, fmt.Errorf("limit must be between 1 and 100")
	}
	g, e := s.db.ActiveGeneration(ctx, repo)
	if e != nil {
		return Projection{Nodes: []Entity{}, Edges: []Claim{}, Repository: repo, AppliedLimits: map[string]int{"nodes": limit, "edges": limit}}, nil
	}
	if e = s.db.RequireProjection(ctx, "ui"); e != nil {
		return Projection{}, fmt.Errorf("knowledge is safe; projection is unavailable: %w", e)
	}
	off := 0
	if cursor != "" {
		continuation, ok := planner.DecodeContinuation(cursor, g.ID, "ui_projection/"+repo)
		if !ok {
			return Projection{}, fmt.Errorf("projection cursor is stale or invalid")
		}
		off = continuation.Offset
	}
	r := Projection{Nodes: []Entity{}, Edges: []Claim{}, Repository: repo, Generation: g.ID, AppliedLimits: map[string]int{"nodes": limit, "edges": limit}}
	rows, e := s.db.QueryCanonical(ctx, `SELECT p.entity_id FROM projection_ui_nodes p JOIN active_projection_builds a ON a.projection_build_id=p.projection_build_id JOIN entities e ON e.entity_id=p.entity_id WHERE a.projection_kind='ui' AND p.generation_id=? ORDER BY e.kind,e.label,e.path,e.entity_id LIMIT ? OFFSET ?`, g.ID, limit+1, off)
	if e != nil {
		return r, e
	}
	var nodeIDs []string
	for rows.Next() {
		var id string
		if e = rows.Scan(&id); e != nil {
			rows.Close()
			return r, e
		}
		nodeIDs = append(nodeIDs, id)
	}
	if e = rows.Err(); e != nil {
		rows.Close()
		return r, e
	}
	rows.Close()
	for _, id := range nodeIDs {
		x, z := s.entity(ctx, g.ID, id)
		if z != nil {
			return r, z
		}
		r.Nodes = append(r.Nodes, x)
	}
	if len(r.Nodes) > limit {
		r.Nodes = r.Nodes[:limit]
		r.Truncated = true
		r.NextCursor = planner.EncodeContinuation(planner.Continuation{Generation: g.ID, Family: "ui_projection/" + repo, Offset: off + limit})
	}
	ids := map[string]bool{}
	for _, x := range r.Nodes {
		h, _ := Decode(x.Handle)
		ids[h.ID] = true
	}
	claims, e := s.db.QueryCanonical(ctx, `SELECT p.claim_id FROM projection_ui_edges p JOIN active_projection_builds a ON a.projection_build_id=p.projection_build_id JOIN claims c ON c.claim_id=p.claim_id WHERE a.projection_kind='ui' AND p.generation_id=? ORDER BY c.predicate,c.claim_id LIMIT ?`, g.ID, limit+1)
	if e != nil {
		return r, e
	}
	var claimIDs []string
	for claims.Next() {
		var id string
		if e = claims.Scan(&id); e != nil {
			claims.Close()
			return r, e
		}
		claimIDs = append(claimIDs, id)
	}
	if e = claims.Err(); e != nil {
		claims.Close()
		return r, e
	}
	claims.Close()
	for _, id := range claimIDs {
		c, z := s.claim(ctx, g.ID, id)
		if z != nil {
			return r, z
		}
		sh, _ := Decode(c.Subject)
		oh, _ := Decode(c.Object)
		if ids[sh.ID] && ids[oh.ID] {
			r.Edges = append(r.Edges, c)
		}
	}
	if len(r.Edges) > limit {
		r.Edges = r.Edges[:limit]
		r.Truncated = true
	}
	return r, nil
}
func (s *Service) Neighbors(ctx context.Context, handle string, types []string, limit int) ([]Claim, bool, error) {
	h, e := Decode(handle)
	if e != nil || h.Type != "e" {
		return nil, false, fmt.Errorf("entity handle required")
	}
	if e = s.active(ctx, h); e != nil {
		return nil, false, e
	}
	if e = s.db.RequireProjection(ctx, "graph"); e != nil {
		return nil, false, fmt.Errorf("knowledge is safe; projection is unavailable: %w", e)
	}
	if limit < 1 || limit > 100 {
		return nil, false, fmt.Errorf("limit must be between 1 and 100")
	}
	q := `SELECT p.claim_id FROM projection_graph_records p JOIN active_projection_builds a ON a.projection_build_id=p.projection_build_id JOIN claims c ON c.claim_id=p.claim_id WHERE a.projection_kind='graph' AND p.generation_id=? AND (p.subject_id=? OR p.object_id=?)`
	args := []any{h.Generation, h.ID, h.ID}
	if len(types) > 0 {
		q += " AND lower(predicate) IN (" + strings.TrimRight(strings.Repeat("?,", len(types)), ",") + ")"
		for _, t := range types {
			args = append(args, strings.ToLower(t))
		}
	}
	q += " ORDER BY predicate,claim_id LIMIT ?"
	args = append(args, limit+1)
	rows, e := s.db.QueryCanonical(ctx, q, args...)
	if e != nil {
		return nil, false, e
	}
	defer rows.Close()
	var out []Claim
	for rows.Next() {
		var id string
		_ = rows.Scan(&id)
		c, z := s.claim(ctx, h.Generation, id)
		if z != nil {
			return nil, false, z
		}
		out = append(out, c)
	}
	tr := len(out) > limit
	if tr {
		out = out[:limit]
	}
	return out, tr, rows.Err()
}
func (s *Service) Excerpt(ctx context.Context, evidence string, before, after, max int) (Excerpt, error) {
	h, e := Decode(evidence)
	if e != nil || h.Type != "v" {
		return Excerpt{}, fmt.Errorf("evidence handle required")
	}
	if e = s.active(ctx, h); e != nil {
		return Excerpt{}, e
	}
	if before < 0 || after < 0 || max < 1 || max > 200 {
		return Excerpt{}, fmt.Errorf("invalid source bounds")
	}
	var content, sid string
	var untracked int
	var x Excerpt
	e = s.db.QueryRowCanonical(ctx, "SELECT f.content,f.source_id,v.repo_id,v.path,v.file_sha256,v.generation_id,g.git_commit,g.dirty,g.untracked_count,v.start_byte,v.end_byte,v.start_line,v.start_column,v.end_line,v.end_column FROM evidence v JOIN source_files f ON f.source_id=v.source_id JOIN generations g ON g.generation_id=v.generation_id WHERE v.generation_id=? AND v.evidence_id=?", h.Generation, h.ID).Scan(&content, &sid, &x.Repository, &x.Path, &x.Revision, &x.Generation, &x.GitCommit, &x.WorkingTree, &untracked, &x.Span.StartByte, &x.Span.EndByte, &x.Span.StartLine, &x.Span.StartColumn, &x.Span.EndLine, &x.Span.EndColumn)
	if e != nil {
		return x, fmt.Errorf("handle is stale or unknown")
	}
	x.WorkingTree = x.WorkingTree || untracked > 0
	all := strings.Split(content, "\n")
	start := maxInt(1, x.Span.StartLine-before)
	end := minInt(len(all), x.Span.EndLine+after)
	if end-start+1 > max {
		end = start + max - 1
		x.Truncated = true
	}
	x.SHA256 = x.Revision
	x.Evidence = evidence
	x.Source = Encode("s", h.Generation, sid)
	x.StartLine = start
	x.EndLine = end
	x.Lines = all[start-1 : end]
	return x, nil
}

// Query is a bounded canonical lexical/structural read. The UI receives the
// same active-generation candidates and stable ordering as store consumers.
func (s *Service) Query(ctx context.Context, in Query) (result QueryResult, retErr error) {
	ctx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()
	defer func() {
		if errors.Is(retErr, context.DeadlineExceeded) || ctx.Err() != nil {
			result = QueryResult{Status: "unknown", Entities: []Entity{}, Trace: []TraceEvent{{Kind: "budget_exhausted", Detail: "query time budget exhausted"}}, AppliedLimits: map[string]int{"time_ms": 1000}}
			retErr = nil
		}
	}()
	if len(in.Text) > 256 || len(in.Repository) > 63 || in.MinimumConfidence < 0 || in.MinimumConfidence > 1 {
		return QueryResult{Status: "unknown", Entities: []Entity{}, Trace: []TraceEvent{{Kind: "unsupported_query", Detail: "query exceeds supported input bounds"}}}, nil
	}
	if strings.TrimSpace(in.Text) == "" {
		return QueryResult{Entities: []Entity{}, Status: "unknown", Trace: []TraceEvent{{Kind: "empty_query", Detail: "query is empty"}}}, nil
	}
	if in.Limit == 0 {
		in.Limit = 20
	}
	if in.Limit < 1 || in.Limit > 100 {
		return QueryResult{}, fmt.Errorf("limit must be between 1 and 100")
	}
	// Query readiness belongs to the selected route and its read snapshot.
	// Full diagnostics validates every projection and needlessly repeats the
	// lookup completeness scan before ExactCandidates validates that snapshot.
	snapshots, e := s.db.Status(ctx, "")
	if e != nil {
		return QueryResult{}, e
	}
	if len(snapshots) == 0 {
		return QueryResult{Status: "unknown", Entities: []Entity{}, Trace: []TraceEvent{{Kind: "no_active_generation", Detail: "index approved sources first"}}}, nil
	}
	capability := in.Capability
	if capability == "" {
		capability = "lexical"
	}
	if capability != "lexical" && capability != "structural" && capability != "path" {
		return QueryResult{Status: "unknown", Entities: []Entity{}, Trace: []TraceEvent{{Kind: "unsupported_query", Detail: "unsupported search capability"}}}, nil
	}
	basis, e := s.db.Coverage(ctx, in.Repository, capability, nil)
	if e != nil {
		return QueryResult{}, e
	}
	active := map[string]bool{}
	for _, snapshot := range snapshots {
		active[snapshot.RepoID] = true
	}
	for _, repo := range s.cfg.SourceRepositories() {
		if !active[repo.ID] && (in.Repository == "" || in.Repository == repo.ID) {
			basis.Complete = false
			basis.Uncertainty = append(basis.Uncertainty, "repository_not_indexed:"+repo.ID)
		}
	}
	if in.Repository != "" {
		if _, e := s.db.ActiveGeneration(ctx, in.Repository); e != nil {
			return QueryResult{Entities: []Entity{}, Status: "unknown", Trace: []TraceEvent{{Kind: "no_active_generation", Detail: in.Repository}}, AppliedLimits: map[string]int{"results": in.Limit}}, nil
		}
	}
	filter := store.QueryFilter{Repository: in.Repository, MinimumConfidence: in.MinimumConfidence}
	candidates, e := s.db.ExactCandidates(ctx, in.Text, filter)
	if e != nil {
		return QueryResult{Status: "unknown", Entities: []Entity{}, Coverage: &basis, Trace: []TraceEvent{{Kind: "projection_unavailable", Detail: "lookup projection is stale or unavailable"}}}, nil
	}
	identities := map[string]bool{}
	for _, c := range candidates {
		identities[c.Entity.ID] = true
	}
	if len(identities) != 1 {
		if len(candidates) == 0 && strings.IndexFunc(in.Text, func(r rune) bool { return unicode.IsLetter(r) || unicode.IsNumber(r) }) < 0 {
			return QueryResult{Status: "unknown", Entities: []Entity{}, Coverage: &basis, Trace: []TraceEvent{{Kind: "unsupported_query", Detail: "punctuation-only text has no lexical search tokens"}}}, nil
		}
		if e = s.db.RequireProjection(ctx, "lexical"); e != nil {
			return QueryResult{Status: "unknown", Entities: []Entity{}, Coverage: &basis, Trace: []TraceEvent{{Kind: "projection_unavailable", Detail: "lexical projection is stale or unavailable"}}}, nil
		}
		candidates, e = s.db.LexicalCandidates(ctx, in.Text, filter)
		if e != nil {
			return QueryResult{}, e
		}
	}
	if capability == "structural" {
		structural, structuralErr := s.db.StructuralCandidates(ctx, in.Text, filter)
		if structuralErr == nil {
			candidates = append(candidates, structural...)
		} else {
			basis.Complete = false
			basis.Uncertainty = append(basis.Uncertainty, "structural_projection_unavailable")
		}
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		a, b := candidates[i], candidates[j]
		if capability == "path" && (a.Entity.Kind == "file") != (b.Entity.Kind == "file") {
			return a.Entity.Kind == "file"
		}
		if a.Entity.ID != b.Entity.ID {
			return a.Entity.ID < b.Entity.ID
		}
		return a.Evidence.ID < b.Evidence.ID
	})
	out := QueryResult{Coverage: &basis, Entities: []Entity{}, Status: "found", AppliedLimits: map[string]int{"results": in.Limit, "candidates": 500, "time_ms": 1000}, Trace: []TraceEvent{{Kind: "deterministic_planner", Detail: "exact identity then lexical fallback"}}}
	if len(candidates) > 500 {
		candidates = candidates[:500]
		out.Truncated = true
		out.Trace = append(out.Trace, TraceEvent{Kind: "budget_exhausted", Detail: "candidate budget bounded retrieval"})
	}
	seen := map[string]bool{}
	for _, c := range candidates {
		if seen[c.Entity.ID] {
			continue
		}
		seen[c.Entity.ID] = true
		x, z := s.entity(ctx, c.Entity.GenerationID, c.Entity.ID)
		if z != nil {
			return out, z
		}
		out.Entities = append(out.Entities, x)
		out.Trace = append(out.Trace, TraceEvent{Kind: "rank", Detail: c.MatchType, Handle: x.Handle, Rank: len(out.Entities)})
		if len(out.Entities) == in.Limit {
			out.Truncated = out.Truncated || len(candidates) > len(out.Entities)
			break
		}
	}
	if len(out.Entities) == 0 {
		if basis.Complete && !out.Truncated {
			out.Status = "not_found"
			out.Trace = append(out.Trace, TraceEvent{Kind: "not_found", Detail: "complete applicable active coverage searched"})
		} else {
			out.Status = "unknown"
			out.Trace = append(out.Trace, TraceEvent{Kind: "coverage_incomplete", Detail: "coverage cannot support absence"})
		}
	}
	return out, nil
}
func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
func SortClaims(v []Claim) { sort.Slice(v, func(i, j int) bool { return v[i].Handle < v[j].Handle }) }

var _ = sql.ErrNoRows
