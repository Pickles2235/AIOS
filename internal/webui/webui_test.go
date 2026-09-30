package webui

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/AdamNi-7080/AIOS/internal/catalog"
	"github.com/AdamNi-7080/AIOS/internal/store"
)

func server(t *testing.T) *Server {
	t.Helper()
	dir := t.TempDir()
	w, e := store.OpenWriter(dir)
	if e != nil {
		t.Fatal(e)
	}
	if e = w.Close(); e != nil {
		t.Fatal(e)
	}
	r, e := store.OpenReadOnly(dir)
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { _ = r.Close() })
	s, e := New(catalog.Config{}, r, nil)
	if e != nil {
		t.Fatal(e)
	}
	return s
}
func TestUIRequiresReadOnlyStoreAndLoopback(t *testing.T) {
	if _, e := New(catalog.Config{}, nil, nil); e == nil {
		t.Fatal("accepted nil store")
	}
}
func TestAPIRejectsWritesAndUnknownPaths(t *testing.T) {
	s := server(t)
	for _, target := range []string{"/api/v1/onboarding/index", "/api/v1/../../etc/passwd"} {
		r := httptest.NewRequest(http.MethodPost, target, nil)
		r.Header.Set("Origin", s.origin)
		w := httptest.NewRecorder()
		s.api(w, r)
		if w.Code != http.StatusNotFound {
			t.Fatalf("%s status=%d", target, w.Code)
		}
	}
}
func TestSessionIsOneUseAndCSRFProtected(t *testing.T) {
	s := server(t)
	body := `{"token":"` + s.capability + `"}`
	r := httptest.NewRequest(http.MethodPost, "/api/v1/session", strings.NewReader(body))
	r.Header.Set("Origin", s.origin)
	w := httptest.NewRecorder()
	s.api(w, r)
	if w.Code != 200 || w.Result().Cookies()[0].HttpOnly != true {
		t.Fatalf("session=%d", w.Code)
	}
	r = httptest.NewRequest(http.MethodPost, "/api/v1/session", strings.NewReader(body))
	r.Header.Set("Origin", s.origin)
	w = httptest.NewRecorder()
	s.api(w, r)
	if w.Code != http.StatusForbidden {
		t.Fatalf("reuse=%d", w.Code)
	}
}

func TestSessionRejectsOversizedTrailingAndUnknownJSON(t *testing.T) {
	for name, body := range map[string]string{
		"oversized": `{"token":"` + strings.Repeat("x", (1<<20)+1) + `"}`,
		"trailing":  `{"token":"x"} {}`,
		"unknown":   `{"token":"x","path":"/etc/passwd"}`,
	} {
		t.Run(name, func(t *testing.T) {
			s := server(t)
			r := httptest.NewRequest(http.MethodPost, "/api/v1/session", strings.NewReader(body))
			r.Header.Set("Origin", s.origin)
			w := httptest.NewRecorder()
			s.api(w, r)
			if w.Code != http.StatusBadRequest {
				t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
			}
		})
	}
}
