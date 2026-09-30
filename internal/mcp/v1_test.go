package mcpserver

import (
	"context"
	"database/sql"
	"encoding/json"
	"reflect"
	"testing"
	"time"

	cachepkg "github.com/AdamNi-7080/AIOS/internal/cache"
	"github.com/AdamNi-7080/AIOS/internal/catalog"
	"github.com/AdamNi-7080/AIOS/internal/model"
	"github.com/AdamNi-7080/AIOS/internal/planner"
	slicepkg "github.com/AdamNi-7080/AIOS/internal/slice"
	"github.com/AdamNi-7080/AIOS/internal/store"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestV1EnvelopeNestsPayloadAndExplainsResultHandle(t *testing.T) {
	ctx := context.Background()
	db, err := store.OpenWriter(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	f := model.File{RepoID: "repo", Path: "src/a.go", SHA256: "one", Size: 12, Language: "go", Classification: "source", Content: "func A() {}"}
	g, err := db.StageGeneration(ctx, model.Snapshot{RepoID: "repo", Root: "/repo", Git: model.GitState{Commit: "one"}, ContentHash: "one", FileCount: 1, TotalBytes: f.Size, IndexedAt: time.Now(), ExtractorVersions: "fixture"}, []model.File{f}, []model.Symbol{{RepoID: "repo", Path: f.Path, Name: "A", Kind: "function", Span: model.Span{EndByte: 10, StartLine: 1, EndLine: 1}, Extractor: "fixture", Confidence: 1}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err = db.ActivateGeneration(ctx, g.ID); err != nil {
		t.Fatal(err)
	}
	s := &v1Service{cfg: catalog.Config{Version: 1, Repositories: []model.Repository{{ID: "repo", Root: "/repo"}}, Limits: catalog.Defaults()}, db: db}
	_, out, err := s.resolve(ctx, nil, resolveIn{scope: scope{RepoID: "repo"}, Text: "A"})
	if err != nil || len(out.Entities) == 0 {
		t.Fatalf("out=%#v err=%v", out, err)
	}
	encoded, err := json.Marshal(out)
	if err != nil {
		t.Fatal(err)
	}
	var wire struct {
		APIVersion   int                `json:"api_version"`
		Result       json.RawMessage    `json:"result"`
		ResultHandle string             `json:"result_handle"`
		Provenance   provenanceEnvelope `json:"provenance"`
	}
	if err = json.Unmarshal(encoded, &wire); err != nil || wire.APIVersion != 1 || len(wire.Result) == 0 || wire.ResultHandle == "" || len(wire.Provenance.EvidenceHandles) == 0 {
		t.Fatalf("wire=%s err=%v", encoded, err)
	}
	_, explained, err := s.explain(ctx, nil, explainIn{Handle: wire.ResultHandle})
	if err != nil || len(explained.Evidence) == 0 || len(explained.Entities) == 0 {
		t.Fatalf("explained=%#v err=%v", explained, err)
	}
	next, err := db.StageGeneration(ctx, model.Snapshot{RepoID: "repo", Root: "/repo", Git: model.GitState{Commit: "two"}, ContentHash: "two", FileCount: 1, TotalBytes: f.Size, IndexedAt: time.Now(), ExtractorVersions: "fixture"}, []model.File{{RepoID: "repo", Path: f.Path, SHA256: "two", Size: f.Size, Language: f.Language, Classification: f.Classification, Content: f.Content}}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err = db.ActivateGeneration(ctx, next.ID); err != nil {
		t.Fatal(err)
	}
	_, stale, err := s.explain(ctx, nil, explainIn{Handle: wire.ResultHandle})
	if err != nil || stale.Status != "unknown" || len(stale.Uncertainty) != 1 || stale.Uncertainty[0] != "result_handle_stale" {
		t.Fatalf("stale=%#v err=%v", stale, err)
	}
}

func TestV1ExposesOnlyRepositoryKnowledgeTools(t *testing.T) {
	db, err := store.OpenWriter(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	serverSession, err := NewV1(catalog.Config{Version: 1, Repositories: []model.Repository{{ID: "repo", Root: "/repo"}}, Limits: catalog.Defaults()}, db).Connect(ctx, serverTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	client := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "1"}, nil)
	clientSession, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	tools, err := clientSession.ListTools(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]bool{}
	for _, name := range []string{"kb.resolve", "kb.search", "kb.query", "kb.slice", "kb.context", "kb.landmarks", "kb.entity", "kb.source", "kb.neighbors", "kb.path", "kb.references", "kb.events", "kb.impact", "kb.explain", "kb.status"} {
		want[name] = true
	}
	if len(tools.Tools) != len(want) {
		t.Fatalf("tools=%d want=%d", len(tools.Tools), len(want))
	}
	for _, tool := range tools.Tools {
		if !want[tool.Name] || tool.Annotations == nil || !tool.Annotations.ReadOnlyHint {
			t.Fatalf("unexpected tool %#v", tool)
		}
	}
	called, err := clientSession.CallTool(ctx, &mcp.CallToolParams{Name: "kb.status", Arguments: map[string]any{}})
	if err != nil || called.IsError || called.StructuredContent == nil {
		t.Fatalf("black-box tool call failed: result=%#v err=%v", called, err)
	}
	if err := clientSession.Close(); err != nil {
		t.Fatal(err)
	}
	if err := serverSession.Wait(); err != nil {
		t.Fatal(err)
	}
}

func TestStatusReportsRepositoryOnlyV1Boundary(t *testing.T) {
	db, err := store.OpenWriter(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	s := &v1Service{db: db}
	_, status, err := s.status(context.Background(), nil, statusIn{})
	languages, _ := status.Capabilities["language_coverage"].(map[string]string)
	if err != nil || status.SupportedAdapters[model.SourceKindRepository] != model.RepositoryAdapterVersion || status.DisabledCapabilities["planning"] != "outside_v1" || languages["kotlin"] != "structural" || languages["python"] != "lexical_only" {
		t.Fatalf("status=%#v err=%v", status, err)
	}
}

func TestSlicesAreBoundedAndGenerationBound(t *testing.T) {
	ctx := context.Background()
	db, err := store.OpenWriter(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	f := model.File{RepoID: "repo", Path: "src/Anchor.java", SHA256: "one", Size: 40, Language: "java", Classification: "source", Content: "class Anchor { void publish() {} }"}
	sp := model.Span{StartLine: 1, EndLine: 1, StartColumn: 1, EndColumn: 20, EndByte: 20}
	g, err := db.StageGeneration(ctx, model.Snapshot{RepoID: "repo", Root: "/repo", Git: model.GitState{Commit: "one"}, ContentHash: "one", FileCount: 1, TotalBytes: f.Size, IndexedAt: time.Now(), ExtractorVersions: "fixture"}, []model.File{f}, []model.Symbol{{RepoID: "repo", Path: f.Path, Name: "Anchor", Kind: "class", Span: sp, Extractor: "fixture", Confidence: 1}}, []model.Edge{{RepoID: "repo", Path: f.Path, Source: "Anchor", Target: "topic", Kind: "PUBLISHES_EVENT", Span: sp, Resolver: "fixture", Confidence: 1}})
	if err != nil {
		t.Fatal(err)
	}
	if err = db.ActivateGeneration(ctx, g.ID); err != nil {
		t.Fatal(err)
	}
	cache, err := cachepkg.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer cache.Close()
	s := &v1Service{cfg: catalog.Config{Version: 1, Repositories: []model.Repository{{ID: "repo", Root: "/repo"}}, Limits: catalog.Defaults()}, db: db, cache: cache}
	for _, kind := range []slicepkg.Kind{slicepkg.RepositoryServiceOverview, slicepkg.ServiceBoundary, slicepkg.ContractSurface, slicepkg.EventFlow, slicepkg.HTTPFlow, slicepkg.Configuration, slicepkg.Dependency, slicepkg.TestFeature, slicepkg.Impact, slicepkg.Neighbourhood} {
		_, out, e := s.slice(ctx, nil, sliceIn{scope: scope{RepoID: "repo"}, Kind: kind, Anchor: "Anchor", MaxEntities: 1, MaxEdges: 1, MaxSourceLines: 1})
		if e != nil || out.Status != "found" || out.Slice == nil || out.Slice.Handle == "" || len(out.Entities) != 1 {
			t.Fatalf("kind=%s out=%#v err=%v", kind, out, e)
		}
	}
	_, ambiguous, err := s.slice(ctx, nil, sliceIn{scope: scope{RepoID: "repo"}, Kind: slicepkg.EventFlow, Anchor: "missing"})
	if err != nil || ambiguous.Status != "unknown" {
		t.Fatalf("ambiguous=%#v err=%v", ambiguous, err)
	}
	_, current, err := s.slice(ctx, nil, sliceIn{scope: scope{RepoID: "repo"}, Kind: slicepkg.EventFlow, Anchor: "Anchor"})
	if err != nil {
		t.Fatal(err)
	}
	_, cached, err := s.slice(ctx, nil, sliceIn{scope: scope{RepoID: "repo"}, Kind: slicepkg.EventFlow, Anchor: "Anchor"})
	if err != nil || cached.Slice.Provenance.CacheState != "cache_hit" || !reflect.DeepEqual(current.Entities, cached.Entities) || !reflect.DeepEqual(current.Claims, cached.Claims) {
		t.Fatalf("cached=%#v err=%v", cached, err)
	}
	_, pkg, err := s.context(ctx, nil, contextIn{SliceHandle: current.Slice.Handle, MaxBytes: 4096, EstimatedTokens: 1024, MaxEntities: 1, MaxEdges: 1, MaxExcerpts: 1, MaxLinesPerExcerpt: 1})
	if err != nil || pkg.AnswerKind != string(slicepkg.EventFlow) || len(pkg.Entities) == 0 {
		t.Fatalf("package=%#v err=%v", pkg, err)
	}
	if _, err := s.decodeSliceHandle(ctx, current.Slice.Handle); err != nil {
		t.Fatal(err)
	}
	next, err := db.StageGeneration(ctx, model.Snapshot{RepoID: "repo", Root: "/repo", Git: model.GitState{Commit: "two"}, ContentHash: "two", FileCount: 1, TotalBytes: f.Size, IndexedAt: time.Now(), ExtractorVersions: "fixture"}, []model.File{{RepoID: "repo", Path: f.Path, SHA256: "two", Size: f.Size, Language: f.Language, Classification: f.Classification, Content: f.Content}}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err = db.ActivateGeneration(ctx, next.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.decodeSliceHandle(ctx, current.Slice.Handle); err == nil {
		t.Fatal("stale slice handle accepted")
	}
}

func TestLandmarksAreBoundedAndRejectStaleCursors(t *testing.T) {
	ctx := context.Background()
	db, err := store.OpenWriter(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	f := model.File{RepoID: "repo", Path: "config/application.yml", SHA256: "one", Size: 18, Language: "yaml", Classification: "configuration", Content: "feature.enabled=true"}
	sp := model.Span{EndByte: 18, StartLine: 1, StartColumn: 1, EndLine: 1, EndColumn: 19}
	g, err := db.StageGeneration(ctx, model.Snapshot{RepoID: "repo", Root: "/repo", Git: model.GitState{Commit: "one"}, ContentHash: "one", FileCount: 1, TotalBytes: f.Size, IndexedAt: time.Now(), ExtractorVersions: "fixture"}, []model.File{f}, nil, []model.Edge{{RepoID: "repo", Path: f.Path, Source: "config", Target: "feature.enabled", Kind: "DEFINES_CONFIGURATION", Span: sp, Resolver: "fixture", Confidence: .95}})
	if err != nil {
		t.Fatal(err)
	}
	if err = db.ActivateGeneration(ctx, g.ID); err != nil {
		t.Fatal(err)
	}
	s := &v1Service{cfg: catalog.Config{Version: 1, Repositories: []model.Repository{{ID: "repo", Root: "/repo"}}, Limits: catalog.Defaults()}, db: db}
	_, out, err := s.landmarks(ctx, nil, landmarksIn{scope: scope{RepoID: "repo"}, Limit: 1})
	if err != nil || len(out.Records) != 1 || !out.Truncated || out.NextCursor == "" || len(out.Records[0].Evidence) == 0 {
		t.Fatalf("out=%#v err=%v", out, err)
	}
	_, next, err := s.landmarks(ctx, nil, landmarksIn{scope: scope{RepoID: "repo"}, Limit: 1, Cursor: out.NextCursor})
	if err != nil || len(next.Records) == 0 {
		t.Fatalf("next=%#v err=%v", next, err)
	}
	nextGeneration, err := db.StageGeneration(ctx, model.Snapshot{RepoID: "repo", Root: "/repo", Git: model.GitState{Commit: "two"}, ContentHash: "two", FileCount: 1, TotalBytes: f.Size, IndexedAt: time.Now(), ExtractorVersions: "fixture"}, []model.File{{RepoID: "repo", Path: f.Path, SHA256: "two", Size: f.Size, Language: f.Language, Classification: f.Classification, Content: f.Content}}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err = db.ActivateGeneration(ctx, nextGeneration.ID); err != nil {
		t.Fatal(err)
	}
	if _, _, err = s.landmarks(ctx, nil, landmarksIn{scope: scope{RepoID: "repo"}, Limit: 1, Cursor: out.NextCursor}); err == nil {
		t.Fatal("stale landmark cursor accepted")
	}
}

func TestContextReturnsBoundedCanonicalPackage(t *testing.T) {
	ctx := context.Background()
	db, err := store.OpenWriter(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	file := model.File{RepoID: "repo", Path: "src/Publish.java", SHA256: "h", Size: 48, Language: "java", Classification: "source", Content: "class Publish { void customerChanged() {} }"}
	g, err := db.StageGeneration(ctx, model.Snapshot{RepoID: "repo", Root: "/repo", Git: model.GitState{Commit: "one"}, ContentHash: "h", FileCount: 1, TotalBytes: file.Size, IndexedAt: time.Now(), ExtractorVersions: "fixture"}, []model.File{file}, []model.Symbol{{RepoID: "repo", Path: file.Path, Name: "Publish", Kind: "class", Span: model.Span{StartLine: 1, EndLine: 1}, Extractor: "fixture", Confidence: 1}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err = db.ActivateGeneration(ctx, g.ID); err != nil {
		t.Fatal(err)
	}
	s := &v1Service{cfg: catalog.Config{Version: 1, Repositories: []model.Repository{{ID: "repo", Root: "/repo"}}, Limits: catalog.Defaults()}, db: db}
	_, out, err := s.context(ctx, nil, contextIn{scope: scope{RepoID: "repo"}, Text: "Publish", MaxBytes: 4096, EstimatedTokens: 1024, MaxEntities: 1, MaxExcerpts: 1, MaxLinesPerExcerpt: 1})
	if err != nil || out.AnswerKind != "exact_lookup" || len(out.Entities) != 1 || len(out.Excerpts) != 1 || out.Repositories[0].GitRevision != "one" || out.TokenEstimator == "" {
		t.Fatalf("out=%#v err=%v", out, err)
	}
	_, _, err = s.entity(ctx, nil, handleIn{Handle: out.Entities[0].Handle})
	if err != nil {
		t.Fatal(err)
	}
	second := model.Snapshot{RepoID: "repo", Root: "/repo", Git: model.GitState{Commit: "two"}, ContentHash: "two", FileCount: 1, TotalBytes: file.Size, IndexedAt: time.Now(), ExtractorVersions: "fixture"}
	file.SHA256 = "two"
	next, err := db.StageGeneration(ctx, second, []model.File{file}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err = db.ActivateGeneration(ctx, next.ID); err != nil {
		t.Fatal(err)
	}
	if _, _, err = s.entity(ctx, nil, handleIn{Handle: out.Entities[0].Handle}); err == nil {
		t.Fatal("stale expansion handle accepted")
	}
}

func TestContextRelationshipProfilesAreConservative(t *testing.T) {
	if got := contextPredicates(planner.ExactLookup); len(got) != 0 {
		t.Fatalf("exact profile=%#v", got)
	}
	for _, family := range []planner.Family{planner.EventTrace, planner.ExplainCause, planner.RouteTrace, planner.Configuration, planner.Negative} {
		if len(contextPredicates(family)) == 0 {
			t.Fatalf("missing profile for %s", family)
		}
	}
	if contextPredicates(planner.Impact) != nil || contextDirection(planner.Impact) != "in" {
		t.Fatal("impact profile widened")
	}
}

func TestQueryReturnsInspectablePlanAndBoundedContinuation(t *testing.T) {
	ctx := context.Background()
	db, err := store.OpenWriter(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	file := model.File{RepoID: "repo", Path: "src/Publish.java", SHA256: "h", Size: 48, Language: "java", Classification: "source", Content: "class Publish { void customerChanged() {} }"}
	snap := model.Snapshot{RepoID: "repo", Root: "/repo", Git: model.GitState{Commit: "one"}, ContentHash: "h", FileCount: 1, TotalBytes: file.Size, IndexedAt: time.Now(), ExtractorVersions: "fixture"}
	g, err := db.StageGeneration(ctx, snap, []model.File{file}, []model.Symbol{{RepoID: "repo", Path: file.Path, Name: "Publish", Kind: "class", Span: model.Span{EndByte: 7, StartLine: 1, StartColumn: 1, EndLine: 1, EndColumn: 8}, Extractor: "fixture", Confidence: 1}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err = db.ActivateGeneration(ctx, g.ID); err != nil {
		t.Fatal(err)
	}
	s := &v1Service{cfg: catalog.Config{Version: 1, Repositories: []model.Repository{{ID: "repo", Root: "/repo"}}, Limits: catalog.Defaults()}, db: db}
	_, out, err := s.query(ctx, nil, queryIn{scope: scope{RepoID: "repo"}, Text: "Publish", Limit: 1})
	if err != nil || out.QueryFamily != "exact_lookup" || out.ExecutedPlan == nil || out.PlanExplanation == nil || len(out.CandidateTrace) != 1 || out.Truncated || out.ExpansionHandle != "" {
		t.Fatalf("out=%#v err=%v", out, err)
	}
	wantRoute := []string{"route:exact:selected", "route:lexical:skipped_unique_exact", "route:graph:skipped_intent", "route:structural:skipped_prior_result", "route:path:skipped_non_path_intent", "route:vector:skipped_prior_result"}
	position := 0
	for _, entry := range out.Trace {
		if position < len(wantRoute) && entry == wantRoute[position] {
			position++
		}
	}
	if position != len(wantRoute) {
		t.Fatalf("route precedence missing from trace: %#v", out.Trace)
	}
	_, repeated, err := s.query(ctx, nil, queryIn{scope: scope{RepoID: "repo"}, Text: "Publish", Limit: 1})
	if err != nil || !reflect.DeepEqual(out.ExecutedPlan, repeated.ExecutedPlan) || !reflect.DeepEqual(out.Trace, repeated.Trace) {
		t.Fatalf("unstable plan: %#v %#v %v", out, repeated, err)
	}
	_, entityBound, err := s.query(ctx, nil, queryIn{scope: scope{RepoID: "repo"}, Text: "Publish", Limit: 20, MaxEntities: 1})
	if err != nil || len(entityBound.Entities) != 1 || entityBound.Truncated || len(entityBound.BudgetStops) != 0 {
		t.Fatalf("entity bound=%#v err=%v", entityBound, err)
	}
}

func TestQueryPlanCacheAddsOnlyDiagnosticProvenance(t *testing.T) {
	ctx := context.Background()
	db, err := store.OpenWriter(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	file := model.File{RepoID: "repo", Path: "src/A.go", SHA256: "one", Size: 20, Language: "go", Classification: "source", Content: "func Publish(){}"}
	g, err := db.StageGeneration(ctx, model.Snapshot{RepoID: "repo", Root: "/repo", Git: model.GitState{Commit: "one"}, ContentHash: "one", FileCount: 1, TotalBytes: file.Size, IndexedAt: time.Now(), ExtractorVersions: "fixture"}, []model.File{file}, []model.Symbol{{RepoID: "repo", Path: file.Path, Name: "Publish", Kind: "function", Span: model.Span{EndByte: 14, StartLine: 1, EndLine: 1}, Extractor: "fixture", Confidence: 1}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err = db.ActivateGeneration(ctx, g.ID); err != nil {
		t.Fatal(err)
	}
	c, err := cachepkg.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	s := &v1Service{cfg: catalog.Config{Version: 1, Repositories: []model.Repository{{ID: "repo", Root: "/repo"}}, Limits: catalog.Defaults()}, db: db, cache: c}
	_, first, err := s.query(ctx, nil, queryIn{scope: scope{RepoID: "repo"}, Text: "Publish"})
	if err != nil {
		t.Fatal(err)
	}
	_, second, err := s.query(ctx, nil, queryIn{scope: scope{RepoID: "repo"}, Text: "Publish"})
	if err != nil {
		t.Fatal(err)
	}
	if first.QueryFamily != second.QueryFamily || len(first.Entities) != len(second.Entities) {
		t.Fatalf("cache changed factual response: %#v %#v", first, second)
	}
	if c.Diagnostics(ctx).Hits == 0 {
		t.Fatal("expected plan cache hit")
	}
}

func TestQueryNegativeNotFoundAndUnknownCoverage(t *testing.T) {
	ctx := context.Background()
	db, err := store.OpenWriter(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	file := model.File{RepoID: "repo", Path: "A.java", SHA256: "h", Size: 10, Language: "java", Classification: "source", Content: "class A {}"}
	g, err := db.StageGeneration(ctx, model.Snapshot{RepoID: "repo", Root: "/repo", Git: model.GitState{Commit: "one"}, ContentHash: "h", FileCount: 1, TotalBytes: file.Size, IndexedAt: time.Now(), ExtractorVersions: "fixture"}, []model.File{file}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err = db.ActivateGeneration(ctx, g.ID); err != nil {
		t.Fatal(err)
	}
	s := &v1Service{cfg: catalog.Config{Version: 1, Repositories: []model.Repository{{ID: "repo", Root: "/repo"}}, Limits: catalog.Defaults()}, db: db}
	_, out, err := s.query(ctx, nil, queryIn{scope: scope{RepoID: "repo"}, Text: "does any caller of MissingService exist"})
	if err != nil || out.Status != "not_found" {
		t.Fatalf("out=%#v err=%v", out, err)
	}
	empty, err := store.OpenWriter(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer empty.Close()
	unknown := &v1Service{cfg: s.cfg, db: empty}
	_, missing, err := unknown.query(ctx, nil, queryIn{scope: scope{RepoID: "repo"}, Text: "does any caller of MissingService exist"})
	if err != nil || missing.Status != "unknown" {
		t.Fatalf("missing=%#v err=%v", missing, err)
	}
}

func TestQueryExplicitIntentIsExplained(t *testing.T) {
	ctx := context.Background()
	db, err := store.OpenWriter(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	file := model.File{RepoID: "repo", Path: "A.java", SHA256: "h", Size: 20, Language: "java", Classification: "source", Content: "class Publish {}"}
	g, err := db.StageGeneration(ctx, model.Snapshot{RepoID: "repo", Root: "/repo", Git: model.GitState{Commit: "one"}, ContentHash: "h", FileCount: 1, TotalBytes: file.Size, IndexedAt: time.Now(), ExtractorVersions: "fixture"}, []model.File{file}, []model.Symbol{{RepoID: "repo", Path: file.Path, Name: "Publish", Kind: "class", Span: model.Span{EndByte: 7, StartLine: 1, EndLine: 1}, Extractor: "fixture", Confidence: 1}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err = db.ActivateGeneration(ctx, g.ID); err != nil {
		t.Fatal(err)
	}
	s := &v1Service{cfg: catalog.Config{Version: 1, Repositories: []model.Repository{{ID: "repo", Root: "/repo"}}, Limits: catalog.Defaults()}, db: db}
	_, out, err := s.query(ctx, nil, queryIn{scope: scope{RepoID: "repo"}, Text: "Publish", Intent: planner.IntentLookup})
	if err != nil || out.PlanExplanation == nil || out.Truncated || len(out.Entities) != 1 {
		t.Fatalf("out=%#v err=%v", out, err)
	}
}

func TestV1HandlersUseCanonicalRecordsOnly(t *testing.T) {
	ctx := context.Background()
	db, err := store.OpenWriter(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	file := model.File{RepoID: "repo", Path: "main.go", SHA256: "one", Size: 29, Language: "go", Classification: "source", Content: "func Publish(){ emit(Event) }"}
	snap := model.Snapshot{RepoID: "repo", Root: "/repo", Git: model.GitState{Commit: "one", Branch: "main"}, ContentHash: "one", FileCount: 1, TotalBytes: file.Size, IndexedAt: time.Now(), ExtractorVersions: model.ExtractorVersion}
	sym := model.Symbol{RepoID: "repo", Path: file.Path, Name: "Publish", Kind: "function", Span: model.Span{StartByte: 0, EndByte: 14, StartLine: 1, StartColumn: 1, EndLine: 1, EndColumn: 15}, Extractor: "fixture", Confidence: 1}
	edge := model.Edge{RepoID: "repo", Path: file.Path, Source: "Publish", Target: "Event", Kind: "EMITS_EVENT", Span: model.Span{StartByte: 16, EndByte: 27, StartLine: 1, StartColumn: 17, EndLine: 1, EndColumn: 28}, Resolver: "fixture", Confidence: .9}
	g, err := db.StageGeneration(ctx, snap, []model.File{file}, []model.Symbol{sym}, []model.Edge{edge})
	if err != nil {
		t.Fatal(err)
	}
	if err = db.ActivateGeneration(ctx, g.ID); err != nil {
		t.Fatal(err)
	}
	service := &v1Service{cfg: catalog.Config{Version: 1, Repositories: []model.Repository{{ID: "repo", Root: "/repo"}}, Limits: catalog.Defaults()}, db: db}
	var subject, object, evidence string
	if err = db.DB().QueryRow(`SELECT subject_id,object_id,evidence_id FROM claims WHERE generation_id=?`, g.ID).Scan(&subject, &object, &evidence); err != nil {
		t.Fatal(err)
	}
	sh, oh, eh := enc("e", g.ID, subject), enc("e", g.ID, object), enc("v", g.ID, evidence)
	if _, r, e := service.resolve(ctx, nil, resolveIn{scope: scope{RepoID: "repo"}, Text: "Publish"}); e != nil || len(r.Entities) == 0 {
		t.Fatalf("resolve %#v %v", r, e)
	}
	if _, r, e := service.search(ctx, nil, searchIn{scope: scope{RepoID: "repo"}, Query: "Event"}); e != nil || len(r.Evidence) == 0 {
		t.Fatalf("search %#v %v", r, e)
	}
	if _, _, e := service.entity(ctx, nil, handleIn{Handle: sh}); e != nil {
		t.Fatal(e)
	}
	if _, _, e := service.source(ctx, nil, sourceIn{Handle: eh}); e != nil {
		t.Fatal(e)
	}
	if _, r, e := service.neighbors(ctx, nil, neighborsIn{Handle: sh, Direction: "out"}); e != nil || len(r.Claims) != 1 {
		t.Fatalf("neighbors %#v %v", r, e)
	}
	if _, _, e := service.references(ctx, nil, referencesIn{Handle: sh, Relation: "callees"}); e != nil {
		t.Fatal(e)
	}
	if _, r, e := service.events(ctx, nil, eventsIn{scope: scope{RepoID: "repo"}, Event: "Event"}); e != nil || len(r.Claims) != 1 {
		t.Fatalf("events %#v %v", r, e)
	}
	if _, _, e := service.impact(ctx, nil, impactIn{Handle: oh}); e != nil {
		t.Fatal(e)
	}
	if _, r, e := service.explain(ctx, nil, explainIn{Subject: sh, Object: oh}); e != nil || len(r.Claims) != 1 {
		t.Fatalf("explain %#v %v", r, e)
	}
	if _, r, e := service.path(ctx, nil, pathIn{From: sh, To: oh}); e != nil || len(r.Claims) == 0 {
		t.Fatalf("path %#v %v", r, e)
	}
	second := snap
	second.ContentHash, second.Git.Commit = "two", "two"
	second.IndexedAt = time.Now()
	file.SHA256 = "two"
	g2, err := db.StageGeneration(ctx, second, []model.File{file}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err = db.ActivateGeneration(ctx, g2.ID); err != nil {
		t.Fatal(err)
	}
	if _, _, err = service.entity(ctx, nil, handleIn{Handle: sh}); err == nil {
		t.Fatal("superseded entity handle remained valid")
	}
	if err = db.DB().QueryRow(`SELECT name FROM sqlite_master WHERE name='edges'`).Scan(new(string)); err != sql.ErrNoRows {
		t.Fatalf("legacy table unexpectedly present: %v", err)
	}
}

func TestRetrievalRanksFiltersPaginatesAndReportsNegativeCoverage(t *testing.T) {
	ctx := context.Background()
	db, err := store.OpenWriter(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	files := []model.File{
		{RepoID: "repo", Path: "src/publish.go", SHA256: "a", Size: 40, Language: "go", Classification: "source", Content: "func Publish() { literalPublish() }"},
		{RepoID: "repo", Path: "docs/publish.md", SHA256: "b", Size: 20, Language: "markdown", Classification: "documentation", Content: "Publish is documented"},
	}
	snap := model.Snapshot{RepoID: "repo", Root: "/repo", Git: model.GitState{Commit: "rev"}, ContentHash: "h", FileCount: 2, TotalBytes: 60, IndexedAt: time.Now(), ExtractorVersions: model.ExtractorVersion}
	_, err = db.StageGeneration(ctx, snap, files, []model.Symbol{{RepoID: "repo", Path: files[0].Path, Name: "Publish", Kind: "function", Span: model.Span{StartByte: 0, EndByte: 14, StartLine: 1, StartColumn: 1, EndLine: 1, EndColumn: 15}, Extractor: "fixture", Confidence: 1}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	var staged string
	if err = db.DB().QueryRow(`SELECT generation_id FROM generation_staging`).Scan(&staged); err != nil {
		t.Fatal(err)
	}
	if err = db.ActivateGeneration(ctx, staged); err != nil {
		t.Fatal(err)
	}
	s := &v1Service{cfg: catalog.Config{Version: 1, Repositories: []model.Repository{{ID: "repo", Root: "/repo"}}, Limits: catalog.Defaults()}, db: db}
	_, resolved, err := s.resolve(ctx, nil, resolveIn{scope: scope{RepoID: "repo"}, Text: "Publish", Limit: 1})
	if err != nil || len(resolved.Entities) != 1 || resolved.Entities[0].Label != "Publish" || !resolved.Truncated {
		t.Fatalf("resolve=%#v err=%v", resolved, err)
	}
	_, next, err := s.resolve(ctx, nil, resolveIn{scope: scope{RepoID: "repo"}, Text: "Publish", Limit: 1, Cursor: resolved.NextCursor})
	if err != nil || len(next.Evidence) != 1 {
		t.Fatalf("next=%#v err=%v", next, err)
	}
	_, filtered, err := s.search(ctx, nil, searchIn{scope: scope{RepoID: "repo"}, Query: "Publish", Classifications: []string{"documentation"}})
	if err != nil || len(filtered.Evidence) == 0 || filtered.Evidence[0].Path != "docs/publish.md" {
		t.Fatalf("filtered=%#v err=%v", filtered, err)
	}
	_, missing, err := s.search(ctx, nil, searchIn{scope: scope{RepoID: "repo"}, Query: "never-present"})
	if err != nil || missing.Status != "not_found" || missing.Coverage["complete"] != true {
		t.Fatalf("missing=%#v err=%v", missing, err)
	}
	if _, _, err = s.search(ctx, nil, searchIn{scope: scope{RepoID: "repo"}, Query: "Publish", Fields: []string{"bad"}}); err == nil {
		t.Fatal("accepted unsupported field")
	}
}

func TestSearchReportsUnknownWhenConfiguredScopeIsNotIndexed(t *testing.T) {
	db, err := store.OpenWriter(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	s := &v1Service{cfg: catalog.Config{Version: 1, Repositories: []model.Repository{{ID: "repo", Root: "/repo"}}, Limits: catalog.Defaults()}, db: db}
	_, out, err := s.search(context.Background(), nil, searchIn{scope: scope{RepoID: "repo"}, Query: "anything"})
	if err != nil || out.Status != "unknown" || out.Coverage["complete"] != false {
		t.Fatalf("out=%#v err=%v", out, err)
	}
}

func TestSearchReturnsUnknownForIncompleteCoverage(t *testing.T) {
	ctx := context.Background()
	db, err := store.OpenWriter(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	f := model.File{RepoID: "repo", Path: "src/A.java", SHA256: "h", Size: 10, Language: "java", Classification: "source", Content: "class A{}"}
	g, err := db.StageGenerationWithCoverage(ctx, model.Snapshot{RepoID: "repo", Root: "/repo", Git: model.GitState{Commit: "one"}, ContentHash: "h", FileCount: 1, TotalBytes: f.Size, IndexedAt: time.Now(), ExtractorVersions: "fixture"}, []model.File{f}, nil, nil, model.CoverageReport{Entries: []model.CoverageEntry{{Path: f.Path, Language: f.Language, Classification: f.Classification, Outcome: "included", Capability: "lexical,structural"}, {Path: "vendor", Outcome: "excluded", Reason: "vendor"}}})
	if err != nil {
		t.Fatal(err)
	}
	if err = db.ActivateGeneration(ctx, g.ID); err != nil {
		t.Fatal(err)
	}
	s := &v1Service{cfg: catalog.Config{Version: 1, Repositories: []model.Repository{{ID: "repo", Root: "/repo"}}, Limits: catalog.Defaults()}, db: db}
	_, out, err := s.search(ctx, nil, searchIn{scope: scope{RepoID: "repo"}, Query: "missing"})
	if err != nil || out.Status != "unknown" || len(out.SearchedScope["exclusions"].([]string)) == 0 || len(out.Uncertainty) == 0 {
		t.Fatalf("out=%#v err=%v", out, err)
	}
}

func TestRetrievalReportsPersistedCompilerCoverageDiagnostics(t *testing.T) {
	ctx := context.Background()
	db, err := store.OpenWriter(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	file := model.File{RepoID: "repo", Path: "src/A.ts", SHA256: "h", Size: 18, Language: "typescript", Classification: "source", Content: "export const A = 1"}
	snap := model.Snapshot{RepoID: "repo", Root: "/repo", Git: model.GitState{Commit: "one"}, ContentHash: "h", FileCount: 1, TotalBytes: file.Size, IndexedAt: time.Now(), ExtractorVersions: "compiler", CompilerDiagnostics: []model.CompilerDiagnostic{{Language: "typescript", Code: "tsconfig_missing", Message: "fallback retained"}}}
	g, err := db.StageGeneration(ctx, snap, []model.File{file}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err = db.ActivateGeneration(ctx, g.ID); err != nil {
		t.Fatal(err)
	}
	s := &v1Service{cfg: catalog.Config{Version: 1, Repositories: []model.Repository{{ID: "repo", Root: "/repo"}}, Limits: catalog.Defaults()}, db: db}
	_, out, err := s.search(ctx, nil, searchIn{scope: scope{RepoID: "repo"}, Query: "A"})
	if err != nil {
		t.Fatal(err)
	}
	diagnostics, ok := out.Coverage["compiler_diagnostics"].([]map[string]string)
	if !ok || len(diagnostics) != 1 || diagnostics[0]["code"] != "tsconfig_missing" {
		t.Fatalf("coverage=%#v", out.Coverage)
	}
}

func TestEventsAndExplainReturnBoundedCanonicalProvenanceAndCoverageDiagnostics(t *testing.T) {
	ctx := context.Background()
	db, err := store.OpenWriter(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	file := model.File{RepoID: "repo", Path: "src/A.java", SHA256: "h", Size: 64, Language: "java", Classification: "source", Content: "void publish() { kafkaTemplate.send(\"topic\", body); }"}
	snap := model.Snapshot{RepoID: "repo", Root: "/repo", Git: model.GitState{Commit: "one"}, ContentHash: "h", FileCount: 1, TotalBytes: file.Size, IndexedAt: time.Now(), ExtractorVersions: model.ExtractorVersion}
	sp := model.Span{StartByte: 17, EndByte: 51, StartLine: 1, StartColumn: 18, EndLine: 1, EndColumn: 52}
	edges := []model.Edge{{RepoID: "repo", Path: file.Path, Source: "publish", Target: "topic", Kind: "PRODUCES_TOPIC", Span: sp, Resolver: "fixture", Confidence: .95}, {RepoID: "repo", Path: file.Path, Source: "publish", Target: "dynamic_topic", Kind: "COVERAGE_GAP", Span: sp, Resolver: "fixture", Derivation: "coverage_gap", Confidence: 0}}
	g, err := db.StageGeneration(ctx, snap, []model.File{file}, nil, edges)
	if err != nil {
		t.Fatal(err)
	}
	if err = db.ActivateGeneration(ctx, g.ID); err != nil {
		t.Fatal(err)
	}
	s := &v1Service{cfg: catalog.Config{Version: 1, Repositories: []model.Repository{{ID: "repo", Root: "/repo"}}, Limits: catalog.Defaults()}, db: db}
	_, events, err := s.events(ctx, nil, eventsIn{scope: scope{RepoID: "repo"}, Event: "topic"})
	if err != nil || len(events.Claims) != 1 || len(events.Evidence) != 1 || len(events.Entities) != 2 {
		t.Fatalf("events=%#v err=%v", events, err)
	}
	if _, ok := events.Coverage["extractor_diagnostics"]; !ok {
		t.Fatalf("coverage=%#v", events.Coverage)
	}
	_, explain, err := s.explain(ctx, nil, explainIn{Subject: events.Claims[0].Subject})
	if err != nil || len(explain.Evidence) == 0 || len(explain.Entities) == 0 {
		t.Fatalf("explain=%#v err=%v", explain, err)
	}
}

func TestReferencesSupportsDomainRelationAliases(t *testing.T) {
	ctx := context.Background()
	db, err := store.OpenWriter(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	file := model.File{RepoID: "repo", Path: "src/A.java", SHA256: "h", Size: 32, Language: "java", Classification: "source", Content: "void run() { fetch(\"/api\"); }"}
	snap := model.Snapshot{RepoID: "repo", Root: "/repo", Git: model.GitState{Commit: "one"}, ContentHash: "h", FileCount: 1, TotalBytes: file.Size, IndexedAt: time.Now(), ExtractorVersions: model.ExtractorVersion}
	sp := model.Span{StartByte: 13, EndByte: 26, StartLine: 1, StartColumn: 14, EndLine: 1, EndColumn: 27}
	g, err := db.StageGeneration(ctx, snap, []model.File{file}, nil, []model.Edge{{RepoID: "repo", Path: file.Path, Source: "run", Target: "/api", Kind: "INVOKES_API", Span: sp, Resolver: "fixture", Confidence: .95}})
	if err != nil {
		t.Fatal(err)
	}
	if err = db.ActivateGeneration(ctx, g.ID); err != nil {
		t.Fatal(err)
	}
	var subject string
	if err = db.DB().QueryRow(`SELECT subject_id FROM claims WHERE generation_id=?`, g.ID).Scan(&subject); err != nil {
		t.Fatal(err)
	}
	s := &v1Service{cfg: catalog.Config{Version: 1, Repositories: []model.Repository{{ID: "repo", Root: "/repo"}}, Limits: catalog.Defaults()}, db: db}
	_, out, err := s.references(ctx, nil, referencesIn{Handle: enc("e", g.ID, subject), Relation: "http_calls"})
	if err != nil || len(out.Claims) != 1 || out.Claims[0].Predicate != "INVOKES_API" {
		t.Fatalf("out=%#v err=%v", out, err)
	}
}

func TestNeighborsTraversesCrossRepositoryEvidenceWithBudgets(t *testing.T) {
	ctx := context.Background()
	db, err := store.OpenWriter(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	stage := func(repo, predicate string) store.Generation {
		f := model.File{RepoID: repo, Path: "A.java", SHA256: repo, Size: 20, Language: "java", Classification: "source", Content: "void event() {}"}
		sp := model.Span{StartByte: 0, EndByte: 10, StartLine: 1, StartColumn: 1, EndLine: 1, EndColumn: 11}
		g, e := db.StageGeneration(ctx, model.Snapshot{RepoID: repo, Root: "/" + repo, Git: model.GitState{Commit: repo}, ContentHash: repo, FileCount: 1, TotalBytes: f.Size, IndexedAt: time.Now(), ExtractorVersions: "fixture"}, []model.File{f}, nil, []model.Edge{{RepoID: repo, Path: f.Path, Source: "run", Target: "OrderChanged", Kind: predicate, Span: sp, Resolver: "fixture", Confidence: .95}})
		if e != nil {
			t.Fatal(e)
		}
		return g
	}
	p, c := stage("producer", "PUBLISHES_EVENT"), stage("consumer", "CONSUMES_EVENT")
	if err := db.ActivateCatalog(ctx, []string{p.ID, c.ID}); err != nil {
		t.Fatal(err)
	}
	var id string
	if err := db.DB().QueryRow(`SELECT object_id FROM claims WHERE generation_id=?`, p.ID).Scan(&id); err != nil {
		t.Fatal(err)
	}
	s := &v1Service{cfg: catalog.Config{Version: 1, Repositories: []model.Repository{{ID: "producer", Root: "/producer"}, {ID: "consumer", Root: "/consumer"}}, Limits: catalog.Defaults()}, db: db}
	_, out, err := s.neighbors(ctx, nil, neighborsIn{Handle: enc("e", p.ID, id), Direction: "out", Depth: 1, Budget: 10, MaxFanout: 10, MaxEntities: 10, MaxEdges: 10})
	if err != nil || len(out.Claims) != 1 || out.Claims[0].Predicate != "EVENT_PRODUCER_CONSUMER" || len(out.Evidence) != 2 {
		t.Fatalf("out=%#v err=%v", out, err)
	}
	_, path, err := s.path(ctx, nil, pathIn{From: enc("e", p.ID, id), To: out.Claims[0].Object, MaxDepth: 1})
	if err != nil || len(path.Claims) != 1 || len(path.Evidence) != 2 {
		t.Fatalf("path=%#v err=%v", path, err)
	}
	_, explained, err := s.explain(ctx, nil, explainIn{Subject: enc("e", p.ID, id), Object: out.Claims[0].Object})
	if err != nil || len(explained.Claims) != 1 || len(explained.Evidence) != 2 {
		t.Fatalf("explain=%#v err=%v", explained, err)
	}
	if _, _, err := s.neighbors(ctx, nil, neighborsIn{Handle: enc("e", p.ID, id), MaxFanout: 101}); err == nil {
		t.Fatal("accepted oversized fanout")
	}
}
