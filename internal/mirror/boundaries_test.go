package mirror

import (
	"context"
	"net/url"
	"os"
	"path/filepath"
	"testing"
)

func TestLocalMirrorOwnsIndependentObjectFiles(t *testing.T) {
	source, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	run(t, source, "init", "-q", "--initial-branch=main")
	if err = os.WriteFile(filepath.Join(source, "code.txt"), []byte("source object"), 0600); err != nil {
		t.Fatal(err)
	}
	run(t, source, "add", ".")
	run(t, source, "-c", "user.name=fixture", "-c", "user.email=fixture@example.test", "commit", "-qm", "source")
	var objects []string
	err = filepath.WalkDir(filepath.Join(source, ".git", "objects"), func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() {
			objects = append(objects, path)
		}
		return nil
	})
	if err != nil || len(objects) == 0 {
		t.Fatal("missing real Git source objects", err)
	}
	data := t.TempDir()
	reg := Registry{Version: 1, Repositories: []Repository{{ID: "source", URL: source, Ref: "refs/heads/main"}}}
	if _, err = Sync(context.Background(), reg, data); err != nil {
		t.Fatal(err)
	}
	for _, path := range objects {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		rel, err := filepath.Rel(filepath.Join(source, ".git"), path)
		if err != nil {
			t.Fatal(err)
		}
		owned, err := os.Stat(filepath.Join(MirrorPath(data, "source"), rel))
		if err != nil {
			t.Fatal(err)
		}
		if os.SameFile(info, owned) {
			t.Fatal("owned mirror shares a writable source object inode")
		}
	}
}

func TestLocalRemoteCannotOverlapOwnedDataOrAlias(t *testing.T) {
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	data := filepath.Join(root, "data")
	inside := filepath.Join(data, "mirrors", "one.git")
	if err = os.MkdirAll(inside, 0700); err != nil {
		t.Fatal(err)
	}
	alias := filepath.Join(root, "alias")
	if err = os.Symlink(inside, alias); err != nil {
		t.Fatal(err)
	}
	for _, remote := range []string{inside, alias, filepath.Join(data, "missing.git"), (&url.URL{Scheme: "file", Path: inside}).String(), root} {
		reg := Registry{Version: 1, Repositories: []Repository{{ID: "survivor", URL: remote, Ref: "refs/heads/main"}}}
		if err = ValidateSourceBoundaries(reg, data); err == nil {
			t.Fatal("owned source overlap accepted", remote)
		}
		if _, err = Sync(context.Background(), reg, data); err == nil {
			t.Fatal("actual mirror sync accepted owned source overlap")
		}
		if _, err = os.Stat(MirrorPath(data, "survivor")); !os.IsNotExist(err) {
			t.Fatal("boundary failure still mutated owned mirrors")
		}
	}
	outside := filepath.Join(root, "unavailable", "remote.git")
	if err = ValidateSourceBoundaries(Registry{Version: 1, Repositories: []Repository{{ID: "source", URL: outside, Ref: "refs/heads/main"}}}, data); err != nil {
		t.Fatal("outside missing source cannot retry", err)
	}
}
