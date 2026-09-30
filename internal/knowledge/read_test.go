package knowledge

import (
	"context"
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
