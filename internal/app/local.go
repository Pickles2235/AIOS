package app

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/AdamNi-7080/AIOS/internal/adapter"
	"github.com/AdamNi-7080/AIOS/internal/catalog"
	"github.com/AdamNi-7080/AIOS/internal/discover"
	"github.com/AdamNi-7080/AIOS/internal/model"
	"github.com/AdamNi-7080/AIOS/internal/store"
)

// IngestLocal compiles approved immutable local snapshots, retaining coherent catalog activation.
func IngestLocal(ctx context.Context, cfg catalog.Config, reg adapter.LocalRegistry, dataDir, repositoryID string) (out IndexResult, retErr error) {
	if err := catalog.Validate(cfg); err != nil {
		return out, err
	}
	if err := adapter.ValidateLocalRegistry(reg); err != nil {
		return out, err
	}
	repos := cfg.SourceRepositories()
	if len(repos) != len(reg.Repositories) {
		return out, fmt.Errorf("catalog and local registry counts differ")
	}
	byID := map[string]adapter.LocalRepository{}
	for _, entry := range reg.Repositories {
		byID[entry.ID] = entry
	}
	for _, repo := range repos {
		entry, ok := byID[repo.ID]
		if !ok {
			return out, fmt.Errorf("repository %q is not in the approved local registry", repo.ID)
		}
		if _, err := adapter.InspectLocal(ctx, entry, dataDir); err != nil {
			return out, fmt.Errorf("repository %s: %w", repo.ID, err)
		}
	}
	selected, err := catalog.Select(cfg, repositoryID)
	if err != nil {
		return out, err
	}
	db, err := store.OpenWriter(dataDir)
	if err != nil {
		return out, err
	}
	defer db.Close()
	defer func() {
		finishSnapshotRetention(db, dataDir, cfg, &out, retErr == nil)
	}()
	ctx, finishObservation := observeIngest(ctx, dataDir, repositoryID)
	defer func() { finishObservation(retErr) }()
	if err = db.ReplaceApprovedOwnership(ctx, repos); err != nil {
		return out, err
	}
	manifest, _ := json.Marshal(struct {
		Config   catalog.Config
		Registry adapter.LocalRegistry
	}{cfg, reg})
	manifestHash := sha256.Sum256(manifest)
	fingerprint := hex.EncodeToString(manifestHash[:])
	revisions := map[string]model.GitState{}
	coverage := map[string]model.CoverageReport{}
	selections := []store.ActivationSelection{}
	pending := []selectedRevision{}
	defer func() {
		if retErr != nil {
			for _, item := range pending {
				_ = db.FailRevision(context.Background(), item.repositoryID, item.revision, item.fingerprint, "local_revision_failed")
			}
		}
	}()
	for i, repo := range selected {
		discovery, e := adapter.CaptureLocalScoped(ctx, byID[repo.ID], dataDir, cfg.Limits, repo)
		if e != nil {
			return out, e
		}
		defer discovery.Close()
		if e = db.QueueSnapshotGC(ctx, discovery.Root); e != nil {
			return out, e
		}
		coverage[repo.ID] = discovery.Coverage
		if _, e = db.Discover(ctx, repo.ID, discovery.Revision, fingerprint); e != nil {
			return out, e
		}
		// A batch may contain unchanged members. Keep those immutable inputs in
		// the coherent catalog without selecting an already-completed revision.
		// index still compares scoped files, so changed inclusion rules rebuild.
		queues, e := db.IngestionStatus(ctx, repo.ID)
		if e != nil {
			return out, e
		}
		if !fullRebuild(ctx) && len(queues) == 1 && queues[0].CurrentRevision == discovery.Revision && queues[0].PendingRevision == "" && queues[0].ManifestFingerprint == fingerprint && queues[0].State == "completed" {
			selected[i].Root = discovery.Root
			revisions[repo.ID] = discovery.Git
			continue
		}
		q, e := db.SelectRepairRevision(ctx, repo.ID, discovery.Revision, fingerprint)
		if e != nil {
			return out, e
		}
		pending = append(pending, selectedRevision{repo.ID, discovery.Revision, fingerprint})
		if e = db.DiscardStagedRepository(ctx, repo.ID); e != nil {
			return out, e
		}
		capturedRepo := repo
		capturedRepo.Root = discovery.Root
		files, e := discover.Files(capturedRepo, cfg.Limits)
		if e != nil {
			return out, e
		}
		previous, e := db.ActiveFiles(ctx, repo.ID)
		if e != nil && !errors.Is(e, sql.ErrNoRows) {
			return out, e
		}
		delta, e := db.RecordSourceDelta(ctx, repo.ID, q.CurrentRevision, discovery.Revision, fingerprint, "local_snapshot", discover.Diff(previous, files))
		if e != nil {
			return out, e
		}
		selections = append(selections, store.ActivationSelection{Repository: repo.ID, Revision: discovery.Revision, Fingerprint: fingerprint, Delta: delta})
		selected[i].Root = discovery.Root
		revisions[repo.ID] = discovery.Git
	}
	return indexMirrorRepositories(ctx, cfg, selected, revisions, db, dataDir, indexInputs{Coverage: coverage, Selections: selections})
}
