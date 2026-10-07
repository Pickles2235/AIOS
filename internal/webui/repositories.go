package webui

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"time"

	"github.com/AdamNi-7080/AIOS/internal/adapter"
	"github.com/AdamNi-7080/AIOS/internal/app"
	"github.com/AdamNi-7080/AIOS/internal/catalog"
	"github.com/AdamNi-7080/AIOS/internal/knowledge"
	"github.com/AdamNi-7080/AIOS/internal/lifecycle"
	"github.com/AdamNi-7080/AIOS/internal/maintenance"
	"github.com/AdamNi-7080/AIOS/internal/mirror"
	"github.com/AdamNi-7080/AIOS/internal/model"
	"github.com/AdamNi-7080/AIOS/internal/store"
)

var managedID = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{0,62}$`)

const removalJournal = "repository-removal.json"

type removalIntent struct {
	Schema     int    `json:"schema"`
	Repository string `json:"repository"`
}

func syncDirectory(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return f.Sync()
}
func writeRemoval(dataDir, id string) error {
	if f, err := lifecycle.OpenMetadata(filepath.Join(dataDir, removalJournal)); err == nil {
		f.Close()
	} else if !os.IsNotExist(err) {
		return err
	}
	b, err := json.Marshal(removalIntent{1, id})
	if err != nil {
		return err
	}
	f, err := os.CreateTemp(dataDir, ".removal-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if _, err = f.Write(b); err == nil {
		err = f.Sync()
	}
	ce := f.Close()
	if err != nil {
		return err
	}
	if ce != nil {
		return ce
	}
	if err = os.Rename(f.Name(), filepath.Join(dataDir, removalJournal)); err != nil {
		return err
	}
	return syncDirectory(dataDir)
}
func cloneSetup(in Setup) Setup {
	b, _ := json.Marshal(in)
	var out Setup
	_ = json.Unmarshal(b, &out)
	return out
}
func approvedIDs(setup Setup) []string {
	out := []string{}
	for _, r := range setup.LocalRepositories {
		out = append(out, r.ID)
	}
	for _, r := range setup.Repositories {
		out = append(out, r.ID)
	}
	return out
}

func approvedAssetSources(setup Setup) mirror.Registry {
	reg := mirror.Registry{Version: 1, Repositories: append([]mirror.Repository(nil), setup.Repositories...)}
	for _, entry := range setup.LocalRepositories {
		reg.Repositories = append(reg.Repositories, mirror.Repository{ID: entry.ID, URL: entry.Path})
	}
	return reg
}

func sourceMode(mode string) string {
	if mode == "local" {
		return "local"
	}
	return "mirror"
}
func omitsApproved(setup Setup, local []adapter.LocalRepository, remote []mirror.Repository) bool {
	ids := map[string]bool{}
	for _, r := range local {
		ids[r.ID] = true
	}
	for _, r := range remote {
		ids[r.ID] = true
	}
	for _, id := range approvedIDs(setup) {
		if !ids[id] {
			return true
		}
	}
	return false
}
func managementConfig(setup Setup) (catalog.Config, mirror.Registry, error) {
	if len(approvedIDs(setup)) == 0 {
		return catalog.Config{Version: 1, Limits: catalog.Defaults()}, mirror.Registry{}, nil
	}
	cfg, reg, err := setupConfig(setup.Repositories)
	if setup.Mode == "local" {
		cfg, err = localSetupConfigForMaintenance(setup.LocalRepositories)
	}
	if err == nil {
		err = applyScope(&cfg, setup.Rules)
	}
	return cfg, reg, err
}

// Cancellation waits for ingestion to release its writer before any durable
// withdrawal or physical purge. runSetup closes jobDone before reacquiring the
// transition mutex to restart maintenance, avoiding a lock-cycle on removal.
func (s *Server) cancelAndWaitBuild(ctx context.Context) error {
	s.mu.Lock()
	done := s.jobDone
	if s.jobCancel != nil {
		s.jobCancel()
	}
	s.mu.Unlock()
	if done != nil {
		select {
		case <-done:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return nil
}

func (s *Server) restoreRemoval() error {
	f, err := lifecycle.OpenMetadata(filepath.Join(s.dataDir, removalJournal))
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	defer f.Close()
	b, err := io.ReadAll(io.LimitReader(f, 1025))
	if err != nil || len(b) > 1024 {
		return fmt.Errorf("invalid removal intent")
	}
	d := json.NewDecoder(bytes.NewReader(b))
	d.DisallowUnknownFields()
	var in removalIntent
	if d.Decode(&in) != nil || d.Decode(&struct{}{}) != io.EOF || in.Schema != 1 || !managedID.MatchString(in.Repository) {
		return fmt.Errorf("invalid removal intent")
	}
	s.removing = in.Repository
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()
	if err = s.finishRemoval(ctx); err != nil {
		s.managementError = "Repository removal needs recovery. Retry Remove; maintenance and evidence reads are paused."
	}
	return nil
}

// Intent precedes every mutation. Recovery repeats this idempotent withdrawal
// before restoring jobs, so neither old setup nor a late worker can resurrect ID.
func (s *Server) finishRemoval(ctx context.Context) error {
	s.mu.Lock()
	id := s.removing
	next := cloneSetup(s.setup)
	if len(approvedIDs(next)) > 0 {
		if err := mirror.ValidatePurgeSourceBoundaries(approvedAssetSources(next), s.dataDir, id); err != nil {
			s.mu.Unlock()
			return err
		}
	}
	next.LocalRepositories = slices.DeleteFunc(next.LocalRepositories, func(r adapter.LocalRepository) bool { return r.ID == id })
	next.Repositories = slices.DeleteFunc(next.Repositories, func(r mirror.Repository) bool { return r.ID == id })
	delete(next.Rules, id)
	cfg, _, err := managementConfig(next)
	if err != nil {
		s.mu.Unlock()
		return err
	}
	next.Active.Sources = slices.DeleteFunc(next.Active.Sources, func(r catalog.Source) bool { return r.ID == id })
	next.Active.Repositories = slices.DeleteFunc(next.Active.Repositories, func(r model.Repository) bool { return r.ID == id })
	next.State = "maintaining"
	next.Error = ""
	next.Completed = 0
	if len(approvedIDs(next)) == 0 {
		next.State = "unconfigured"
		next.Active = cfg
	}
	s.setup = next
	s.read = knowledge.New(next.Active, s.db)
	err = s.persistSetup()
	s.activity = slices.DeleteFunc(s.activity, func(e ActivityEvent) bool { return e.Repository == id })
	s.mu.Unlock()
	if err != nil {
		return err
	}
	db, err := store.OpenWriter(s.dataDir)
	if err != nil {
		return err
	}
	defer db.Close()
	if err = db.PurgeRepository(ctx, id); err == nil {
		err = db.CompactPurgedData(ctx)
	}
	if err != nil {
		return err
	}
	if err = lifecycle.PurgeRepositoryAssets(ctx, s.dataDir, id); err != nil {
		return err
	}
	// New durably removes jobs absent from the approved source list, including
	// the last repository. Do not launch a worker during removal recovery.
	sources := []maintenance.Source{}
	for _, r := range next.LocalRepositories {
		sources = append(sources, maintenance.Source{ID: r.ID, Mode: "direct", Path: r.Path})
	}
	for _, r := range next.Repositories {
		sources = append(sources, maintenance.Source{ID: r.ID, Mode: "mirror"})
	}
	e, err := maintenance.New(s.dataDir, sources, func(context.Context, string) (maintenance.Outcome, error) {
		return maintenance.Outcome{}, fmt.Errorf("recovery has no worker")
	}, maintenance.Defaults())
	if err != nil {
		return err
	}
	e.Close()
	if err = os.Remove(filepath.Join(s.dataDir, removalJournal)); err != nil {
		return err
	}
	if err = syncDirectory(s.dataDir); err != nil {
		return err
	}
	s.mu.Lock()
	s.removing = ""
	s.managementError = ""
	s.mu.Unlock()
	return nil
}

func (s *Server) managementAPI(w http.ResponseWriter, r *http.Request) {
	read := r.URL.Path == "/api/v1/repositories/purge-status"
	if (read && r.Method != http.MethodGet) || (!read && r.Method != http.MethodPost) {
		fail(w, 405, "unsupported repository method")
		return
	}
	if !s.authorised(r, !read) {
		fail(w, 403, "valid session, origin and CSRF required")
		return
	}
	if read {
		id := r.URL.Query().Get("repository")
		if !managedID.MatchString(id) {
			fail(w, 400, "invalid repository ID")
			return
		}
		n, err := s.db.RepositoryOwnedRecords(r.Context(), id)
		if err != nil {
			fail(w, 503, "unable to verify canonical purge")
			return
		}
		for _, rel := range []string{filepath.Join("snapshots", id), filepath.Join("mirrors", id+".git")} {
			if _, err = os.Lstat(filepath.Join(s.dataDir, rel)); err == nil {
				n++
			} else if !os.IsNotExist(err) {
				fail(w, 503, "unable to verify owned assets")
				return
			}
		}
		s.mu.Lock()
		pending := s.removing == id
		approved := slices.Contains(approvedIDs(s.setup), id)
		s.mu.Unlock()
		if pending || approved {
			n++
		}
		jsonBody(w, map[string]any{"repository": id, "owned_records": n, "pending": pending})
		return
	}
	if r.URL.Path == "/api/v1/repositories/add" {
		s.addRepository(w, r)
		return
	}
	var in struct {
		Repository string      `json:"repository"`
		Rules      *ScopeRules `json:"rules,omitempty"`
	}
	if decode(r, &in) != nil || (in.Repository != "" && !managedID.MatchString(in.Repository)) {
		fail(w, 400, "invalid repository request")
		return
	}
	if in.Rules != nil && r.URL.Path != "/api/v1/repositories/rules" {
		fail(w, 400, "rules require the scope endpoint")
		return
	}
	s.setupTransitionMu.Lock()
	s.mu.Lock()
	setup := cloneSetup(s.setup)
	pending := s.removing
	s.mu.Unlock()
	global := r.URL.Path == "/api/v1/repositories/rebuild" && in.Repository == ""
	if !global && !slices.Contains(approvedIDs(setup), in.Repository) && !(pending != "" && pending == in.Repository) {
		s.setupTransitionMu.Unlock()
		fail(w, 404, "repository is not approved")
		return
	}
	if pending != "" && !(r.URL.Path == "/api/v1/repositories/remove" && pending == in.Repository) {
		s.setupTransitionMu.Unlock()
		fail(w, 409, "finish pending repository removal first")
		return
	}
	if len(setup.Active.Sources) > 0 && sourceMode(setup.ActiveMode) != sourceMode(setup.Mode) && r.URL.Path != "/api/v1/repositories/remove" && !global {
		s.setupTransitionMu.Unlock()
		fail(w, 409, "Build the whole selected source mode before managing individual repositories")
		return
	}
	if r.URL.Path == "/api/v1/repositories/retry" {
		s.maintenanceMu.Lock()
		e := s.maintainer
		var err error
		if e == nil {
			err = fmt.Errorf("maintenance paused")
		} else {
			err = e.Request(in.Repository, "check_now")
		}
		s.maintenanceMu.Unlock()
		s.setupTransitionMu.Unlock()
		if err != nil {
			fail(w, 409, "Build or finish recovery before retrying this repository")
			return
		}
		w.WriteHeader(202)
		jsonBody(w, map[string]string{"state": "queued"})
		return
	}
	s.stopMaintenance()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()
	if err := s.cancelAndWaitBuild(ctx); err != nil {
		s.setupTransitionMu.Unlock()
		fail(w, 503, "build cancellation has not completed")
		return
	}
	if r.URL.Path == "/api/v1/repositories/remove" {
		if len(approvedIDs(setup)) > 0 {
			if err := mirror.ValidatePurgeSourceBoundaries(approvedAssetSources(setup), s.dataDir, in.Repository); err != nil {
				s.setupTransitionMu.Unlock()
				fail(w, 409, "An approved source overlaps owned purge paths. Correct its approved path or URL before removal; no assets were deleted.")
				return
			}
		}
		if pending == "" {
			if err := writeRemoval(s.dataDir, in.Repository); err != nil {
				s.setupTransitionMu.Unlock()
				fail(w, 500, "unable to persist removal intent")
				return
			}
			s.mu.Lock()
			s.removing = in.Repository
			s.mu.Unlock()
		}
		err := s.finishRemoval(ctx)
		if err != nil {
			s.mu.Lock()
			s.managementError = "Repository removal needs recovery. Retry Remove; maintenance and evidence reads are paused."
			s.mu.Unlock()
		}
		s.setupTransitionMu.Unlock()
		if err != nil {
			fail(w, 503, "Removal recorded; owned data cleanup needs recovery. Retry Remove to finish.")
			return
		}
		s.startMaintenance()
		jsonBody(w, map[string]string{"state": "removed"})
		return
	}
	if r.URL.Path == "/api/v1/repositories/rules" {
		if in.Rules == nil {
			s.setupTransitionMu.Unlock()
			s.startMaintenance()
			fail(w, 400, "scope rules required")
			return
		}
		if setup.Rules == nil {
			setup.Rules = map[string]ScopeRules{}
		}
		setup.Rules[in.Repository] = *in.Rules
		if _, _, err := managementConfig(setup); err != nil {
			s.setupTransitionMu.Unlock()
			s.startMaintenance()
			fail(w, 400, "invalid include/exclude/language scope")
			return
		}
		s.mu.Lock()
		old := s.setup
		s.setup = setup
		s.setup.State = "maintaining"
		err := s.persistSetup()
		if err != nil {
			s.setup = old
		}
		s.mu.Unlock()
		s.setupTransitionMu.Unlock()
		s.startMaintenance()
		if err != nil {
			fail(w, 500, "unable to save repository scope")
			return
		}
		w.WriteHeader(202)
		jsonBody(w, map[string]string{"state": "queued"})
		return
	}
	// Rebuild runs fresh extraction; the full approved catalog validates once
	// before atomic promotion. A scoped rebuild leaves other inputs untouched.
	if len(approvedIDs(setup)) == 0 {
		s.setupTransitionMu.Unlock()
		fail(w, 409, "add a repository before rebuilding")
		return
	}
	cfg, reg, err := managementConfig(setup)
	if err != nil {
		s.setupTransitionMu.Unlock()
		s.startMaintenance()
		fail(w, 400, "invalid approved configuration")
		return
	}
	jobCtx, jobCancel := context.WithTimeout(context.Background(), 15*time.Minute)
	jobCtx = app.WithProgress(app.WithFullRebuild(jobCtx), s.recordProgress)
	s.mu.Lock()
	s.jobCancel = jobCancel
	s.jobDone = make(chan struct{})
	s.setup.State = "ingesting"
	s.setup.Error = ""
	err = s.persistSetup()
	if err != nil {
		jobCancel()
		s.jobCancel = nil
		close(s.jobDone)
		s.jobDone = nil
		s.setup = setup
	}
	s.mu.Unlock()
	if err != nil {
		s.setupTransitionMu.Unlock()
		s.startMaintenance()
		fail(w, 500, "unable to persist rebuild request")
		return
	}
	go s.runManagedRebuild(jobCtx, cfg, reg, setup, in.Repository)
	s.setupTransitionMu.Unlock()
	w.WriteHeader(202)
	jsonBody(w, map[string]string{"state": "rebuilding"})
}

func (s *Server) runManagedRebuild(ctx context.Context, cfg catalog.Config, reg mirror.Registry, setup Setup, id string) {
	if id == "" {
		s.runSetup(ctx, cfg, reg, setup.LocalRepositories)
		return
	}
	var err error
	var discovery adapter.Discovery
	if setup.Mode == "local" {
		for _, entry := range setup.LocalRepositories {
			if entry.ID == id {
				for _, repo := range cfg.SourceRepositories() {
					if repo.ID == id {
						discovery, err = adapter.CaptureLocalScopedWithPrepare(ctx, entry, s.dataDir, cfg.Limits, repo, func(ctx context.Context, root string) error { return app.QueueOwnedSnapshot(ctx, s.dataDir, root) })
					}
				}
				break
			}
		}
	} else {
		for _, entry := range reg.Repositories {
			if entry.ID == id {
				one := mirror.Registry{Version: 1, Repositories: []mirror.Repository{entry}}
				if _, err = mirror.Sync(ctx, one, s.dataDir); err == nil {
					discovery, err = (adapter.RepositoryGit{Registry: one, DataDir: s.dataDir, BeforeSnapshot: func(ctx context.Context, root string) error { return app.QueueOwnedSnapshot(ctx, s.dataDir, root) }}).Discover(ctx, id)
				}
				break
			}
		}
	}
	if err == nil {
		_, err = app.IngestMaintained(ctx, cfg, discovery, mirror.Fingerprint(reg)+"-force", s.dataDir)
	}
	s.finishSetup(ctx, cfg, err)
}

func (s *Server) addRepository(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Local  *adapter.LocalRepository `json:"local_repository,omitempty"`
		Mirror *mirror.Repository       `json:"mirror_repository,omitempty"`
		Rules  ScopeRules               `json:"rules"`
	}
	if decode(r, &in) != nil || (in.Local == nil) == (in.Mirror == nil) {
		fail(w, 400, "choose one source matching the configured mode")
		return
	}
	s.setupTransitionMu.Lock()
	s.mu.Lock()
	next := cloneSetup(s.setup)
	pending := s.removing
	running := s.jobCancel != nil
	s.mu.Unlock()
	if pending != "" || running || (len(next.Active.Sources) > 0 && sourceMode(next.ActiveMode) != sourceMode(next.Mode)) {
		s.setupTransitionMu.Unlock()
		fail(w, 409, "wait for Build or removal recovery")
		return
	}
	var id string
	if in.Local != nil && next.Mode == "local" {
		id = in.Local.ID
		if err := adapter.ValidateLocalRegistry(adapter.LocalRegistry{Version: 1, Repositories: []adapter.LocalRepository{*in.Local}}); err != nil {
			s.setupTransitionMu.Unlock()
			fail(w, 400, "invalid Direct source")
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
		_, err := adapter.InspectLocal(ctx, *in.Local, s.dataDir)
		cancel()
		if err != nil {
			s.setupTransitionMu.Unlock()
			fail(w, 400, "Direct source must be an accessible workspace outside owned data")
			return
		}
		next.LocalRepositories = append(next.LocalRepositories, *in.Local)
	} else if in.Mirror != nil && next.Mode != "local" {
		id = in.Mirror.ID
		next.Repositories = append(next.Repositories, *in.Mirror)
	} else {
		s.setupTransitionMu.Unlock()
		fail(w, 400, "use the globally configured source mode")
		return
	}
	if next.Rules == nil {
		next.Rules = map[string]ScopeRules{}
	}
	next.Rules[id] = in.Rules
	cfg, _, err := managementConfig(next)
	if err == nil && next.Mode != "local" {
		err = lifecycle.ValidateInstallationSourceBoundaries(mirror.Registry{Version: 1, Repositories: next.Repositories}, s.dataDir)
	}
	if err != nil {
		s.setupTransitionMu.Unlock()
		fail(w, 400, "invalid source, duplicate ID or scope; at most 100 repositories")
		return
	}
	s.stopMaintenance()
	s.mu.Lock()
	old := s.setup
	next.State = "maintaining"
	next.Active = cfg
	next.ActiveMode = next.Mode
	s.setup = next
	err = s.persistSetup()
	if err != nil {
		s.setup = old
	} else {
		s.read = knowledge.New(cfg, s.db)
	}
	s.mu.Unlock()
	s.setupTransitionMu.Unlock()
	s.startMaintenance()
	if err != nil {
		fail(w, 500, "unable to persist approved source")
		return
	}
	w.WriteHeader(202)
	jsonBody(w, map[string]string{"state": "queued", "repository": id})
}
