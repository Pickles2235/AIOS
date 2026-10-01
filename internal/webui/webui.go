// Package webui serves the retained browser client and a read-only loopback API.
package webui

import (
	"context"
	"crypto/rand"
	"embed"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"net"
	"net/http"
	"os/exec"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/AdamNi-7080/AIOS/internal/catalog"
	"github.com/AdamNi-7080/AIOS/internal/knowledge"
	"github.com/AdamNi-7080/AIOS/internal/localname"
	"github.com/AdamNi-7080/AIOS/internal/semantic"
	"github.com/AdamNi-7080/AIOS/internal/store"
)

//go:embed dist/*
var assets embed.FS

type Server struct {
	listener                          net.Listener
	origin, capability, session, csrf string
	read                              *knowledge.Service
	mu                                sync.Mutex
	dataDir                           string
	db                                *store.Store
	setup                             Setup
	jobCancel                         context.CancelFunc
	stopDaemon                        func() error
	stopState                         string
	activity                          []ActivityEvent
	activitySequence                  uint64
	namespaceMu                       sync.Mutex
	namespace                         string
	namespaceError                    string
	namespaceLease                    *localname.Lease
}

func token() string {
	b := make([]byte, 32)
	_, _ = rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b)
}
func New(cfg catalog.Config, db *store.Store, listener net.Listener) (*Server, error) {
	if db == nil {
		return nil, fmt.Errorf("read-only knowledge store is required")
	}
	if listener == nil {
		var e error
		listener, e = net.Listen("tcp4", "127.0.0.1:0")
		if e != nil {
			return nil, e
		}
	}
	host, _, e := net.SplitHostPort(listener.Addr().String())
	if e != nil || !net.ParseIP(host).IsLoopback() {
		_ = listener.Close()
		return nil, fmt.Errorf("UI must bind loopback")
	}
	if _, e := store.LoadInstance(filepath.Dir(db.Path())); e != nil {
		_ = listener.Close()
		return nil, e
	}
	s := &Server{db: db, dataDir: filepath.Dir(db.Path()), listener: listener, origin: "http://" + listener.Addr().String(), capability: token(), read: knowledge.New(cfg, db)}
	if e = s.restoreSetup(cfg); e != nil {
		_ = listener.Close()
		return nil, e
	}
	if e = s.restoreNamespace(); e != nil {
		_ = listener.Close()
		return nil, e
	}
	return s, nil
}
func (s *Server) URL() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	origin := s.namespaceOriginLocked()
	if origin == "" {
		origin = s.origin
	}
	return origin + "/#token=" + s.capability
}
func (s *Server) FreshURL() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.capability = token()
	origin := s.namespaceOriginLocked()
	if origin == "" {
		origin = s.origin
	}
	return origin + "/#token=" + s.capability
}
func (s *Server) Close() error {
	s.cancelJob()
	s.namespaceMu.Lock()
	s.mu.Lock()
	lease := s.namespaceLease
	s.namespaceLease = nil
	s.mu.Unlock()
	if lease != nil {
		lease.Close()
	}
	s.namespaceMu.Unlock()
	return s.listener.Close()
}
func (s *Server) SetStopDaemon(stop func() error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.stopDaemon = stop
}
func (s *Server) authorised(r *http.Request, csrf bool) bool {
	// Browsers omit Origin on ordinary same-origin GETs. Only state-changing
	// request shapes require Origin plus CSRF; every route remains session bound.
	origin := r.Header.Get("Origin")
	if !s.requestOriginAllowed(r) || (csrf && origin == "") {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	c, e := r.Cookie("aios_kb_session")
	if e != nil || c.Value == "" || c.Value != s.session {
		return false
	}
	return !csrf || r.Header.Get("X-CSRF-Token") == s.csrf
}
func jsonBody(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}
func fail(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": message})
}
func decode(r *http.Request, v any) error {
	defer r.Body.Close()
	payload, err := io.ReadAll(io.LimitReader(r.Body, (1<<20)+1))
	if err != nil {
		return err
	}
	if len(payload) > 1<<20 {
		return fmt.Errorf("request body exceeds limit")
	}
	decoder := json.NewDecoder(strings.NewReader(string(payload)))
	decoder.DisallowUnknownFields()
	if err = decoder.Decode(v); err != nil {
		return err
	}
	if err = decoder.Decode(&struct{}{}); err != io.EOF {
		return fmt.Errorf("request must contain exactly one JSON object")
	}
	return nil
}
func (s *Server) api(w http.ResponseWriter, r *http.Request) {
	switch r.URL.Path {
	case "/api/v1/namespace", "/api/v1/namespace/open":
		s.namespaceAPI(w, r)
		return
	case "/api/v1/runtime":
		if r.Method != http.MethodGet || !s.authorised(r, false) {
			fail(w, 403, "authorised session required")
			return
		}
		_, javaErr := exec.LookPath("javac")
		_, nodeErr := exec.LookPath("node")
		available, reason := semantic.RuntimeAvailability()
		jsonBody(w, map[string]any{"bundled_assets": map[string]any{"ui": true, "embedding_model": semantic.ModelIdentity, "embedding_runtime_available": available, "embedding_runtime_reason": reason}, "compiler_coverage": map[string]any{"structural": "bundled Tree-sitter Java/Kotlin/JavaScript/TypeScript; unsupported input is disclosed", "javac_command_detected": javaErr == nil, "node_detected": nodeErr == nil, "semantic_compilers": "optional; JDK or Node plus a configured TypeScript module; command presence does not certify an installed JDK or successful compilation"}})
		return
	case "/api/v1/daemon/status":
		if r.Method != http.MethodGet || !s.authorised(r, false) {
			fail(w, 403, "authorised session required")
			return
		}
		s.mu.Lock()
		state := s.stopState
		managed := s.stopDaemon != nil
		s.mu.Unlock()
		if state == "" {
			state = "running"
		}
		jsonBody(w, map[string]any{"state": state, "managed": managed})
		return
	case "/api/v1/daemon/stop":
		if r.Method != http.MethodPost || !s.authorised(r, true) {
			fail(w, 403, "valid origin/session/CSRF required")
			return
		}
		var in struct{}
		if decode(r, &in) != nil {
			fail(w, 400, "invalid stop request")
			return
		}
		s.mu.Lock()
		stop := s.stopDaemon
		s.mu.Unlock()
		if stop == nil {
			fail(w, 409, "foreground UI has no managed daemon; stop its terminal process")
			return
		}
		s.mu.Lock()
		if s.stopState == "stopping" {
			s.mu.Unlock()
			fail(w, 409, "stop already requested")
			return
		}
		s.stopState = "stopping"
		s.mu.Unlock()
		jsonBody(w, map[string]any{"stopping": true, "restart": "aios daemon start; then aios daemon open"})
		go func() {
			time.Sleep(200 * time.Millisecond)
			if stop() != nil {
				s.mu.Lock()
				s.stopState = "failed"
				s.mu.Unlock()
			}
		}()
		return
	case "/api/v1/session":
		if r.Method == http.MethodGet {
			if !s.authorised(r, false) {
				fail(w, 403, "session expired")
				return
			}
			s.mu.Lock()
			v := s.csrf
			s.mu.Unlock()
			w.Header().Set("Cache-Control", "no-store")
			jsonBody(w, map[string]string{"csrf_token": v})
			return
		}
		if r.Method != http.MethodPost || r.Header.Get("Origin") == "" || !s.requestOriginAllowed(r) {
			fail(w, http.StatusForbidden, "same-origin session required")
			return
		}
		var in struct {
			Token string `json:"token"`
		}
		if decode(r, &in) != nil {
			fail(w, 400, "invalid session request")
			return
		}
		s.mu.Lock()
		defer s.mu.Unlock()
		if in.Token == "" || in.Token != s.capability {
			fail(w, 403, "expired capability")
			return
		}
		s.capability = ""
		s.session = token()
		s.csrf = token()
		http.SetCookie(w, &http.Cookie{Name: "aios_kb_session", Value: s.session, Path: "/", HttpOnly: true, SameSite: http.SameSiteStrictMode})
		jsonBody(w, map[string]string{"csrf_token": s.csrf})
		return
	case "/api/v1/onboarding/preview":
		s.previewAPI(w, r)
		return
	case "/api/v1/activity":
		s.activityAPI(w, r)
		return
	case "/api/v1/onboarding", "/api/v1/onboarding/configure", "/api/v1/onboarding/start", "/api/v1/onboarding/cancel":
		s.setupAPI(w, r)
		return
	case "/api/v1/instance", "/api/v1/instance/logo", "/api/v1/instance/logo/generate":
		s.instanceAPI(w, r)
		return
	case "/api/v1/status":
		if r.Method != http.MethodGet || !s.authorised(r, false) {
			fail(w, 403, "authorised same-origin session required")
			return
		}
		v, e := s.readService().Status(r.Context())
		if e != nil {
			fail(w, 500, "knowledge status unavailable")
			return
		}
		jsonBody(w, v)
		return
	case "/api/v1/projection":
		if r.Method != http.MethodGet || !s.authorised(r, false) {
			fail(w, 403, "authorised same-origin session required")
			return
		}
		repo := r.URL.Query().Get("repo")
		if repo == "" {
			fail(w, 400, "repo is required")
			return
		}
		v, e := s.readService().Projection(r.Context(), repo, r.URL.Query().Get("cursor"), 20)
		if e != nil {
			fail(w, 400, e.Error())
			return
		}
		jsonBody(w, v)
		return
	case "/api/v1/entity", "/api/v1/neighbors", "/api/v1/evidence", "/api/v1/query":
		if r.Method != http.MethodPost || !s.authorised(r, true) {
			fail(w, 403, "valid origin, session, and CSRF token required")
			return
		}
	default:
		fail(w, http.StatusNotFound, "read-only knowledge endpoint not found")
		return
	}
	switch r.URL.Path {
	case "/api/v1/entity":
		var in struct {
			Handle string `json:"handle"`
		}
		if decode(r, &in) != nil {
			fail(w, 400, "invalid request")
			return
		}
		v, e := s.readService().Entity(r.Context(), in.Handle)
		if e != nil {
			fail(w, 409, e.Error())
			return
		}
		jsonBody(w, v)
	case "/api/v1/neighbors":
		var in struct {
			Handle string   `json:"handle"`
			Types  []string `json:"types"`
			Limit  int      `json:"limit"`
		}
		if decode(r, &in) != nil {
			fail(w, 400, "invalid request")
			return
		}
		v, tr, e := s.readService().Neighbors(r.Context(), in.Handle, in.Types, in.Limit)
		if e != nil {
			fail(w, 409, e.Error())
			return
		}
		jsonBody(w, map[string]any{"claims": v, "truncated": tr})
	case "/api/v1/evidence":
		var in struct {
			Evidence string `json:"evidence"`
			Before   int    `json:"before"`
			After    int    `json:"after"`
			MaxLines int    `json:"max_lines"`
		}
		if decode(r, &in) != nil {
			fail(w, 400, "invalid request")
			return
		}
		v, e := s.readService().Excerpt(r.Context(), in.Evidence, in.Before, in.After, in.MaxLines)
		if e != nil {
			fail(w, 409, e.Error())
			return
		}
		jsonBody(w, v)
	case "/api/v1/query":
		var in knowledge.Query
		if decode(r, &in) != nil {
			fail(w, 400, "invalid request")
			return
		}
		v, e := s.readService().Query(r.Context(), in)
		if e != nil {
			fail(w, 400, e.Error())
			return
		}
		jsonBody(w, v)
	}
}
func (s *Server) Serve(ctx context.Context) error {
	sub, e := fs.Sub(assets, "dist")
	if e != nil {
		return e
	}
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !s.requestOriginAllowed(r) {
			fail(w, 403, "unapproved local Host or origin")
			return
		}
		w.Header().Set("Content-Security-Policy", "default-src 'self'; connect-src 'self'; style-src 'self'; base-uri 'none'; frame-ancestors 'none'")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "no-referrer")
		if strings.HasPrefix(r.URL.Path, "/api/") {
			s.api(w, r)
			return
		}
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			fail(w, 405, "method not allowed")
			return
		}
		p := path.Clean(r.URL.Path)
		if p == "/" {
			p = "/index.html"
		}
		if _, e := fs.Stat(sub, strings.TrimPrefix(p, "/")); e != nil {
			p = "/index.html"
		}
		http.FileServer(http.FS(sub)).ServeHTTP(w, r.WithContext(ctx))
	})
	server := &http.Server{Handler: h}
	go func() { <-ctx.Done(); s.cancelJob(); _ = server.Close() }()
	e = server.Serve(s.listener)
	if e == http.ErrServerClosed {
		return nil
	}
	return e
}
