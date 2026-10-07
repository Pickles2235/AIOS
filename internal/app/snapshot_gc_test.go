package app

import (
	"context"
	"errors"
	"fmt"
	"github.com/AdamNi-7080/AIOS/internal/adapter"
	"github.com/AdamNi-7080/AIOS/internal/catalog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/AdamNi-7080/AIOS/internal/model"
	"github.com/AdamNi-7080/AIOS/internal/resourcepolicy"
	"github.com/AdamNi-7080/AIOS/internal/snapshotlease"
	"github.com/AdamNi-7080/AIOS/internal/store"
)

func TestReclaimPrunedSnapshotsPreservesRetainedAndInUse(t *testing.T) {
	ctx := context.Background()
	data := t.TempDir()
	source := filepath.Join(t.TempDir(), "source.txt")
	if err := os.WriteFile(source, []byte("source remains"), 0600); err != nil {
		t.Fatal(err)
	}
	db, err := store.OpenWriter(data)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	roots := make([]string, 0, 5)
	for i, revision := range []string{"one", "two", "three", "four", "five"} {
		root := filepath.Join(data, "snapshots", "repo", revision, "manifest")
		if err := os.MkdirAll(root, 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, "a.go"), []byte(strings.Repeat("x", 1000+i)), 0400); err != nil {
			t.Fatal(err)
		}
		roots = append(roots, root)
		snap := model.Snapshot{RepoID: "repo", Root: root, Git: model.GitState{Commit: revision}, ContentHash: revision, ExtractorVersions: "fixture", IndexedAt: time.Now()}
		f := model.File{RepoID: "repo", Path: "a.go", SHA256: revision, Size: 1, Language: "go", Content: "x"}
		if err := db.ReplaceRepository(ctx, snap, []model.File{f}, nil, nil); err != nil {
			t.Fatal(err)
		}
	}
	pruned, err := db.RetainPrunedRoots(ctx, 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(pruned) != 2 {
		t.Fatalf("pruned=%v", pruned)
	}
	lease, err := snapshotlease.Acquire(data, roots[0])
	if err != nil {
		t.Fatal(err)
	}
	if err := reclaimPrunedSnapshots(ctx, db, data, pruned); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(roots[0]); err != nil {
		t.Fatalf("in-use root removed: %v", err)
	}
	if _, err := os.Stat(roots[1]); err != nil {
		t.Fatalf("per-repository lease did not protect sibling capture: %v", err)
	}
	if err := lease.Close(); err != nil {
		t.Fatal(err)
	}
	if err := reclaimPrunedSnapshots(ctx, db, data, pruned); err != nil {
		t.Fatal(err)
	}
	for _, old := range roots[:2] {
		if _, err := os.Stat(old); !os.IsNotExist(err) {
			t.Fatalf("released root survived: %s %v", old, err)
		}
	}
	if _, err := os.Stat(filepath.Join(data, "snapshots", "repo", "one")); !os.IsNotExist(err) {
		t.Fatalf("empty revision parent survived: %v", err)
	}
	entries, err := os.ReadDir(filepath.Join(data, "snapshots", ".leases"))
	if err != nil || len(entries) != 1 {
		t.Fatalf("lease metadata=%d err=%v", len(entries), err)
	}
	for _, root := range roots[2:] {
		if _, err := os.Stat(root); err != nil {
			t.Fatalf("retained root lost: %s %v", root, err)
		}
	}
	if b, err := os.ReadFile(source); err != nil || string(b) != "source remains" {
		t.Fatalf("external source changed: %q %v", b, err)
	}
	if active, err := db.ActiveGeneration(ctx, "repo"); err != nil || active.ContentHash != "five" {
		t.Fatalf("active=%#v err=%v", active, err)
	}
}

func TestReclaimPrunedSnapshotsRejectsSymlinkAncestors(t *testing.T) {
	ctx := context.Background()
	data := t.TempDir()
	outside := t.TempDir()
	victim := filepath.Join(outside, "victim")
	if err := os.MkdirAll(victim, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(victim, "source"), []byte("keep"), 0600); err != nil {
		t.Fatal(err)
	}
	db, err := store.OpenWriter(data)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	base := filepath.Join(data, "snapshots")
	if err := os.Symlink(outside, base); err != nil {
		t.Fatal(err)
	}
	if err := reclaimPrunedSnapshots(ctx, db, data, []string{filepath.Join(base, "repo", "rev", "manifest")}); err == nil {
		t.Fatal("accepted symlink snapshot base")
	}
	if err := os.Remove(base); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(base, "repo"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(victim, filepath.Join(base, "repo", "rev")); err != nil {
		t.Fatal(err)
	}
	if err := reclaimPrunedSnapshots(ctx, db, data, []string{filepath.Join(base, "repo", "rev", "manifest")}); err != nil {
		t.Fatal(err)
	}
	if b, err := os.ReadFile(filepath.Join(victim, "source")); err != nil || string(b) != "keep" {
		t.Fatalf("external target changed: %q %v", b, err)
	}
}

func TestReclaimPrunedSnapshotsAcknowledgesAlreadyRemovedRoot(t *testing.T) {
	ctx := context.Background()
	data := t.TempDir()
	db, err := store.OpenWriter(data)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	root := filepath.Join(data, "snapshots", "repo", "rev", "manifest")
	if _, err := db.RetainPrunedRoots(ctx, 2); err != nil {
		t.Fatal(err)
	}
	if _, err := db.DB().Exec(`INSERT INTO snapshot_gc_candidates(root) VALUES(?)`, root); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(root, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(root); err != nil {
		t.Fatal(err)
	}
	if err := reclaimPrunedSnapshots(ctx, db, data, []string{root}); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := db.DB().QueryRow(`SELECT count(*) FROM snapshot_gc_candidates WHERE root=?`, root).Scan(&count); err != nil || count != 0 {
		t.Fatalf("candidate not acknowledged: %d %v", count, err)
	}
	// A stale durable candidate can be retried after a crash with no filesystem write.
	if err := reclaimPrunedSnapshots(ctx, db, data, []string{root}); err != nil {
		t.Fatal(err)
	}
}

func TestLocalActivationReclaimsOldOwnedSnapshots(t *testing.T) {
	ctx := context.Background()
	source, data := canonicalTempDir(t), canonicalTempDir(t)
	t.Cleanup(func() {
		_ = filepath.Walk(data, func(path string, info os.FileInfo, e error) error {
			if e == nil && info.IsDir() {
				return os.Chmod(path, 0700)
			}
			return e
		})
	})
	git := func(args ...string) {
		t.Helper()
		b, e := exec.Command("git", append([]string{"-C", source}, args...)...).CombinedOutput()
		if e != nil {
			t.Fatalf("git: %v %s", e, b)
		}
	}
	git("init", "-q", "--initial-branch=main")
	file := filepath.Join(source, "State.java")
	if err := os.WriteFile(file, []byte("class Worker0 {}"), 0640); err != nil {
		t.Fatal(err)
	}
	git("add", ".")
	git("-c", "user.name=t", "-c", "user.email=t@e", "commit", "-qm", "one")
	cfg := catalog.Config{Version: 1, Limits: catalog.Defaults(), Sources: []catalog.Source{{Kind: model.SourceKindRepository, ID: "repo"}}}
	reg := adapter.LocalRegistry{Version: 1, Repositories: []adapter.LocalRepository{{ID: "repo", Path: source}}}
	roots := []string{}
	for i := 0; i < 6; i++ {
		if err := os.WriteFile(file, []byte(fmt.Sprintf("class Worker%d {}", i)), 0640); err != nil {
			t.Fatal(err)
		}
		out, err := IngestLocal(ctx, cfg, reg, data, "")
		if err != nil {
			t.Fatal(err)
		}
		roots = append(roots, out.Snapshots[0].Root)
		if i >= 3 {
			if _, err := os.Stat(roots[i-3]); !os.IsNotExist(err) {
				t.Fatalf("old snapshot survives activation %d: %v", i, err)
			}
		}
	}
	for _, root := range roots[3:] {
		if _, err := os.Stat(root); err != nil {
			t.Fatalf("retained snapshot removed: %v", err)
		}
	}
	if b, err := os.ReadFile(file); err != nil || string(b) != "class Worker5 {}" {
		t.Fatalf("source changed: %q %v", b, err)
	}
	entries, err := os.ReadDir(filepath.Join(data, "snapshots", ".leases"))
	if err != nil || len(entries) != 1 {
		t.Fatalf("lease metadata=%d err=%v", len(entries), err)
	}
}

func TestFailedCapturesAreReclaimedAndNextRevisionCanActivate(t *testing.T) {
	source, data := canonicalTempDir(t), canonicalTempDir(t)
	t.Cleanup(func() {
		_ = filepath.Walk(data, func(path string, info os.FileInfo, e error) error {
			if e == nil && info.IsDir() {
				return os.Chmod(path, 0700)
			}
			return e
		})
	})
	git := func(args ...string) {
		t.Helper()
		b, e := exec.Command("git", append([]string{"-C", source}, args...)...).CombinedOutput()
		if e != nil {
			t.Fatalf("git: %v %s", e, b)
		}
	}
	git("init", "-q", "--initial-branch=main")
	file := filepath.Join(source, "Worker.java")
	if err := os.WriteFile(file, []byte("class Worker0 {}"), 0640); err != nil {
		t.Fatal(err)
	}
	git("add", ".")
	git("-c", "user.name=t", "-c", "user.email=t@e", "commit", "-qm", "one")
	cfg := catalog.Config{Version: 1, Limits: catalog.Defaults(), Sources: []catalog.Source{{Kind: model.SourceKindRepository, ID: "repo"}}}
	reg := adapter.LocalRegistry{Version: 1, Repositories: []adapter.LocalRepository{{ID: "repo", Path: source}}}
	first, err := IngestLocal(context.Background(), cfg, reg, data, "")
	if err != nil {
		t.Fatal(err)
	}
	activeRoot := first.Snapshots[0].Root
	baseline, err := resourcepolicy.CheckBudget(data, 1<<30, 0, 1)
	if err != nil {
		t.Fatal(err)
	}
	for i := 1; i <= 5; i++ {
		if err := os.WriteFile(file, []byte(fmt.Sprintf("class Worker%d {}", i)), 0640); err != nil {
			t.Fatal(err)
		}
		ctx, cancel := context.WithCancel(context.Background())
		ctx = WithProgress(ctx, func(p Progress) {
			if p.Stage == "discovered" {
				cancel()
			}
		})
		if _, err := IngestLocal(ctx, cfg, reg, data, ""); err == nil {
			t.Fatal("canceled capture activated")
		}
		entries, err := os.ReadDir(filepath.Dir(activeRoot))
		if err != nil {
			t.Fatal(err)
		}
		roots := 0
		for _, entry := range entries {
			if entry.IsDir() && strings.HasPrefix(entry.Name(), "local-") {
				roots++
			}
		}
		if roots != 1 {
			t.Fatalf("failed capture %d left %d snapshot roots", i, roots)
		}
	}
	if _, err := os.Stat(activeRoot); err != nil {
		t.Fatal(err)
	}
	after, err := resourcepolicy.CheckBudget(data, baseline.OwnedBytes+5<<20, 0, 1<<20)
	if err != nil || after.StorageState != "available" {
		t.Fatalf("failed captures exhausted admission: %+v %v", after, err)
	}
	final, err := IngestLocal(context.Background(), cfg, reg, data, "")
	if err != nil {
		t.Fatalf("retry failed: %v", err)
	}
	if final.Snapshots[0].Root == activeRoot {
		t.Fatal("new revision not activated")
	}
	if b, err := os.ReadFile(file); err != nil || string(b) != "class Worker5 {}" {
		t.Fatalf("source changed: %q %v", b, err)
	}
}

func TestFailedCaptureCandidateSurvivesRestart(t *testing.T) {
	ctx := context.Background()
	data := t.TempDir()
	root := filepath.Join(data, "snapshots", "repo", "rev", "manifest")
	if err := os.MkdirAll(root, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "orphan"), []byte("discard"), 0400); err != nil {
		t.Fatal(err)
	}
	db, err := store.OpenWriter(data)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.QueueSnapshotGC(ctx, root); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	db, err = store.OpenWriter(data)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	pending, err := db.SnapshotGCCandidates(ctx)
	if err != nil || len(pending) != 1 || pending[0] != root {
		t.Fatalf("restart candidates=%v err=%v", pending, err)
	}
	if err := reclaimPrunedSnapshots(ctx, db, data, pending); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(root); !os.IsNotExist(err) {
		t.Fatalf("orphan survived restart: %v", err)
	}
	pending, err = db.SnapshotGCCandidates(ctx)
	if err != nil || len(pending) != 0 {
		t.Fatalf("candidate not acknowledged: %v %v", pending, err)
	}
}

func TestRestartOverBudgetPreflightReclaimsDurableOrphan(t *testing.T) {
	ctx := context.Background()
	data := t.TempDir()
	root := filepath.Join(data, "snapshots", "repo", "rev", "manifest")
	db, err := store.OpenWriter(data)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.QueueSnapshotGC(ctx, root); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(root); !os.IsNotExist(err) {
		t.Fatalf("root published before durable registration: %v", err)
	}
	if err := os.MkdirAll(root, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "orphan.bin"), make([]byte, 2<<20), 0400); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	observed, err := resourcepolicy.CheckBudget(data, 1<<30, 0, 1)
	if err != nil {
		t.Fatal(err)
	}
	cap := observed.OwnedBytes + (1 << 20) - 1
	if status, err := resourcepolicy.CheckBudget(data, cap, 0, 1<<20); !errors.Is(err, resourcepolicy.ErrStorageBudget) || status.StorageState != "budget" {
		t.Fatalf("expected admission block: %+v %v", status, err)
	}
	if err := ReclaimPendingSnapshots(ctx, data); err != nil {
		t.Fatal(err)
	}
	if status, err := resourcepolicy.CheckBudget(data, cap, 0, 1<<20); err != nil || status.StorageState != "available" {
		t.Fatalf("admission did not recover: %+v %v", status, err)
	}
	if _, err := os.Stat(root); !os.IsNotExist(err) {
		t.Fatalf("orphan root survived preflight: %v", err)
	}
}
