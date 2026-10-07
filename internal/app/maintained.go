package app

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/AdamNi-7080/AIOS/internal/adapter"
	"github.com/AdamNi-7080/AIOS/internal/catalog"
	"github.com/AdamNi-7080/AIOS/internal/discover"
	"github.com/AdamNi-7080/AIOS/internal/model"
	"github.com/AdamNi-7080/AIOS/internal/store"
)

// IngestMaintained updates one approved immutable input without inspecting
// broken unrelated source paths. Bootstrap may promote healthy members while
// failed members remain absent/unknown and retry independently.
func IngestMaintained(ctx context.Context, cfg catalog.Config, discovery adapter.Discovery, fingerprint, dataDir string) (out IndexResult, retErr error) {
	defer discovery.Close()
	if err := catalog.Validate(cfg); err != nil {
		return out, err
	}
	var selected model.Repository
	for _, repo := range cfg.SourceRepositories() {
		if repo.ID == discovery.Identity.ID {
			selected = repo
		}
	}
	if selected.ID == "" || discovery.Revision == "" || fingerprint == "" {
		return out, fmt.Errorf("maintained input is not approved")
	}
	owned, err := filepath.Abs(filepath.Join(dataDir, "snapshots", selected.ID))
	if err != nil {
		return out, err
	}
	real, err := filepath.EvalSymlinks(discovery.Root)
	if err != nil || real != discovery.Root {
		return out, fmt.Errorf("maintained snapshot must be canonical")
	}
	rel, err := filepath.Rel(owned, real)
	if err != nil || rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return out, fmt.Errorf("maintained snapshot must belong to the approved owned repository")
	}
	db, err := store.OpenWriter(dataDir)
	if err != nil {
		return out, err
	}
	defer db.Close()
	if err = db.QueueSnapshotGC(ctx, discovery.Root); err != nil {
		return out, err
	}
	defer func() {
		_ = discovery.Close()
		finishSnapshotRetention(db, dataDir, cfg, &out, retErr == nil)
	}()
	ctx, finishObservation := observeIngest(ctx, dataDir, selected.ID)
	defer func() { finishObservation(retErr) }()
	if err = db.ReplaceApprovedOwnership(ctx, cfg.SourceRepositories()); err != nil {
		return out, err
	}
	if _, err = db.Discover(ctx, selected.ID, discovery.Revision, fingerprint); err != nil {
		return out, err
	}
	queue, err := db.SelectRepairRevision(ctx, selected.ID, discovery.Revision, fingerprint)
	if err != nil {
		return out, err
	}
	defer func() {
		if retErr != nil {
			_ = db.FailRevision(context.Background(), selected.ID, discovery.Revision, fingerprint, "maintained_revision_failed")
		}
	}()
	if err = db.DiscardStagedRepository(ctx, selected.ID); err != nil {
		return out, err
	}
	// Preserve all existing canonical members without requiring unavailable
	// workspaces. Keep full approval separate from the currently active subset.
	active := cfg
	active.Sources = nil
	active.Repositories = nil
	for _, source := range cfg.Sources {
		if source.ID == selected.ID {
			active.Sources = append(active.Sources, source)
			continue
		}
		if _, e := db.ActiveGeneration(ctx, source.ID); e == nil {
			active.Sources = append(active.Sources, source)
		} else if !errors.Is(e, sql.ErrNoRows) {
			return out, e
		}
	}
	if len(cfg.Sources) == 0 {
		for _, repo := range cfg.Repositories {
			if repo.ID == selected.ID {
				active.Repositories = append(active.Repositories, repo)
				continue
			}
			if _, e := db.ActiveGeneration(ctx, repo.ID); e == nil {
				active.Repositories = append(active.Repositories, repo)
			} else if !errors.Is(e, sql.ErrNoRows) {
				return out, e
			}
		}
	}
	selected.Root = discovery.Root
	files, err := discover.Files(selected, cfg.Limits)
	if err != nil {
		return out, err
	}
	previous, oldErr := db.ActiveFiles(ctx, selected.ID)
	if errors.Is(oldErr, sql.ErrNoRows) {
		previous = nil
	} else if oldErr != nil {
		return out, oldErr
	}
	delta, err := db.RecordSourceDelta(ctx, selected.ID, queue.CurrentRevision, discovery.Revision, fingerprint, "maintained_snapshot", discover.Diff(previous, files))
	if err != nil {
		return out, err
	}
	git := discovery.Git
	if git.Commit == "" {
		git = model.GitState{Commit: discovery.Revision, Branch: "captured-mirror-revision"}
	}
	return indexMirrorRepositories(ctx, active, []model.Repository{selected}, map[string]model.GitState{selected.ID: git}, db, dataDir, indexInputs{Coverage: map[string]model.CoverageReport{selected.ID: discovery.Coverage}, Selections: []store.ActivationSelection{{Repository: selected.ID, Revision: discovery.Revision, Fingerprint: fingerprint, Delta: delta}}})
}
