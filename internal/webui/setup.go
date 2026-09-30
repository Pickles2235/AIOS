package webui

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/AdamNi-7080/AIOS/internal/app"
	"github.com/AdamNi-7080/AIOS/internal/catalog"
	"github.com/AdamNi-7080/AIOS/internal/knowledge"
	"github.com/AdamNi-7080/AIOS/internal/mirror"
	"github.com/AdamNi-7080/AIOS/internal/model"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

type Setup struct {
	State        string              `json:"state"`
	Error        string              `json:"error,omitempty"`
	Completed    int                 `json:"completed_repositories"`
	Repositories []mirror.Repository `json:"repositories,omitempty"`
	Active       catalog.Config      `json:"active_catalog"`
}

func (s *Server) readService() *knowledge.Service { s.mu.Lock(); defer s.mu.Unlock(); return s.read }
func (s *Server) persistSetup() error {
	target := filepath.Join(s.dataDir, "setup.json")
	if info, err := os.Lstat(target); err == nil && !info.Mode().IsRegular() {
		return fmt.Errorf("setup metadata must be a regular file")
	} else if err != nil && !os.IsNotExist(err) {
		return err
	}
	b, err := json.Marshal(s.setup)
	if err != nil {
		return err
	}
	f, err := os.CreateTemp(s.dataDir, ".setup-*")
	if err != nil {
		return err
	}
	name := f.Name()
	defer os.Remove(name)
	if _, err = f.Write(append(b, '\n')); err == nil {
		err = f.Sync()
	}
	ce := f.Close()
	if err != nil {
		return err
	}
	if ce != nil {
		return ce
	}
	return os.Rename(name, target)
}
func (s *Server) restoreSetup(cfg catalog.Config) error {
	s.setup = Setup{State: "unconfigured", Active: cfg}
	path := filepath.Join(s.dataDir, "setup.json")
	if info, err := os.Lstat(path); err == nil {
		if !info.Mode().IsRegular() || info.Mode().Perm() != 0600 || info.Size() > 1<<20 {
			return fmt.Errorf("unsafe setup metadata")
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if err = json.Unmarshal(b, &s.setup); err != nil {
			return err
		}
		if s.setup.State == "syncing" || s.setup.State == "ingesting" {
			s.setup.State = "interrupted"
			s.setup.Error = "Setup was interrupted. Retry to resume safely."
			if err = s.persistSetup(); err != nil {
				return err
			}
		}
		if len(s.setup.Active.Sources) > 0 {
			s.read = knowledge.New(s.setup.Active, s.db)
		}
	} else if !os.IsNotExist(err) {
		return err
	}
	if len(s.setup.Active.Sources) == 0 {
		snapshots, err := s.db.Status(context.Background(), "")
		if err != nil {
			return err
		}
		active := catalog.Config{Version: 1, Limits: catalog.Defaults()}
		for _, snapshot := range snapshots {
			active.Sources = append(active.Sources, catalog.Source{Kind: model.SourceKindRepository, ID: snapshot.RepoID})
		}
		if len(active.Sources) > 0 {
			s.setup.Active = active
			s.read = knowledge.New(active, s.db)
		}
	}
	return nil
}
func setupConfig(repos []mirror.Repository) (catalog.Config, mirror.Registry, error) {
	reg := mirror.Registry{Version: 1, Repositories: repos}
	if err := mirror.Validate(reg); err != nil {
		return catalog.Config{}, reg, err
	}
	cfg := catalog.Config{Version: 1, Limits: catalog.Defaults(), RetentionGenerations: 3}
	for _, r := range repos {
		cfg.Sources = append(cfg.Sources, catalog.Source{Kind: model.SourceKindRepository, ID: r.ID})
	}
	return cfg, reg, catalog.Validate(cfg)
}
func (s *Server) setupAPI(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == "/api/v1/onboarding" {
		if r.Method != http.MethodGet || !s.authorised(r, false) {
			fail(w, 403, "valid session required")
			return
		}
		s.mu.Lock()
		v := s.setup
		s.mu.Unlock()
		jsonBody(w, v)
		return
	}
	if r.Method != http.MethodPost || !s.authorised(r, true) {
		fail(w, 403, "valid session, origin and CSRF required")
		return
	}
	switch r.URL.Path {
	case "/api/v1/onboarding/configure":
		var in struct {
			Repositories []mirror.Repository `json:"repositories"`
		}
		if decode(r, &in) != nil {
			fail(w, 400, "invalid setup request")
			return
		}
		if _, _, err := setupConfig(in.Repositories); err != nil {
			fail(w, 400, err.Error())
			return
		}
		s.mu.Lock()
		defer s.mu.Unlock()
		if s.jobCancel != nil {
			fail(w, 409, "setup is already running")
			return
		}
		old := s.setup
		s.setup.Repositories = in.Repositories
		s.setup.State = "configured"
		s.setup.Error = ""
		s.setup.Completed = 0
		if err := s.persistSetup(); err != nil {
			s.setup = old
			fail(w, 500, "unable to save approved setup")
			return
		}
		jsonBody(w, s.setup)
	case "/api/v1/onboarding/start":
		var in struct{}
		if decode(r, &in) != nil {
			fail(w, 400, "invalid setup request")
			return
		}
		s.mu.Lock()
		defer s.mu.Unlock()
		if s.jobCancel != nil {
			fail(w, 409, "setup is already running")
			return
		}
		cfg, reg, err := setupConfig(s.setup.Repositories)
		if err != nil {
			fail(w, 400, err.Error())
			return
		}
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
		s.jobCancel = cancel
		s.setup.State = "syncing"
		s.setup.Error = ""
		s.setup.Completed = 0
		if err = s.persistSetup(); err != nil {
			cancel()
			s.jobCancel = nil
			fail(w, 500, "unable to start setup")
			return
		}
		go s.runSetup(ctx, cfg, reg)
		w.WriteHeader(202)
		jsonBody(w, s.setup)
	case "/api/v1/onboarding/cancel":
		var in struct{}
		if decode(r, &in) != nil {
			fail(w, 400, "invalid setup request")
			return
		}
		s.cancelJob()
		jsonBody(w, map[string]string{"state": "cancelling"})
	}
}
func (s *Server) cancelJob() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.jobCancel != nil {
		s.jobCancel()
	}
}
func (s *Server) runSetup(ctx context.Context, cfg catalog.Config, reg mirror.Registry) {
	var retErr error
	defer func() {
		s.mu.Lock()
		defer s.mu.Unlock()
		if retErr != nil {
			s.setup.State = "failed"
			s.setup.Error = retErr.Error()
			if ctx.Err() != nil {
				s.setup.State = "interrupted"
				s.setup.Error = "Setup interrupted; retry preserves the last active generation."
			}
		} else {
			s.setup.State = "ready"
			s.setup.Active = cfg
			s.read = knowledge.New(cfg, s.db)
		}
		if s.jobCancel != nil {
			s.jobCancel()
			s.jobCancel = nil
		}
		if err := s.persistSetup(); err != nil {
			s.setup.Error = "Unable to persist setup status; inspect owned data permissions."
		}
	}()
	synced, err := mirror.Sync(ctx, reg, s.dataDir)
	if err != nil {
		retErr = err
		return
	}
	s.mu.Lock()
	s.setup.Completed = len(synced)
	s.setup.State = "ingesting"
	err = s.persistSetup()
	s.mu.Unlock()
	if err != nil {
		retErr = err
		return
	}
	dir, err := os.MkdirTemp(s.dataDir, ".setup-input-*")
	if err != nil {
		retErr = err
		return
	}
	defer os.RemoveAll(dir)
	configPath := filepath.Join(dir, "catalog.json")
	registryPath := filepath.Join(dir, "mirrors.json")
	for path, v := range map[string]any{configPath: cfg, registryPath: reg} {
		b, e := json.Marshal(v)
		if e != nil {
			retErr = e
			return
		}
		if e = os.WriteFile(path, b, 0600); e != nil {
			retErr = e
			return
		}
	}
	_, retErr = app.IngestMirrorCatalog(ctx, configPath, registryPath, s.dataDir)
}
