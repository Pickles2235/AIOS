package lifecycle

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestOwnedSealedSnapshotDeletionAndForeignLinkPreflight(t *testing.T) {
	root := canonicalTemp(t)
	owned := filepath.Join(root, "install")
	snapshot := filepath.Join(owned, "data", "snapshots", "fixture", "sealed")
	if err := os.MkdirAll(snapshot, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(snapshot, "Worker.java"), []byte("class Worker {}"), 0400); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(snapshot, 0500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(snapshot, 0700) })
	outside := filepath.Join(root, "source")
	if err := os.Mkdir(outside, 0700); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(outside, "untouched")
	if err := os.WriteFile(file, []byte("source"), 0600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(owned, "foreign-link")
	if err := os.Symlink(outside, link); err != nil {
		t.Fatal(err)
	}
	if removeOwnedTree(context.Background(), owned) == nil {
		t.Fatal("linked tree accepted")
	}
	info, _ := os.Stat(snapshot)
	if info.Mode().Perm() != 0500 {
		t.Fatal("failed preflight mutated snapshot permissions")
	}
	if err := os.Remove(link); err != nil {
		t.Fatal(err)
	}
	if err := removeOwnedTree(context.Background(), owned); err != nil {
		t.Fatal("sealed owned data could not be deleted", err)
	}
	if _, err := os.Stat(owned); !os.IsNotExist(err) {
		t.Fatal("owned installation remains")
	}
	body, err := os.ReadFile(file)
	if err != nil || string(body) != "source" {
		t.Fatal("deletion affected external source")
	}
}
