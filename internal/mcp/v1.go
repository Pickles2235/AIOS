package mcpserver

import (
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"fmt"
	cachepkg "github.com/AdamNi-7080/AIOS/internal/cache"
	"github.com/AdamNi-7080/AIOS/internal/catalog"
	contextpkg "github.com/AdamNi-7080/AIOS/internal/context"
	"github.com/AdamNi-7080/AIOS/internal/model"
	"github.com/AdamNi-7080/AIOS/internal/planner"
	"github.com/AdamNi-7080/AIOS/internal/policy"
	slicepkg "github.com/AdamNi-7080/AIOS/internal/slice"
	"github.com/AdamNi-7080/AIOS/internal/store"
	vectorpkg "github.com/AdamNi-7080/AIOS/internal/vector"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"sync"
	"time"
)

type v1Service struct {
	cfg            catalog.Config
	db             *store.Store
	cache          *cachepkg.Store
	plannerMu      sync.Mutex
	plannerMetrics map[string]int
	embedder       vectorpkg.Embedder
}
type scope struct {
	RepoID string `json:"repo_id,omitempty"`
}
type statusIn struct{ scope }
type resolveIn struct {
	scope
	Text              string   `json:"text"`
	Cursor            string   `json:"cursor,omitempty"`
	Kinds             []string `json:"kinds,omitempty"`
	Fields            []string `json:"fields,omitempty"`
	Languages         []string `json:"languages,omitempty"`
	Classifications   []string `json:"classifications,omitempty"`
	Version           string   `json:"version,omitempty"`
	MinimumConfidence float64  `json:"minimum_confidence,omitempty"`
	Limit             int      `json:"limit,omitempty"`
}
type searchIn struct {
	scope
	Query             string   `json:"query"`
	Modes             []string `json:"modes,omitempty"`
	Kinds             []string `json:"kinds,omitempty"`
	Fields            []string `json:"fields,omitempty"`
	Languages         []string `json:"languages,omitempty"`
	Classifications   []string `json:"classifications,omitempty"`
	Version           string   `json:"version,omitempty"`
	MinimumConfidence float64  `json:"minimum_confidence,omitempty"`
	Limit             int      `json:"limit,omitempty"`
	Cursor            string   `json:"cursor,omitempty"`
}
type queryIn struct {
	scope
	Text              string         `json:"text"`
	Intent            planner.Intent `json:"intent,omitempty"`
	Anchor            string         `json:"anchor,omitempty"`
	From              string         `json:"from,omitempty"`
	To                string         `json:"to,omitempty"`
	RelationshipTypes []string       `json:"relationship_types,omitempty"`
	Direction         string         `json:"direction,omitempty"`
	RequireEvidence   bool           `json:"require_evidence,omitempty"`
	Kinds             []string       `json:"kinds,omitempty"`
	Fields            []string       `json:"fields,omitempty"`
	Languages         []string       `json:"languages,omitempty"`
	Classifications   []string       `json:"classifications,omitempty"`
	Version           string         `json:"version,omitempty"`
	MinimumConfidence float64        `json:"minimum_confidence,omitempty"`
	TimeMS            int            `json:"time_ms,omitempty"`
	MaxCandidates     int            `json:"max_candidates,omitempty"`
	MaxEntities       int            `json:"max_entities,omitempty"`
	MaxEdges          int            `json:"max_edges,omitempty"`
	Depth             int            `json:"depth,omitempty"`
	Limit             int            `json:"limit,omitempty"`
	Cursor            string         `json:"cursor,omitempty"`
	RetrievalMode     string         `json:"retrieval_mode,omitempty"`
}
type contextIn struct {
	scope
	Text               string   `json:"text"`
	Kinds              []string `json:"kinds,omitempty"`
	Fields             []string `json:"fields,omitempty"`
	Languages          []string `json:"languages,omitempty"`
	Classifications    []string `json:"classifications,omitempty"`
	Version            string   `json:"version,omitempty"`
	MinimumConfidence  float64  `json:"minimum_confidence,omitempty"`
	TimeMS             int      `json:"time_ms,omitempty"`
	MaxCandidates      int      `json:"max_candidates,omitempty"`
	Depth              int      `json:"depth,omitempty"`
	MaxBytes           int      `json:"max_bytes,omitempty"`
	EstimatedTokens    int      `json:"estimated_tokens,omitempty"`
	MaxEntities        int      `json:"max_entities,omitempty"`
	MaxEdges           int      `json:"max_edges,omitempty"`
	MaxExcerpts        int      `json:"max_excerpts,omitempty"`
	MaxLinesPerExcerpt int      `json:"max_lines_per_excerpt,omitempty"`
	SliceHandle        string   `json:"slice_handle,omitempty"`
}
type sliceIn struct {
	scope
	Kind           slicepkg.Kind `json:"kind"`
	Anchor         string        `json:"anchor,omitempty"`
	EntityHandles  []string      `json:"entity_handles,omitempty"`
	Depth          int           `json:"depth,omitempty"`
	MaxFanout      int           `json:"max_fanout,omitempty"`
	MaxEntities    int           `json:"max_entities,omitempty"`
	MaxEdges       int           `json:"max_edges,omitempty"`
	MaxSourceLines int           `json:"max_source_lines,omitempty"`
}
type handleIn struct {
	Handle string `json:"handle"`
}
type sourceIn struct {
	Handle   string `json:"handle"`
	Before   int    `json:"before,omitempty"`
	After    int    `json:"after,omitempty"`
	MaxLines int    `json:"max_lines,omitempty"`
}
type neighborsIn struct {
	Handle        string   `json:"handle"`
	Types         []string `json:"types,omitempty"`
	Direction     string   `json:"direction,omitempty"`
	Depth, Budget int
	MaxFanout     int    `json:"max_fanout,omitempty"`
	MaxEntities   int    `json:"max_entities,omitempty"`
	MaxEdges      int    `json:"max_edges,omitempty"`
	Cursor        string `json:"cursor,omitempty"`
}
type pathIn struct {
	From     string   `json:"from"`
	To       string   `json:"to"`
	Types    []string `json:"types,omitempty"`
	MaxDepth int      `json:"max_depth,omitempty"`
	MaxPaths int      `json:"max_paths,omitempty"`
}
type referencesIn struct {
	scope
	Handle, Relation string
	Limit            int
	Cursor           string `json:"cursor,omitempty"`
}
type eventsIn struct {
	scope
	Event string
	Limit int
}
type impactIn struct {
	scope
	Handle       string
	Depth, Limit int
	Cursor       string `json:"cursor,omitempty"`
}
type explainIn struct {
	Handle  string   `json:"handle,omitempty"`
	Subject string   `json:"subject,omitempty"`
	Object  string   `json:"object,omitempty"`
	Claim   string   `json:"claim,omitempty"`
	Cursor  string   `json:"cursor,omitempty"`
	Query   *queryIn `json:"query,omitempty"`
}
type landmarksIn struct {
	scope
	Service           string   `json:"service,omitempty"`
	Kinds             []string `json:"kinds,omitempty"`
	MinimumConfidence float64  `json:"minimum_confidence,omitempty"`
	Limit             int      `json:"limit,omitempty"`
	Cursor            string   `json:"cursor,omitempty"`
}
type entityOut struct {
	Handle, Kind, Label, Identity, Extractor, Repository, Path, Generation, Evidence string
	Confidence                                                                       float64    `json:"confidence,omitempty"`
	Span                                                                             model.Span `json:"span,omitempty"`
}
type evidenceOut struct {
	Handle, Source, Repository, Path, Revision, Generation, Excerpt string
	SourceKind                                                      string `json:"source_kind,omitempty"`
	SourceAdapterVersion                                            string `json:"source_adapter_version,omitempty"`
	Span                                                            model.Span
}
type claimOut struct {
	Handle, Subject, Predicate, Object, Evidence, Extractor, Derivation string
	Confidence                                                          float64
	Generation                                                          string
	Resolver                                                            string `json:"resolver,omitempty"`
	MatchingInputs                                                      string `json:"matching_inputs,omitempty"`
	ObjectEvidence                                                      string `json:"object_evidence,omitempty"`
}
type result struct {
	Status             string          `json:"status"`
	Entities           []entityOut     `json:"entities,omitempty"`
	Evidence           []evidenceOut   `json:"evidence,omitempty"`
	Claims             []claimOut      `json:"claims,omitempty"`
	NextCursor         string          `json:"next_cursor,omitempty"`
	Truncated          bool            `json:"truncated,omitempty"`
	TruncationReason   string          `json:"truncation_reason,omitempty"`
	AppliedLimits      map[string]int  `json:"applied_limits"`
	Trace              []string        `json:"plan_trace"`
	Coverage           map[string]any  `json:"coverage"`
	QueryFamily        string          `json:"query_family,omitempty"`
	QueryFeatures      any             `json:"query_features,omitempty"`
	ExecutedPlan       any             `json:"executed_plan,omitempty"`
	PlanExplanation    any             `json:"plan_explanation,omitempty"`
	CandidateTrace     []candidateOut  `json:"candidate_generation_trace,omitempty"`
	BudgetStops        []planner.Stop  `json:"budget_stops,omitempty"`
	ExpansionHandle    string          `json:"expansion_handle,omitempty"`
	SearchedScope      map[string]any  `json:"searched_scope,omitempty"`
	Uncertainty        []string        `json:"uncertainty,omitempty"`
	NegativeEvidence   string          `json:"negative_evidence,omitempty"`
	Slice              *slicepkg.Slice `json:"slice,omitempty"`
	FilteredCandidates []string        `json:"filtered_candidates,omitempty"`
}

// provenanceEnvelope is the V1 wire contract. The nested result remains the
// existing factual payload; this envelope records that projections and scores
// are retrieval aids, never authority.
type provenanceEnvelope struct {
	AssertionStatus model.AssertionStatus `json:"assertion_status"`
	CanonicalIDs    []string              `json:"canonical_ids,omitempty"`
	EvidenceHandles []string              `json:"evidence_handles,omitempty"`
	Generations     []string              `json:"ir_generations,omitempty"`
	ProjectionState string                `json:"projection_state"`
	FactState       string                `json:"fact_state"`
	Bounded         bool                  `json:"bounded"`
}

type resultPayload result

func (r result) MarshalJSON() ([]byte, error) {
	provenance := resultProvenance(r)
	return json.Marshal(struct {
		APIVersion   int                `json:"api_version"`
		Provenance   provenanceEnvelope `json:"provenance"`
		Result       resultPayload      `json:"result"`
		ResultHandle string             `json:"result_handle,omitempty"`
	}{APIVersion: 1, Provenance: provenance, Result: resultPayload(r), ResultHandle: resultHandle(r, provenance)})
}

func resultProvenance(r result) provenanceEnvelope {
	p := provenanceEnvelope{AssertionStatus: model.AssertionDirect, ProjectionState: "canonical_ir", FactState: "direct", Bounded: r.Truncated}
	seen := map[string]bool{}
	add := func(handle string, evidence bool) {
		if handle == "" || seen[handle] {
			return
		}
		seen[handle] = true
		p.CanonicalIDs = append(p.CanonicalIDs, handle)
		if h, err := dec(handle); err == nil {
			if h.G != "" {
				p.Generations = append(p.Generations, h.G)
			}
			if evidence || h.T == "v" {
				p.EvidenceHandles = append(p.EvidenceHandles, handle)
			}
		}
	}
	for _, entity := range r.Entities {
		add(entity.Handle, false)
		add(entity.Evidence, true)
	}
	for _, evidence := range r.Evidence {
		add(evidence.Handle, true)
	}
	for _, claim := range r.Claims {
		add(claim.Handle, false)
		add(claim.Evidence, true)
		add(claim.ObjectEvidence, true)
	}
	if r.Status == "unknown" {
		p.AssertionStatus, p.FactState = model.AssertionUnavailable, "unavailable"
	}
	if r.Truncated {
		p.AssertionStatus, p.FactState = model.AssertionBounded, "bounded"
	}
	sort.Strings(p.CanonicalIDs)
	sort.Strings(p.EvidenceHandles)
	sort.Strings(p.Generations)
	p.Generations = uniqueStrings(p.Generations)
	return p
}

func resultHandle(r result, p provenanceEnvelope) string {
	if len(p.Generations) == 0 {
		return ""
	}
	value, _ := json.Marshal(struct{ Entities, Evidence, Claims []string }{p.CanonicalIDs, p.EvidenceHandles, claimHandles(r.Claims)})
	return enc("r", p.Generations[0], base64.RawURLEncoding.EncodeToString(value))
}
func claimHandles(claims []claimOut) []string {
	out := make([]string, 0, len(claims))
	for _, claim := range claims {
		out = append(out, claim.Handle)
	}
	sort.Strings(out)
	return uniqueStrings(out)
}
func uniqueStrings(in []string) []string {
	out := in[:0]
	for _, value := range in {
		if len(out) == 0 || out[len(out)-1] != value {
			out = append(out, value)
		}
	}
	return out
}

type candidateOut struct {
	Handle        string                 `json:"handle"`
	Match         string                 `json:"match"`
	Sources       []planner.Source       `json:"sources"`
	Ranks         map[planner.Source]int `json:"ranks"`
	Confidence    float64                `json:"confidence"`
	GraphDistance int                    `json:"graph_distance,omitempty"`
	Rank          int                    `json:"rank"`
	VectorScore   *float64               `json:"vector_score,omitempty"`
	RetrievalAid  bool                   `json:"retrieval_aid,omitempty"`
}
type sourceOut struct {
	Evidence           evidenceOut `json:"evidence,omitempty"`
	Source             string      `json:"source"`
	Lines              []string    `json:"lines"`
	StartLine, EndLine int
	Truncated          bool           `json:"truncated,omitempty"`
	TruncationReason   string         `json:"truncation_reason,omitempty"`
	AppliedLimits      map[string]int `json:"applied_limits"`
	Trace              []string       `json:"plan_trace"`
}
type landmarkOut struct {
	Handle, Kind, Repository, Service, Label, CatalogRevision string
	Summary, Coverage                                         map[string]any
	Evidence, Claims, Dependencies, SourceGenerations         []string
	Builder, BuilderVersion, RecomputationReason              string
	Confidence                                                float64
}
type landmarksResult struct {
	Status, NextCursor string
	Records            []landmarkOut
	Truncated          bool
	AppliedLimits      map[string]int
	Coverage           map[string]any
}

func (r landmarksResult) MarshalJSON() ([]byte, error) {
	type payload landmarksResult
	evidence := []string{}
	ids := []string{}
	for _, record := range r.Records {
		ids = append(ids, record.Handle)
		evidence = append(evidence, record.Evidence...)
	}
	sort.Strings(ids)
	sort.Strings(evidence)
	state := model.AssertionDerived
	if r.Status == "unknown" {
		state = model.AssertionUnavailable
	} else if r.Truncated {
		state = model.AssertionBounded
	}
	return json.Marshal(struct {
		APIVersion int                `json:"api_version"`
		Provenance provenanceEnvelope `json:"provenance"`
		Result     payload            `json:"result"`
	}{1, provenanceEnvelope{AssertionStatus: state, CanonicalIDs: uniqueStrings(ids), EvidenceHandles: uniqueStrings(evidence), ProjectionState: "landmarks", FactState: string(state), Bounded: r.Truncated}, payload(r)})
}

type statusResult struct{ store.Status }

func (r statusResult) MarshalJSON() ([]byte, error) {
	type payload statusResult
	state := model.AssertionDirect
	if !r.Health.Healthy {
		state = model.AssertionUnavailable
	}
	return json.Marshal(struct {
		APIVersion int                `json:"api_version"`
		Provenance provenanceEnvelope `json:"provenance"`
		Result     payload            `json:"result"`
	}{1, provenanceEnvelope{AssertionStatus: state, ProjectionState: "operator_status", FactState: string(state)}, payload(r)})
}

type contextResult struct{ contextpkg.Package }

func (r contextResult) MarshalJSON() ([]byte, error) {
	type payload contextResult
	return json.Marshal(struct {
		APIVersion int                `json:"api_version"`
		Provenance provenanceEnvelope `json:"provenance"`
		Result     payload            `json:"result"`
	}{1, provenanceEnvelope{AssertionStatus: model.AssertionDerived, ProjectionState: "canonical_ir", FactState: "derived", Bounded: r.Truncated}, payload(r)})
}

type entityResult struct{ entityOut }

func (r entityResult) MarshalJSON() ([]byte, error) {
	type payload entityResult
	return json.Marshal(struct {
		APIVersion int                `json:"api_version"`
		Provenance provenanceEnvelope `json:"provenance"`
		Result     payload            `json:"result"`
	}{1, provenanceEnvelope{AssertionStatus: model.AssertionDirect, CanonicalIDs: []string{r.Handle}, EvidenceHandles: []string{r.Evidence}, Generations: []string{r.Generation}, ProjectionState: "canonical_ir", FactState: "direct"}, payload(r)})
}

type sourceResult struct{ sourceOut }

func (r sourceResult) MarshalJSON() ([]byte, error) {
	type payload sourceResult
	return json.Marshal(struct {
		APIVersion int                `json:"api_version"`
		Provenance provenanceEnvelope `json:"provenance"`
		Result     payload            `json:"result"`
	}{1, provenanceEnvelope{AssertionStatus: model.AssertionDirect, EvidenceHandles: []string{r.Evidence.Handle}, Generations: []string{r.Evidence.Generation}, ProjectionState: "canonical_ir", FactState: "direct", Bounded: r.Truncated}, payload(r)})
}

func NewV1(cfg catalog.Config, db *store.Store) *mcp.Server {
	return NewV1WithCache(cfg, db, cachepkg.Disabled("not configured"))
}
func NewV1WithCache(cfg catalog.Config, db *store.Store, cache *cachepkg.Store) *mcp.Server {
	s := &v1Service{cfg: cfg, db: db, cache: cache, plannerMetrics: map[string]int{}, embedder: vectorpkg.NewBundled(filepath.Dir(db.Path()))}
	v := mcp.NewServer(&mcp.Implementation{Name: "aios", Version: "1.0.0"}, nil)
	mcp.AddTool(v, tool("kb.resolve", "Resolve canonical entities."), s.resolve)
	mcp.AddTool(v, tool("kb.search", "Search canonical source evidence."), s.search)
	mcp.AddTool(v, tool("kb.query", "Plan and execute bounded hybrid repository retrieval."), s.query)
	mcp.AddTool(v, tool("kb.slice", "Read a bounded evidence-backed architecture slice."), s.slice)
	mcp.AddTool(v, tool("kb.context", "Compile a bounded canonical evidence package."), s.context)
	mcp.AddTool(v, tool("kb.landmarks", "Read bounded deterministic precomputed knowledge."), s.landmarks)
	mcp.AddTool(v, tool("kb.entity", "Fetch a canonical entity."), s.entity)
	mcp.AddTool(v, tool("kb.source", "Fetch bounded cited source lines."), s.source)
	mcp.AddTool(v, tool("kb.neighbors", "Traverse typed relationships."), s.neighbors)
	mcp.AddTool(v, tool("kb.path", "Find bounded typed paths."), s.path)
	mcp.AddTool(v, tool("kb.references", "Find typed references."), s.references)
	mcp.AddTool(v, tool("kb.events", "Find event relationships."), s.events)
	mcp.AddTool(v, tool("kb.impact", "Find bounded dependents."), s.impact)
	mcp.AddTool(v, tool("kb.explain", "Explain an entity or relationship."), s.explain)
	mcp.AddTool(v, tool("kb.status", "Read active indexing status and diagnostics."), s.status)
	return v
}
func (s *v1Service) status(ctx context.Context, _ *mcp.CallToolRequest, in statusIn) (*mcp.CallToolResult, statusResult, error) {
	if err := s.repo(in.RepoID); err != nil {
		return nil, statusResult{}, err
	}
	out, err := s.db.Diagnostics(ctx, in.RepoID)
	if s.cache != nil {
		out.Cache = s.cache.Diagnostics(ctx)
	}
	out.Planner = s.plannerDiagnostics()
	return nil, statusResult{out}, err
}

func (s *v1Service) plannerMetric(name string) {
	s.plannerMu.Lock()
	defer s.plannerMu.Unlock()
	if s.plannerMetrics == nil {
		s.plannerMetrics = map[string]int{}
	}
	s.plannerMetrics[name]++
}
func (s *v1Service) plannerDiagnostics() map[string]int {
	s.plannerMu.Lock()
	defer s.plannerMu.Unlock()
	out := map[string]int{}
	for k, v := range s.plannerMetrics {
		out[k] = v
	}
	return out
}

// resolvePlan caches a redacted plan template. The request text participates
// only through its hash and is restored in memory before a response is made.
func (s *v1Service) resolvePlan(ctx context.Context, text, repo string, budget planner.Budget, requestFingerprint string) (planner.Family, planner.Features, planner.Node, []string, error) {
	family, features, plan, err := planner.Compile(text, budget)
	if err != nil {
		return "", planner.Features{}, planner.Node{}, nil, err
	}
	if s.cache == nil || !s.cache.Available() {
		return family, features, plan, []string{"cache_unavailable"}, nil
	}
	stamp, err := s.generationStamp(ctx, repo)
	if err != nil {
		return family, features, plan, []string{"cache_miss"}, nil
	}
	diagnostics, _ := s.db.Diagnostics(ctx, repo)
	projections := map[string]string{}
	for _, p := range diagnostics.Projections {
		projections[p.Kind] = p.Fingerprint
	}
	meta := cachepkg.Metadata{Kind: cachepkg.QueryPlan, RequestFingerprint: requestFingerprint, SourceScope: s.coveredRepositories(repo), GenerationFingerprint: stamp, ProjectionFingerprints: projections, BudgetFingerprint: cachepkg.Fingerprint(plannerLimits(budget)), OutputShape: "redacted_plan_template", Builder: cachepkg.BuilderVersion}
	type template struct {
		Family   planner.Family   `json:"family"`
		Features planner.Features `json:"features"`
		Plan     planner.Node     `json:"plan"`
	}
	entry, hit, err := s.cache.Do(ctx, meta, func(current cachepkg.Metadata) bool {
		return current.GenerationFingerprint == stamp && reflect.DeepEqual(current.ProjectionFingerprints, projections)
	}, func(context.Context) ([]byte, error) {
		return json.Marshal(template{family, features, redactPlan(plan)})
	})
	if err != nil {
		return family, features, plan, []string{"cache_unavailable"}, nil
	}
	var cached template
	if json.Unmarshal(entry.Payload, &cached) == nil && cached.Family != "" {
		return cached.Family, cached.Features, restorePlan(cached.Plan, text), []string{map[bool]string{true: "cache_hit", false: "cache_miss"}[hit]}, nil
	}
	return family, features, plan, []string{"cache_miss"}, nil
}
func redactPlan(n planner.Node) planner.Node {
	n.Text = ""
	for i := range n.Inputs {
		n.Inputs[i] = ptrPlan(redactPlan(*n.Inputs[i]))
	}
	return n
}
func restorePlan(n planner.Node, text string) planner.Node {
	if n.Operator == planner.Exact || n.Operator == planner.Lexical {
		n.Text = text
	}
	for i := range n.Inputs {
		n.Inputs[i] = ptrPlan(restorePlan(*n.Inputs[i], text))
	}
	return n
}
func ptrPlan(n planner.Node) *planner.Node { return &n }
func tool(n, d string) *mcp.Tool {
	f := false
	return &mcp.Tool{
		Name: n, Description: d,
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true, IdempotentHint: true, OpenWorldHint: &f},
		OutputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"api_version":   map[string]any{"type": "integer"},
				"provenance":    map[string]any{"type": "object"},
				"result":        map[string]any{"type": "object"},
				"result_handle": map[string]any{"type": "string"},
			},
			"required":             []string{"api_version", "provenance", "result"},
			"additionalProperties": false,
		},
	}
}
func lim(n, d int) (int, error) {
	if n == 0 {
		return d, nil
	}
	if n < 1 || n > 100 {
		return 0, fmt.Errorf("limit must be between 1 and 100")
	}
	return n, nil
}
func depth(n, d int) (int, error) {
	if n == 0 {
		return d, nil
	}
	if n < 1 || n > 4 {
		return 0, fmt.Errorf("depth must be between 1 and 4")
	}
	return n, nil
}
func (s *v1Service) repo(id string) error {
	if id == "" {
		return nil
	}
	for _, r := range s.cfg.SourceRepositories() {
		if r.ID == id {
			return nil
		}
	}
	return fmt.Errorf("repository %q is not in catalog", id)
}

type h struct{ T, G, I string }

func enc(t, g, id string) string {
	b, _ := json.Marshal(h{t, g, id})
	return "kb." + t + "1." + base64.RawURLEncoding.EncodeToString(b)
}
func dec(x string) (h, error) {
	var v h
	p := strings.SplitN(x, ".", 3)
	if len(p) != 3 || p[0] != "kb" {
		return v, fmt.Errorf("invalid handle")
	}
	b, e := base64.RawURLEncoding.DecodeString(p[2])
	if e != nil || json.Unmarshal(b, &v) != nil || v.T+"1" != p[1] {
		return v, fmt.Errorf("invalid handle")
	}
	return v, nil
}
func (s *v1Service) active(ctx context.Context, g h) (store.Generation, error) {
	var repo string
	err := s.db.QueryRowCanonical(ctx, `SELECT repo_id FROM generations WHERE generation_id=?`, g.G).Scan(&repo)
	if err != nil {
		return store.Generation{}, fmt.Errorf("handle is stale or unknown")
	}
	a, e := s.db.ActiveGeneration(ctx, repo)
	if e != nil || a.ID != g.G {
		return store.Generation{}, fmt.Errorf("handle is stale or unknown")
	}
	return a, nil
}
func scanEntity(rows *sql.Rows) (entityOut, error) {
	var x entityOut
	var eid string
	if e := rows.Scan(&eid, &x.Kind, &x.Label, &x.Identity, &x.Extractor, &x.Repository, &x.Path, &x.Generation, &x.Evidence); e != nil {
		return x, e
	}
	x.Handle = enc("e", x.Generation, eid)
	return x, nil
}
func scanEvidence(rows interface{ Scan(...any) error }) (evidenceOut, error) {
	var x evidenceOut
	var id, sid string
	if e := rows.Scan(&id, &sid, &x.Repository, &x.Path, &x.Revision, &x.Generation, &x.Span.StartByte, &x.Span.EndByte, &x.Span.StartLine, &x.Span.StartColumn, &x.Span.EndLine, &x.Span.EndColumn, &x.Excerpt); e != nil {
		return x, e
	}
	x.Handle = enc("v", x.Generation, id)
	x.Source = enc("s", x.Generation, sid)
	x.SourceKind, x.SourceAdapterVersion = model.SourceKindRepository, model.RepositoryAdapterVersion
	return x, nil
}
func scanClaim(rows interface{ Scan(...any) error }) (claimOut, error) {
	var x claimOut
	var id, s, o, e string
	if z := rows.Scan(&id, &s, &x.Predicate, &o, &e, &x.Extractor, &x.Derivation, &x.Confidence, &x.Generation); z != nil {
		return x, z
	}
	x.Handle = enc("c", x.Generation, id)
	x.Subject = enc("e", x.Generation, s)
	x.Object = enc("e", x.Generation, o)
	x.Evidence = enc("v", x.Generation, e)
	return x, nil
}
func (s *v1Service) scanCrossClaim(c store.CrossClaim) claimOut {
	return claimOut{Handle: enc("x", c.CatalogRevision, c.ID), Subject: s.entityHandleForID(c.SubjectID), Predicate: c.Predicate, Object: s.entityHandleForID(c.ObjectID), Evidence: s.evidenceHandleForID(c.SubjectEvidenceID), ObjectEvidence: s.evidenceHandleForID(c.ObjectEvidenceID), Extractor: c.Resolver, Resolver: c.Resolver, MatchingInputs: c.MatchingInputs, Derivation: c.Derivation, Confidence: c.Confidence, Generation: c.CatalogRevision}
}
func (s *v1Service) resolve(ctx context.Context, _ *mcp.CallToolRequest, in resolveIn) (*mcp.CallToolResult, result, error) {
	if e := s.repo(in.RepoID); e != nil {
		return nil, result{}, e
	}
	q, e := policy.Query(in.Text)
	if e != nil {
		return nil, result{}, e
	}
	l, e := lim(in.Limit, 20)
	if e != nil {
		return nil, result{}, e
	}
	_, out, err := s.retrieve(ctx, q, retrievalInput{repo: in.RepoID, kinds: in.Kinds, fields: in.Fields, languages: in.Languages, classifications: in.Classifications, version: in.Version, minimumConfidence: in.MinimumConfidence, limit: l, cursor: in.Cursor, entities: true})
	if err != nil || out.Status != "found" {
		return nil, out, err
	}
	identities := map[string]bool{}
	for _, entity := range out.Entities {
		if strings.EqualFold(entity.Label, q) {
			identities[entity.Repository+"/"+entity.Path+"/"+entity.Identity] = true
		}
	}
	if len(identities) > 1 {
		out.Status = "unknown"
		out.Uncertainty = append(out.Uncertainty, "ambiguous_target; specify repository, kind, path, or language")
		out.Trace = append(out.Trace, "ambiguous_resolution")
	}
	return nil, out, nil
}
func (s *v1Service) search(ctx context.Context, _ *mcp.CallToolRequest, in searchIn) (*mcp.CallToolResult, result, error) {
	if e := s.repo(in.RepoID); e != nil {
		return nil, result{}, e
	}
	q, e := policy.Query(in.Query)
	if e != nil {
		return nil, result{}, e
	}
	l, e := lim(in.Limit, 20)
	if e != nil {
		return nil, result{}, e
	}
	if len(in.Modes) > 0 {
		for _, mode := range in.Modes {
			if mode != "exact" && mode != "fts" {
				return nil, result{}, fmt.Errorf("unsupported search mode")
			}
		}
	}
	return s.retrieve(ctx, q, retrievalInput{repo: in.RepoID, kinds: in.Kinds, fields: in.Fields, languages: in.Languages, classifications: in.Classifications, version: in.Version, minimumConfidence: in.MinimumConfidence, limit: l, cursor: in.Cursor})
}

func (s *v1Service) query(ctx context.Context, _ *mcp.CallToolRequest, in queryIn) (*mcp.CallToolResult, result, error) {
	if err := s.repo(in.RepoID); err != nil {
		return nil, result{}, err
	}
	in.RetrievalMode = strings.ToLower(strings.TrimSpace(in.RetrievalMode))
	if in.RetrievalMode != "" && in.RetrievalMode != "auto" && in.RetrievalMode != "vector" {
		return nil, result{}, fmt.Errorf("retrieval_mode must be auto or vector")
	}
	request, err := (planner.Request{Text: in.Text, Repository: in.RepoID, Version: in.Version, Intent: in.Intent, Anchor: in.Anchor, From: in.From, To: in.To, RelationshipTypes: in.RelationshipTypes, Direction: in.Direction, RequireEvidence: in.RequireEvidence}).Normalized()
	if err != nil {
		return nil, result{}, err
	}
	text := request.Text
	if text != "" {
		text, err = policy.Query(text)
		if err != nil {
			return nil, result{}, err
		}
		request.Text = text
	}
	b, err := (planner.Budget{TimeMS: in.TimeMS, Candidates: in.MaxCandidates, Entities: in.MaxEntities, Edges: in.MaxEdges, Depth: in.Depth, Results: in.Limit}).Normalized()
	if err != nil {
		return nil, result{}, err
	}
	selected, err := planner.Plan(request, b)
	if err != nil {
		return nil, result{}, err
	}
	s.plannerMetric("strategy_" + string(selected.Strategy))
	if request.Intent == planner.IntentPathFinding {
		_, out, err := s.path(ctx, nil, pathIn{From: request.From, To: request.To, Types: request.RelationshipTypes, MaxDepth: b.Depth, MaxPaths: b.Results})
		out.PlanExplanation = selected
		out.Trace = append([]string{"deterministic_staged_planner", "route:exact:selected_handles", "route:lexical:skipped_explicit_path", "route:graph:skipped_explicit_path", "route:structural:skipped_explicit_path", "route:path:selected", "route:vector:skipped_prior_result"}, out.Trace...)
		return nil, out, err
	}
	if request.Intent == planner.IntentArchitectureSlice {
		kind, ok := planner.SliceKind(text)
		if !ok {
			return nil, result{Status: "unknown", PlanExplanation: selected, AppliedLimits: plannerLimits(b), Trace: []string{"deterministic_staged_planner", "architecture_slice_kind_required"}, Uncertainty: []string{"specify a fixed architecture slice kind in text"}}, nil
		}
		_, out, err := s.slice(ctx, nil, sliceIn{scope: scope{RepoID: in.RepoID}, Kind: slicepkg.Kind(kind), Anchor: request.Anchor, Depth: b.Depth, MaxEntities: b.Entities, MaxEdges: b.Edges})
		out.PlanExplanation = selected
		out.Trace = append([]string{"deterministic_staged_planner", "route:exact:selected_anchor", "route:lexical:skipped_explicit_slice", "route:graph:skipped_explicit_slice", "route:structural:selected_slice_profile", "route:path:skipped_explicit_slice", "route:vector:skipped_prior_result"}, out.Trace...)
		return nil, out, err
	}
	family, features, plan, cacheTrace, err := s.resolvePlan(ctx, text, in.RepoID, b, cachepkg.Fingerprint(struct {
		Request                                   planner.Request
		Kinds, Fields, Languages, Classifications []string
		MinimumConfidence                         float64
	}{request, in.Kinds, in.Fields, in.Languages, in.Classifications, in.MinimumConfidence}))
	if err != nil {
		return nil, result{}, err
	}
	if kind, ok := planner.SliceKind(text); ok {
		cacheTrace = append(cacheTrace, "slice_shape_available:"+kind+"; explicit kb.slice anchor required")
	}
	stamp, err := s.generationStamp(ctx, in.RepoID)
	if err != nil {
		return nil, result{Status: "unknown", QueryFamily: string(family), QueryFeatures: features, ExecutedPlan: plan, PlanExplanation: selected, AppliedLimits: plannerLimits(b), Coverage: map[string]any{"complete": false, "gap": "no_active_generation"}, Trace: append([]string{"coverage_incomplete"}, cacheTrace...)}, nil
	}
	offset := 0
	if in.Cursor != "" {
		c, ok := planner.DecodeContinuation(in.Cursor, stamp, string(family))
		if !ok {
			return nil, result{}, fmt.Errorf("invalid continuation handle")
		}
		offset = c.Offset
	}
	filter := store.QueryFilter{Repository: in.RepoID, Version: in.Version, Kinds: in.Kinds, Fields: in.Fields, Languages: in.Languages, Classifications: in.Classifications, MinimumConfidence: in.MinimumConfidence}
	capability := "exact"
	if selected.Strategy == planner.StrategyLexical {
		capability = "lexical"
	}
	if selected.Strategy == planner.StrategyGraph || selected.Strategy == planner.StrategyPath || selected.Strategy == planner.StrategySlice {
		capability = "structural"
	}
	basis, err := s.db.Coverage(ctx, in.RepoID, capability, in.Fields)
	if err != nil {
		return nil, result{}, err
	}
	searchedScope := map[string]any{"repositories": basis.Repositories, "fields": in.Fields, "indexes": basis.Indexes, "generations": basis.Generations, "exclusions": basis.Exclusions}
	for _, uncertainty := range basis.Uncertainty {
		if strings.HasPrefix(uncertainty, "index_unavailable:") {
			return nil, result{Status: "unknown", QueryFamily: string(family), QueryFeatures: features, ExecutedPlan: plan, PlanExplanation: selected, AppliedLimits: plannerLimits(b), Coverage: coverageMap(basis, stamp, in.Fields, s.compilerDiagnostics(ctx, in.RepoID)), SearchedScope: searchedScope, Uncertainty: append(basis.Uncertainty, "rebuild the required projection and retry"), Trace: []string{"coverage_incomplete"}}, nil
		}
	}
	started := time.Now()
	exec := planner.Execution{Started: started}
	routeTrace := []string{}
	filter.RelationshipTypes = request.RelationshipTypes
	var exact []store.Candidate
	var exactErr error
	if request.Intent != planner.IntentTextSearch {
		exact, exactErr = s.db.ExactCandidates(ctx, text, filter)
		if exactErr != nil {
			routeTrace = append(routeTrace, "route:exact:unavailable")
		} else if len(exact) == 0 {
			routeTrace = append(routeTrace, "route:exact:insufficient")
		} else {
			routeTrace = append(routeTrace, "route:exact:selected")
		}
	} else {
		routeTrace = append(routeTrace, "route:exact:skipped_explicit_text_search")
	}
	var lexical, structural, landmarks, graph, vectors []store.Candidate
	vectorScores := map[string]float64{}
	vectorCondition := ""
	if exactErr == nil {
		for _, c := range exact {
			exec.Add(planner.SourceExact, []planner.Candidate{plannerCandidate(c)}, b)
		}
	}
	uniqueExact := map[string]bool{}
	for _, c := range exact {
		uniqueExact[c.Entity.ID] = true
	}
	// A generic lookup stops at exactly one canonical identity. All other
	// modes use exact only as an anchor, then invoke their explicit capability.
	if request.Intent == planner.IntentTextSearch || (request.Intent == planner.IntentLookup && len(uniqueExact) != 1) {
		lexical, err = s.db.LexicalCandidates(ctx, text, filter)
		if err != nil {
			return nil, result{}, err
		}
		for _, c := range lexical {
			exec.Add(planner.SourceLexical, []planner.Candidate{plannerCandidate(c)}, b)
		}
		if len(lexical) == 0 {
			routeTrace = append(routeTrace, "route:lexical:insufficient")
		} else {
			routeTrace = append(routeTrace, "route:lexical:selected")
		}
	} else {
		routeTrace = append(routeTrace, "route:lexical:skipped_unique_exact")
	}
	if selected.Strategy == planner.StrategyGraph {
		seeds := make([]string, 0, len(exact))
		for _, c := range exact {
			seeds = append(seeds, c.Entity.ID)
		}
		if len(seeds) == 0 {
			exec.Stop("anchor_unresolved", 0, 1)
		}
		var z error
		graph, z = s.db.GraphCandidates(ctx, seeds, filter, b.Depth, b.Candidates)
		if z != nil {
			return nil, result{}, z
		}
		for _, c := range graph {
			exec.Add(planner.SourceGraph, []planner.Candidate{plannerCandidate(c)}, b)
		}
		if len(graph) == 0 {
			routeTrace = append(routeTrace, "route:graph:insufficient")
		} else {
			routeTrace = append(routeTrace, "route:graph:selected")
		}
	} else {
		routeTrace = append(routeTrace, "route:graph:skipped_intent")
	}
	if len(exec.Candidates) == 0 {
		structural, err = s.db.StructuralCandidates(ctx, text, filter)
		if err != nil {
			routeTrace = append(routeTrace, "route:structural:unavailable")
		} else {
			for _, c := range structural {
				exec.Add(planner.SourceStructural, []planner.Candidate{plannerCandidate(c)}, b)
			}
			if len(structural) == 0 {
				routeTrace = append(routeTrace, "route:structural:insufficient")
			} else {
				routeTrace = append(routeTrace, "route:structural:selected")
			}
		}
	} else {
		routeTrace = append(routeTrace, "route:structural:skipped_prior_result")
	}
	routeTrace = append(routeTrace, "route:path:skipped_non_path_intent")
	// Semantic ranking is deliberately last and never changes a successful
	// exact/lexical/graph answer. Explicit vector mode remains bounded by the
	// same scope, generation and filter constraints.
	if request.Intent == planner.IntentNegativeVerification {
		routeTrace = append(routeTrace, "route:vector:skipped_negative_verification")
	} else if in.RetrievalMode == "vector" || (in.RetrievalMode != "vector" && len(exec.Candidates) == 0) {
		if !s.cfg.Vector.Enabled {
			vectorCondition = "vector_disabled"
		} else if s.embedder == nil {
			vectorCondition = "vector_unavailable"
		} else {
			vectorCtx, vectorCancel := context.WithDeadline(ctx, started.Add(time.Duration(b.TimeMS)*time.Millisecond))
			candidates, scores, e := s.db.VectorCandidates(vectorCtx, text, filter, s.embedder, b.Candidates)
			exhausted := vectorCtx.Err() != nil
			vectorCancel()
			if e != nil {
				vectorCondition = "vector_unavailable"
				if exhausted {
					vectorCondition = "vector_time_budget"
					exec.Stop("time_budget", int(time.Since(started).Milliseconds()), b.TimeMS)
				}
			} else {
				vectors = candidates
				for i, c := range candidates {
					vectorScores[c.Entity.ID+"\x00"+c.Evidence.ID] = scores[i]
					exec.Add(planner.SourceVector, []planner.Candidate{plannerCandidate(c)}, b)
				}
			}
		}
		if vectorCondition != "" {
			routeTrace = append(routeTrace, "route:vector:unavailable")
		} else if len(vectors) == 0 {
			routeTrace = append(routeTrace, "route:vector:insufficient")
		} else {
			routeTrace = append(routeTrace, "route:vector:selected")
		}
	} else {
		routeTrace = append(routeTrace, "route:vector:skipped_prior_result")
	}
	fused := planner.Fuse(exec.Candidates)
	trace := append([]string{"deterministic_staged_planner", "canonical_evidence_only", "stable_fusion"}, routeTrace...)
	trace = append(trace, cacheTrace...)
	r := result{Status: "found", QueryFamily: string(family), QueryFeatures: features, ExecutedPlan: plan, PlanExplanation: selected, AppliedLimits: plannerLimits(b), Coverage: coverageMap(basis, stamp, in.Fields, s.compilerDiagnostics(ctx, in.RepoID)), SearchedScope: searchedScope, Uncertainty: basis.Uncertainty, Trace: trace, BudgetStops: exec.Stops}
	if vectorCondition != "" {
		r.Trace = append(r.Trace, vectorCondition)
		if in.RetrievalMode == "vector" {
			r.Uncertainty = append(r.Uncertainty, vectorCondition+"; exact, lexical and structural retrieval remain available")
		}
	}
	byKey := map[string]store.Candidate{}
	for _, c := range append(append(append(append(append(exact, lexical...), structural...), landmarks...), graph...), vectors...) {
		byKey[c.Entity.ID+"\x00"+c.Evidence.ID] = c
	}
	if len(fused) == 0 {
		if in.RetrievalMode == "vector" && vectorCondition != "" {
			r.Status = "unknown"
			if vectorCondition == "vector_time_budget" {
				r.Trace = append(r.Trace, "budget_incomplete")
				r.Uncertainty = append(r.Uncertainty, "query time budget exhausted; deterministic canonical retrieval remains available")
			} else {
				r.Trace = append(r.Trace, "requested_vector_unavailable")
				r.Uncertainty = append(r.Uncertainty, "requested vector projection is unavailable; rebuild it or use deterministic non-vector retrieval")
			}
		} else if len(exec.Stops) > 0 || !basis.Complete {
			r.Status = "unknown"
			if len(exec.Stops) > 0 {
				r.Trace = append(r.Trace, "budget_incomplete")
			} else {
				r.Trace = append(r.Trace, "coverage_incomplete")
				r.Uncertainty = append(r.Uncertainty, "narrow scope or repair ingestion")
			}
		} else {
			r.Status = "not_found"
			r.Trace = append(r.Trace, "active_scope_fully_searched")
			negative, e := s.recordNegative(ctx, string(family), text, "planner-v1:"+stamp+":"+string(family), searchedScope, basis)
			if e != nil {
				return nil, r, e
			}
			r.NegativeEvidence = negative
		}
		return nil, r, nil
	}
	if offset > len(fused) {
		return nil, result{}, fmt.Errorf("invalid continuation handle")
	}
	pageSize := minInt(b.Results, b.Entities)
	end := minInt(offset+pageSize, len(fused))
	if b.Entities < b.Results && end < len(fused) {
		exec.Stop("entity_budget", end-offset, b.Entities)
	}
	for rank, c := range fused[offset:end] {
		trace := candidateOut{Handle: c.Key, Match: c.Match, Sources: c.Sources, Ranks: c.Ranks, Confidence: c.Confidence, GraphDistance: c.GraphDistance, Rank: offset + rank + 1}
		if score, ok := vectorScores[c.Key]; ok {
			trace.VectorScore = &score
			trace.RetrievalAid = true
		}
		r.CandidateTrace = append(r.CandidateTrace, trace)
		origin := byKey[c.Key]
		e := entityOut{Handle: enc("e", origin.Entity.GenerationID, origin.Entity.ID), Kind: origin.Entity.Kind, Label: origin.Entity.Label, Repository: origin.Entity.RepoID, Path: origin.Entity.Path, Generation: origin.Entity.GenerationID, Evidence: enc("v", origin.Evidence.GenerationID, origin.Evidence.ID), Confidence: origin.Confidence, Span: origin.Evidence.Span}
		v := evidenceOut{Handle: enc("v", origin.Evidence.GenerationID, origin.Evidence.ID), Source: enc("s", origin.Evidence.GenerationID, origin.Evidence.SourceID), Repository: origin.Evidence.RepoID, Path: origin.Evidence.Path, Revision: origin.Evidence.SHA256, Generation: origin.Evidence.GenerationID, Excerpt: origin.Evidence.Excerpt, SourceKind: model.SourceKindRepository, SourceAdapterVersion: model.RepositoryAdapterVersion, Span: origin.Evidence.Span}
		r.Entities = append(r.Entities, e)
		r.Evidence = append(r.Evidence, v)
		claims, z := s.claimsForEntity(ctx, origin.Entity.ID, origin.Entity.GenerationID)
		if z != nil {
			return nil, r, z
		}
		for _, claim := range claims {
			if len(r.Claims) >= b.Edges {
				exec.Stop("edge_budget", len(r.Claims), b.Edges)
				break
			}
			r.Claims = append(r.Claims, claim)
		}
	}
	r.Claims = uniqueClaims(r.Claims)
	r.BudgetStops = exec.Stops
	if selected.Strategy == planner.StrategyLexical {
		_, _ = s.db.RecordDiagnostic(ctx, model.DiagnosticEvent{Code: store.DiagnosticPlannerFallback, Severity: model.DiagnosticInfo, Scope: model.DiagnosticScope{Repository: in.RepoID, Query: cachepkg.Fingerprint(text)}, Remediation: "use a canonical handle or narrower structural intent when exact resolution is required", Metadata: map[string]string{"strategy": string(selected.Strategy)}})
	}
	if len(exec.Stops) > 0 {
		_, _ = s.db.RecordDiagnostic(ctx, model.DiagnosticEvent{Code: store.DiagnosticPlannerBudgetExceeded, Severity: model.DiagnosticWarning, Scope: model.DiagnosticScope{Repository: in.RepoID, Query: cachepkg.Fingerprint(text)}, Remediation: "narrow scope or request the continuation handle", Metadata: map[string]string{"reason": exec.Stops[0].Reason}})
	}
	if end < len(fused) || len(exec.Stops) > 0 {
		r.Truncated = true
		if len(exec.Stops) > 0 {
			r.TruncationReason = exec.Stops[0].Reason
		} else {
			r.TruncationReason = "result_budget"
		}
		r.NextCursor = planner.EncodeContinuation(planner.Continuation{Generation: stamp, Family: string(family), Offset: end})
		r.ExpansionHandle = r.NextCursor
	}
	return nil, r, nil
}

// sliceToken is opaque at the MCP boundary. Its contents are deliberately
// limited to normalized request data, never facts or SQLite identifiers.
type sliceToken struct {
	Selector slicepkg.Selector
	Budget   slicepkg.Budget
}

func (s *v1Service) encodeSliceHandle(stamp string, t sliceToken) string {
	b, _ := json.Marshal(t)
	return enc("a", stamp, base64.RawURLEncoding.EncodeToString(b))
}
func (s *v1Service) decodeSliceHandle(ctx context.Context, handle string) (sliceToken, error) {
	h, err := dec(handle)
	if err != nil || h.T != "a" {
		return sliceToken{}, fmt.Errorf("slice handle required")
	}
	b, err := base64.RawURLEncoding.DecodeString(h.I)
	var t sliceToken
	if err != nil || json.Unmarshal(b, &t) != nil {
		return sliceToken{}, fmt.Errorf("invalid slice handle")
	}
	var normalized slicepkg.Selector
	if normalized, err = t.Selector.Normalized(); err != nil {
		return sliceToken{}, fmt.Errorf("invalid slice handle")
	}
	t.Selector = normalized
	if t.Budget, err = t.Budget.Normalized(); err != nil {
		return sliceToken{}, fmt.Errorf("invalid slice handle")
	}
	stamp, err := s.generationStamp(ctx, t.Selector.Repository)
	if err != nil || stamp != h.G {
		return sliceToken{}, fmt.Errorf("slice handle is stale or unknown")
	}
	return t, nil
}

func sliceLimits(b slicepkg.Budget) map[string]int {
	return map[string]int{"depth": b.Depth, "max_fanout": b.Fanout, "max_entities": b.Entities, "max_edges": b.Edges, "max_source_lines": b.SourceLines}
}

func (s *v1Service) slice(ctx context.Context, rq *mcp.CallToolRequest, in sliceIn) (*mcp.CallToolResult, result, error) {
	selector, err := (slicepkg.Selector{Kind: in.Kind, Anchor: in.Anchor, EntityHandles: in.EntityHandles, Repository: in.RepoID}).Normalized()
	if err != nil {
		return nil, result{}, err
	}
	b, err := (slicepkg.Budget{Depth: in.Depth, Fanout: in.MaxFanout, Entities: in.MaxEntities, Edges: in.MaxEdges, SourceLines: in.MaxSourceLines}).Normalized()
	if err != nil {
		return nil, result{}, err
	}
	stamp, err := s.generationStamp(ctx, selector.Repository)
	if err != nil {
		return s.buildSlice(ctx, rq, in)
	}
	if s.cache == nil || !s.cache.Available() {
		return s.buildSlice(ctx, rq, in)
	}
	diagnostics, _ := s.db.Diagnostics(ctx, selector.Repository)
	projections := map[string]string{}
	for _, p := range diagnostics.Projections {
		projections[p.Kind] = p.Fingerprint
	}
	meta := cachepkg.Metadata{Kind: cachepkg.PrecomputedSlice, RequestFingerprint: cachepkg.Fingerprint(sliceToken{Selector: selector, Budget: b}), SourceScope: s.coveredRepositories(selector.Repository), GenerationFingerprint: stamp, ProjectionFingerprints: projections, BudgetFingerprint: cachepkg.Fingerprint(b), OutputShape: "architecture_slice_v1", Builder: cachepkg.BuilderVersion}
	entry, hit, err := s.cache.Do(ctx, meta, func(current cachepkg.Metadata) bool {
		return current.GenerationFingerprint == stamp && reflect.DeepEqual(current.ProjectionFingerprints, projections)
	}, func(context.Context) ([]byte, error) {
		_, out, e := s.buildSlice(ctx, rq, in)
		if e != nil {
			return nil, e
		}
		return json.Marshal(resultPayload(out))
	})
	if err != nil {
		return s.buildSlice(ctx, rq, in)
	}
	var out result
	if json.Unmarshal(entry.Payload, &out) != nil || s.validateSliceResult(ctx, out) != nil {
		return s.buildSlice(ctx, rq, in)
	}
	if out.Slice != nil {
		out.Slice.Provenance.CacheState = map[bool]string{true: "cache_hit", false: "cache_miss"}[hit]
	}
	out.Trace = append(out.Trace, map[bool]string{true: "cache_hit", false: "cache_miss"}[hit])
	return nil, out, nil
}

func (s *v1Service) validateSliceResult(ctx context.Context, r result) error {
	if r.Slice == nil {
		return fmt.Errorf("cached slice has no descriptor")
	}
	if _, err := s.decodeSliceHandle(ctx, r.Slice.Handle); err != nil {
		return err
	}
	for _, x := range r.Entities {
		h, e := dec(x.Handle)
		if e != nil {
			return e
		}
		if _, e = s.active(ctx, h); e != nil {
			return e
		}
	}
	for _, x := range r.Evidence {
		h, e := dec(x.Handle)
		if e != nil {
			return e
		}
		if _, e = s.active(ctx, h); e != nil {
			return e
		}
	}
	return nil
}

func (s *v1Service) buildSlice(ctx context.Context, _ *mcp.CallToolRequest, in sliceIn) (*mcp.CallToolResult, result, error) {
	selector, err := (slicepkg.Selector{Kind: in.Kind, Anchor: in.Anchor, EntityHandles: in.EntityHandles, Repository: in.RepoID}).Normalized()
	if err != nil {
		return nil, result{}, err
	}
	if err := s.repo(selector.Repository); err != nil {
		return nil, result{}, err
	}
	b, err := (slicepkg.Budget{Depth: in.Depth, Fanout: in.MaxFanout, Entities: in.MaxEntities, Edges: in.MaxEdges, SourceLines: in.MaxSourceLines}).Normalized()
	if err != nil {
		return nil, result{}, err
	}
	stamp, err := s.generationStamp(ctx, selector.Repository)
	if err != nil {
		return nil, result{Status: "unknown", AppliedLimits: sliceLimits(b), Coverage: map[string]any{"complete": false, "gap": "no_active_generation"}, Trace: []string{"slice_coverage_incomplete"}}, nil
	}
	anchors, uncertainty, err := s.sliceAnchors(ctx, selector)
	if err != nil {
		return nil, result{}, err
	}
	base := &slicepkg.Slice{Kind: selector.Kind, GenerationFingerprint: stamp, Repositories: s.coveredRepositories(selector.Repository), Budget: b, Trace: []string{"explicit_anchor_resolution", "fixed_slice_profile", "canonical_claim_adjacency"}}
	for _, a := range anchors {
		base.Anchors = append(base.Anchors, a.Handle)
	}
	base.Handle = s.encodeSliceHandle(stamp, sliceToken{Selector: selector, Budget: b})
	if len(uncertainty) > 0 {
		base.Uncertainty = uncertainty
		base.Trace = append(base.Trace, "ambiguous_or_unsupported_anchor")
		return nil, result{Status: "unknown", AppliedLimits: sliceLimits(b), Coverage: map[string]any{"complete": false, "gap": "ambiguous_anchor"}, Uncertainty: uncertainty, Trace: base.Trace, Slice: base}, nil
	}
	basis, err := s.db.Coverage(ctx, selector.Repository, "structural", nil)
	if err != nil {
		return nil, result{}, err
	}
	base.Coverage = coverageMap(basis, stamp, nil, s.compilerDiagnostics(ctx, selector.Repository))
	if !basis.Complete {
		base.Uncertainty = append(base.Uncertainty, basis.Uncertainty...)
		base.Trace = append(base.Trace, "coverage_incomplete")
		return nil, result{Status: "unknown", AppliedLimits: sliceLimits(b), Coverage: base.Coverage, Uncertainty: base.Uncertainty, Trace: base.Trace, Slice: base}, nil
	}
	predicates, direction := slicepkg.Predicates(selector.Kind)
	r := result{Status: "found", AppliedLimits: sliceLimits(b), Coverage: base.Coverage, Trace: base.Trace, Slice: base}
	front := make([]h, 0, len(anchors))
	seenEntity := map[string]bool{}
	seenClaim := map[string]bool{}
	for _, a := range anchors {
		x, _ := dec(a.Handle)
		front = append(front, x)
		seenEntity[x.G+"/"+x.I] = true
		r.Entities = append(r.Entities, a)
	}
	for level := 0; level < b.Depth && len(front) > 0; level++ {
		next := []h{}
		for _, node := range front {
			claims, truncated, e := s.claims(ctx, node.G, node.I, predicates, direction, b.Fanout)
			if e != nil {
				return nil, r, e
			}
			if truncated {
				r.Truncated = true
				r.TruncationReason = "fanout_budget"
				base.Omissions = append(base.Omissions, slicepkg.Omission{Kind: "claim", Handle: enc("e", node.G, node.I), Reason: "fanout_budget", Expansion: map[string]any{"operation": "kb.neighbors", "input": map[string]any{"handle": enc("e", node.G, node.I), "depth": 1, "max_fanout": b.Fanout}}})
			}
			for _, c := range claims {
				if len(r.Claims) >= b.Edges {
					r.Truncated = true
					r.TruncationReason = "edge_budget"
					base.Omissions = append(base.Omissions, slicepkg.Omission{Kind: "claim", Handle: c.Handle, Reason: "edge_budget", Expansion: map[string]any{"operation": "kb.neighbors", "input": map[string]any{"handle": enc("e", node.G, node.I)}}})
					break
				}
				if seenClaim[c.Handle] {
					continue
				}
				seenClaim[c.Handle] = true
				r.Claims = append(r.Claims, c)
				for _, eh := range []string{c.Subject, c.Object} {
					x, _ := dec(eh)
					key := x.G + "/" + x.I
					if !seenEntity[key] {
						if len(seenEntity) >= b.Entities {
							r.Truncated = true
							r.TruncationReason = "entity_budget"
							base.Omissions = append(base.Omissions, slicepkg.Omission{Kind: "entity", Handle: eh, Reason: "entity_budget", Expansion: map[string]any{"operation": "kb.entity", "input": map[string]any{"handle": eh}}})
							continue
						}
						seenEntity[key] = true
						next = append(next, x)
					}
				}
			}
		}
		front = next
	}
	if err := s.attachProvenance(ctx, "", &r); err != nil {
		return nil, r, err
	}
	// Return evidence excerpts bounded per source line budget. The original
	// evidence handle remains the expansion point for the full cited span.
	for i := range r.Evidence {
		_, src, e := s.source(ctx, nil, sourceIn{Handle: r.Evidence[i].Handle, MaxLines: b.SourceLines})
		if e != nil {
			return nil, r, e
		}
		r.Evidence[i].Excerpt = strings.Join(src.Lines, "\n")
		if src.Truncated {
			r.Truncated = true
			if r.TruncationReason == "" {
				r.TruncationReason = "source_line_budget"
			}
			base.Omissions = append(base.Omissions, slicepkg.Omission{Kind: "source_range", Handle: r.Evidence[i].Handle, Reason: "source_line_budget", Expansion: map[string]any{"operation": "kb.source", "input": map[string]any{"handle": r.Evidence[i].Handle}}})
		}
	}
	sort.SliceStable(r.Entities, func(i, j int) bool {
		return r.Entities[i].Repository+"\x00"+r.Entities[i].Path+"\x00"+r.Entities[i].Handle < r.Entities[j].Repository+"\x00"+r.Entities[j].Path+"\x00"+r.Entities[j].Handle
	})
	sort.SliceStable(r.Claims, func(i, j int) bool {
		return r.Claims[i].Predicate+r.Claims[i].Subject+r.Claims[i].Object+r.Claims[i].Handle < r.Claims[j].Predicate+r.Claims[j].Subject+r.Claims[j].Object+r.Claims[j].Handle
	})
	base.Trace = append(base.Trace, "direct_and_compiler_resolved_evidence_preferred", "stable_deduplication")
	return nil, r, nil
}

func (s *v1Service) sliceAnchors(ctx context.Context, selector slicepkg.Selector) ([]entityOut, []string, error) {
	if len(selector.EntityHandles) > 0 {
		out := []entityOut{}
		for _, handle := range selector.EntityHandles {
			h, e := dec(handle)
			if e != nil || h.T != "e" {
				return nil, nil, fmt.Errorf("entity handle required")
			}
			if _, e = s.active(ctx, h); e != nil {
				return nil, nil, e
			}
			x, e := s.entitySummary(ctx, handle)
			if e != nil {
				return nil, nil, e
			}
			if selector.Repository != "" && x.Repository != selector.Repository {
				return nil, nil, fmt.Errorf("anchor is outside repository scope")
			}
			out = append(out, x)
		}
		return out, nil, nil
	}
	_, resolved, err := s.resolve(ctx, nil, resolveIn{scope: scope{RepoID: selector.Repository}, Text: selector.Anchor, Limit: 20})
	if err != nil {
		return nil, nil, err
	}
	// Extraction can create a source-level endpoint beside a declared symbol.
	// Prefer the declared symbol as the anchor, but never pick among peers.
	seen := map[string]entityOut{}
	for _, x := range resolved.Entities {
		if strings.EqualFold(x.Label, selector.Anchor) && strings.HasPrefix(x.Kind, "symbol:") {
			seen[x.Handle] = x
		}
	}
	if len(seen) != 1 {
		return nil, []string{"ambiguous_anchor; use kb.resolve then entity_handles"}, nil
	}
	for _, x := range seen {
		return []entityOut{x}, nil, nil
	}
	return nil, []string{"anchor_not_found"}, nil
}

func (s *v1Service) landmarks(ctx context.Context, _ *mcp.CallToolRequest, in landmarksIn) (*mcp.CallToolResult, landmarksResult, error) {
	if err := s.repo(in.RepoID); err != nil {
		return nil, landmarksResult{}, err
	}
	if in.MinimumConfidence < 0 || in.MinimumConfidence > 1 {
		return nil, landmarksResult{}, fmt.Errorf("minimum_confidence must be between 0 and 1")
	}
	limit, err := lim(in.Limit, 20)
	if err != nil {
		return nil, landmarksResult{}, err
	}
	revision, err := s.db.ActiveCatalogRevision(ctx)
	if err != nil {
		return nil, landmarksResult{Status: "unknown", AppliedLimits: map[string]int{"limit": limit}, Coverage: map[string]any{"complete": false, "gap": "no_active_generation"}}, nil
	}
	offset := 0
	if in.Cursor != "" {
		c, ok := planner.DecodeContinuation(in.Cursor, revision.ID, "landmarks")
		if !ok {
			return nil, landmarksResult{}, fmt.Errorf("invalid continuation handle")
		}
		offset = c.Offset
	}
	records, truncated, err := s.db.Landmarks(ctx, store.LandmarkFilter{Repository: in.RepoID, Service: in.Service, Kinds: in.Kinds, MinimumConfidence: in.MinimumConfidence, Limit: limit, Offset: offset})
	if err != nil {
		return nil, landmarksResult{}, err
	}
	out := landmarksResult{Status: "found", AppliedLimits: map[string]int{"limit": limit}, Coverage: map[string]any{"complete": true, "active_catalog_revision": revision.ID, "source": "canonical_ir_landmarks"}, Truncated: truncated}
	if len(records) == 0 {
		out.Status = "not_found"
	}
	for _, record := range records {
		value := landmarkOut{Handle: enc("l", record.CatalogRevision, record.ID), Kind: record.Kind, Repository: record.Repository, Service: record.Service, Label: record.Label, CatalogRevision: record.CatalogRevision, Summary: record.Summary, Coverage: record.Coverage, Dependencies: append([]string(nil), record.DependencyIDs...), SourceGenerations: append([]string(nil), record.SourceGenerations...), Builder: record.BuilderName, BuilderVersion: record.BuilderVersion, RecomputationReason: record.RecomputationReason, Confidence: record.Confidence}
		for _, id := range record.EvidenceIDs {
			if generation, e := s.handleGeneration(ctx, "evidence", id); e == nil {
				value.Evidence = append(value.Evidence, enc("v", generation, id))
			}
		}
		for _, id := range record.ClaimIDs {
			if generation, e := s.handleGeneration(ctx, "claims", id); e == nil {
				value.Claims = append(value.Claims, enc("c", generation, id))
			}
		}
		out.Records = append(out.Records, value)
	}
	if truncated {
		out.NextCursor = planner.EncodeContinuation(planner.Continuation{Generation: revision.ID, Family: "landmarks", Offset: offset + len(records)})
	}
	return nil, out, nil
}

func (s *v1Service) handleGeneration(ctx context.Context, table, id string) (string, error) {
	var generation string
	if table != "evidence" && table != "claims" {
		return "", fmt.Errorf("invalid canonical table")
	}
	err := s.db.QueryRowCanonical(ctx, `SELECT generation_id FROM `+table+` WHERE `+strings.TrimSuffix(table, "s")+`_id=?`, id).Scan(&generation)
	return generation, err
}

// context compiles a package from the same inspected planner result as kb.query.
// It adds no inference: its entries are canonical entities, claims and source.
func (s *v1Service) context(ctx context.Context, _ *mcp.CallToolRequest, in contextIn) (*mcp.CallToolResult, contextResult, error) {
	b, err := (contextpkg.Budget{MaxBytes: in.MaxBytes, EstimatedTokens: in.EstimatedTokens, Entities: in.MaxEntities, Edges: in.MaxEdges, Excerpts: in.MaxExcerpts, LinesPerExcerpt: in.MaxLinesPerExcerpt}).Normalized()
	if err != nil {
		return nil, contextResult{}, err
	}
	if in.SliceHandle != "" && strings.TrimSpace(in.Text) != "" {
		return nil, contextResult{}, fmt.Errorf("use text or slice_handle, not both")
	}
	var raw result
	if in.SliceHandle != "" {
		token, e := s.decodeSliceHandle(ctx, in.SliceHandle)
		if e != nil {
			return nil, contextResult{}, e
		}
		_, raw, e = s.slice(ctx, nil, sliceIn{scope: scope{RepoID: token.Selector.Repository}, Kind: token.Selector.Kind, Anchor: token.Selector.Anchor, EntityHandles: token.Selector.EntityHandles, Depth: token.Budget.Depth, MaxFanout: token.Budget.Fanout, MaxEntities: token.Budget.Entities, MaxEdges: token.Budget.Edges, MaxSourceLines: token.Budget.SourceLines})
		if e != nil {
			return nil, contextResult{}, e
		}
		raw.QueryFamily = string(token.Selector.Kind)
	} else {
		_, raw, err = s.query(ctx, nil, queryIn{scope: in.scope, Text: in.Text, Kinds: in.Kinds, Fields: in.Fields, Languages: in.Languages, Classifications: in.Classifications, Version: in.Version, MinimumConfidence: in.MinimumConfidence, TimeMS: in.TimeMS, MaxCandidates: in.MaxCandidates, MaxEdges: planner.MaxItems, Depth: in.Depth, Limit: planner.MaxResult})
	}
	if err != nil {
		return nil, contextResult{}, err
	}
	if raw.Status == "unknown" || raw.Status == "not_found" {
		out, compileErr := contextpkg.Compile(contextpkg.Input{Package: contextpkg.Package{Status: raw.Status, AnswerKind: raw.QueryFamily, Derivation: map[string]string{"source": "canonical_repository_knowledge_v1"}, ExecutedPlan: raw.ExecutedPlan, Trace: raw.Trace, Coverage: raw.Coverage, Uncertainty: contextUncertainty(raw)}, Budget: b})
		return nil, contextResult{out}, compileErr
	}
	family := planner.Family(raw.QueryFamily)
	allowed := contextPredicates(family)
	if raw.Slice == nil {
		raw.Claims = filterContextClaims(raw.Claims, allowed)
	}
	for _, entity := range raw.Entities {
		if raw.Slice != nil {
			break
		}
		h, e := dec(entity.Handle)
		if e != nil {
			continue
		}
		cross, _, e := s.crossClaims(ctx, h.I, allowed, contextDirection(family), planner.MaxItems)
		if e != nil {
			return nil, contextResult{}, e
		}
		raw.Claims = append(raw.Claims, cross...)
	}
	raw.Claims = uniqueClaims(raw.Claims)
	if err := s.attachProvenance(ctx, "", &raw); err != nil {
		return nil, contextResult{}, err
	}
	tiers := contextTiers(raw)
	entities := make([]contextpkg.Entity, 0, len(raw.Entities))
	anchors := make([]string, 0, len(raw.Entities))
	for _, e := range raw.Entities {
		anchors = append(anchors, e.Handle)
		h, _ := dec(e.Handle)
		entities = append(entities, contextpkg.Entity{Handle: e.Handle, Kind: e.Kind, Label: e.Label, Identity: e.Identity, Repository: e.Repository, Path: e.Path, Generation: e.Generation, Evidence: e.Evidence, Confidence: e.Confidence, Role: "answer_anchor", EvidenceTier: tiers[h.I]})
	}
	relationships := make([]contextpkg.Relationship, 0, len(raw.Claims))
	for _, c := range raw.Claims {
		relationships = append(relationships, contextpkg.Relationship{Handle: c.Handle, Subject: c.Subject, Predicate: c.Predicate, Object: c.Object, Evidence: c.Evidence, ObjectEvidence: c.ObjectEvidence, Extractor: c.Extractor, Derivation: c.Derivation, Generation: c.Generation, Confidence: c.Confidence, Role: "decisive", EvidenceTier: claimTier(c)})
	}
	excerpts := make([]contextpkg.Excerpt, 0, len(raw.Evidence))
	for _, e := range raw.Evidence {
		_, source, sourceErr := s.source(ctx, nil, sourceIn{Handle: e.Handle, MaxLines: b.LinesPerExcerpt})
		if sourceErr != nil {
			return nil, contextResult{}, sourceErr
		}
		excerpts = append(excerpts, contextpkg.Excerpt{Handle: e.Handle, Source: e.Source, Repository: e.Repository, Path: e.Path, FileSHA256: e.Revision, Generation: e.Generation, Text: strings.Join(source.Lines, "\n"), Span: contextpkg.Span{StartLine: source.StartLine, EndLine: source.EndLine}, OriginalLines: e.Span.EndLine - e.Span.StartLine + 1, Truncated: source.Truncated})
	}
	p := contextpkg.Package{Status: raw.Status, AnswerKind: raw.QueryFamily, AnswerAnchors: anchors, Repositories: s.contextGenerations(ctx, in.RepoID), Derivation: map[string]string{"source": "canonical_repository_knowledge_v1", "selection": "deterministic_context_compiler_v1"}, ExecutedPlan: raw.ExecutedPlan, Trace: append(raw.Trace, "deterministic_context_compiler"), Coverage: raw.Coverage, Uncertainty: contextUncertainty(raw)}
	out, compileErr := contextpkg.Compile(contextpkg.Input{Package: p, Entities: entities, Relationships: relationships, Excerpts: excerpts, Budget: b})
	return nil, contextResult{out}, compileErr
}

func contextPredicates(f planner.Family) []string {
	switch f {
	case planner.EventTrace:
		return []string{"PUBLISHES_EVENT", "CONSUMES_EVENT", "PRODUCES_TOPIC", "CONSUMES_TOPIC", "SENDS_QUEUE", "LISTENS_QUEUE", "EVENT_PRODUCER_CONSUMER"}
	case planner.ExplainCause:
		return []string{"HAS_LOCAL_GUARD", "EMITS_LOG", "EMITS_ERROR", "COVERAGE_GAP"}
	case planner.RouteTrace:
		return []string{"DECLARES_EFFECTIVE_ENDPOINT", "DECLARES_ENDPOINT", "DECLARES_UI_ROUTE", "INVOKES_API"}
	case planner.Configuration:
		return []string{"DEFINES_CONFIGURATION", "REFERENCES_CONFIGURATION", "BINDS_CONFIGURATION"}
	case planner.Impact:
		return nil
	case planner.Negative:
		return []string{"COVERAGE_GAP"}
	default:
		return []string{}
	}
}
func contextDirection(f planner.Family) string {
	if f == planner.Impact {
		return "in"
	}
	return "both"
}
func filterContextClaims(in []claimOut, allowed []string) []claimOut {
	if allowed == nil {
		return in
	}
	out := []claimOut{}
	for _, c := range in {
		if contains(allowed, c.Predicate) {
			out = append(out, c)
		}
	}
	return out
}
func contextTiers(raw result) map[string]int {
	out := map[string]int{}
	for _, c := range raw.CandidateTrace {
		key := strings.SplitN(c.Handle, "\x00", 2)[0]
		tier := 3
		for _, source := range c.Sources {
			if source == planner.SourceExact {
				tier = 0
			}
			if source == planner.SourceStructural && tier > 1 {
				tier = 1
			}
			if source == planner.SourceGraph && tier > 2 {
				tier = 2
			}
		}
		out[key] = tier
	}
	return out
}
func claimTier(c claimOut) int {
	if c.Derivation == "compiler_resolved" || c.Resolver != "" {
		return 0
	}
	if c.Derivation == "syntax_derived" {
		return 1
	}
	return 2
}
func contextUncertainty(raw result) []string {
	out := []string{}
	if raw.Status == "unknown" {
		out = append(out, "active coverage is incomplete")
	}
	if len(raw.BudgetStops) > 0 {
		out = append(out, "planner budget stopped before complete retrieval")
	}
	if _, ok := raw.Coverage["extractor_diagnostics"]; ok {
		out = append(out, "extractor coverage diagnostics are present")
	}
	return out
}
func (s *v1Service) contextGenerations(ctx context.Context, repo string) []contextpkg.RepositoryGeneration {
	q := `SELECT g.repo_id,g.generation_id,g.git_commit,g.content_hash,g.extractor_versions FROM generations g JOIN active_generations a ON a.generation_id=g.generation_id`
	args := []any{}
	if repo != "" {
		q += " WHERE g.repo_id=?"
		args = append(args, repo)
	}
	q += " ORDER BY g.repo_id"
	rows, err := s.db.QueryCanonical(ctx, q, args...)
	if err != nil {
		return nil
	}
	defer rows.Close()
	out := []contextpkg.RepositoryGeneration{}
	for rows.Next() {
		var x contextpkg.RepositoryGeneration
		if rows.Scan(&x.Repository, &x.Generation, &x.GitRevision, &x.ContentHash, &x.ExtractorVersions) == nil {
			out = append(out, x)
		}
	}
	return out
}

func plannerLimits(b planner.Budget) map[string]int {
	return map[string]int{"time_ms": b.TimeMS, "max_candidates": b.Candidates, "max_entities": b.Entities, "max_edges": b.Edges, "depth": b.Depth, "limit": b.Results}
}
func candidateSource(c store.Candidate) planner.Source {
	if c.MatchType == "lexical" {
		return planner.SourceLexical
	}
	return planner.SourceExact
}

// fuseStoreCandidates shares the planner's conservative exact-over-lexical
// ordering with compact legacy retrieval tools without widening their API.
func fuseStoreCandidates(candidates []store.Candidate) []store.Candidate {
	byKey := map[string]store.Candidate{}
	inputs := make([]planner.Candidate, 0, len(candidates))
	for _, c := range candidates {
		key := c.Entity.ID + "\x00" + c.Evidence.ID
		if old, ok := byKey[key]; !ok || matchRank(c.MatchType) < matchRank(old.MatchType) {
			byKey[key] = c
		}
		p := plannerCandidate(c)
		p.Sources = []planner.Source{candidateSource(c)}
		p.Ranks = map[planner.Source]int{candidateSource(c): p.SourceRank}
		inputs = append(inputs, p)
	}
	fused := planner.Fuse(inputs)
	out := make([]store.Candidate, 0, len(fused))
	for _, c := range fused {
		out = append(out, byKey[c.Key])
	}
	return out
}
func plannerCandidate(c store.Candidate) planner.Candidate {
	return planner.Candidate{Key: c.Entity.ID + "\x00" + c.Evidence.ID, Repository: c.Entity.RepoID, Path: c.Entity.Path, Evidence: c.Evidence.ID, Match: c.MatchType, SpanStart: c.Evidence.Span.StartByte, SourceRank: matchRank(c.MatchType), Confidence: c.Confidence}
}
func hasTraverse(n planner.Node) bool {
	if n.Operator == planner.Traverse {
		return true
	}
	for _, x := range n.Inputs {
		if hasTraverse(*x) {
			return true
		}
	}
	return false
}
func (s *v1Service) entity(ctx context.Context, _ *mcp.CallToolRequest, in handleIn) (*mcp.CallToolResult, entityResult, error) {
	x, e := dec(in.Handle)
	if e != nil || x.T != "e" {
		return nil, entityResult{}, fmt.Errorf("entity handle required")
	}
	if _, e = s.active(ctx, x); e != nil {
		return nil, entityResult{}, e
	}
	rows, e := s.db.QueryCanonical(ctx, `SELECT entity_id,kind,label,identity,extractor,repo_id,path,generation_id,evidence_id FROM entities WHERE entity_id=? AND generation_id=?`, x.I, x.G)
	if e != nil {
		return nil, entityResult{}, e
	}
	defer rows.Close()
	if !rows.Next() {
		return nil, entityResult{}, fmt.Errorf("handle is stale or unknown")
	}
	out, e := scanEntity(rows)
	return nil, entityResult{out}, e
}
func (s *v1Service) source(ctx context.Context, _ *mcp.CallToolRequest, in sourceIn) (*mcp.CallToolResult, sourceResult, error) {
	x, e := dec(in.Handle)
	if e != nil || (x.T != "s" && x.T != "v") {
		return nil, sourceResult{}, fmt.Errorf("source or evidence handle required")
	}
	if _, e = s.active(ctx, x); e != nil {
		return nil, sourceResult{}, e
	}
	max := in.MaxLines
	if max == 0 {
		max = 40
	}
	if max < 1 || max > 200 || in.Before < 0 || in.After < 0 {
		return nil, sourceResult{}, fmt.Errorf("invalid source bounds")
	}
	q := `SELECT f.content,f.source_id,f.path`
	args := []any{x.G, x.I}
	if x.T == "v" {
		q = `SELECT f.content,f.source_id,f.path,v.start_line,v.end_line FROM evidence v JOIN source_files f ON f.source_id=v.source_id WHERE v.generation_id=? AND v.evidence_id=?`
	} else {
		q = `SELECT content,source_id,path,1,(length(content)-length(replace(content,char(10),''))+1) FROM source_files WHERE generation_id=? AND source_id=?`
	}
	var content, sid, path string
	var a, b int
	if e = s.db.QueryRowCanonical(ctx, q, args...).Scan(&content, &sid, &path, &a, &b); e != nil {
		return nil, sourceResult{}, fmt.Errorf("handle is stale or unknown")
	}
	lines := strings.Split(content, "\n")
	start := maxInt(1, a-in.Before)
	end := minInt(len(lines), b+in.After)
	r := sourceOut{Source: enc("s", x.G, sid), StartLine: start, EndLine: end, AppliedLimits: map[string]int{"max_lines": max}, Trace: []string{"canonical-source"}}
	if end-start+1 > max {
		end = start + max - 1
		r.EndLine = end
		r.Truncated = true
		r.TruncationReason = "max_lines"
	}
	r.Lines = lines[start-1 : end]
	_ = path
	return nil, sourceResult{r}, nil
}
func (s *v1Service) claims(ctx context.Context, gid, eid string, types []string, dir string, limit int) ([]claimOut, bool, error) {
	if err := s.db.RequireProjection(ctx, "graph"); err != nil {
		return nil, false, fmt.Errorf("knowledge is safe; projection is unavailable: %w", err)
	}
	if dir == "" {
		dir = "both"
	}
	if dir != "in" && dir != "out" && dir != "both" {
		return nil, false, fmt.Errorf("direction must be in, out, or both")
	}
	q := `SELECT c.claim_id,c.subject_id,c.predicate,c.object_id,c.evidence_id,c.extractor,c.derivation,c.confidence,c.generation_id FROM projection_graph_records p JOIN active_projection_builds a ON a.projection_build_id=p.projection_build_id JOIN claims c ON c.claim_id=p.claim_id WHERE a.projection_kind='graph' AND p.generation_id=?`
	args := []any{gid}
	if dir == "out" {
		q += " AND p.subject_id=?"
		args = append(args, eid)
	} else if dir == "in" {
		q += " AND p.object_id=?"
		args = append(args, eid)
	} else {
		q += " AND (p.subject_id=? OR p.object_id=?)"
		args = append(args, eid, eid)
	}
	if len(types) > 0 {
		q += " AND lower(c.predicate) IN (" + marks(len(types)) + ")"
		for _, v := range types {
			args = append(args, strings.ToLower(v))
		}
	}
	q += " ORDER BY c.predicate,c.claim_id LIMIT ?"
	args = append(args, limit+1)
	rows, e := s.db.QueryCanonical(ctx, q, args...)
	if e != nil {
		return nil, false, e
	}
	defer rows.Close()
	var out []claimOut
	for rows.Next() {
		v, z := scanClaim(rows)
		if z != nil {
			return nil, false, z
		}
		out = append(out, v)
	}
	return out, len(out) > limit, rows.Err()
}
func (s *v1Service) entityHandleForID(id string) string {
	var generation string
	if s.db.QueryRowCanonical(context.Background(), `SELECT generation_id FROM entities WHERE entity_id=?`, id).Scan(&generation) != nil {
		return ""
	}
	return enc("e", generation, id)
}
func (s *v1Service) evidenceHandleForID(id string) string {
	var generation string
	if s.db.QueryRowCanonical(context.Background(), `SELECT generation_id FROM evidence WHERE evidence_id=?`, id).Scan(&generation) != nil {
		return ""
	}
	return enc("v", generation, id)
}
func (s *v1Service) crossClaims(ctx context.Context, eid string, types []string, dir string, limit int) ([]claimOut, bool, error) {
	revision, err := s.db.ActiveCatalogRevision(ctx)
	if err != nil {
		return nil, false, nil
	}
	claims, truncated, err := s.db.CrossClaims(ctx, revision.ID, eid, dir, types, limit)
	if err != nil {
		return nil, false, err
	}
	out := make([]claimOut, 0, len(claims))
	for _, c := range claims {
		out = append(out, s.scanCrossClaim(c))
	}
	return out, truncated, nil
}
func (s *v1Service) neighbors(ctx context.Context, _ *mcp.CallToolRequest, in neighborsIn) (*mcp.CallToolResult, result, error) {
	x, e := dec(in.Handle)
	if e != nil || x.T != "e" {
		return nil, result{}, fmt.Errorf("entity handle required")
	}
	if _, e = s.active(ctx, x); e != nil {
		return nil, result{}, e
	}
	d, e := depth(in.Depth, 1)
	if e != nil {
		return nil, result{}, e
	}
	l, e := lim(in.Budget, 50)
	if e != nil {
		return nil, result{}, e
	}
	offset, e := cursor(in.Cursor, x.G)
	if e != nil {
		return nil, result{}, e
	}
	if in.MaxFanout == 0 {
		in.MaxFanout = 100
	}
	if in.MaxEntities == 0 {
		in.MaxEntities = 100
	}
	if in.MaxEdges == 0 {
		in.MaxEdges = l
	}
	if in.MaxFanout < 1 || in.MaxFanout > 100 || in.MaxEntities < 1 || in.MaxEntities > 100 || in.MaxEdges < 1 || in.MaxEdges > 100 {
		return nil, result{}, fmt.Errorf("traversal budgets must be between 1 and 100")
	}
	if in.MaxEdges < l {
		l = in.MaxEdges
	}
	r := result{AppliedLimits: map[string]int{"depth": d, "budget": l, "max_fanout": in.MaxFanout, "max_entities": in.MaxEntities, "max_edges": in.MaxEdges}, Trace: []string{"resolve_active_catalog_revision", "canonical-and-cross-typed-adjacency"}}
	g, err := s.db.GenerationByID(ctx, x.G)
	if err != nil {
		return nil, r, err
	}
	basis, err := s.applyCoverage(ctx, g.RepoID, "structural", &r)
	if err != nil {
		return nil, r, err
	}
	front := []h{x}
	seen := map[string]bool{}
	seenEntities := map[string]bool{x.G + "/" + x.I: true}
	skipped := 0
	for n := 0; n < d; n++ {
		var next []h
		for _, node := range front {
			cs, tr, z := s.claims(ctx, node.G, node.I, in.Types, in.Direction, in.MaxFanout)
			if z != nil {
				return nil, r, z
			}
			xcs, xtr, z := s.crossClaims(ctx, node.I, in.Types, in.Direction, in.MaxFanout)
			if z != nil {
				return nil, r, z
			}
			cs = append(cs, xcs...)
			tr = tr || xtr
			r.Truncated = r.Truncated || tr
			sort.SliceStable(cs, func(i, j int) bool {
				return cs[i].Predicate+cs[i].Subject+cs[i].Object+cs[i].Handle < cs[j].Predicate+cs[j].Subject+cs[j].Object+cs[j].Handle
			})
			for _, c := range cs {
				if skipped < offset {
					skipped++
					continue
				}
				if len(r.Claims) >= l {
					r.Truncated = true
					break
				}
				if !seen[c.Handle] {
					seen[c.Handle] = true
					r.Claims = append(r.Claims, c)
				}
				for _, handle := range []string{c.Subject, c.Object} {
					p, _ := dec(handle)
					if p.I != node.I && !seenEntities[p.G+"/"+p.I] && len(seenEntities) < in.MaxEntities {
						seenEntities[p.G+"/"+p.I] = true
						next = append(next, p)
					}
				}
			}
			if len(r.Claims) >= l {
				break
			}
		}
		front = next
		if len(front) == 0 {
			break
		}
	}
	if r.Truncated {
		r.TruncationReason = "traversal_budget"
		r.NextCursor = makeCursor(offset+len(r.Claims), x.G)
	}
	if e := s.attachProvenance(ctx, x.G, &r); e != nil {
		return nil, r, e
	}
	if e := s.finalizeEmpty(ctx, "traversal", in.Handle, "graph-v1:"+x.G, basis, &r); e != nil {
		return nil, r, e
	}
	return nil, r, nil
}
func (s *v1Service) impact(ctx context.Context, rq *mcp.CallToolRequest, in impactIn) (*mcp.CallToolResult, result, error) {
	return s.neighbors(ctx, rq, neighborsIn{Handle: in.Handle, Depth: in.Depth, Budget: in.Limit, Direction: "in"})
}
func (s *v1Service) references(ctx context.Context, rq *mcp.CallToolRequest, in referencesIn) (*mcp.CallToolResult, result, error) {
	m := map[string]struct{ t, d string }{
		"callers": {"CALLS", "in"}, "callees": {"CALLS", "out"}, "references": {"REFERENCES", "both"}, "definitions": {"DECLARES", "in"}, "implementations": {"IMPLEMENTS", "in"}, "imports": {"IMPORTS", "out"}, "exports": {"EXPORTS", "out"}, "inheritance": {"EXTENDS", "out"}, "aliases": {"ALIASES", "both"}, "modules": {"MODULE", "both"},
		"routes": {"DECLARES_EFFECTIVE_ENDPOINT", "out"}, "http_calls": {"INVOKES_API", "out"}, "event_publications": {"PUBLISHES_EVENT", "out"}, "event_subscriptions": {"SUBSCRIBES_EVENT", "out"}, "log_emissions": {"EMITS_LOG", "out"}, "error_emissions": {"EMITS_ERROR", "out"}, "local_guards": {"HAS_LOCAL_GUARD", "out"}, "configuration_consumers": {"REFERENCES_CONFIGURATION", "in"}, "configuration_definitions": {"DEFINES_CONFIGURATION", "in"},
	}[strings.ToLower(in.Relation)]
	if m.t == "" {
		return nil, result{}, fmt.Errorf("unsupported relation")
	}
	return s.neighbors(ctx, rq, neighborsIn{Handle: in.Handle, Types: []string{m.t}, Direction: m.d, Budget: in.Limit, Cursor: in.Cursor})
}
func (s *v1Service) events(ctx context.Context, _ *mcp.CallToolRequest, in eventsIn) (*mcp.CallToolResult, result, error) {
	if e := s.repo(in.RepoID); e != nil {
		return nil, result{}, e
	}
	q, e := policy.Query(in.Event)
	if e != nil {
		return nil, result{}, e
	}
	l, e := lim(in.Limit, 50)
	if e != nil {
		return nil, result{}, e
	}
	r := result{AppliedLimits: map[string]int{"limit": l}, Trace: []string{"canonical-event-claims"}}
	basis, e := s.applyCoverage(ctx, in.RepoID, "structural", &r)
	if e != nil {
		return nil, r, e
	}
	sqlq := `SELECT c.claim_id,c.subject_id,c.predicate,c.object_id,c.evidence_id,c.extractor,c.derivation,c.confidence,c.generation_id FROM claims c JOIN active_generations a ON a.generation_id=c.generation_id JOIN entities o ON o.entity_id=c.object_id WHERE lower(o.label)=lower(?) AND (upper(c.predicate) LIKE '%EVENT%' OR upper(c.predicate) LIKE '%TOPIC%' OR upper(c.predicate) LIKE '%QUEUE%')`
	args := []any{q}
	if in.RepoID != "" {
		sqlq += " AND o.repo_id=?"
		args = append(args, in.RepoID)
	}
	sqlq += " LIMIT ?"
	args = append(args, l+1)
	rows, e := s.db.QueryCanonical(ctx, sqlq, args...)
	if e != nil {
		return nil, r, e
	}
	defer rows.Close()
	for rows.Next() {
		x, z := scanClaim(rows)
		if z != nil {
			return nil, r, z
		}
		r.Claims = append(r.Claims, x)
	}
	if revision, z := s.db.ActiveCatalogRevision(ctx); z == nil && len(r.Claims) < l {
		xs, tr, z := s.db.CrossClaimsForLabel(ctx, revision.ID, q, l-len(r.Claims))
		if z != nil {
			return nil, r, z
		}
		for _, x := range xs {
			r.Claims = append(r.Claims, s.scanCrossClaim(x))
		}
		if tr {
			r.Truncated = true
			r.TruncationReason = "result_limit"
		}
		r.Trace = append(r.Trace, "cross_repository_event_identities")
	}
	sort.SliceStable(r.Claims, func(i, j int) bool {
		return r.Claims[i].Predicate+r.Claims[i].Subject+r.Claims[i].Object+r.Claims[i].Handle < r.Claims[j].Predicate+r.Claims[j].Subject+r.Claims[j].Object+r.Claims[j].Handle
	})
	if len(r.Claims) > l {
		r.Claims = r.Claims[:l]
		r.Truncated = true
		r.TruncationReason = "result_limit"
	}
	if e := rows.Err(); e != nil {
		return nil, r, e
	}
	if e := s.attachProvenance(ctx, activeGenerationForClaims(r.Claims), &r); e != nil {
		return nil, r, e
	}
	if e := s.finalizeEmpty(ctx, "events", q, "events-v1:"+strings.Join(basis.Generations, "/"), basis, &r); e != nil {
		return nil, r, e
	}
	return nil, r, nil
}
func (s *v1Service) path(ctx context.Context, _ *mcp.CallToolRequest, in pathIn) (*mcp.CallToolResult, result, error) {
	a, e := dec(in.From)
	if e != nil || a.T != "e" {
		return nil, result{}, fmt.Errorf("entity handle required")
	}
	b, e := dec(in.To)
	if e != nil || b.T != "e" {
		return nil, result{}, fmt.Errorf("path endpoints must be active entities")
	}
	if _, e = s.active(ctx, a); e != nil {
		return nil, result{}, e
	}
	if _, e = s.active(ctx, b); e != nil {
		return nil, result{}, e
	}
	d, e := depth(in.MaxDepth, 3)
	if e != nil {
		return nil, result{}, e
	}
	n := in.MaxPaths
	if n == 0 {
		n = 1
	}
	if n < 1 || n > 10 {
		return nil, result{}, fmt.Errorf("max_paths must be between 1 and 10")
	}
	type step struct {
		id h
		cs []claimOut
	}
	q := []step{{a, nil}}
	seen := map[string]bool{a.G + "/" + a.I: true}
	r := result{AppliedLimits: map[string]int{"max_depth": d, "max_paths": n}, Trace: []string{"resolve_active_catalog_revision", "canonical-and-cross-bounded-path"}}
	basis, err := s.applyCoverage(ctx, "", "structural", &r)
	if err != nil {
		return nil, r, err
	}
	for len(q) > 0 && len(r.Claims) < n*d {
		z := q[0]
		q = q[1:]
		if len(z.cs) >= d {
			continue
		}
		cs, _, er := s.claims(ctx, z.id.G, z.id.I, in.Types, "both", 100)
		if er != nil {
			return nil, r, er
		}
		xcs, _, er := s.crossClaims(ctx, z.id.I, in.Types, "both", 100)
		if er != nil {
			return nil, r, er
		}
		cs = append(cs, xcs...)
		sort.SliceStable(cs, func(i, j int) bool {
			return cs[i].Predicate+cs[i].Subject+cs[i].Object+cs[i].Handle < cs[j].Predicate+cs[j].Subject+cs[j].Object+cs[j].Handle
		})
		for _, c := range cs {
			p, _ := dec(c.Object)
			next := p
			if next.I == z.id.I && next.G == z.id.G {
				p, _ = dec(c.Subject)
				next = p
			}
			trail := append(append([]claimOut{}, z.cs...), c)
			if next.I == b.I && next.G == b.G {
				r.Claims = append(r.Claims, trail...)
				if n == 1 {
					if err := s.attachProvenance(ctx, "", &r); err != nil {
						return nil, r, err
					}
					r.Status = "found"
					return nil, r, nil
				}
			} else if !seen[next.G+"/"+next.I] {
				seen[next.G+"/"+next.I] = true
				q = append(q, step{next, trail})
			}
		}
	}
	if len(q) > 0 {
		r.Truncated = true
		r.TruncationReason = "path_budget"
	}
	if err := s.attachProvenance(ctx, "", &r); err != nil {
		return nil, r, err
	}
	if err := s.finalizeEmpty(ctx, "path", in.From+"/"+in.To, "path-v1:"+strings.Join(basis.Generations, "/"), basis, &r); err != nil {
		return nil, r, err
	}
	return nil, r, nil
}
func (s *v1Service) explain(ctx context.Context, _ *mcp.CallToolRequest, in explainIn) (*mcp.CallToolResult, result, error) {
	if in.Handle != "" {
		return s.explainHandle(ctx, in.Handle)
	}
	if in.Query != nil {
		return s.query(ctx, nil, *in.Query)
	}
	x, e := dec(in.Subject)
	if e != nil || x.T != "e" {
		return nil, result{}, fmt.Errorf("entity handle required")
	}
	if _, e = s.active(ctx, x); e != nil {
		return nil, result{}, e
	}
	r := result{AppliedLimits: map[string]int{"limit": 50}, Trace: []string{"canonical-and-cross-evidence-explain"}}
	cs, tr, e := s.claims(ctx, x.G, x.I, nil, "both", 50)
	if e != nil {
		return nil, r, e
	}
	xcs, xtr, e := s.crossClaims(ctx, x.I, nil, "both", 50)
	if e != nil {
		return nil, r, e
	}
	cs = append(cs, xcs...)
	tr = tr || xtr
	sort.SliceStable(cs, func(i, j int) bool {
		return cs[i].Predicate+cs[i].Subject+cs[i].Object+cs[i].Handle < cs[j].Predicate+cs[j].Subject+cs[j].Object+cs[j].Handle
	})
	if in.Object != "" {
		o, z := dec(in.Object)
		if z != nil || o.T != "e" {
			return nil, r, fmt.Errorf("object entity handle required")
		}
		if _, z = s.active(ctx, o); z != nil {
			return nil, r, z
		}
		for _, c := range cs {
			if c.Subject == in.Object || c.Object == in.Object {
				r.Claims = append(r.Claims, c)
			}
		}
	} else {
		r.Claims = cs
	}
	r.Truncated = tr
	if tr {
		r.TruncationReason = "result_limit"
	}
	if e := s.attachProvenance(ctx, x.G, &r); e != nil {
		return nil, r, e
	}
	return nil, r, nil
}

func (s *v1Service) explainHandle(ctx context.Context, handle string) (*mcp.CallToolResult, result, error) {
	handleValue, err := dec(handle)
	if err != nil {
		return nil, result{}, fmt.Errorf("invalid explain handle")
	}
	if handleValue.T == "r" {
		if _, err = s.active(ctx, handleValue); err != nil {
			return nil, result{Status: "unknown", Uncertainty: []string{"result_handle_stale"}}, nil
		}
		payload, err := base64.RawURLEncoding.DecodeString(handleValue.I)
		if err != nil {
			return nil, result{}, fmt.Errorf("invalid result handle")
		}
		var references struct{ Entities, Evidence, Claims []string }
		if json.Unmarshal(payload, &references) != nil {
			return nil, result{}, fmt.Errorf("invalid result handle")
		}
		out := result{Status: "found", AppliedLimits: map[string]int{"evidence": 50, "claims": 50}, Trace: []string{"result_handle", "canonical_evidence_chain"}}
		for _, evidence := range references.Evidence {
			if len(out.Evidence) == 50 {
				out.Truncated = true
				break
			}
			if value, e := s.evidence(ctx, evidence); e == nil {
				out.Evidence = append(out.Evidence, value)
			} else {
				out.Status = "unknown"
				out.Uncertainty = append(out.Uncertainty, "missing_evidence")
			}
		}
		for _, entity := range references.Entities {
			if len(out.Entities) == 50 {
				out.Truncated = true
				break
			}
			if value, e := s.entitySummary(ctx, entity); e == nil {
				out.Entities = append(out.Entities, value)
			}
		}
		for _, claim := range references.Claims {
			if len(out.Claims) == 50 {
				out.Truncated = true
				break
			}
			if value, e := s.claimSummary(ctx, claim); e == nil {
				out.Claims = append(out.Claims, value)
			}
		}
		return nil, out, nil
	}
	if handleValue.T == "l" {
		revision, revisionErr := s.db.ActiveCatalogRevision(ctx)
		if revisionErr != nil || revision.ID != handleValue.G {
			return nil, result{Status: "unknown", Uncertainty: []string{"landmark_handle_stale"}}, nil
		}
		var evidenceJSON, claimsJSON string
		err = s.db.QueryRowCanonical(ctx, `SELECT evidence_ids,claim_ids FROM precomputed_records p JOIN active_projection_builds a ON a.projection_build_id=p.projection_build_id WHERE a.projection_kind='landmarks' AND p.catalog_revision_id=? AND p.record_id=?`, handleValue.G, handleValue.I).Scan(&evidenceJSON, &claimsJSON)
		if err != nil {
			return nil, result{Status: "unknown", Uncertainty: []string{"landmark_unavailable"}}, nil
		}
		var evidenceIDs, claimIDs []string
		_ = json.Unmarshal([]byte(evidenceJSON), &evidenceIDs)
		_ = json.Unmarshal([]byte(claimsJSON), &claimIDs)
		out := result{Status: "found", AppliedLimits: map[string]int{"evidence": 50, "claims": 50}, Trace: []string{"landmark_projection", "canonical_evidence_chain"}}
		for _, id := range evidenceIDs {
			if len(out.Evidence) == 50 {
				out.Truncated = true
				break
			}
			if generation, e := s.handleGeneration(ctx, "evidence", id); e == nil {
				if value, e := s.evidence(ctx, enc("v", generation, id)); e == nil {
					out.Evidence = append(out.Evidence, value)
				}
			}
		}
		for _, id := range claimIDs {
			if len(out.Claims) == 50 {
				out.Truncated = true
				break
			}
			if generation, e := s.handleGeneration(ctx, "claims", id); e == nil {
				if value, e := s.claimSummary(ctx, enc("c", generation, id)); e == nil {
					out.Claims = append(out.Claims, value)
				}
			}
		}
		return nil, out, nil
	}
	if handleValue.T == "a" {
		token, decodeErr := s.decodeSliceHandle(ctx, handle)
		if decodeErr != nil {
			return nil, result{Status: "unknown", Uncertainty: []string{"slice_handle_stale"}}, nil
		}
		return s.slice(ctx, nil, sliceIn{scope: scope{RepoID: token.Selector.Repository}, Kind: token.Selector.Kind, Anchor: token.Selector.Anchor, EntityHandles: token.Selector.EntityHandles, Depth: token.Budget.Depth, MaxFanout: token.Budget.Fanout, MaxEntities: token.Budget.Entities, MaxEdges: token.Budget.Edges, MaxSourceLines: token.Budget.SourceLines})
	}
	if handleValue.T == "x" {
		revision, revisionErr := s.db.ActiveCatalogRevision(ctx)
		if revisionErr != nil || revision.ID != handleValue.G {
			return nil, result{Status: "unknown", Uncertainty: []string{"relationship_handle_stale"}}, nil
		}
		var cross store.CrossClaim
		err = s.db.QueryRowCanonical(ctx, `SELECT cross_claim_id,catalog_revision_id,subject_id,predicate,object_id,subject_evidence_id,object_evidence_id,resolver,matching_inputs,derivation,confidence FROM cross_claims WHERE catalog_revision_id=? AND cross_claim_id=?`, handleValue.G, handleValue.I).Scan(&cross.ID, &cross.CatalogRevision, &cross.SubjectID, &cross.Predicate, &cross.ObjectID, &cross.SubjectEvidenceID, &cross.ObjectEvidenceID, &cross.Resolver, &cross.MatchingInputs, &cross.Derivation, &cross.Confidence)
		if err != nil {
			return nil, result{Status: "unknown", Uncertainty: []string{"relationship_unavailable"}}, nil
		}
		out := result{Status: "found", Claims: []claimOut{s.scanCrossClaim(cross)}, AppliedLimits: map[string]int{"claims": 1}, Trace: []string{"canonical_cross_relationship"}}
		if err = s.attachProvenance(ctx, "", &out); err != nil {
			return nil, out, err
		}
		return nil, out, nil
	}
	if _, err = s.active(ctx, handleValue); err != nil {
		return nil, result{Status: "unknown", Uncertainty: []string{"handle_stale_or_unavailable"}}, nil
	}
	switch handleValue.T {
	case "e":
		return s.explain(ctx, nil, explainIn{Subject: handle})
	case "v":
		value, err := s.evidence(ctx, handle)
		return nil, result{Status: map[bool]string{true: "found", false: "unknown"}[err == nil], Evidence: []evidenceOut{value}, AppliedLimits: map[string]int{"evidence": 1}, Trace: []string{"canonical_evidence"}}, err
	case "c":
		claim, err := s.claimSummary(ctx, handle)
		out := result{Status: map[bool]string{true: "found", false: "unknown"}[err == nil], Claims: []claimOut{claim}, AppliedLimits: map[string]int{"claims": 1}, Trace: []string{"canonical_claim"}}
		if err == nil {
			err = s.attachProvenance(ctx, handleValue.G, &out)
		}
		return nil, out, err
	default:
		return nil, result{}, fmt.Errorf("handle type is not explainable")
	}
}

func (s *v1Service) claimSummary(ctx context.Context, handle string) (claimOut, error) {
	handleValue, err := dec(handle)
	if err != nil || handleValue.T != "c" {
		return claimOut{}, fmt.Errorf("invalid claim handle")
	}
	row := s.db.QueryRowCanonical(ctx, `SELECT claim_id,subject_id,predicate,object_id,evidence_id,extractor,derivation,confidence,generation_id FROM claims WHERE generation_id=? AND claim_id=?`, handleValue.G, handleValue.I)
	return scanClaim(row)
}

func activeGenerationForClaims(claims []claimOut) string {
	if len(claims) > 0 {
		return claims[0].Generation
	}
	return ""
}

// attachProvenance returns the bounded canonical evidence and endpoints behind
// traversal claims. Coverage gaps are themselves canonical claims with cited
// source, rather than a negative inference from missing relationships.
func (s *v1Service) attachProvenance(ctx context.Context, generation string, r *result) error {
	if r.Coverage == nil {
		r.Coverage = map[string]any{}
	}
	seenEvidence, seenEntity := map[string]bool{}, map[string]bool{}
	for _, claim := range r.Claims {
		for _, evidenceHandle := range []string{claim.Evidence, claim.ObjectEvidence} {
			if evidenceHandle == "" || seenEvidence[evidenceHandle] {
				continue
			}
			evidence, err := s.evidence(ctx, evidenceHandle)
			if err != nil {
				return err
			}
			r.Evidence = append(r.Evidence, evidence)
			seenEvidence[evidenceHandle] = true
		}
		for _, handle := range []string{claim.Subject, claim.Object} {
			if !seenEntity[handle] {
				entity, err := s.entitySummary(ctx, handle)
				if err != nil {
					return err
				}
				r.Entities = append(r.Entities, entity)
				seenEntity[handle] = true
			}
		}
	}
	if generation == "" {
		return nil
	}
	rows, err := s.db.QueryCanonical(ctx, `SELECT c.claim_id,c.subject_id,c.predicate,c.object_id,c.evidence_id,c.extractor,c.derivation,c.confidence,c.generation_id FROM claims c WHERE c.generation_id=? AND c.predicate='COVERAGE_GAP' ORDER BY c.claim_id LIMIT 21`, generation)
	if err != nil {
		return err
	}
	defer rows.Close()
	var diagnostics []claimOut
	for rows.Next() {
		c, err := scanClaim(rows)
		if err != nil {
			return err
		}
		diagnostics = append(diagnostics, c)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	if len(diagnostics) > 20 {
		diagnostics = diagnostics[:20]
		r.Truncated = true
		if r.TruncationReason == "" {
			r.TruncationReason = "coverage_diagnostic_limit"
		}
	}
	if len(diagnostics) > 0 {
		r.Coverage["extractor_diagnostics"] = diagnostics
	}
	return nil
}

func (s *v1Service) evidence(ctx context.Context, handle string) (evidenceOut, error) {
	h, err := dec(handle)
	if err != nil || h.T != "v" {
		return evidenceOut{}, fmt.Errorf("invalid evidence handle")
	}
	row := s.db.QueryRowCanonical(ctx, `SELECT e.evidence_id,e.source_id,e.repo_id,e.path,e.file_sha256,e.generation_id,e.start_byte,e.end_byte,e.start_line,e.start_column,e.end_line,e.end_column,e.excerpt,g.source_kind,g.source_adapter_version FROM evidence e JOIN generations g ON g.generation_id=e.generation_id WHERE e.generation_id=? AND e.evidence_id=?`, h.G, h.I)
	var x evidenceOut
	var id, sid string
	err = row.Scan(&id, &sid, &x.Repository, &x.Path, &x.Revision, &x.Generation, &x.Span.StartByte, &x.Span.EndByte, &x.Span.StartLine, &x.Span.StartColumn, &x.Span.EndLine, &x.Span.EndColumn, &x.Excerpt, &x.SourceKind, &x.SourceAdapterVersion)
	if err != nil {
		return x, err
	}
	x.Handle = enc("v", x.Generation, id)
	x.Source = enc("s", x.Generation, sid)
	return x, nil
}
func (s *v1Service) entitySummary(ctx context.Context, handle string) (entityOut, error) {
	h, err := dec(handle)
	if err != nil || h.T != "e" {
		return entityOut{}, fmt.Errorf("invalid entity handle")
	}
	row := s.db.QueryRowCanonical(ctx, `SELECT entity_id,kind,label,identity,extractor,repo_id,path,generation_id,evidence_id FROM entities WHERE generation_id=? AND entity_id=?`, h.G, h.I)
	var x entityOut
	var id string
	err = row.Scan(&id, &x.Kind, &x.Label, &x.Identity, &x.Extractor, &x.Repository, &x.Path, &x.Generation, &x.Evidence)
	if err != nil {
		return x, err
	}
	x.Handle = enc("e", x.Generation, id)
	x.Evidence = enc("v", x.Generation, x.Evidence)
	return x, nil
}
func marks(n int) string { return strings.TrimRight(strings.Repeat("?,", n), ",") }
func contains(a []string, v string) bool {
	for _, x := range a {
		if x == v {
			return true
		}
	}
	return false
}

type cur struct {
	Offset     int
	Generation string
}

func (s *v1Service) generationStamp(ctx context.Context, repo string) (string, error) {
	ids := []string{}
	for _, r := range s.cfg.SourceRepositories() {
		if repo != "" && repo != r.ID {
			continue
		}
		g, e := s.db.ActiveGeneration(ctx, r.ID)
		if e != nil {
			continue
		}
		ids = append(ids, g.ID)
	}
	sort.Strings(ids)
	if len(ids) == 0 {
		return "", fmt.Errorf("no active generation")
	}
	return strings.Join(ids, "/"), nil
}
func cursor(s, stamp string) (int, error) {
	if s == "" {
		return 0, nil
	}
	var c cur
	b, e := base64.RawURLEncoding.DecodeString(s)
	if e != nil || json.Unmarshal(b, &c) != nil || c.Offset < 0 || c.Generation != stamp {
		return 0, fmt.Errorf("invalid cursor")
	}
	return c.Offset, nil
}
func makeCursor(n int, stamp string) string {
	b, _ := json.Marshal(cur{Offset: n, Generation: stamp})
	return base64.RawURLEncoding.EncodeToString(b)
}

type retrievalInput struct {
	repo, version                             string
	kinds, fields, languages, classifications []string
	minimumConfidence                         float64
	limit                                     int
	cursor                                    string
	entities                                  bool
}

func (s *v1Service) retrieve(ctx context.Context, query string, in retrievalInput) (*mcp.CallToolResult, result, error) {
	stamp, err := s.generationStamp(ctx, in.repo)
	if err != nil {
		return nil, result{Status: "unknown", AppliedLimits: map[string]int{"limit": in.limit}, Coverage: map[string]any{"repositories": in.repo, "complete": false, "gap": "no_active_generation"}, Trace: []string{"coverage_incomplete"}}, nil
	}
	offset, err := cursor(in.cursor, stamp)
	if err != nil {
		return nil, result{}, err
	}
	basis, err := s.db.Coverage(ctx, in.repo, "lexical", in.fields)
	if err != nil {
		return nil, result{}, err
	}
	coverage := coverageMap(basis, stamp, in.fields, s.compilerDiagnostics(ctx, in.repo))
	scope := map[string]any{"repositories": basis.Repositories, "fields": in.fields, "indexes": basis.Indexes, "generations": basis.Generations, "exclusions": basis.Exclusions}
	for _, uncertainty := range basis.Uncertainty {
		if strings.HasPrefix(uncertainty, "index_unavailable:") {
			return nil, result{Status: "unknown", AppliedLimits: map[string]int{"limit": in.limit}, Coverage: coverage, SearchedScope: scope, Uncertainty: append(basis.Uncertainty, "rebuild the required projection and retry"), Trace: []string{"coverage_incomplete"}}, nil
		}
	}
	filter := store.QueryFilter{Repository: in.repo, Version: in.version, Kinds: in.kinds, Languages: in.languages, Classifications: in.classifications, Fields: in.fields, MinimumConfidence: in.minimumConfidence}
	candidates, cacheTrace, err := s.cachedCandidates(ctx, query, in, stamp, filter)
	if err != nil {
		return nil, result{}, err
	}
	candidates = fuseStoreCandidates(candidates)
	sort.SliceStable(candidates, func(i, j int) bool {
		a, b := candidates[i], candidates[j]
		if matchRank(a.MatchType) != matchRank(b.MatchType) {
			return matchRank(a.MatchType) < matchRank(b.MatchType)
		}
		if fieldRank(a.Field) != fieldRank(b.Field) {
			return fieldRank(a.Field) < fieldRank(b.Field)
		}
		if a.Confidence != b.Confidence {
			return a.Confidence > b.Confidence
		}
		return a.Entity.RepoID+"\x00"+a.Entity.Path+"\x00"+a.Evidence.ID+"\x00"+a.Entity.ID < b.Entity.RepoID+"\x00"+b.Entity.Path+"\x00"+b.Evidence.ID+"\x00"+b.Entity.ID
	})
	r := result{Status: "found", AppliedLimits: map[string]int{"limit": in.limit}, Coverage: coverage, SearchedScope: scope, Uncertainty: basis.Uncertainty, Trace: append([]string{"canonical_candidates", "exact_before_lexical", "fielded_fts5_bm25", "stable_repository_path_span_handle_tie_break"}, cacheTrace...)}
	if len(candidates) == 0 {
		if !basis.Complete {
			r.Status = "unknown"
			r.Uncertainty = append(r.Uncertainty, "coverage_incomplete; narrow scope or repair ingestion")
			r.Trace = append(r.Trace, "coverage_incomplete")
		} else {
			r.Status = "not_found"
			r.Trace = append(r.Trace, "active_scope_fully_searched")
			negative, e := s.recordNegative(ctx, "lexical_search", query, "lexical-v1:"+stamp, scope, basis)
			if e != nil {
				return nil, r, e
			}
			r.NegativeEvidence = negative
		}
		return nil, r, nil
	}
	if offset > len(candidates) {
		return nil, r, fmt.Errorf("invalid cursor")
	}
	end := minInt(offset+in.limit, len(candidates))
	for _, c := range candidates[offset:end] {
		e := entityOut{Handle: enc("e", c.Entity.GenerationID, c.Entity.ID), Kind: c.Entity.Kind, Label: c.Entity.Label, Identity: c.Entity.Identity, Repository: c.Entity.RepoID, Path: c.Entity.Path, Generation: c.Entity.GenerationID, Evidence: enc("v", c.Evidence.GenerationID, c.Evidence.ID), Confidence: c.Confidence, Span: c.Evidence.Span}
		v := evidenceOut{Handle: enc("v", c.Evidence.GenerationID, c.Evidence.ID), Source: enc("s", c.Evidence.GenerationID, c.Evidence.SourceID), Repository: c.Evidence.RepoID, Path: c.Evidence.Path, Revision: c.Evidence.SHA256, Generation: c.Evidence.GenerationID, Excerpt: c.Evidence.Excerpt, SourceKind: model.SourceKindRepository, SourceAdapterVersion: model.RepositoryAdapterVersion, Span: c.Evidence.Span}
		if in.entities {
			r.Entities = append(r.Entities, e)
		}
		r.Evidence = append(r.Evidence, v)
		claims, err := s.claimsForEntity(ctx, c.Entity.ID, c.Entity.GenerationID)
		if err != nil {
			return nil, r, err
		}
		r.Claims = append(r.Claims, claims...)
		r.Trace = append(r.Trace, "candidate:"+c.MatchType+":"+c.Field)
	}
	r.Claims = uniqueClaims(r.Claims)
	if end < len(candidates) {
		r.Truncated, r.TruncationReason, r.NextCursor = true, "result_limit", makeCursor(end, stamp)
	}
	return nil, r, nil
}

func (s *v1Service) cachedCandidates(ctx context.Context, query string, in retrievalInput, stamp string, filter store.QueryFilter) ([]store.Candidate, []string, error) {
	if s.cache == nil || !s.cache.Available() {
		c, err := s.db.SearchCandidates(ctx, query, filter)
		return c, []string{"cache_unavailable"}, err
	}
	diagnostics, _ := s.db.Diagnostics(ctx, in.repo)
	projections := map[string]string{}
	for _, p := range diagnostics.Projections {
		projections[p.Kind] = p.Fingerprint
	}
	meta := cachepkg.Metadata{Kind: cachepkg.Candidate, RequestFingerprint: cachepkg.Fingerprint(query), SourceScope: s.coveredRepositories(in.repo), GenerationFingerprint: stamp, ProjectionFingerprints: projections, BudgetFingerprint: cachepkg.Fingerprint(struct {
		Kinds, Fields, Languages, Classifications []string
		Confidence                                float64
	}{in.kinds, in.fields, in.languages, in.classifications, in.minimumConfidence}), OutputShape: "candidate_references", Builder: cachepkg.BuilderVersion}
	entry, hit, err := s.cache.Do(ctx, meta, func(current cachepkg.Metadata) bool {
		return current.GenerationFingerprint == stamp && reflect.DeepEqual(current.ProjectionFingerprints, projections)
	}, func(context.Context) ([]byte, error) {
		c, e := s.db.SearchCandidates(ctx, query, filter)
		if e != nil {
			return nil, e
		}
		return json.Marshal(store.CandidateReferences(c))
	})
	if err != nil {
		return nil, []string{"cache_unavailable"}, err
	}
	var refs []store.CandidateReference
	if err = json.Unmarshal(entry.Payload, &refs); err != nil {
		return nil, []string{"cache_miss"}, err
	}
	candidates, err := s.db.HydrateCandidateReferences(ctx, refs)
	if err != nil {
		return nil, []string{"cache_stale_rejected"}, err
	}
	return candidates, []string{map[bool]string{true: "cache_hit", false: "cache_miss"}[hit]}, nil
}

func coverageMap(b store.CoverageBasis, stamp string, fields []string, diagnostics []map[string]string) map[string]any {
	return map[string]any{"complete": b.Complete, "capability": b.Capability, "repositories": b.Repositories, "generations": b.Generations, "coverage_records": b.CoverageIDs, "indexes": b.Indexes, "fields": fields, "exclusions": b.Exclusions, "active_generation_stamp": stamp, "compiler_diagnostics": diagnostics}
}

func (s *v1Service) applyCoverage(ctx context.Context, repo, capability string, r *result) (store.CoverageBasis, error) {
	basis, err := s.db.Coverage(ctx, repo, capability, nil)
	if err != nil {
		return basis, err
	}
	r.Coverage = coverageMap(basis, strings.Join(basis.Generations, "/"), nil, s.compilerDiagnostics(ctx, repo))
	r.SearchedScope = map[string]any{"repositories": basis.Repositories, "fields": []string{}, "indexes": basis.Indexes, "generations": basis.Generations, "exclusions": basis.Exclusions}
	r.Uncertainty = append(r.Uncertainty, basis.Uncertainty...)
	return basis, nil
}

func (s *v1Service) finalizeEmpty(ctx context.Context, assertion, target, plan string, basis store.CoverageBasis, r *result) error {
	if len(r.Entities) > 0 || len(r.Claims) > 0 || len(r.Evidence) > 0 {
		r.Status = "found"
		return nil
	}
	if r.Truncated || !basis.Complete {
		r.Status = "unknown"
		if !basis.Complete {
			r.Uncertainty = append(r.Uncertainty, "coverage_incomplete; narrow scope or repair ingestion")
		}
		return nil
	}
	r.Status = "not_found"
	negative, err := s.recordNegative(ctx, assertion, target, plan, r.SearchedScope, basis)
	if err == nil {
		r.NegativeEvidence = negative
	}
	return err
}

// recordNegative stores only a coverage-bound absence assertion in the cache
// projection. It is deliberately best-effort: canonical retrieval remains
// valid when cache storage is unavailable.
func (s *v1Service) recordNegative(ctx context.Context, assertion, target, plan string, scope map[string]any, basis store.CoverageBasis) (string, error) {
	if s.cache == nil || !s.cache.Available() {
		return "", nil
	}
	projections := map[string]string{}
	for _, index := range basis.Indexes {
		projections[index] = cachepkg.Fingerprint(index)
	}
	meta := cachepkg.Metadata{Kind: cachepkg.NegativeEvidence, RequestFingerprint: cachepkg.Fingerprint([]string{assertion, target, plan}), SourceScope: basis.Repositories, GenerationFingerprint: strings.Join(basis.Generations, "/"), ProjectionFingerprints: projections, CoverageFingerprint: cachepkg.Fingerprint(struct{ Coverage, Exclusions, Uncertainty []string }{basis.CoverageIDs, basis.Exclusions, basis.Uncertainty}), BudgetFingerprint: "complete_coverage_only", OutputShape: "supported_not_found", Builder: cachepkg.BuilderVersion}
	payload, _ := json.Marshal(map[string]any{"state": "not_found", "coverage_fingerprint": meta.CoverageFingerprint, "scope_fingerprint": cachepkg.Fingerprint(scope)})
	if err := s.cache.Put(ctx, meta, payload); err != nil {
		return "", err
	}
	return cachepkg.Key(meta), nil
}

func (s *v1Service) compilerDiagnostics(ctx context.Context, repo string) []map[string]string {
	q := `SELECT d.language,d.code,d.message,d.path FROM compiler_diagnostics d JOIN active_generations a ON a.generation_id=d.generation_id`
	args := []any{}
	if repo != "" {
		q += ` WHERE a.repo_id=?`
		args = append(args, repo)
	}
	q += ` ORDER BY d.language,d.path,d.code LIMIT 100`
	rows, err := s.db.QueryCanonical(ctx, q, args...)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var out []map[string]string
	for rows.Next() {
		var language, code, message, path string
		if rows.Scan(&language, &code, &message, &path) == nil {
			out = append(out, map[string]string{"language": language, "code": code, "message": message, "path": path})
		}
	}
	return out
}
func (s *v1Service) coveredRepositories(repo string) []string {
	out := []string{}
	for _, configured := range s.cfg.SourceRepositories() {
		if repo == "" || repo == configured.ID {
			out = append(out, configured.ID)
		}
	}
	return out
}
func (s *v1Service) claimsForEntity(ctx context.Context, entityID, generation string) ([]claimOut, error) {
	rows, err := s.db.QueryCanonical(ctx, `SELECT claim_id,subject_id,predicate,object_id,evidence_id,extractor,derivation,confidence,generation_id FROM claims WHERE generation_id=? AND (subject_id=? OR object_id=?) ORDER BY claim_id`, generation, entityID, entityID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []claimOut
	for rows.Next() {
		c, err := scanClaim(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}
func uniqueClaims(in []claimOut) []claimOut {
	seen := map[string]bool{}
	out := make([]claimOut, 0, len(in))
	for _, claim := range in {
		if !seen[claim.Handle] {
			seen[claim.Handle] = true
			out = append(out, claim)
		}
	}
	return out
}
func matchRank(v string) int {
	switch v {
	case "exact_identifier":
		return 0
	case "exact_path":
		return 1
	case "exact_literal":
		return 2
	case "exact_protocol_config":
		return 3
	default:
		return 4
	}
}
func fieldRank(v string) int {
	for i, x := range []string{"symbol", "path", "configuration", "documentation", "strings", "logs_errors", "source"} {
		if v == x {
			return i
		}
	}
	return 99
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

var _ = sort.Strings
var _ = sql.ErrNoRows
