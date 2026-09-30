package store

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/AdamNi-7080/AIOS/internal/model"
)

func TestDiagnosticLedgerIsRedactedAndResolvable(t *testing.T) {
	db, err := OpenWriter(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err = db.RecordDiagnostic(context.Background(), model.DiagnosticEvent{Code: DiagnosticExtractionFailure, Severity: model.DiagnosticWarning, Scope: model.DiagnosticScope{Repository: "repo", Path: "src/a.go"}, Remediation: "repair extractor", Metadata: map[string]string{"language": "go"}}); err != nil {
		t.Fatal(err)
	}
	if _, err = db.RecordDiagnostic(context.Background(), model.DiagnosticEvent{Code: DiagnosticExtractionFailure, Severity: model.DiagnosticWarning, Metadata: map[string]string{"query_text": "secret"}}); err == nil {
		t.Fatal("unsafe metadata accepted")
	}
	events, err := db.DiagnosticEvents(context.Background(), "repo", false, 10)
	if err != nil || len(events) != 1 || events[0].Scope.Path != "src/a.go" {
		t.Fatalf("events=%#v err=%v", events, err)
	}
	if err = db.ResolveDiagnostic(context.Background(), events[0].ID, "fixed"); err != nil {
		t.Fatal(err)
	}
	active, err := db.DiagnosticEvents(context.Background(), "repo", false, 10)
	if err != nil || len(active) != 0 {
		t.Fatalf("active=%#v err=%v", active, err)
	}
	history, err := db.DiagnosticEvents(context.Background(), "repo", true, 10)
	if err != nil || len(history) != 1 || history[0].ResolvedAt == "" || !strings.EqualFold(history[0].Resolution, "fixed") {
		t.Fatalf("history=%#v err=%v", history, err)
	}
}

func TestStructuralCoverageIgnoresNonStructuralMetadataFiles(t *testing.T) {
	ctx := context.Background()
	db, err := OpenWriter(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	files := []model.File{{RepoID: "repo", Path: "src/A.java", SHA256: "a", Size: 10, Language: "java", Classification: "source", Content: "class A{}"}, {RepoID: "repo", Path: "CODEOWNERS", SHA256: "b", Size: 4, Language: "text", Classification: "source", Content: "team"}}
	report := model.CoverageReport{Entries: []model.CoverageEntry{{Path: files[0].Path, Language: "java", Classification: "source", Outcome: "included", Capability: "lexical,structural"}, {Path: files[1].Path, Language: "text", Classification: "source", Outcome: "included", Capability: "lexical"}}}
	g, err := db.StageGenerationWithCoverage(ctx, model.Snapshot{RepoID: "repo", Root: "/repo", Git: model.GitState{Commit: "one"}, ContentHash: "one", FileCount: 2, TotalBytes: 14, IndexedAt: time.Now(), ExtractorVersions: "fixture"}, files, nil, nil, report)
	if err != nil {
		t.Fatal(err)
	}
	if err = db.ActivateGeneration(ctx, g.ID); err != nil {
		t.Fatal(err)
	}
	basis, err := db.Coverage(ctx, "repo", "structural", nil)
	if err != nil || !basis.Complete {
		t.Fatalf("basis=%#v err=%v", basis, err)
	}
}

func TestStageGenerationPersistsCoverageOutcomes(t *testing.T) {
	db, err := OpenWriter(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ctx := context.Background()
	f := model.File{RepoID: "repo", Path: "src/A.java", SHA256: "a", Size: 10, Language: "java", Classification: "source", Content: "class A{}"}
	g, err := db.StageGenerationWithCoverage(ctx, model.Snapshot{RepoID: "repo", Root: "/repo", Git: model.GitState{Commit: "one"}, ContentHash: "h", FileCount: 1, TotalBytes: f.Size, IndexedAt: time.Now(), ExtractorVersions: "fixture"}, []model.File{f}, nil, nil, model.CoverageReport{Entries: []model.CoverageEntry{{Path: f.Path, Language: f.Language, Classification: f.Classification, Outcome: "included", Capability: "lexical,structural"}, {Path: "vendor", Outcome: "excluded", Reason: "vendor"}}})
	if err != nil {
		t.Fatal(err)
	}
	if err = db.ActivateGeneration(ctx, g.ID); err != nil {
		t.Fatal(err)
	}
	var status string
	var excluded int
	if err = db.DB().QueryRow(`SELECT status FROM coverage_runs WHERE generation_id=?`, g.ID).Scan(&status); err != nil || status != "incomplete" {
		t.Fatalf("status=%q err=%v", status, err)
	}
	if err = db.DB().QueryRow(`SELECT count(*) FROM coverage_entries WHERE outcome='excluded' AND reason='vendor'`).Scan(&excluded); err != nil || excluded != 1 {
		t.Fatalf("excluded=%d err=%v", excluded, err)
	}
}

func TestNegativeEvidenceInvalidatesOnlyChangedGeneration(t *testing.T) {
	db, err := OpenWriter(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ctx := context.Background()
	stage := func(repo, rev string) Generation {
		f := model.File{RepoID: repo, Path: "A.txt", SHA256: rev, Size: 1, Language: "text", Classification: "source", Content: "A"}
		g, e := db.StageGeneration(ctx, model.Snapshot{RepoID: repo, Root: "/" + repo, Git: model.GitState{Commit: rev}, ContentHash: rev, FileCount: 1, TotalBytes: 1, IndexedAt: time.Now(), ExtractorVersions: "fixture"}, []model.File{f}, nil, nil)
		if e != nil {
			t.Fatal(e)
		}
		return g
	}
	a1, b1 := stage("a", "a1"), stage("b", "b1")
	if err = db.ActivateCatalog(ctx, []string{a1.ID, b1.ID}); err != nil {
		t.Fatal(err)
	}
	basis, err := db.Coverage(ctx, "a", "lexical", nil)
	if err != nil || !basis.Complete {
		t.Fatalf("basis=%#v err=%v", basis, err)
	}
	negative, err := db.RecordNegative(ctx, "search", "missing", "p", map[string]any{"repositories": []string{"a"}}, basis)
	if err != nil {
		t.Fatal(err)
	}
	b2 := stage("b", "b2")
	if err = db.ActivateCatalog(ctx, []string{a1.ID, b2.ID}); err != nil {
		t.Fatal(err)
	}
	var invalidated string
	if err = db.DB().QueryRow(`SELECT invalidated_at FROM negative_evidence WHERE negative_id=?`, negative.ID).Scan(&invalidated); err != nil || invalidated != "" {
		t.Fatalf("unrelated invalidated=%q err=%v", invalidated, err)
	}
	a2 := stage("a", "a2")
	if err = db.ActivateCatalog(ctx, []string{a2.ID, b2.ID}); err != nil {
		t.Fatal(err)
	}
	if err = db.DB().QueryRow(`SELECT invalidated_at FROM negative_evidence WHERE negative_id=?`, negative.ID).Scan(&invalidated); err != nil || invalidated == "" {
		t.Fatalf("relevant invalidated=%q err=%v", invalidated, err)
	}
}

func TestReadOnlyStoreCannotWrite(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	writer, err := OpenWriter(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := writer.ReplaceRepository(ctx, model.Snapshot{RepoID: "repo", Root: "/repo", Git: model.GitState{Commit: "abc", Branch: "main"}, ContentHash: "hash", IndexedAt: time.Now(), ExtractorVersions: model.ExtractorVersion}, nil, nil, nil); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	reader, err := OpenReadOnly(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	if err := reader.ReplaceRepository(ctx, model.Snapshot{RepoID: "new", Root: "/new"}, nil, nil, nil); err == nil || !strings.Contains(err.Error(), "read-only") {
		t.Fatalf("read-only store accepted ReplaceRepository: %v", err)
	}
	if _, err := reader.DB().ExecContext(ctx, `CREATE TABLE forbidden(value TEXT)`); err == nil {
		t.Fatal("read-only database accepted a write")
	}
	status, err := reader.Status(ctx, "repo")
	if err != nil || len(status) != 1 {
		t.Fatalf("read-only query failed: %#v, %v", status, err)
	}
}

func TestDiscardStagedRepositoryPreservesActiveGeneration(t *testing.T) {
	ctx := context.Background()
	db, err := OpenWriter(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	active := canonicalFixture(t, db, "one")
	file := model.File{RepoID: "repo", Path: "src/New.java", SHA256: "two", Size: 9, Language: "java", Classification: "source", Content: "class New"}
	staged, err := db.StageGeneration(ctx, model.Snapshot{RepoID: "repo", Root: "/repo", Git: model.GitState{Commit: "two"}, ContentHash: "two", FileCount: 1, TotalBytes: file.Size, IndexedAt: time.Now(), ExtractorVersions: "fixture"}, []model.File{file}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err = db.DiscardStagedRepository(ctx, "repo"); err != nil {
		t.Fatal(err)
	}
	if _, err = db.GenerationByID(ctx, staged.ID); err == nil {
		t.Fatal("staged generation survived discard")
	}
	got, err := db.ActiveGeneration(ctx, "repo")
	if err != nil || got.ID != active.ID {
		t.Fatalf("active=%#v err=%v", got, err)
	}
	if results, queryErr := db.ExactCandidates(ctx, "Publish", QueryFilter{Repository: "repo"}); queryErr != nil || len(results) == 0 {
		t.Fatalf("last known good query failed: results=%d err=%v", len(results), queryErr)
	}
}

func TestInvalidActivationPreservesActiveCatalog(t *testing.T) {
	ctx := context.Background()
	db, err := OpenWriter(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	active := canonicalFixture(t, db, "one")
	before, err := db.ActiveCatalogRevision(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err = db.ActivateCatalog(ctx, []string{"not-a-generation"}); err == nil {
		t.Fatal("invalid activation succeeded")
	}
	after, err := db.ActiveCatalogRevision(ctx)
	if err != nil || after.ID != before.ID {
		t.Fatalf("catalog changed: before=%s after=%s err=%v", before.ID, after.ID, err)
	}
	got, err := db.ActiveGeneration(ctx, "repo")
	if err != nil || got.ID != active.ID {
		t.Fatalf("active generation changed: %#v err=%v", got, err)
	}
	if results, queryErr := db.ExactCandidates(ctx, "Publish", QueryFilter{Repository: "repo"}); queryErr != nil || len(results) == 0 {
		t.Fatalf("last known good query failed: results=%d err=%v", len(results), queryErr)
	}
}

func TestOpenReadOnlyRejectsUnsafeOrCorruptDatabase(t *testing.T) {
	for _, test := range []struct {
		name  string
		setup func(t *testing.T, dir string)
	}{
		{name: "symlink", setup: func(t *testing.T, dir string) {
			t.Helper()
			target := filepath.Join(t.TempDir(), "other.db")
			if err := os.WriteFile(target, []byte("not a database"), 0o600); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(target, filepath.Join(dir, DatabaseName)); err != nil {
				t.Fatal(err)
			}
		}},
		{name: "insecure mode", setup: func(t *testing.T, dir string) {
			t.Helper()
			writer, err := OpenWriter(dir)
			if err != nil {
				t.Fatal(err)
			}
			if err := writer.Close(); err != nil {
				t.Fatal(err)
			}
			if err := os.Chmod(filepath.Join(dir, DatabaseName), 0o644); err != nil {
				t.Fatal(err)
			}
		}},
		{name: "corrupt", setup: func(t *testing.T, dir string) {
			t.Helper()
			writer, err := OpenWriter(dir)
			if err != nil {
				t.Fatal(err)
			}
			if err := writer.Close(); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(dir, DatabaseName), []byte("not a sqlite database"), 0o600); err != nil {
				t.Fatal(err)
			}
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			dir := t.TempDir()
			if err := os.Chmod(dir, 0o700); err != nil {
				t.Fatal(err)
			}
			test.setup(t, dir)
			if reader, err := OpenReadOnly(dir); err == nil {
				reader.Close()
				t.Fatal("OpenReadOnly accepted unsafe database")
			}
		})
	}
}

func TestPreviousInputsReuseOnlyDerivedRecords(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	db, err := OpenWriter(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	f := model.File{RepoID: "repo", Path: "a.ts", SHA256: "one", Size: 12, Language: "typescript", Content: "export fn(){}"}
	s := model.Symbol{RepoID: "repo", Path: f.Path, Name: "fn", Kind: "function", Identity: "ts:fn", Span: model.Span{EndByte: 11, StartLine: 1, StartColumn: 1, EndLine: 1, EndColumn: 12}, Extractor: "fixture", Confidence: 1}
	e := model.Edge{RepoID: "repo", Path: f.Path, Source: "fn", Target: "Topic", SourceIdentity: "ts:fn", TargetIdentity: "topic:one", Kind: "PUBLISHES_EVENT", Span: s.Span, Resolver: "fixture", Confidence: 1}
	if err = db.ReplaceRepository(ctx, model.Snapshot{RepoID: "repo", Root: "/repo", Git: model.GitState{Commit: "one"}, ContentHash: "one", ExtractorVersions: "fixture", IndexedAt: time.Now()}, []model.File{f}, []model.Symbol{s}, []model.Edge{e}); err != nil {
		t.Fatal(err)
	}
	files, err := db.ActiveFiles(ctx, "repo")
	if err != nil || len(files) != 1 || files[0].Content != f.Content {
		t.Fatalf("manifest %#v: %v", files, err)
	}
	syms, edges, err := db.PreviousInputs(ctx, "repo")
	if err != nil || len(syms) != 1 || len(edges) != 1 {
		t.Fatalf("inputs %d %d: %v", len(syms), len(edges), err)
	}
	if syms[0].Identity != s.Identity || edges[0].TargetIdentity != e.TargetIdentity {
		t.Fatalf("inputs changed %#v %#v", syms[0], edges[0])
	}
}

func TestRetainPurgesOnlyUnreferencedHistoricalGenerations(t *testing.T) {
	ctx := context.Background()
	db, err := OpenWriter(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	for _, revision := range []string{"one", "two", "three"} {
		f := model.File{RepoID: "repo", Path: "a.go", SHA256: revision, Size: 1, Language: "go", Content: "x"}
		if err = db.ReplaceRepository(ctx, model.Snapshot{RepoID: "repo", Root: "/repo", Git: model.GitState{Commit: revision}, ContentHash: revision, ExtractorVersions: "fixture", IndexedAt: time.Now()}, []model.File{f}, nil, nil); err != nil {
			t.Fatal(err)
		}
	}
	if err = db.Retain(ctx, 1); err != nil {
		t.Fatal(err)
	}
	var n int
	if err = db.DB().QueryRow(`SELECT count(*) FROM generations`).Scan(&n); err != nil || n != 2 {
		t.Fatalf("generations=%d err=%v", n, err)
	}
	if active, err := db.ActiveGeneration(ctx, "repo"); err != nil || active.ContentHash != "three" {
		t.Fatalf("active=%#v err=%v", active, err)
	}
}

func TestResetDerivedDataRemovesOnlyOwnedDatabaseFiles(t *testing.T) {
	dir := t.TempDir()
	db, err := OpenWriter(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err = db.Close(); err != nil {
		t.Fatal(err)
	}
	keep := filepath.Join(dir, "operator-note.txt")
	if err = os.WriteFile(keep, []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err = ResetDerivedData(dir); err != nil {
		t.Fatal(err)
	}
	if _, err = os.Stat(filepath.Join(dir, DatabaseName)); !os.IsNotExist(err) {
		t.Fatalf("database survives reset: %v", err)
	}
	if b, err := os.ReadFile(keep); err != nil || string(b) != "keep" {
		t.Fatalf("unrelated file changed: %q %v", b, err)
	}
}
