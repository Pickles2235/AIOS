package mirror

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func registry(url string) Registry {
	r := Registry{Version: 1}
	for i := 0; i < 25; i++ {
		id := "repo" + string(rune('a'+i))
		r.Repositories = append(r.Repositories, Repository{ID: id, URL: url, Ref: "refs/heads/main"})
	}
	return r
}
func TestValidateAndFingerprintAreDeterministic(t *testing.T) {
	r := registry("file:///tmp/remote")
	if err := Validate(r); err != nil {
		t.Fatal(err)
	}
	if Fingerprint(r) != Fingerprint(r) {
		t.Fatal("unstable fingerprint")
	}
	r.Repositories = r.Repositories[:24]
	if err := Validate(r); err == nil {
		t.Fatal("accepted non-estate registry")
	}
}
func TestSyncWritesOnlyAgentOwnedMirror(t *testing.T) {
	source := t.TempDir()
	run(t, source, "init", "-q", "--initial-branch=main")
	if err := os.WriteFile(filepath.Join(source, "a.txt"), []byte("a"), 0600); err != nil {
		t.Fatal(err)
	}
	run(t, source, "add", ".")
	run(t, source, "-c", "user.name=t", "-c", "user.email=t@e", "commit", "-qm", "one")
	r := registry(source)
	data := t.TempDir()
	got, err := Sync(context.Background(), r, data)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 25 {
		t.Fatalf("got %d", len(got))
	}
	if _, err = os.Stat(filepath.Join(source, "a.txt")); err != nil {
		t.Fatal(err)
	}
	if _, err = os.Stat(MirrorPath(data, "repoa")); err != nil {
		t.Fatal(err)
	}
}
func TestSnapshotIsReadOnlyAndUsesMirrorRevision(t *testing.T) {
	source := t.TempDir()
	run(t, source, "init", "-q", "--initial-branch=main")
	if err := os.WriteFile(filepath.Join(source, "a.txt"), []byte("one"), 0600); err != nil {
		t.Fatal(err)
	}
	run(t, source, "add", ".")
	run(t, source, "-c", "user.name=t", "-c", "user.email=t@e", "commit", "-qm", "one")
	data := t.TempDir()
	synced, err := Sync(context.Background(), registry(source), data)
	if err != nil {
		t.Fatal(err)
	}
	path, err := Snapshot(context.Background(), synced[0].Mirror, data, "repoa", synced[0].Revision, "fingerprint")
	if err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(filepath.Join(path, "a.txt"))
	if err != nil || string(b) != "one" {
		t.Fatalf("snapshot=%q err=%v", b, err)
	}
	if err = os.WriteFile(filepath.Join(path, "a.txt"), []byte("bad"), 0600); err == nil {
		t.Fatal("snapshot was writable")
	}
	if err = os.Chmod(path, 0700); err != nil {
		t.Fatal(err)
	}
}
func TestTreeChangesUsesGitRenameAndDeletionEvidence(t *testing.T) {
	source := t.TempDir()
	run(t, source, "init", "-q", "--initial-branch=main")
	for name, body := range map[string]string{"old.txt": "same", "gone.txt": "gone", "change.txt": "old"} {
		if err := os.WriteFile(filepath.Join(source, name), []byte(body), 0600); err != nil {
			t.Fatal(err)
		}
	}
	run(t, source, "add", ".")
	run(t, source, "-c", "user.name=t", "-c", "user.email=t@e", "commit", "-qm", "one")
	data := t.TempDir()
	first, err := Sync(context.Background(), registry(source), data)
	if err != nil {
		t.Fatal(err)
	}
	run(t, source, "mv", "old.txt", "new.txt")
	if err = os.Remove(filepath.Join(source, "gone.txt")); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(source, "change.txt"), []byte("new"), 0600); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(source, "added.txt"), []byte("added"), 0600); err != nil {
		t.Fatal(err)
	}
	run(t, source, "add", "-A")
	run(t, source, "-c", "user.name=t", "-c", "user.email=t@e", "commit", "-qm", "two")
	second, err := Sync(context.Background(), registry(source), data)
	if err != nil {
		t.Fatal(err)
	}
	changes, err := TreeChanges(context.Background(), first[0].Mirror, first[0].Revision, second[0].Revision)
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	for _, c := range changes {
		seen[c.Kind] = true
	}
	for _, kind := range []string{"added", "modified", "renamed", "removed"} {
		if !seen[kind] {
			t.Fatalf("changes=%#v missing %s", changes, kind)
		}
	}
}
func run(t *testing.T, dir string, args ...string) {
	t.Helper()
	if b, e := exec.Command("git", append([]string{"-C", dir}, args...)...).CombinedOutput(); e != nil {
		t.Fatalf("git %v: %v: %s", args, e, b)
	}
}
