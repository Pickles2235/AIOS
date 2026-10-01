package webui

import (
	"context"
	"encoding/json"
	"github.com/AdamNi-7080/AIOS/internal/catalog"
	"github.com/AdamNi-7080/AIOS/internal/knowledge"
	"github.com/AdamNi-7080/AIOS/internal/mirror"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestAuthenticatedMirrorOnboardingRetryAndRestart(t *testing.T) {
	s := server(t)
	s.session = "session"
	s.csrf = "csrf"
	defer s.listener.Close()
	t.Cleanup(func() {
		_ = filepath.WalkDir(s.dataDir, func(path string, d fs.DirEntry, err error) error {
			if err == nil && d.IsDir() {
				return os.Chmod(path, 0700)
			}
			return err
		})
	})
	source := t.TempDir()
	git := func(args ...string) string {
		t.Helper()
		out, err := exec.Command("git", append([]string{"-C", source}, args...)...).CombinedOutput()
		if err != nil {
			t.Fatalf("git: %v %s", err, out)
		}
		return string(out)
	}
	git("init", "-q", "--initial-branch=main")
	os.WriteFile(filepath.Join(source, "Publish.java"), []byte("class Publish {}"), 0600)
	git("add", ".")
	git("-c", "user.name=fixture", "-c", "user.email=fixture@example.test", "commit", "-qm", "one")
	before := git("status", "--porcelain=v1")
	request := func(path string, body any, csrf bool) *httptest.ResponseRecorder {
		t.Helper()
		b, _ := json.Marshal(body)
		r := httptest.NewRequest(http.MethodPost, path, strings.NewReader(string(b)))
		r.Host = s.listener.Addr().String()
		r.Header.Set("Origin", s.origin)
		r.AddCookie(&http.Cookie{Name: "aios_kb_session", Value: s.session})
		if csrf {
			r.Header.Set("X-CSRF-Token", s.csrf)
		}
		w := httptest.NewRecorder()
		s.api(w, r)
		return w
	}
	input := map[string]any{"repositories": []mirror.Repository{{ID: "repo", URL: source, Ref: "refs/heads/main"}}}
	if w := request("/api/v1/onboarding/configure", input, false); w.Code != 403 {
		t.Fatal("missing CSRF accepted")
	}
	if w := request("/api/v1/onboarding/configure", map[string]any{"repositories": []mirror.Repository{{ID: "repo", URL: "ext::touch /tmp/unsafe", Ref: "refs/heads/main"}}}, true); w.Code != 400 {
		t.Fatal("remote command accepted")
	}
	if w := request("/api/v1/onboarding/configure", input, true); w.Code != 200 {
		t.Fatalf("configure: %d %s", w.Code, w.Body)
	}
	if w := request("/api/v1/onboarding/start", struct{}{}, true); w.Code != 202 {
		t.Fatalf("start: %d %s", w.Code, w.Body)
	}
	deadline := time.Now().Add(30 * time.Second)
	for {
		s.mu.Lock()
		state := s.setup.State
		diagnostic := s.setup.Error
		s.mu.Unlock()
		if state == "ready" {
			break
		}
		if state == "failed" || time.Now().After(deadline) {
			t.Fatalf("job %s: %s", state, diagnostic)
		}
		time.Sleep(10 * time.Millisecond)
	}
	status, err := s.readService().Status(context.Background())
	if err != nil || len(status.Repositories) != 1 || !status.Repositories[0].Active {
		t.Fatalf("active status: %#v %v", status, err)
	}
	w := request("/api/v1/query", map[string]any{"text": "Publish", "repository": "repo"}, true)
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"status":"found"`) || !strings.Contains(w.Body.String(), `"evidence":"kb.v1.`) {
		t.Fatalf("query: %d %s", w.Code, w.Body)
	}
	var result knowledge.QueryResult
	if err = json.Unmarshal(w.Body.Bytes(), &result); err != nil || len(result.Entities) == 0 {
		t.Fatalf("query JSON: %v", err)
	}
	ew := request("/api/v1/evidence", map[string]any{"evidence": result.Entities[0].Evidence, "before": 2, "after": 2, "max_lines": 40}, true)
	if ew.Code != 200 || !strings.Contains(ew.Body.String(), "class Publish {}") {
		t.Fatalf("evidence: %d %s", ew.Code, ew.Body)
	}
	if git("status", "--porcelain=v1") != before {
		t.Fatal("source mutated")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	cfg, reg, err := setupConfig([]mirror.Repository{{ID: "repo", URL: source, Ref: "refs/heads/main"}})
	if err != nil {
		t.Fatal(err)
	}
	s.runSetup(ctx, cfg, reg)
	after, err := s.readService().Status(context.Background())
	if err != nil || after.ActiveCatalog != status.ActiveCatalog {
		t.Fatal("interrupted setup changed active generation")
	}
	s.mu.Lock()
	s.setup.State = "ingesting"
	if err = s.persistSetup(); err != nil {
		t.Fatal(err)
	}
	s.mu.Unlock()
	restarted, err := New(catalog.Config{}, s.db, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer restarted.listener.Close()
	if restarted.setup.State != "interrupted" {
		t.Fatal("unfinished job not interrupted on restart")
	}
	after, err = restarted.readService().Status(context.Background())
	if err != nil || after.ActiveCatalog != status.ActiveCatalog || !after.Repositories[0].Active {
		t.Fatal("restart lost active evidence")
	}
}
