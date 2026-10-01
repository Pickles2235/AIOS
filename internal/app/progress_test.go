package app

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/AdamNi-7080/AIOS/internal/adapter"
	"github.com/AdamNi-7080/AIOS/internal/catalog"
	"github.com/AdamNi-7080/AIOS/internal/knowledge"
	"github.com/AdamNi-7080/AIOS/internal/model"
	"github.com/AdamNi-7080/AIOS/internal/store"
)

func TestStagedProgressCannotAnswerAndInterruptedBatchRetainsLastGood(t *testing.T) {
	data := canonicalTempDir(t)
	t.Cleanup(func() {
		_ = filepath.WalkDir(data, func(p string, d os.DirEntry, e error) error {
			if e == nil && d.IsDir() {
				return os.Chmod(p, 0700)
			}
			return e
		})
	})
	sourceA, sourceB := canonicalTempDir(t), canonicalTempDir(t)
	git := func(root string, args ...string) {
		t.Helper()
		b, e := exec.Command("git", append([]string{"-C", root}, args...)...).CombinedOutput()
		if e != nil {
			t.Fatalf("fixture git: %v %s", e, b)
		}
	}
	for _, source := range []string{sourceA, sourceB} {
		git(source, "init", "-q", "--initial-branch=main")
		if e := os.WriteFile(filepath.Join(source, "Worker.java"), []byte("class OriginalWorker {}\n"), 0600); e != nil {
			t.Fatal(e)
		}
		git(source, "add", ".")
		git(source, "-c", "user.name=fixture", "-c", "user.email=fixture@example.test", "commit", "-qm", "first")
	}
	cfg := catalog.Config{Version: 1, Limits: catalog.Defaults(), Sources: []catalog.Source{{Kind: model.SourceKindRepository, ID: "first"}, {Kind: model.SourceKindRepository, ID: "second"}}}
	reg := adapter.LocalRegistry{Version: 1, Repositories: []adapter.LocalRepository{{ID: "first", Path: sourceA}, {ID: "second", Path: sourceB}}}
	if _, e := IngestLocal(context.Background(), cfg, reg, data, ""); e != nil {
		t.Fatal(e)
	}
	db, e := store.OpenReadOnly(data)
	if e != nil {
		t.Fatal(e)
	}
	defer db.Close()
	reader := knowledge.New(cfg, db)
	before, e := reader.Status(context.Background())
	if e != nil {
		t.Fatal(e)
	}
	if e = os.WriteFile(filepath.Join(sourceA, "Worker.java"), []byte("class FutureWorker {}\n"), 0600); e != nil {
		t.Fatal(e)
	}
	git(sourceA, "add", ".")
	git(sourceA, "-c", "user.name=fixture", "-c", "user.email=fixture@example.test", "commit", "-qm", "future")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	staged, activated := false, false
	ctx = WithProgress(ctx, func(p Progress) {
		if p.Stage == "activated" {
			activated = true
		}
		if p.Stage == "staged" {
			staged = true
			status, e := reader.Status(context.Background())
			if e != nil || status.ActiveCatalog != before.ActiveCatalog {
				t.Fatal("staged generation replaced active knowledge")
			}
			answer, e := reader.Query(context.Background(), knowledge.Query{Text: "FutureWorker", Repository: "first"})
			if e != nil {
				t.Fatal(e)
			}
			for _, entity := range answer.Entities {
				if entity.Label == "FutureWorker" {
					t.Fatal("staged evidence answered a query")
				}
			}
			cancel()
		}
	})
	if _, e = IngestLocal(ctx, cfg, reg, data, ""); e == nil {
		t.Fatal("interrupted batch passed")
	}
	if !staged || activated {
		t.Fatalf("completed-boundary activity dishonest: staged=%v activated=%v error=%v", staged, activated, e)
	}
	after, e := reader.Status(context.Background())
	if e != nil || after.ActiveCatalog != before.ActiveCatalog {
		t.Fatal("partial batch erased last good knowledge")
	}
	answer, e := reader.Query(context.Background(), knowledge.Query{Text: "OriginalWorker", Repository: "first"})
	if e != nil || answer.Status != "found" {
		t.Fatal("last good evidence unavailable")
	}
	if _, e = IngestLocal(context.Background(), cfg, reg, data, ""); e != nil {
		t.Fatal("interrupted batch could not resume", e)
	}
	answer, e = reader.Query(context.Background(), knowledge.Query{Text: "FutureWorker", Repository: "first"})
	if e != nil || answer.Status != "found" {
		t.Fatal("resumed batch did not activate changed evidence", e)
	}
	// The same committed revision with narrower approved scope must rebuild,
	// rather than treating revision equality as proof the catalog is unchanged.
	cfg.Sources[0].Exclude = []string{"Worker.java"}
	if _, e = IngestLocal(context.Background(), cfg, reg, data, ""); e != nil {
		t.Fatal("same-revision scope rebuild failed", e)
	}
	answer, e = reader.Query(context.Background(), knowledge.Query{Text: "FutureWorker", Repository: "first"})
	if e != nil {
		t.Fatal(e)
	}
	for _, entity := range answer.Entities {
		if entity.Label == "FutureWorker" {
			t.Fatal("scope change retained excluded evidence")
		}
	}
	for _, source := range []string{sourceA, sourceB} {
		b, e := exec.Command("git", "-C", source, "status", "--porcelain=v1").Output()
		if e != nil || len(b) != 0 {
			t.Fatal("interruption modified source")
		}
	}
}
