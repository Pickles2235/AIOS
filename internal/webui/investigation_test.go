package webui

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/AdamNi-7080/AIOS/internal/catalog"
)

func TestInvestigationHistoryPersistsBoundsAndClears(t *testing.T) {
	s := server(t)
	for i := 0; i < historyMax+2; i++ {
		h := investigationHistory{Entries: []string{strings.Repeat("q", i+1)}, MaxEntries: historyMax}
		if err := s.writeHistory(h); err != nil {
			t.Fatal(err)
		}
	}
	h, err := s.readHistory()
	if err != nil || len(h.Entries) != 1 || h.MaxEntries != historyMax {
		t.Fatalf("history=%+v err=%v", h, err)
	}
	entries := make([]string, historyMax+3)
	for i := range entries {
		entries[i] = strings.Repeat("q", i+1)
	}
	if err := s.writeHistory(investigationHistory{Entries: entries}); err != nil {
		t.Fatal(err)
	}
	if info, err := os.Stat(s.historyPath()); err != nil || info.Mode().Perm() != 0600 {
		t.Fatalf("history permission=%v err=%v", info, err)
	}
	h, err = s.readHistory()
	if err != nil || len(h.Entries) != historyMax {
		t.Fatalf("bounded=%+v err=%v", h, err)
	}
	restarted, err := New(catalog.Config{}, s.db, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer restarted.Close()
	restored, err := restarted.readHistory()
	if err != nil || len(restored.Entries) != historyMax {
		t.Fatalf("restarted=%+v err=%v", restored, err)
	}
	if err := s.writeHistory(investigationHistory{Entries: []string{}, MaxEntries: historyMax}); err != nil {
		t.Fatal(err)
	}
	h, err = s.readHistory()
	if err != nil || len(h.Entries) != 0 {
		t.Fatalf("cleared=%+v err=%v", h, err)
	}
}

func TestInvestigationAPIRequiresSessionAndCSRF(t *testing.T) {
	s := server(t)
	call := func(method, path, body string, authorised bool) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, path, strings.NewReader(body))
		r.Host = s.listener.Addr().String()
		r.Header.Set("Origin", s.origin)
		if authorised {
			r.AddCookie(&http.Cookie{Name: "aios_kb_session", Value: s.session})
			r.Header.Set("X-CSRF-Token", s.csrf)
		}
		w := httptest.NewRecorder()
		s.api(w, r)
		return w
	}
	if w := call(http.MethodPost, "/api/v1/investigation", `{"text":"Publish"}`, false); w.Code != 403 {
		t.Fatalf("unauthorised=%d", w.Code)
	}
	s.session, s.csrf = "session", "csrf"
	if w := call(http.MethodPost, "/api/v1/investigation", `{"text":"/bogus Publish"}`, true); w.Code != 200 {
		t.Fatalf("investigation=%d %s", w.Code, w.Body.String())
	}
	if w := call(http.MethodGet, "/api/v1/history", "", true); w.Code != 200 {
		t.Fatalf("history=%d", w.Code)
	} else {
		var h investigationHistory
		if err := json.Unmarshal(w.Body.Bytes(), &h); err != nil || len(h.Entries) != 0 {
			t.Fatalf("history=%+v err=%v", h, err)
		}
	}
	if w := call(http.MethodPost, "/api/v1/history/clear", `{}`, true); w.Code != 200 {
		t.Fatalf("clear=%d", w.Code)
	}
}
