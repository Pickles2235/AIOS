package app

import (
	"context"
	"github.com/AdamNi-7080/AIOS/internal/adapter"
	"github.com/AdamNi-7080/AIOS/internal/catalog"
	"github.com/AdamNi-7080/AIOS/internal/knowledge"
	"github.com/AdamNi-7080/AIOS/internal/model"
	"github.com/AdamNi-7080/AIOS/internal/store"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestLocalIngestCitesCapturedCommitAndRetainsGenerationOnFailure(t *testing.T) {
	source := t.TempDir()
	data := t.TempDir()
	t.Cleanup(func() {
		_ = filepath.WalkDir(data, func(path string, d fs.DirEntry, err error) error {
			if err == nil && d.IsDir() {
				return os.Chmod(path, 0700)
			}
			return err
		})
	})
	git := func(args ...string) string {
		t.Helper()
		b, e := exec.Command("git", append([]string{"-C", source}, args...)...).CombinedOutput()
		if e != nil {
			t.Fatalf("git: %v %s", e, b)
		}
		return string(b)
	}
	git("init", "-q", "--initial-branch=main")
	file := filepath.Join(source, "Publish.java")
	os.WriteFile(file, []byte("class Publish {}"), 0640)
	git("add", ".")
	git("-c", "user.name=t", "-c", "user.email=t@e", "commit", "-qm", "one")
	cfg := catalog.Config{Version: 1, Limits: catalog.Defaults(), Sources: []catalog.Source{{Kind: model.SourceKindRepository, ID: "repo"}}}
	reg := adapter.LocalRegistry{Version: 1, Repositories: []adapter.LocalRepository{{ID: "repo", Path: source}}}
	before, _ := os.Stat(file)
	out, err := IngestLocal(context.Background(), cfg, reg, data, "")
	if err != nil || len(out.Snapshots) != 1 {
		t.Fatalf("local ingest: %#v %v", out, err)
	}
	db, err := store.OpenReadOnly(data)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	reader := knowledge.New(cfg, db)
	status, err := reader.Status(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if out.Snapshots[0].Root == source || out.Snapshots[0].Git.Commit == "" {
		t.Fatal("compiled live source")
	}
	result, err := reader.Query(context.Background(), knowledge.Query{Text: "Publish", Repository: "repo"})
	if err != nil || result.Status != "found" || len(result.Entities) == 0 {
		t.Fatalf("query: %#v %v", result, err)
	}
	excerpt, err := reader.Excerpt(context.Background(), result.Entities[0].Evidence, 0, 0, 10)
	if err != nil || excerpt.Generation != result.Entities[0].Generation || excerpt.Lines[0] != "class Publish {}" || excerpt.GitCommit != out.Snapshots[0].Git.Commit || excerpt.SHA256 == "" {
		t.Fatalf("evidence: %#v %v", excerpt, err)
	}
	os.WriteFile(file, []byte("class Changed {}"), before.Mode().Perm())
	if _, err = IngestLocal(context.Background(), cfg, reg, data, ""); err == nil {
		t.Fatal("dirty ingest accepted")
	}
	after, err := reader.Status(context.Background())
	if err != nil || after.ActiveCatalog != status.ActiveCatalog {
		t.Fatal("failure replaced active generation")
	}
	body, _ := os.ReadFile(file)
	info, _ := os.Stat(file)
	if string(body) != "class Changed {}" || info.Mode() != before.Mode() {
		t.Fatal("dirty source mutated")
	}
	git("add", ".")
	git("-c", "user.name=t", "-c", "user.email=t@e", "commit", "-qm", "two")
	if _, err = IngestLocal(context.Background(), cfg, reg, data, "repo"); err != nil {
		t.Fatal(err)
	}
	after, err = reader.Status(context.Background())
	if err != nil || after.ActiveCatalog == status.ActiveCatalog {
		t.Fatal("committed local delta not activated")
	}
}
