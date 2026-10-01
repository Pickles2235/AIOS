package adapter

import (
	"context"
	"github.com/AdamNi-7080/AIOS/internal/catalog"
	"github.com/AdamNi-7080/AIOS/internal/model"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func workingFixture(t *testing.T) (LocalRepository, string) {
	t.Helper()
	source := canonicalTempDir(t)
	data := canonicalTempDir(t)
	run(t, source, "init", "-q", "--initial-branch=main")
	if err := os.WriteFile(filepath.Join(source, "a.txt"), []byte("committed"), 0640); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, ".gitignore"), []byte("ignored/\n"), 0600); err != nil {
		t.Fatal(err)
	}
	run(t, source, "add", ".")
	run(t, source, "-c", "user.name=t", "-c", "user.email=t@e", "commit", "-qm", "one")
	t.Cleanup(func() {
		_ = filepath.Walk(data, func(path string, info os.FileInfo, err error) error {
			if err == nil && info.IsDir() {
				return os.Chmod(path, 0700)
			}
			return err
		})
	})
	return LocalRepository{ID: "repo", Path: source}, data
}

func TestWorkingCaptureHonorsRepositoryAndMachineIgnoresAndDeletion(t *testing.T) {
	entry, data := workingFixture(t)
	global := filepath.Join(canonicalTempDir(t), "gitconfig")
	ignore := filepath.Join(filepath.Dir(global), "ignore")
	if err := os.WriteFile(ignore, []byte("machine-ignored.txt\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(global, []byte("[core]\nexcludesFile = "+ignore+"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GIT_CONFIG_GLOBAL", global)
	if err := os.Mkdir(filepath.Join(entry.Path, "ignored"), 0700); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"ignored/hidden.txt", "machine-ignored.txt", "eligible.txt"} {
		if err := os.WriteFile(filepath.Join(entry.Path, path), []byte("current"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Remove(filepath.Join(entry.Path, "a.txt")); err != nil {
		t.Fatal(err)
	}
	indexBefore, err := os.ReadFile(filepath.Join(entry.Path, ".git/index"))
	if err != nil {
		t.Fatal(err)
	}
	discovery, err := CaptureLocal(context.Background(), entry, data, catalog.Defaults())
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"a.txt", "ignored/hidden.txt", "machine-ignored.txt"} {
		if _, err := os.Stat(filepath.Join(discovery.Root, path)); !os.IsNotExist(err) {
			t.Fatalf("deleted/ignored path captured: %s", path)
		}
	}
	if body, err := os.ReadFile(filepath.Join(discovery.Root, "eligible.txt")); err != nil || string(body) != "current" {
		t.Fatal("eligible current bytes missing")
	}
	if !discovery.Git.Dirty || discovery.Git.UntrackedCount != 1 {
		t.Fatalf("dishonest working provenance: %+v", discovery.Git)
	}
	indexAfter, err := os.ReadFile(filepath.Join(entry.Path, ".git/index"))
	if err != nil || string(indexBefore) != string(indexAfter) {
		t.Fatal("capture changed Git index")
	}
}

func TestWorkingCaptureRejectsABAEditWithRestoredBytesAndMtime(t *testing.T) {
	entry, data := workingFixture(t)
	path := filepath.Join(entry.Path, "a.txt")
	if err := os.WriteFile(path, []byte("dirty baseline"), 0640); err != nil {
		t.Fatal(err)
	}
	before, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if changeStamp(before) == "" {
		t.Fatal("supported platform lacks change metadata")
	}
	ctx := context.WithValue(context.Background(), captureCheckpointKey{}, func() {
		time.Sleep(time.Millisecond)
		if err := os.WriteFile(path, []byte("transient edit"), 0640); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("dirty baseline"), 0640); err != nil {
			t.Fatal(err)
		}
		if err := os.Chtimes(path, before.ModTime(), before.ModTime()); err != nil {
			t.Fatal(err)
		}
	})
	discovery, err := CaptureLocal(ctx, entry, data, catalog.Defaults())
	if err == nil || discovery.Root != "" {
		t.Fatal("published across concurrent ABA edit")
	}
	if body, err := os.ReadFile(path); err != nil || string(body) != "dirty baseline" {
		t.Fatal("capture changed source")
	}
	paths, err := filepath.Glob(filepath.Join(data, "snapshots", "repo", "*", "local-*"))
	if err != nil || len(paths) != 0 {
		t.Fatal("failed capture retained published or staging bytes")
	}
}

func TestWorkingCaptureExcludesBeforeOpeningOrApplyingEligibleBounds(t *testing.T) {
	entry, data := workingFixture(t)
	if err := os.WriteFile(filepath.Join(entry.Path, "excluded.dat"), make([]byte, 2<<20), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(data, "outside"), filepath.Join(entry.Path, "excluded.link")); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(entry.Path, "a.txt"), []byte("eligible edit"), 0640); err != nil {
		t.Fatal(err)
	}
	indexBefore, err := os.ReadFile(filepath.Join(entry.Path, ".git/index"))
	if err != nil {
		t.Fatal(err)
	}
	scope := model.Repository{ID: entry.ID, Exclude: []string{"excluded.*"}}
	d, err := CaptureLocalScoped(context.Background(), entry, data, catalog.Defaults(), scope)
	if err != nil {
		t.Fatal("excluded input poisoned eligible capture", err)
	}
	if len(d.Coverage.Entries) != 2 {
		t.Fatalf("excluded coverage missing: %+v", d.Coverage)
	}
	for _, e := range d.Coverage.Entries {
		if e.Outcome != "excluded" || e.Reason != "catalog_pattern" {
			t.Fatal("dishonest exclusion coverage")
		}
	}
	if body, err := os.ReadFile(filepath.Join(d.Root, "a.txt")); err != nil || string(body) != "eligible edit" {
		t.Fatal("eligible edit absent")
	}
	if _, err := os.Stat(filepath.Join(d.Root, "excluded.dat")); !os.IsNotExist(err) {
		t.Fatal("excluded bytes copied")
	}
	indexAfter, err := os.ReadFile(filepath.Join(entry.Path, ".git/index"))
	if err != nil || string(indexBefore) != string(indexAfter) {
		t.Fatal("source index changed")
	}
}
