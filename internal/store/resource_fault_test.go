package store

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/AdamNi-7080/AIOS/internal/model"
)

func TestFullDiskAtStageAndActivationPreservesCanonicalAndSource(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "source.txt")
	if err := os.WriteFile(source, []byte("immutable source"), 0600); err != nil {
		t.Fatal(err)
	}
	db, err := OpenWriter(filepath.Join(root, "owned"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	old := canonicalFixture(t, db, "old")
	ctx := context.Background()
	stage := func() (Generation, error) {
		file := model.File{RepoID: "repo", Path: "source.txt", SHA256: "new", Size: 3, Content: "new", Language: "documentation"}
		return db.StageGeneration(ctx, model.Snapshot{RepoID: "repo", Root: source, Git: model.GitState{Commit: "new"}, ContentHash: "new", FileCount: 1, TotalBytes: 3, IndexedAt: time.Now(), ExtractorVersions: "fixture"}, []model.File{file}, nil, nil)
	}
	injected := &os.PathError{Op: "write", Path: "/private/secret", Err: syscall.ENOSPC}
	db.stageFault = func() error { return injected }
	if _, err = stage(); !errors.Is(err, syscall.ENOSPC) {
		t.Fatalf("stage ENOSPC not surfaced: %v", err)
	}
	db.stageFault = nil
	var staged int
	if err = db.DB().QueryRow(`SELECT count(*) FROM generation_staging WHERE repo_id='repo'`).Scan(&staged); err != nil || staged != 0 {
		t.Fatal("failed stage was visible")
	}
	newer, err := stage()
	if err != nil {
		t.Fatal(err)
	}
	db.activationFault = func() error { return injected }
	if err = db.ActivateGeneration(ctx, newer.ID); !errors.Is(err, syscall.ENOSPC) {
		t.Fatalf("activation ENOSPC not surfaced: %v", err)
	}
	db.activationFault = nil
	active, err := db.ActiveGeneration(ctx, "repo")
	if err != nil || active.ID != old.ID {
		t.Fatal("full disk displaced last-good generation")
	}
	cancelCtx, cancel := context.WithCancel(context.Background())
	db.activationFault = func() error { cancel(); return nil }
	if err = db.ActivateGeneration(cancelCtx, newer.ID); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled precommit activation succeeded: %v", err)
	}
	db.activationFault = nil
	active, err = db.ActiveGeneration(ctx, "repo")
	if err != nil || active.ID != old.ID {
		t.Fatal("cancelled activation displaced last-good generation")
	}
	got, err := os.ReadFile(source)
	if err != nil || string(got) != "immutable source" {
		t.Fatal("full disk mutated source")
	}
}
