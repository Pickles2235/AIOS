package webui

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"github.com/AdamNi-7080/AIOS/internal/knowledge"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
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

func TestSourceActionListingHasNoNativeSideEffectsAndInvokeRequiresCSRF(t *testing.T) {
	s, _, _ := managementServer(t, "local")
	found, err := s.readService().Query(context.Background(), knowledge.Query{Text: "Publish", Repository: "one", Limit: 1})
	if err != nil || len(found.Entities) != 1 {
		t.Fatalf("canonical fixture: %+v %v", found, err)
	}
	bin := t.TempDir()
	marker := filepath.Join(bin, "dispatched")
	t.Setenv("AIOS_TEST_NATIVE_DISPATCH", marker)
	for _, name := range []string{"open", "code"} {
		if err := os.WriteFile(filepath.Join(bin, name), []byte("#!/bin/sh\n: > \"$AIOS_TEST_NATIVE_DISPATCH\"\n"), 0700); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", bin)
	handle := found.Entities[0].Handle
	if w := approvedRequest(t, s, "/api/v1/source-actions", map[string]string{"handle": handle}); w.Code != 200 {
		t.Fatalf("listing %d %s", w.Code, w.Body)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatal("listing dispatched a native application")
	}
	r := httptest.NewRequest(http.MethodPost, "/api/v1/source-actions/open", strings.NewReader(`{"handle":"`+handle+`","kind":"finder"}`))
	r.Host = s.listener.Addr().String()
	r.Header.Set("Origin", s.origin)
	r.AddCookie(&http.Cookie{Name: "aios_session", Value: s.session})
	w := httptest.NewRecorder()
	s.api(w, r)
	if w.Code != 403 {
		t.Fatalf("missing CSRF status=%d", w.Code)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatal("unauthorised request dispatched native application")
	}
	if runtime.GOOS == "darwin" {
		w := approvedRequest(t, s, "/api/v1/source-actions/open", map[string]string{"handle": handle, "kind": "finder"})
		if w.Code != 200 {
			t.Fatalf("explicit invoke %d %s", w.Code, w.Body)
		}
		if _, err := os.Stat(marker); err != nil {
			t.Fatalf("authorised action not dispatched: %v", err)
		}
	}
}

func TestEditorArgumentsPreserveColonFilenames(t *testing.T) {
	target := "/owned/source/A:123:B.ts"
	args := editorArguments(target, 42)
	if len(args) != 2 || args[0] != "--" || args[1] != target {
		t.Fatalf("ambiguous filename altered: %q", args)
	}
	args = editorArguments("/owned/source/ordinary.ts", 42)
	if len(args) != 2 || args[0] != "--goto" || args[1] != "/owned/source/ordinary.ts:42" {
		t.Fatalf("ordinary source line lost: %q", args)
	}
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	content := []byte("export const value=1;\n")
	name := "A:123:B.ts"
	if err := os.WriteFile(filepath.Join(root, name), content, 0600); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(content)
	if got, ok := matchingLocalFile(root, name, hex.EncodeToString(sum[:])); !ok || got != filepath.Join(root, name) {
		t.Fatalf("valid colon source rejected: %q %v", got, ok)
	}
}
