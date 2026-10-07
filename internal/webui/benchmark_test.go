package webui

import (
	"context"
	"encoding/json"
	"github.com/AdamNi-7080/AIOS/internal/benchmark"
	"github.com/AdamNi-7080/AIOS/internal/catalog"
	"github.com/AdamNi-7080/AIOS/internal/store"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func labRequest(t *testing.T, s *Server, path, body string, csrf bool) *httptest.ResponseRecorder {
	t.Helper()
	method := http.MethodGet
	if body != "" {
		method = http.MethodPost
	}
	r := httptest.NewRequest(method, path, strings.NewReader(body))
	r.Host = s.listener.Addr().String()
	r.Header.Set("Origin", s.origin)
	r.AddCookie(&http.Cookie{Name: "aios_kb_session", Value: "session"})
	if csrf {
		r.Header.Set("X-CSRF-Token", "csrf")
	}
	w := httptest.NewRecorder()
	s.api(w, r)
	return w
}
func TestBenchmarkDemoIsolationPersistenceAndSecurity(t *testing.T) {
	s := server(t)
	defer s.Close()
	s.session = "session"
	s.csrf = "csrf"
	before, e := s.db.Status(context.Background(), "")
	if e != nil {
		t.Fatal(e)
	}
	w := labRequest(t, s, "/api/v1/benchmarks/run", `{"mode":"demo"}`, false)
	if w.Code != 403 {
		t.Fatal("CSRF bypass")
	}
	w = labRequest(t, s, "/api/v1/benchmarks/run", `{"mode":"demo"}`, true)
	if w.Code != 200 {
		t.Fatalf("run %d %s", w.Code, w.Body)
	}
	var report benchmark.LabReport
	if json.Unmarshal(w.Body.Bytes(), &report) != nil || len(report.Results) != 5 {
		t.Fatal(w.Body)
	}
	after, _ := s.db.Status(context.Background(), "")
	if len(before) != len(after) {
		t.Fatal("Demo changed user KB")
	}
	s.benchmarkMu.Lock()
	w = labRequest(t, s, "/api/v1/benchmarks/export", "", false)
	s.benchmarkMu.Unlock()
	if w.Code != 200 || w.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("prior report unavailable or cached")
	}
	st, e := os.Stat(filepath.Join(s.dataDir, "benchmark-latest.json"))
	if e != nil || st.Mode().Perm() != 0600 {
		t.Fatal("private persistence")
	}
	for _, entry := range mustEntries(t, s.dataDir) {
		if strings.HasPrefix(entry.Name(), ".benchmark-work-") {
			t.Fatal("temporary source retained")
		}
	}
	w = labRequest(t, s, "/api/v1/benchmarks/cases", `{"cases":[{"id":"x","query":"x","expected_state":"unknown","declared_snapshot":"zzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzz"}]}`, true)
	if w.Code != 400 {
		t.Fatal("invalid digest accepted")
	}
}
func mustEntries(t *testing.T, p string) []os.DirEntry {
	t.Helper()
	es, e := os.ReadDir(p)
	if e != nil {
		t.Fatal(e)
	}
	return es
}
func TestBenchmarkMyKnowledgeFrozenReplay(t *testing.T) {
	root := t.TempDir()
	writer, e := benchmark.BuildDemo(context.Background(), root)
	if e != nil {
		t.Fatal(e)
	}
	data := filepath.Dir(writer.Path())
	writer.Close()
	db, e := store.OpenReadOnly(data)
	if e != nil {
		t.Fatal(e)
	}
	defer db.Close()
	s, e := New(catalog.Config{}, db, nil)
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	s.session = "session"
	s.csrf = "csrf"
	cases := benchmark.DemoCases()
	b, _ := json.Marshal(map[string]any{"cases": cases})
	w := labRequest(t, s, "/api/v1/benchmarks/cases", string(b), true)
	if w.Code != 200 {
		t.Fatal(w.Body)
	}
	w = labRequest(t, s, "/api/v1/benchmarks/run", `{"mode":"my_knowledge"}`, true)
	if w.Code != 200 {
		t.Fatalf("copy/replay %d %s", w.Code, w.Body)
	}
	var first benchmark.LabReport
	json.Unmarshal(w.Body.Bytes(), &first)
	// A new server sees persisted case binding and exact prior report.
	restarted, e := New(catalog.Config{}, db, nil)
	if e != nil {
		t.Fatal(e)
	}
	defer restarted.Close()
	restarted.session = "session"
	restarted.csrf = "csrf"
	w = labRequest(t, restarted, "/api/v1/benchmarks/cases", "", false)
	var saved struct {
		Cases []benchmark.LabCase `json:"cases"`
	}
	json.Unmarshal(w.Body.Bytes(), &saved)
	if len(saved.Cases) != 5 || saved.Cases[0].DeclaredSnapshot != first.SourceSnapshotSHA256 {
		t.Fatal("case binding did not survive restart")
	}
	w = labRequest(t, restarted, "/api/v1/benchmarks/run", `{"mode":"my_knowledge"}`, true)
	if w.Code != 200 {
		t.Fatal(w.Body)
	}
	var second benchmark.LabReport
	json.Unmarshal(w.Body.Bytes(), &second)
	if second.SourceSnapshotSHA256 != first.SourceSnapshotSHA256 || second.Results[0].StaleExpectation {
		t.Fatal("frozen replay drift")
	}
}
