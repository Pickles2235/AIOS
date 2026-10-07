package webui

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

const historyMax = 20

type investigationHistory struct {
	Entries    []string `json:"entries"`
	MaxEntries int      `json:"max_entries"`
}

func (s *Server) historyPath() string { return filepath.Join(s.dataDir, "investigation-history.json") }

func (s *Server) readHistory() (investigationHistory, error) {
	h := investigationHistory{Entries: []string{}, MaxEntries: historyMax}
	b, err := os.ReadFile(s.historyPath())
	if os.IsNotExist(err) {
		return h, nil
	}
	if err != nil {
		return h, err
	}
	if err = json.Unmarshal(b, &h); err != nil {
		return h, fmt.Errorf("invalid local history: %w", err)
	}
	if len(h.Entries) > historyMax {
		h.Entries = h.Entries[:historyMax]
	}
	h.MaxEntries = historyMax
	return h, nil
}

func (s *Server) writeHistory(h investigationHistory) error {
	b, err := json.Marshal(h)
	if err != nil {
		return err
	}
	file, err := os.CreateTemp(s.dataDir, ".investigation-history-*")
	if err != nil {
		return err
	}
	defer os.Remove(file.Name())
	if err = file.Chmod(0600); err == nil {
		_, err = file.Write(b)
	}
	if err == nil {
		err = file.Sync()
	}
	if closeErr := file.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	return os.Rename(file.Name(), s.historyPath())
}

func (s *Server) investigationAPI(w http.ResponseWriter, r *http.Request) {
	if !s.authorised(r, r.Method == http.MethodPost) {
		fail(w, 403, "valid origin, session and CSRF token required")
		return
	}
	s.mu.Lock()
	pending := s.removing != ""
	s.mu.Unlock()
	if pending {
		fail(w, 503, "repository removal is recovering")
		return
	}
	s.historyMu.Lock()
	defer s.historyMu.Unlock()
	switch r.URL.Path {
	case "/api/v1/history":
		if r.Method != http.MethodGet {
			fail(w, 405, "method not allowed")
			return
		}
		h, err := s.readHistory()
		if err != nil {
			fail(w, 500, "local history unavailable")
			return
		}
		jsonBody(w, h)
	case "/api/v1/history/clear":
		if r.Method != http.MethodPost {
			fail(w, 405, "method not allowed")
			return
		}
		var in struct{}
		if decode(r, &in) != nil {
			fail(w, 400, "invalid request")
			return
		}
		if err := s.writeHistory(investigationHistory{Entries: []string{}, MaxEntries: historyMax}); err != nil {
			fail(w, 500, "cannot clear local history")
			return
		}
		jsonBody(w, investigationHistory{Entries: []string{}, MaxEntries: historyMax})
	case "/api/v1/investigation":
		if r.Method != http.MethodPost {
			fail(w, 405, "method not allowed")
			return
		}
		var in struct {
			Text string `json:"text"`
		}
		if decode(r, &in) != nil {
			fail(w, 400, "invalid request")
			return
		}
		v, err := s.readService().Investigation(r.Context(), in.Text)
		if err != nil {
			fail(w, 409, "investigation source generation changed or is unavailable")
			return
		}
		if strings.TrimSpace(in.Text) != "" && len(in.Text) <= 256 && v.Intent != "unsupported" {
			h, e := s.readHistory()
			if e != nil {
				fail(w, 500, "local history unavailable")
				return
			}
			entries := []string{strings.TrimSpace(in.Text)}
			for _, prior := range h.Entries {
				if prior != entries[0] && len(entries) < historyMax {
					entries = append(entries, prior)
				}
			}
			if e = s.writeHistory(investigationHistory{Entries: entries, MaxEntries: historyMax}); e != nil {
				fail(w, 500, "cannot save local history")
				return
			}
		}
		jsonBody(w, v)
	default:
		fail(w, 404, "unknown investigation endpoint")
	}
}
