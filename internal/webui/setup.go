package webui

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/AdamNi-7080/AIOS/internal/adapter"
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
	Mode              string                    `json:"mode,omitempty"`
	LocalRepositories []adapter.LocalRepository `json:"local_repositories,omitempty"`
	State             string                    `json:"state"`
	Error             string                    `json:"error,omitempty"`
	Completed         int                       `json:"completed_repositories"`
	Repositories      []mirror.Repository       `json:"repositories,omitempty"`
	Active            catalog.Config            `json:"active_catalog"`
	Rules             map[string]ScopeRules     `json:"rules,omitempty"`
	ActiveMode        string                    `json:"active_mode,omitempty"`
	PendingRemoval    string                    `json:"pending_removal,omitempty"`
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
	if err = os.Rename(name, target); err != nil {
		return err
	}
	return syncDirectory(s.dataDir)
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
		if s.setup.ActiveMode == "" && s.setup.State == "ready" {
			s.setup.ActiveMode = s.setup.Mode
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
		v.PendingRemoval = s.removing
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
			Mode              string                    `json:"mode,omitempty"`
			LocalRepositories []adapter.LocalRepository `json:"local_repositories,omitempty"`
			Repositories      []mirror.Repository       `json:"repositories"`
			Rules             map[string]ScopeRules     `json:"rules,omitempty"`
		}
		if decode(r, &in) != nil {
			fail(w, 400, "invalid setup request")
			return
		}
		if in.Mode != "" && in.Mode != "mirror" && in.Mode != "local" {
			fail(w, 400, "unsupported source mode")
			return
		}
		if (in.Mode == "local" && len(in.Repositories) != 0) || (in.Mode != "local" && len(in.LocalRepositories) != 0) {
			fail(w, 400, "Choose Direct OR Mirror; mixed source arrays are rejected.")
			return
		}
		var approved catalog.Config
		var validationErr error
		if in.Mode == "local" {
			approved, validationErr = localSetupConfig(in.LocalRepositories, s.dataDir)
		} else {
			approved, _, validationErr = setupConfig(in.Repositories)
			if validationErr == nil {
				validationErr = mirror.ValidateSourceBoundaries(mirror.Registry{Version: 1, Repositories: in.Repositories}, s.dataDir)
			}
		}
		if validationErr == nil {
			validationErr = applyScope(&approved, in.Rules)
		}
		if err := validationErr; err != nil {
			fail(w, 400, err.Error())
			return
		}
		s.setupTransitionMu.Lock()
		defer s.setupTransitionMu.Unlock()
		s.mu.Lock()
		if omitsApproved(s.setup, in.LocalRepositories, in.Repositories) || (s.removing != "" && sourceMode(in.Mode) != sourceMode(s.setup.Mode)) {
			s.mu.Unlock()
			fail(w, 409, "Remove repositories using the explicit purge control before omitting their IDs from setup.")
			return
		}
		s.mu.Unlock()
		s.stopMaintenance()
		s.mu.Lock()
		defer s.mu.Unlock()
		if s.jobCancel != nil {
			fail(w, 409, "setup is already running")
			return
		}
		old := s.setup
		if in.Mode == "local" {
			in.Repositories = nil
		} else {
			in.LocalRepositories = nil
		}
		s.setup.Mode = in.Mode
		s.setup.LocalRepositories = in.LocalRepositories
		s.setup.Repositories = in.Repositories
		s.setup.Rules = in.Rules
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
		s.setupTransitionMu.Lock()
		defer s.setupTransitionMu.Unlock()
		s.mu.Lock()
		pendingRemoval := s.removing != ""
		s.mu.Unlock()
		if pendingRemoval {
			fail(w, 409, "Finish repository removal before starting a build. Corrected source approvals are saved.")
			return
		}
		s.stopMaintenance()
		s.mu.Lock()
		defer s.mu.Unlock()
		if s.jobCancel != nil {
			fail(w, 409, "setup is already running")
			return
		}
		cfg, reg, err := setupConfig(s.setup.Repositories)
		mode := s.setup.Mode
		locals := append([]adapter.LocalRepository(nil), s.setup.LocalRepositories...)
		if mode == "local" {
			cfg, err = localSetupConfig(locals, s.dataDir)
		}
		if err == nil {
			err = applyScope(&cfg, s.setup.Rules)
		}
		if err != nil {
			fail(w, 400, err.Error())
			return
		}
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
		ctx = app.WithProgress(ctx, s.recordProgress)
		s.jobCancel = cancel
		s.jobDone = make(chan struct{})
		s.setup.State = "syncing"
		if mode == "local" {
			s.setup.State = "ingesting"
		}
		s.setup.Error = ""
		s.setup.Completed = 0
		if err = s.persistSetup(); err != nil {
			cancel()
			s.jobCancel = nil
			close(s.jobDone)
			s.jobDone = nil
			fail(w, 500, "unable to start setup")
			return
		}
		go s.runSetup(ctx, cfg, reg, locals)
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
func (s *Server) runSetup(ctx context.Context, cfg catalog.Config, reg mirror.Registry, local ...[]adapter.LocalRepository) {
	var retErr error
	defer func() { s.finishSetup(ctx, cfg, retErr) }()
	if len(local) > 0 && len(local[0]) > 0 {
		_, retErr = app.IngestLocal(ctx, cfg, adapter.LocalRegistry{Version: 1, Repositories: local[0]}, s.dataDir, "")
		if retErr == nil {
			s.mu.Lock()
			s.setup.Completed = len(local[0])
			s.mu.Unlock()
		}
		return
	}
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

func localSetupConfig(entries []adapter.LocalRepository, dataDir string) (catalog.Config, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	reg := adapter.LocalRegistry{Version: 1, Repositories: entries}
	if err := adapter.ValidateLocalRegistry(reg); err != nil {
		return catalog.Config{}, err
	}
	cfg := catalog.Config{Version: 1, Limits: catalog.Defaults(), RetentionGenerations: 3}
	for _, entry := range entries {
		if _, err := adapter.InspectLocal(ctx, entry, dataDir); err != nil {
			return cfg, err
		}
		cfg.Sources = append(cfg.Sources, catalog.Source{Kind: model.SourceKindRepository, ID: entry.ID})
	}
	return cfg, catalog.Validate(cfg)
}

func (s *Server) finishSetup(ctx context.Context, cfg catalog.Config, retErr error) {
	s.mu.Lock()
	if retErr != nil {
		s.setup.State = "failed"
		s.setup.Error = "Build failed; the last active knowledge remains available. Validate the sources and retry."
		var gitErr *mirror.GitError
		if errors.As(retErr, &gitErr) {
			s.setup.Error = gitErr.Remediation()
		}
		if ctx.Err() != nil {
			s.setup.State = "interrupted"
			s.setup.Error = "Setup interrupted; retry preserves the last active generation."
		}
	} else {
		s.setup.State = "ready"
		s.setup.Active = cfg
		s.setup.ActiveMode = s.setup.Mode
		s.read = knowledge.New(cfg, s.db)
	}
	if s.jobCancel != nil {
		s.jobCancel()
		s.jobCancel = nil
	}
	if err := s.persistSetup(); err != nil {
		s.setup.Error = "Unable to persist setup status; inspect owned data permissions."
	}
	if s.jobDone != nil {
		close(s.jobDone)
		s.jobDone = nil
	}
	s.mu.Unlock()
	s.startMaintenance()
}
