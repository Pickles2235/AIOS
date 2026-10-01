package webui

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestDaemonStopAuthenticationFailureAndFreshCapability(t *testing.T) {
	s := server(t)
	defer s.Close()
	s.session = "fixture-session"
	s.csrf = "fixture-csrf"
	s.SetStopDaemon(func() error { return fmt.Errorf("private path secret must not persist") })
	request := func(authorized bool) *httptest.ResponseRecorder {
		r := httptest.NewRequest("POST", "/api/v1/daemon/stop", strings.NewReader(`{}`))
		r.Host = s.listener.Addr().String()
		if authorized {
			r.Header.Set("Origin", s.origin)
			r.Header.Set("X-CSRF-Token", s.csrf)
			r.AddCookie(&http.Cookie{Name: "aios_kb_session", Value: s.session})
		}
		w := httptest.NewRecorder()
		s.api(w, r)
		return w
	}
	if request(false).Code != 403 {
		t.Fatal("unauthenticated stop accepted")
	}
	if request(true).Code != 200 {
		t.Fatal("authorized stop failed")
	}
	if request(true).Code != 409 {
		t.Fatal("duplicate stop accepted")
	}
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		s.mu.Lock()
		state := s.stopState
		s.mu.Unlock()
		if state == "failed" {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	r := httptest.NewRequest("GET", "/api/v1/daemon/status", nil)
	r.Host = s.listener.Addr().String()
	r.AddCookie(&http.Cookie{Name: "aios_kb_session", Value: s.session})
	w := httptest.NewRecorder()
	s.api(w, r)
	if !strings.Contains(w.Body.String(), `"failed"`) || strings.Contains(w.Body.String(), "secret") {
		t.Fatal("stop failure hidden or source exposed")
	}
	old := s.URL()
	fresh := s.FreshURL()
	if fresh == old || !strings.Contains(fresh, "#token=") {
		t.Fatal("launch link not renewed")
	}
	// Issuing a link alone leaves the current authenticated session intact.
	if !s.authorised(r, false) {
		t.Fatal("renewing link invalidated active browser prematurely")
	}
}
