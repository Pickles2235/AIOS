package store

import (
	"context"
	"testing"
	"time"

	"github.com/AdamNi-7080/AIOS/internal/model"
)

func TestPurgeRepositoryKeepsSurvivorAndRollsBackFailure(t *testing.T) {
	ctx := context.Background()
	db, err := OpenWriter(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	canonicalFixture(t, db, "old")
	active := canonicalFixture(t, db, "active")
	file := model.File{RepoID: "other", Path: "src/Survivor.go", SHA256: "survivor", Size: 17, Language: "go", Content: "func Survivor(){}"}
	sp := model.Span{EndByte: 17, StartLine: 1, StartColumn: 1, EndLine: 1, EndColumn: 18}
	survivor, err := db.StageGeneration(ctx, model.Snapshot{RepoID: "other", Root: "/other", Git: model.GitState{Commit: "survivor"}, ContentHash: "survivor", FileCount: 1, TotalBytes: 17, IndexedAt: time.Now(), ExtractorVersions: model.ExtractorVersion}, []model.File{file}, []model.Symbol{{RepoID: "other", Path: file.Path, Name: "Survivor", Kind: "function", Span: sp, Extractor: "fixture", Confidence: 1}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err = db.ActivateCatalog(ctx, []string{active.ID, survivor.ID}); err != nil {
		t.Fatal(err)
	}
	catalog, err := db.ActiveCatalogRevision(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.DB().Exec(`CREATE TRIGGER purge_fault BEFORE DELETE ON ir_repositories BEGIN SELECT RAISE(ABORT,'injected purge failure'); END`); err != nil {
		t.Fatal(err)
	}
	if err = db.PurgeRepository(ctx, "repo"); err == nil {
		t.Fatal("purge fault did not fail")
	}
	kept, err := db.ActiveGeneration(ctx, "repo")
	if err != nil || kept.ID != active.ID {
		t.Fatalf("failed purge destroyed last good: %+v %v", kept, err)
	}
	now, err := db.ActiveCatalogRevision(ctx)
	if err != nil || now.ID != catalog.ID {
		t.Fatal("failed purge published replacement")
	}
	if _, err = db.DB().Exec(`DROP TRIGGER purge_fault`); err != nil {
		t.Fatal(err)
	}
	if err = db.PurgeRepository(ctx, "repo"); err != nil {
		t.Fatal(err)
	}
	if _, err = db.ActiveGeneration(ctx, "repo"); err == nil {
		t.Fatal("removed generation still active")
	}
	kept, err = db.ActiveGeneration(ctx, "other")
	if err != nil || kept.ID != survivor.ID {
		t.Fatalf("purge changed survivor: %+v %v", kept, err)
	}
	for _, q := range []string{
		`SELECT count(*) FROM ir_repositories WHERE repository_id='repo'`,
		`SELECT count(*) FROM ir_source_revisions WHERE repository_id='repo'`,
		`SELECT count(*) FROM ir_entities WHERE repository_id='repo'`,
		`SELECT count(*) FROM generations WHERE repo_id='repo'`,
		`SELECT count(*) FROM source_files WHERE repo_id='repo'`,
		`SELECT count(*) FROM evidence WHERE repo_id='repo'`,
		`SELECT count(*) FROM entities WHERE repo_id='repo'`,
		`SELECT count(*) FROM source_fts WHERE repo_id='repo'`,
		`SELECT count(*) FROM search_fts WHERE repo_id='repo'`,
		`SELECT count(*) FROM catalog_revision_members WHERE repo_id='repo'`,
	} {
		var n int
		if err = db.DB().QueryRow(q).Scan(&n); err != nil || n != 0 {
			t.Fatalf("purge left owned rows: %d %v %s", n, err, q)
		}
	}
	if err = db.CompactPurgedData(ctx); err != nil {
		t.Fatal(err)
	}
	results, err := db.ExactCandidates(ctx, "Survivor", QueryFilter{Repository: "other"})
	if err != nil || len(results) != 1 {
		t.Fatalf("survivor evidence lost: %d %v", len(results), err)
	}
	if err = db.PurgeRepository(ctx, "other"); err != nil {
		t.Fatal(err)
	}
	var n int
	if err = db.DB().QueryRow(`SELECT count(*) FROM generations`).Scan(&n); err != nil || n != 0 {
		t.Fatalf("last repository was not fully removed: %d %v", n, err)
	}
}
