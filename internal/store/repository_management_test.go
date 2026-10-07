package store

import (
	"context"
	"path/filepath"
	"slices"
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
	if _, err = db.DB().Exec(`INSERT INTO ir_source_revisions(revision_id,repository_id,source_kind,source_adapter_version,git_commit,content_hash,extractor_versions,indexed_at) VALUES('retired-ir-revision','repo','repository','fixture','retired','retired','fixture','2026-10-07T00:00:00Z')`); err != nil {
		t.Fatal(err)
	}
	if _, err = db.DB().Exec(`INSERT INTO ir_locations VALUES('retired-location','retired-ir-revision','source','src/retired.go','retired',0,1,1,1,1,2)`); err != nil {
		t.Fatal(err)
	}
	var retiredFact string
	if err = db.DB().QueryRow(`SELECT fact_id FROM ir_facts ORDER BY fact_id LIMIT 1`).Scan(&retiredFact); err != nil {
		t.Fatal(err)
	}
	if _, err = db.DB().Exec(`INSERT INTO ir_fact_observations VALUES('retired-observation',?,'retired-ir-revision','retired-evidence','retired-location','fixture','1',1,'2026-10-07T00:00:00Z')`, retiredFact); err != nil {
		t.Fatal(err)
	}
	file := model.File{RepoID: "other", Path: "src/Survivor.go", SHA256: "survivor", Size: 17, Language: "go", Content: "func Survivor(){}"}
	sp := model.Span{EndByte: 17, StartLine: 1, StartColumn: 1, EndLine: 1, EndColumn: 18}
	survivor, err := db.StageGeneration(ctx, model.Snapshot{RepoID: "other", Root: "/other", Git: model.GitState{Commit: "survivor"}, ContentHash: "survivor", FileCount: 1, TotalBytes: 17, IndexedAt: time.Now(), ExtractorVersions: model.ExtractorVersion}, []model.File{file}, []model.Symbol{{RepoID: "other", Path: file.Path, Name: "Survivor", Kind: "function", Span: sp, Extractor: "fixture", Confidence: 1}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err = db.ActivateCatalog(ctx, []string{active.ID, survivor.ID}); err != nil {
		t.Fatal(err)
	}
	ownedWithoutCandidates, err := db.RepositoryOwnedRecords(ctx, "repo")
	if err != nil {
		t.Fatal(err)
	}
	snapshotBase := filepath.Join(filepath.Dir(db.Path()), "snapshots")
	removedCandidates := []string{
		filepath.Join(snapshotBase, "repo", ".staging", ".staging-local-interrupted"),
		filepath.Join(snapshotBase, "repo", "retired-generation"),
	}
	keptCandidates := []string{
		filepath.Join(snapshotBase, "other", ".staging", ".staging-mirror-survivor"),
		filepath.Join(snapshotBase, "repo-extra", "prefix-collision"),
		filepath.Join(filepath.Dir(snapshotBase), "external", "unowned"),
	}
	for _, root := range append(append([]string{}, removedCandidates...), keptCandidates...) {
		if err = db.QueueSnapshotGC(ctx, root); err != nil {
			t.Fatal(err)
		}
	}
	ownedBefore, err := db.RepositoryOwnedRecords(ctx, "repo")
	if err != nil || ownedBefore != ownedWithoutCandidates+len(removedCandidates) {
		t.Fatalf("repository GC candidate count = %d, want %d: %v", ownedBefore, ownedWithoutCandidates+len(removedCandidates), err)
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
	var retiredRecords int
	if err = db.DB().QueryRow(`SELECT
		(SELECT count(*) FROM ir_source_revisions WHERE revision_id='retired-ir-revision') +
		(SELECT count(*) FROM ir_locations WHERE location_id='retired-location') +
		(SELECT count(*) FROM ir_fact_observations WHERE fact_observation_id='retired-observation')`).Scan(&retiredRecords); err != nil || retiredRecords != 3 {
		t.Fatalf("failed purge changed retired IR history: %d %v", retiredRecords, err)
	}
	gotCandidates, err := db.SnapshotGCCandidates(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, root := range removedCandidates {
		if !slices.Contains(gotCandidates, root) {
			t.Fatalf("failed purge deleted GC candidate %q", root)
		}
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
	gotCandidates, err = db.SnapshotGCCandidates(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, root := range removedCandidates {
		if slices.Contains(gotCandidates, root) {
			t.Fatalf("purge left repository GC candidate %q", root)
		}
	}
	for _, root := range keptCandidates {
		if !slices.Contains(gotCandidates, root) {
			t.Fatalf("purge deleted unrelated GC candidate %q", root)
		}
	}
	ownedAfter, err := db.RepositoryOwnedRecords(ctx, "repo")
	if err != nil || ownedAfter != 0 {
		t.Fatalf("purge status retained repository-owned records: %d %v", ownedAfter, err)
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
	if err = db.DB().QueryRow(`SELECT
		(SELECT count(*) FROM ir_source_revisions WHERE revision_id='retired-ir-revision') +
		(SELECT count(*) FROM ir_locations WHERE location_id='retired-location') +
		(SELECT count(*) FROM ir_fact_observations WHERE fact_observation_id='retired-observation')`).Scan(&retiredRecords); err != nil || retiredRecords != 0 {
		t.Fatalf("purge left retired IR history: %d %v", retiredRecords, err)
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

func TestPurgeRepositoryBeforeSnapshotCandidateTableExists(t *testing.T) {
	ctx := context.Background()
	db, err := OpenWriter(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	canonicalFixture(t, db, "old")
	owned, err := db.RepositoryOwnedRecords(ctx, "repo")
	if err != nil || owned == 0 {
		t.Fatalf("owned records before purge = %d: %v", owned, err)
	}
	if err = db.PurgeRepository(ctx, "repo"); err != nil {
		t.Fatal(err)
	}
	owned, err = db.RepositoryOwnedRecords(ctx, "repo")
	if err != nil || owned != 0 {
		t.Fatalf("owned records after purge = %d: %v", owned, err)
	}
}
