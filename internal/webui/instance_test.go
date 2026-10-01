package webui

import (
	"github.com/AdamNi-7080/AIOS/internal/store"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestInstanceMutationRequiresCSRFAndPersistsIdentity(t *testing.T) {
	s := server(t)
	s.session = "session"
	s.csrf = "csrf"
	first, err := store.LoadInstance(s.dataDir)
	if err != nil {
		t.Fatal(err)
	}
	for _, authorized := range []bool{false, true} {
		r := httptest.NewRequest(http.MethodPost, "/api/v1/instance", strings.NewReader(`{"name":"Research","seed_colour":"#123456"}`))
		r.Host = s.listener.Addr().String()
		r.Header.Set("Origin", s.origin)
		r.AddCookie(&http.Cookie{Name: "aios_kb_session", Value: "session"})
		if authorized {
			r.Header.Set("X-CSRF-Token", "csrf")
		}
		w := httptest.NewRecorder()
		s.api(w, r)
		if authorized && w.Code != 200 {
			t.Fatalf("save: %d %s", w.Code, w.Body)
		}
		if !authorized && w.Code != 403 {
			t.Fatal("missing CSRF accepted")
		}
	}
	second, err := store.LoadInstance(s.dataDir)
	if err != nil || second.ID != first.ID || second.Name != "Research" {
		t.Fatalf("persistence: %#v %v", second, err)
	}
	r := httptest.NewRequest(http.MethodGet, "/api/v1/instance", nil)
	r.Host = s.listener.Addr().String()
	w := httptest.NewRecorder()
	s.api(w, r)
	if w.Code != 403 {
		t.Fatal("unauthenticated metadata exposed")
	}
}
