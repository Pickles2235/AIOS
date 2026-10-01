package adapter

import (
	"context"
	"fmt"
	"github.com/AdamNi-7080/AIOS/internal/catalog"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestLocalSnapshotsArePinnedBoundedAndNonMutating(t *testing.T) {
	source := canonicalTempDir(t)
	data := canonicalTempDir(t)
	t.Cleanup(func() {
		_ = filepath.WalkDir(data, func(path string, d fs.DirEntry, err error) error {
			if err == nil && d.IsDir() {
				return os.Chmod(path, 0700)
			}
			return err
		})
	})
	run(t, source, "init", "-q", "--initial-branch=main")
	os.WriteFile(filepath.Join(source, "a.txt"), []byte("one"), 0640)
	run(t, source, "add", ".")
	run(t, source, "-c", "user.name=t", "-c", "user.email=t@e", "commit", "-qm", "one")
	entry := LocalRepository{ID: "repo", Path: source}
	before, _ := os.Stat(filepath.Join(source, "a.txt"))
	indexBefore, _ := os.ReadFile(filepath.Join(source, ".git", "index"))
	first, err := CaptureLocal(context.Background(), entry, data, catalog.Defaults())
	if err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(filepath.Join(first.Root, "a.txt"))
	if err != nil || string(body) != "one" {
		t.Fatalf("snapshot: %q %v", body, err)
	}
	rootInfo, err := os.Stat(first.Root)
	if err != nil || rootInfo.Mode().Perm() != 0500 {
		t.Fatal("published snapshot root is not sealed")
	}
	fileInfo, err := os.Stat(filepath.Join(first.Root, "a.txt"))
	if err != nil || fileInfo.Mode().Perm() != 0400 {
		t.Fatal("published snapshot file is not sealed")
	}
	again, err := CaptureLocal(context.Background(), entry, data, catalog.Defaults())
	if err != nil || again.Root != first.Root {
		t.Fatalf("snapshot reuse: %#v %v", again, err)
	}
	os.WriteFile(filepath.Join(source, "a.txt"), []byte("changed"), before.Mode().Perm())
	dirty, err := CaptureLocal(context.Background(), entry, data, catalog.Defaults())
	if err != nil || !dirty.Git.Dirty || dirty.Revision == first.Revision || dirty.Root == first.Root {
		t.Fatalf("dirty capture/provenance: %+v %v", dirty, err)
	}
	dirtyBody, _ := os.ReadFile(filepath.Join(dirty.Root, "a.txt"))
	oldBody, _ := os.ReadFile(filepath.Join(first.Root, "a.txt"))
	if string(dirtyBody) != "changed" || string(oldBody) != "one" {
		t.Fatal("dirty capture modified old immutable input")
	}
	os.WriteFile(filepath.Join(source, "a.txt"), []byte("one"), before.Mode().Perm())
	os.WriteFile(filepath.Join(source, "untracked.txt"), []byte("private"), 0600)
	untracked, err := CaptureLocal(context.Background(), entry, data, catalog.Defaults())
	if err != nil || untracked.Git.UntrackedCount != 1 {
		t.Fatalf("untracked capture: %+v %v", untracked, err)
	}
	untrackedBody, _ := os.ReadFile(filepath.Join(untracked.Root, "untracked.txt"))
	if string(untrackedBody) != "private" {
		t.Fatal("eligible untracked file missing")
	}
	b, _ := os.ReadFile(filepath.Join(source, "untracked.txt"))
	if string(b) != "private" {
		t.Fatal("untracked content changed")
	}
	os.Remove(filepath.Join(source, "untracked.txt"))
	after, _ := os.Stat(filepath.Join(source, "a.txt"))
	indexAfter, _ := os.ReadFile(filepath.Join(source, ".git", "index"))
	if after.Mode() != before.Mode() || string(indexAfter) != string(indexBefore) {
		t.Fatal("source mode or Git index mutated")
	}
	if _, err = CaptureLocal(context.Background(), entry, filepath.Join(source, "owned-data"), catalog.Defaults()); err == nil {
		t.Fatal("overlapping data accepted")
	}
	limits := catalog.Defaults()
	limits.MaxFileBytes = 2
	if _, err = CaptureLocal(context.Background(), entry, data, limits); err == nil {
		t.Fatal("oversized snapshot accepted")
	}
	os.Symlink("a.txt", filepath.Join(source, "link"))
	run(t, source, "add", ".")
	run(t, source, "-c", "user.name=t", "-c", "user.email=t@e", "commit", "-qm", "link")
	if _, err = CaptureLocal(context.Background(), entry, data, catalog.Defaults()); err == nil {
		t.Fatal("tracked symlink accepted")
	}
}

func TestConcurrentWorkspaceEditNeverPublishesMixedSnapshot(t *testing.T) {
	source := canonicalTempDir(t)
	data := canonicalTempDir(t)
	run(t, source, "init", "-q", "--initial-branch=main")
	for i := 0; i < 200; i++ {
		if err := os.WriteFile(filepath.Join(source, fmt.Sprintf("f-%03d.txt", i)), []byte(strings.Repeat("original", 1024)), 0600); err != nil {
			t.Fatal(err)
		}
	}
	run(t, source, "add", ".")
	run(t, source, "-c", "user.name=t", "-c", "user.email=t@e", "commit", "-qm", "one")
	done := make(chan struct{})
	mutated := make(chan bool, 1)
	go func() {
		for {
			select {
			case <-done:
				mutated <- false
				return
			default:
			}
			matches, _ := filepath.Glob(filepath.Join(data, "snapshots", "repo", "*", "local-*"))
			if len(matches) > 0 {
				_ = os.WriteFile(filepath.Join(source, "f-000.txt"), []byte("concurrent edit"), 0600)
				mutated <- true
				return
			}
			time.Sleep(time.Millisecond)
		}
	}()
	discovery, err := CaptureLocal(context.Background(), LocalRepository{ID: "repo", Path: source}, data, catalog.Defaults())
	close(done)
	if !<-mutated {
		t.Fatal("test did not edit during capture")
	}
	if err == nil || discovery.Root != "" {
		t.Fatal("published a snapshot after concurrent workspace edit")
	}
	body, _ := os.ReadFile(filepath.Join(source, "f-000.txt"))
	if string(body) != "concurrent edit" {
		t.Fatal("capture reverted concurrent source edit")
	}
}

// macOS temporary directories may be reached through /var aliases. Product
// inputs require canonical paths; only these owned fixture paths are resolved.
func canonicalTempDir(t *testing.T) string {
	t.Helper()
	path, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return path
}
