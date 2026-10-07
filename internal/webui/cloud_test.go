package webui

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSourceActionsRejectUnsafeAndChangedLocalFiles(t *testing.T) {
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(root, "src"), 0700); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(root, "src", "a.go")
	content := []byte("package src\n")
	if err := os.WriteFile(target, content, 0600); err != nil {
		t.Fatal(err)
	}
	h := sha256.Sum256(content)
	digest := hex.EncodeToString(h[:])
	if got, ok := matchingLocalFile(root, "src/a.go", digest); !ok || got != target {
		t.Fatalf("matching file=%q %v", got, ok)
	}
	if _, ok := matchingLocalFile(root, "../escape", digest); ok {
		t.Fatal("accepted traversal")
	}
	if _, ok := matchingLocalFile(root, "src/a.go", "wrong"); ok {
		t.Fatal("accepted changed hash")
	}
	if err := os.Symlink(target, filepath.Join(root, "src", "link.go")); err != nil {
		t.Fatal(err)
	}
	if _, ok := matchingLocalFile(root, "src/link.go", digest); ok {
		t.Fatal("accepted target symlink")
	}
	other := t.TempDir()
	if err := os.Symlink(other, filepath.Join(root, "alias")); err != nil {
		t.Fatal(err)
	}
	if _, ok := matchingLocalFile(root, "alias/a.go", digest); ok {
		t.Fatal("accepted ancestor symlink")
	}
	if err := os.WriteFile(target, []byte("changed"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, ok := matchingLocalFile(root, "src/a.go", digest); ok {
		t.Fatal("accepted changed source")
	}
}

func TestTrustedRemoteLinksAreCommitPinnedAndEscaped(t *testing.T) {
	commit := strings.Repeat("a", 40)
	got, ok := trustedRemoteLink("https://github.com/team/repo.git", commit, "src/a b.go")
	if !ok || got != "https://github.com/team/repo/blob/"+commit+"/src/a%20b.go" {
		t.Fatalf("remote %q %v", got, ok)
	}
	for _, raw := range []string{"https://github.com.evil.test/team/repo", "https://user:pass@github.com/team/repo", "file:///tmp/repo", "https://github.com/team/repo?token=secret"} {
		if _, ok := trustedRemoteLink(raw, commit, "src/a.go"); ok {
			t.Fatalf("accepted unsafe remote %q", raw)
		}
	}
	if _, ok := trustedRemoteLink("https://gitlab.com/team/repo", "branch", "src/a.go"); ok {
		t.Fatal("accepted mutable revision")
	}
}
