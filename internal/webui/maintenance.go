package webui

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/AdamNi-7080/AIOS/internal/adapter"
	"github.com/AdamNi-7080/AIOS/internal/app"
	"github.com/AdamNi-7080/AIOS/internal/catalog"
	"github.com/AdamNi-7080/AIOS/internal/knowledge"
	"github.com/AdamNi-7080/AIOS/internal/maintenance"
	"github.com/AdamNi-7080/AIOS/internal/mirror"
	"github.com/AdamNi-7080/AIOS/internal/model"
)

func (s *Server) stopMaintenance() {
	s.maintenanceMu.Lock()
	defer s.maintenanceMu.Unlock()
	if s.maintainer != nil {
		s.maintainer.Close()
		s.maintainer = nil
	}
}

func (s *Server) startMaintenance() {
	s.setupTransitionMu.Lock()
	defer s.setupTransitionMu.Unlock()
	s.maintenanceMu.Lock()
	defer s.maintenanceMu.Unlock()
	if s.maintenanceContext == nil || s.maintenanceContext.Err() != nil || s.maintainer != nil {
		return
	}
	s.mu.Lock()
	setup := s.setup
	pendingRemoval := s.removing != ""
	s.mu.Unlock()
	if pendingRemoval || (len(setup.LocalRepositories) == 0 && len(setup.Repositories) == 0) {
		return
	}
	if setup.State == "unconfigured" || setup.State == "configured" || setup.State == "syncing" || setup.State == "ingesting" {
		return
	}
	if len(setup.Active.Sources) > 0 && sourceMode(setup.ActiveMode) != sourceMode(setup.Mode) {
		s.maintenanceError = "Maintenance is paused until the selected source mode builds successfully; prior knowledge remains available."
		return
	}
	cfg, _, err := setupConfig(setup.Repositories)
	if setup.Mode == "local" {
		cfg, err = localSetupConfigForMaintenance(setup.LocalRepositories)
	} else if err == nil {
		err = mirror.ValidateSourceBoundaries(mirror.Registry{Version: 1, Repositories: setup.Repositories}, s.dataDir)
	}
	if err == nil {
		err = applyScope(&cfg, setup.Rules)
	}
	if err != nil {
		s.maintenanceError = "Maintenance configuration is invalid; validate approved sources."
		return
	}
	sources := []maintenance.Source{}
	if setup.Mode == "local" {
		for _, entry := range setup.LocalRepositories {
			sources = append(sources, maintenance.Source{ID: entry.ID, Mode: "direct", Path: entry.Path})
		}
	} else {
		for _, entry := range setup.Repositories {
			sources = append(sources, maintenance.Source{ID: entry.ID, Mode: "mirror"})
		}
	}
	encoded, _ := json.Marshal(struct {
		Config catalog.Config
		Local  []adapter.LocalRepository
		Mirror []mirror.Repository
	}{cfg, setup.LocalRepositories, setup.Repositories})
	sum := sha256.Sum256(encoded)
	fingerprint := hex.EncodeToString(sum[:])
	run := func(ctx context.Context, id string) (maintenance.Outcome, error) {
		ctx = app.WithProgress(ctx, s.recordProgress)
		var discovery adapter.Discovery
		var err error
		if setup.Mode == "local" {
			var scope model.Repository
			for _, repo := range cfg.SourceRepositories() {
				if repo.ID == id {
					scope = repo
				}
			}
			for _, entry := range setup.LocalRepositories {
				if entry.ID == id {
					discovery, err = adapter.CaptureLocalScoped(ctx, entry, s.dataDir, cfg.Limits, scope)
					break
				}
			}
		} else {
			var entry mirror.Repository
			for _, candidate := range setup.Repositories {
				if candidate.ID == id {
					entry = candidate
					break
				}
			}
			registry := mirror.Registry{Version: 1, Repositories: []mirror.Repository{entry}}
			if _, err = mirror.Sync(ctx, registry, s.dataDir); err == nil {
				discovery, err = (adapter.RepositoryGit{Registry: registry, DataDir: s.dataDir}).Discover(ctx, id)
			}
		}
		outcome := maintenance.Outcome{Revision: discovery.Revision}
		if err != nil {
			return outcome, err
		}
		indexed, err := app.IngestMaintained(ctx, cfg, discovery, fingerprint, s.dataDir)
		if err != nil {
			return outcome, err
		}
		outcome.Generation = indexed.ActiveGenerations[id]
		for _, count := range indexed.Changes {
			outcome.ChangedFiles += count
		}
		s.mu.Lock()
		s.setup.Active = cfg
		s.setup.ActiveMode = setup.Mode
		s.read = knowledge.New(cfg, s.db)
		if s.setup.State == "failed" || s.setup.State == "interrupted" {
			s.setup.State = "maintaining"
			s.setup.Error = "Successful knowledge is available. Inspect repository health for sources still needing attention."
		}
		err = s.persistSetup()
		s.mu.Unlock()
		if err != nil {
			return outcome, fmt.Errorf("maintenance setup status could not persist")
		}
		return outcome, nil
	}
	engine, err := maintenance.New(s.dataDir, sources, run, maintenance.Defaults())
	if err != nil {
		s.maintenanceError = "Unable to restore durable maintenance; last-good knowledge remains available."
		return
	}
	active := map[string]maintenance.CanonicalState{}
	ctx, cancel := context.WithTimeout(s.maintenanceContext, 5*time.Second)
	defer cancel()
	for _, source := range sources {
		generation, e := s.db.ActiveGeneration(ctx, source.ID)
		if errors.Is(e, sql.ErrNoRows) {
			continue
		}
		if e != nil {
			engine.Close()
			s.maintenanceError = "Unable to reconcile canonical maintenance identity."
			return
		}
		queues, e := s.db.IngestionStatus(ctx, source.ID)
		if e != nil {
			engine.Close()
			s.maintenanceError = "Unable to reconcile durable source selection."
			return
		}
		revision := ""
		if len(queues) == 1 {
			revision = queues[0].CurrentRevision
		}
		if revision == "" {
			snapshots, e := s.db.Status(ctx, source.ID)
			if e != nil || len(snapshots) != 1 {
				engine.Close()
				s.maintenanceError = "Unable to reconcile source provenance."
				return
			}
			revision = snapshots[0].Git.Commit
		}
		active[source.ID] = maintenance.CanonicalState{Revision: revision, Generation: generation.ID, ActivatedAt: generation.ActivatedAt}
	}
	if err = engine.RestoreCanonical(active); err != nil {
		engine.Close()
		s.maintenanceError = "Unable to persist canonical maintenance identity."
		return
	}
	if err = engine.Start(s.maintenanceContext); err != nil {
		s.maintenanceError = "Unable to start durable maintenance; last-good knowledge remains available."
		return
	}
	s.maintenanceError = ""
	s.maintainer = engine
}

// Already approved paths need not all be accessible to check a healthy member.
func localSetupConfigForMaintenance(entries []adapter.LocalRepository) (catalog.Config, error) {
	reg := adapter.LocalRegistry{Version: 1, Repositories: entries}
	if err := adapter.ValidateLocalRegistry(reg); err != nil {
		return catalog.Config{}, err
	}
	cfg := catalog.Config{Version: 1, Limits: catalog.Defaults(), RetentionGenerations: 3}
	for _, entry := range entries {
		cfg.Sources = append(cfg.Sources, catalog.Source{Kind: "repository", ID: entry.ID})
	}
	return cfg, catalog.Validate(cfg)
}

func (s *Server) maintenanceAPI(w http.ResponseWriter, r *http.Request) {
	if (r.Method != http.MethodGet && r.Method != http.MethodPost) || !s.authorised(r, r.Method == http.MethodPost) {
		fail(w, 403, "valid origin/session/CSRF required")
		return
	}
	if (r.Method == http.MethodGet && r.URL.Path != "/api/v1/jobs" && r.URL.Path != "/api/v1/repositories") || (r.Method == http.MethodPost && r.URL.Path != "/api/v1/jobs/configure" && r.URL.Path != "/api/v1/repositories/check-now") {
		fail(w, 405, "unsupported maintenance method")
		return
	}
	s.maintenanceMu.Lock()
	defer s.maintenanceMu.Unlock()
	engine := s.maintainer
	if r.Method == http.MethodGet {
		status := maintenance.Status{Jobs: []maintenance.Job{}, MirrorIntervalSeconds: 900, RetryMaximumAttempts: 5, QueueCapacity: 100, WorkerCapacity: 1, QuietSeconds: 1, MaximumDebounceSeconds: 5, PersistenceError: s.maintenanceError}
		if engine != nil {
			status = engine.Status()
		}
		s.mu.Lock()
		managementError := s.managementError
		s.mu.Unlock()
		if managementError != "" {
			status.PersistenceError = managementError
		}
		if r.URL.Path == "/api/v1/repositories" {
			rows := []map[string]any{}
			for _, job := range status.Jobs {
				provenance := "git_commit"
				if job.Mode == "direct" {
					provenance = "working_tree"
				}
				rows = append(rows, map[string]any{"id": job.Repository, "mode": job.Mode, "provenance": provenance, "health": job})
			}
			jsonBody(w, map[string]any{"repositories": rows})
			return
		}
		jsonBody(w, status)
		return
	}
	if engine == nil {
		fail(w, 409, "Maintenance becomes available after Build or recovery; inspect setup status")
		return
	}
	switch r.URL.Path {
	case "/api/v1/repositories/check-now":
		var in struct {
			Repository string `json:"repository"`
		}
		if decode(r, &in) != nil {
			fail(w, 400, "invalid Check now request")
			return
		}
		if err := engine.Request(in.Repository, "check_now"); err != nil {
			fail(w, 409, "Unable to queue approved repository; inspect maintenance health")
			return
		}
		w.WriteHeader(202)
		jsonBody(w, map[string]string{"state": "queued"})
	case "/api/v1/jobs/configure":
		var in struct {
			Interval int `json:"mirror_interval_seconds"`
		}
		if decode(r, &in) != nil {
			fail(w, 400, "invalid polling configuration")
			return
		}
		if err := engine.SetMirrorInterval(in.Interval); err != nil {
			fail(w, 400, "Choose a polling interval between 10 and 86400 seconds; inspect owned storage permissions")
			return
		}
		jsonBody(w, engine.Status())
	default:
		fail(w, 405, "read-only maintenance endpoint")
	}
}
