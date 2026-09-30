package app

import (
	"context"
	"fmt"
	"github.com/AdamNi-7080/AIOS/internal/adapter"
	"github.com/AdamNi-7080/AIOS/internal/catalog"
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
	if err = db.ReplaceApprovedOwnership(ctx, repos); err != nil {
		return out, err
	}
	fingerprint := adapter.LocalFingerprint(reg)
	revisions := map[string]model.GitState{}
	pending := []selectedRevision{}
	defer func() {
		if retErr != nil {
			for _, item := range pending {
				_ = db.FailRevision(context.Background(), item.repositoryID, item.revision, item.fingerprint, retErr.Error())
			}
		}
	}()
	for i, repo := range selected {
		discovery, e := adapter.CaptureLocal(ctx, byID[repo.ID], dataDir, cfg.Limits)
		if e != nil {
			return out, e
		}
		if _, e = db.Discover(ctx, repo.ID, discovery.Revision, fingerprint); e != nil {
			return out, e
		}
		q, e := db.SelectRevision(ctx, repo.ID, discovery.Revision, fingerprint)
		if e != nil {
			return out, e
		}
		pending = append(pending, selectedRevision{repo.ID, discovery.Revision, fingerprint})
		if e = db.DiscardStagedRepository(ctx, repo.ID); e != nil {
			return out, e
		}
		if _, e = db.RecordSourceDelta(ctx, repo.ID, q.CurrentRevision, discovery.Revision, fingerprint, "local_snapshot", nil); e != nil {
			return out, e
		}
		selected[i].Root = discovery.Root
		revisions[repo.ID] = model.GitState{Commit: discovery.Revision, Branch: "captured-local-commit"}
	}
	out, err = indexMirrorRepositories(ctx, cfg, selected, revisions, db, dataDir)
	if err != nil {
		return out, err
	}
	for _, item := range pending {
		g, e := db.ActiveGeneration(ctx, item.repositoryID)
		if e != nil {
			return out, e
		}
		if e = db.FinalizeDelta(ctx, item.repositoryID, item.revision, item.fingerprint, g.ID); e != nil {
			return out, e
		}
		if e = db.CompleteRevision(ctx, item.repositoryID, item.revision, item.fingerprint); e != nil {
			return out, e
		}
	}
	return out, nil
}
