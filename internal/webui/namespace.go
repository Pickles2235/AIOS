package webui

import (
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/AdamNi-7080/AIOS/internal/lifecycle"
	"github.com/AdamNi-7080/AIOS/internal/localname"
)

type namespaceConfig struct {
	Name string `json:"namespace"`
}

func (s *Server) namespacePath() string { return filepath.Join(s.dataDir, "namespace.json") }
func (s *Server) restoreNamespace() error {
	p := s.namespacePath()
	info, e := os.Lstat(p)
	if os.IsNotExist(e) {
		return nil
	}
	if e != nil {
		return e
	}
	if !info.Mode().IsRegular() || info.Mode().Perm() != 0600 || info.Size() > 4096 {
		return fmt.Errorf("namespace metadata must be bounded and owner-only")
	}
	f, e := lifecycle.OpenMetadata(p)
	if e != nil {
		return e
	}
	defer f.Close()
	opened, e := f.Stat()
	if e != nil || !os.SameFile(info, opened) {
		return fmt.Errorf("namespace metadata changed while opening")
	}
	var cfg namespaceConfig
	d := json.NewDecoder(io.LimitReader(f, 4097))
	d.DisallowUnknownFields()
	if d.Decode(&cfg) != nil || d.Decode(&struct{}{}) != io.EOF {
		return fmt.Errorf("invalid namespace metadata")
	}
	if e = localname.Validate(cfg.Name); e != nil {
		return e
	}
	s.namespace = cfg.Name
	lease, e := localname.Acquire(cfg.Name)
	if e != nil {
		s.namespaceError = "Selected namespace is unavailable; open recovery localhost and select an alternative. No automatic rename occurred."
		return nil
	}
	s.namespaceLease = lease
	return nil
}
func (s *Server) persistNamespace(name string) error {
	p := s.namespacePath()
	if i, e := os.Lstat(p); e == nil {
		if !i.Mode().IsRegular() || i.Mode().Perm() != 0600 {
			return fmt.Errorf("unsafe namespace metadata")
		}
		f, err := lifecycle.OpenMetadata(p)
		if err != nil {
			return err
		}
		f.Close()
	} else if !os.IsNotExist(e) {
		return e
	}
	f, e := os.CreateTemp(s.dataDir, ".namespace-*")
	if e != nil {
		return e
	}
	temp := f.Name()
	defer os.Remove(temp)
	b, _ := json.Marshal(namespaceConfig{Name: name})
	if _, e = f.Write(append(b, '\n')); e == nil {
		e = f.Sync()
	}
	ce := f.Close()
	if e != nil {
		return e
	}
	if ce != nil {
		return ce
	}
	if e = os.Rename(temp, p); e != nil {
		return e
	}
	dir, e := os.Open(s.dataDir)
	if e != nil {
		return e
	}
	defer dir.Close()
	return dir.Sync()
}
func (s *Server) namespaceOriginLocked() string {
	if s.namespaceLease == nil || s.namespaceLease.Health() != nil {
		return ""
	}
	_, port, _ := net.SplitHostPort(s.listener.Addr().String())
	return "http://" + net.JoinHostPort(s.namespaceLease.Host, port)
}
func (s *Server) originAllowedLocked(origin string) bool {
	_, port, _ := net.SplitHostPort(s.listener.Addr().String())
	named := s.namespaceOriginLocked()
	return origin == s.origin || origin == "http://localhost:"+port || (named != "" && origin == named)
}
func (s *Server) requestOriginAllowed(r *http.Request) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	origin := "http://" + r.Host
	return s.originAllowedLocked(origin) && (r.Header.Get("Origin") == "" || r.Header.Get("Origin") == origin)
}
func (s *Server) namespaceStatusLocked() map[string]any {
	origin := s.namespaceOriginLocked()
	native := origin != "" && s.namespaceLease != nil && s.namespaceLease.Native
	errText := s.namespaceError
	if s.namespace != "" && origin == "" && errText == "" {
		errText = "Selected namespace registration is unavailable. Use recovery localhost and select a name explicitly; no automatic rename occurred."
	}
	_, port, _ := net.SplitHostPort(s.listener.Addr().String())
	return map[string]any{"namespace": s.namespace, "persisted": s.namespace != "", "active": origin != "", "local_only": true, "port": port, "local_url": origin, "recovery_url": strings.Replace(s.origin, "127.0.0.1", "localhost", 1), "native_dns_sd": native, "developer_only": runtime.GOOS != "darwin" || runtime.GOARCH != "arm64", "error": errText, "address_contract": "Explicit HTTP port on a loopback listener. Native DNS-SD record is local-only; no LAN HTTP listener, hosts-file edit or privileged route."}
}
func (s *Server) namespaceAPI(w http.ResponseWriter, r *http.Request) {
	if (r.Method != http.MethodGet && r.Method != http.MethodPost) || !s.authorised(r, r.Method == http.MethodPost) {
		fail(w, 403, "valid origin/session/CSRF required")
		return
	}
	if r.Method == http.MethodGet {
		s.mu.Lock()
		result := s.namespaceStatusLocked()
		s.mu.Unlock()
		jsonBody(w, result)
		return
	}
	if r.URL.Path == "/api/v1/namespace/open" {
		var in struct{}
		if decode(r, &in) != nil {
			fail(w, 400, "invalid open request")
			return
		}
		s.namespaceMu.Lock()
		defer s.namespaceMu.Unlock()
		s.mu.Lock()
		active := s.namespaceOriginLocked() != ""
		s.mu.Unlock()
		if !active {
			fail(w, 409, "Select an available namespace first; recovery localhost remains available")
			return
		}
		jsonBody(w, map[string]string{"url": s.FreshURL()})
		return
	}
	var in namespaceConfig
	if decode(r, &in) != nil || localname.Validate(in.Name) != nil {
		fail(w, 400, "Choose a lowercase namespace with 1–63 DNS-label characters")
		return
	}
	s.namespaceMu.Lock()
	defer s.namespaceMu.Unlock()
	s.mu.Lock()
	same := s.namespace == in.Name && s.namespaceLease != nil && s.namespaceLease.Health() == nil
	var unavailable *localname.Lease
	if s.namespace == in.Name && s.namespaceLease != nil && !same {
		unavailable = s.namespaceLease
		s.namespaceLease = nil
	}
	s.mu.Unlock()
	if unavailable != nil {
		unavailable.Close()
	}
	if same {
		s.mu.Lock()
		result := s.namespaceStatusLocked()
		s.mu.Unlock()
		jsonBody(w, result)
		return
	}
	lease, e := localname.Acquire(in.Name)
	if e != nil {
		alternatives := []string{}
		for i := 2; i <= 4; i++ {
			candidate := in.Name
			if len(candidate) > 60 {
				candidate = candidate[:60]
			}
			candidate = fmt.Sprintf("%s-%d", candidate, i)
			if l, e := localname.Acquire(candidate); e == nil {
				alternatives = append(alternatives, candidate)
				l.Close()
			}
		}
		w.WriteHeader(409)
		jsonBody(w, map[string]any{"error": "Namespace collision or local registration unavailable. Select a suggested name yourself; no configuration was changed.", "suggestions": alternatives, "requires_selection": true})
		return
	}
	if e = s.persistNamespace(in.Name); e != nil {
		lease.Close()
		fail(w, 500, "unable to save selected namespace")
		return
	}
	s.mu.Lock()
	old := s.namespaceLease
	s.namespaceLease = lease
	s.namespace = in.Name
	s.namespaceError = ""
	result := s.namespaceStatusLocked()
	s.mu.Unlock()
	if old != nil {
		old.Close()
	}
	result["launch_url"] = s.FreshURL()
	jsonBody(w, result)
}
