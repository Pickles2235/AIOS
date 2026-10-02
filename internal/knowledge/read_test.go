package knowledge

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/AdamNi-7080/AIOS/internal/catalog"
	"github.com/AdamNi-7080/AIOS/internal/model"
	"github.com/AdamNi-7080/AIOS/internal/store"
)

func fixture(t *testing.T) (*store.Store, *Service) {
	t.Helper()
	db, err := store.OpenWriter(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	f := model.File{RepoID: "repo", Path: "src/a.go", SHA256: "one", Size: 28, Language: "go", Classification: "source", Content: "func Publish() { emit(Event) }"}
	sp := model.Span{StartByte: 0, EndByte: 14, StartLine: 1, StartColumn: 1, EndLine: 1, EndColumn: 15}
	g, err := db.StageGeneration(context.Background(), model.Snapshot{RepoID: "repo", Root: "/repo", Git: model.GitState{Commit: "one"}, ContentHash: "one", FileCount: 1, TotalBytes: f.Size, IndexedAt: time.Now(), ExtractorVersions: model.ExtractorVersion}, []model.File{f}, []model.Symbol{{RepoID: "repo", Path: f.Path, Name: "Publish", Kind: "function", Span: sp, Extractor: "fixture", Confidence: 1}}, []model.Edge{{RepoID: "repo", Path: f.Path, Source: "Publish", Target: "Event", Kind: "EMITS_EVENT", Span: sp, Resolver: "fixture", Confidence: .9}})
	if err != nil {
		t.Fatal(err)
	}
	if err = db.ActivateGeneration(context.Background(), g.ID); err != nil {
		t.Fatal(err)
	}
	return db, New(catalog.Config{Repositories: []model.Repository{{ID: "repo", Root: "/repo"}}}, db)
}
func TestProjectionAndEvidenceAreBoundedAndGenerationBacked(t *testing.T) {
	db, s := fixture(t)
	defer db.Close()
	p, err := s.Projection(context.Background(), "repo", "", 1)
	if err != nil || len(p.Nodes) != 1 || p.Generation == "" || !p.Truncated {
		t.Fatalf("projection=%#v err=%v", p, err)
	}
	e, err := s.Excerpt(context.Background(), p.Nodes[0].Evidence, 0, 0, 1)
	if err != nil || e.Path == "" || e.StartLine < 1 || len(e.Lines) != 1 {
		t.Fatalf("excerpt=%#v err=%v", e, err)
	}
}
func TestStaleHandleIsRejected(t *testing.T) {
	db, s := fixture(t)
	defer db.Close()
	p, err := s.Projection(context.Background(), "repo", "", 1)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.Entity(context.Background(), p.Nodes[0].Handle); err != nil {
		t.Fatal(err)
	}
	if _, err = s.Entity(context.Background(), Encode("e", "missing", "id")); err == nil {
		t.Fatal("accepted stale handle")
	}
}

func TestQueryAbsenceRequiresCompleteCoverage(t *testing.T) {
	db, s := fixture(t)
	defer db.Close()
	ctx := context.Background()
	r, err := s.Query(ctx, Query{Text: "missing_symbol"})
	if err != nil || r.Status != "not_found" || r.Coverage == nil || !r.Coverage.Complete {
		t.Fatalf("complete absence: %#v %v", r, err)
	}
	if _, err = db.DB().ExecContext(ctx, `UPDATE coverage_runs SET status='incomplete'`); err != nil {
		t.Fatal(err)
	}
	r, err = s.Query(ctx, Query{Text: "missing_symbol"})
	if err != nil || r.Status != "unknown" || r.Coverage == nil || r.Coverage.Complete {
		t.Fatalf("incomplete absence: %#v %v", r, err)
	}
	r, err = s.Query(ctx, Query{Text: "Publish"})
	if err != nil || r.Status != "found" || len(r.Entities) == 0 || r.Coverage.Complete {
		t.Fatalf("positive evidence with gaps: %#v %v", r, err)
	}
}

func TestQueryUnavailableEmptyUnsupportedAndBudget(t *testing.T) {
	db, s := fixture(t)
	defer db.Close()
	for _, tc := range []struct {
		query Query
		kind  string
	}{
		{Query{}, "empty_query"},
		{Query{Text: strings.Repeat("a", 257)}, "unsupported_query"},
		{Query{Text: "Publish", MinimumConfidence: 2}, "unsupported_query"},
	} {
		r, err := s.Query(context.Background(), tc.query)
		if err != nil || r.Status != "unknown" || len(r.Trace) != 1 || r.Trace[0].Kind != tc.kind {
			t.Fatalf("%s: %#v %v", tc.kind, r, err)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	r, err := s.Query(ctx, Query{Text: "Publish"})
	if err != nil || r.Status != "unknown" || r.Trace[0].Kind != "budget_exhausted" {
		t.Fatalf("budget: %#v %v", r, err)
	}
	if _, err = db.DB().ExecContext(context.Background(), `DELETE FROM active_projection_builds WHERE projection_kind='lexical'`); err != nil {
		t.Fatal(err)
	}
	r, err = s.Query(context.Background(), Query{Text: "missing_symbol"})
	if err != nil || r.Status != "unknown" || r.Trace[0].Kind != "projection_unavailable" {
		t.Fatalf("outage: %#v %v", r, err)
	}
	// An exact positive still has canonical evidence when lexical projection is absent.
	r, err = s.Query(context.Background(), Query{Text: "Event"})
	if err != nil || r.Status != "found" || r.Coverage.Complete {
		t.Fatalf("exact: %#v %v", r, err)
	}
}

func TestProjectionCursorRejectsChangedGeneration(t *testing.T) {
	db, s := fixture(t)
	defer db.Close()
	ctx := context.Background()
	p, err := s.Projection(ctx, "repo", "", 1)
	if err != nil || p.NextCursor == "" {
		t.Fatalf("cursor: %#v %v", p, err)
	}
	if _, err = s.Projection(ctx, "repo", p.NextCursor, 1); err != nil {
		t.Fatal(err)
	}
	f := model.File{RepoID: "repo", Path: "src/b.go", SHA256: "two", Size: 14, Language: "go", Classification: "source", Content: "func Next() {}"}
	g, err := db.StageGeneration(ctx, model.Snapshot{RepoID: "repo", Root: "/repo", Git: model.GitState{Commit: "two"}, ContentHash: "two", FileCount: 1, TotalBytes: f.Size, IndexedAt: time.Now(), ExtractorVersions: model.ExtractorVersion}, []model.File{f}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err = db.ActivateGeneration(ctx, g.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = s.Projection(ctx, "repo", p.NextCursor, 1); err == nil {
		t.Fatal("accepted cursor from previous generation")
	}
}

func TestIncompleteLookupCannotCertifyNegativeKnowledge(t *testing.T) {
	db, s := fixture(t)
	defer db.Close()
	if _, e := db.DB().Exec(`DELETE FROM projection_lookup_records WHERE entity_id=(SELECT entity_id FROM entities WHERE label='Publish' LIMIT 1)`); e != nil {
		t.Fatal(e)
	}
	result, e := s.Query(context.Background(), Query{Text: "missing_symbol"})
	if e != nil || result.Status != "unknown" || len(result.Trace) == 0 || result.Trace[0].Kind != "projection_unavailable" {
		t.Fatalf("incomplete lookup certified absence: %+v %v", result, e)
	}
	if e := db.RebuildProjections(context.Background(), []string{"lookup"}); e != nil {
		t.Fatal(e)
	}
	result, e = s.Query(context.Background(), Query{Text: "Publish"})
	if e != nil || result.Status != "found" {
		t.Fatal("repaired lookup lost canonical result", result.Status, e)
	}
}
