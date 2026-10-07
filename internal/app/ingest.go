package app

import (
	"context"
	"fmt"

	"github.com/AdamNi-7080/AIOS/internal/adapter"
	"github.com/AdamNi-7080/AIOS/internal/catalog"
	ingestrules "github.com/AdamNi-7080/AIOS/internal/ingest"
	"github.com/AdamNi-7080/AIOS/internal/mirror"
	"github.com/AdamNi-7080/AIOS/internal/model"
	"github.com/AdamNi-7080/AIOS/internal/store"
)

type selectedRevision struct {
	repositoryID string
	revision     string
	fingerprint  string
}

// IngestMirrorCatalog compiles every approved mirror revision and publishes
// the complete catalog in one activation. It is the only supported bootstrap
// path; subsequent updates use IngestMirrorRevision.
func IngestMirrorCatalog(ctx context.Context, configPath, registryPath, dataDir string) (out IndexResult, retErr error) {
	cfg, registry, repositories, err := loadMirrorInputs(configPath, registryPath)
	if err != nil {
		return IndexResult{}, err
	}
	db, err := store.OpenWriter(dataDir)
	if err != nil {
		return IndexResult{}, err
	}
	defer db.Close()
	ctx, finishObservation := observeIngest(ctx, dataDir, "catalog")
	defer func() { finishObservation(retErr) }()
	if err = db.ReplaceApprovedOwnership(ctx, repositories); err != nil {
		return IndexResult{}, err
	}
	fingerprint := mirror.Fingerprint(registry)
	selected := make([]selectedRevision, 0, len(repositories))
	selections := []store.ActivationSelection{}
	defer func() {
		if retErr == nil {
			return
		}
		for _, item := range selected {
			_ = db.FailRevision(context.Background(), item.repositoryID, item.revision, item.fingerprint, "mirror_revision_failed")
		}
	}()
	adapterSource := adapter.RepositoryGit{Registry: registry, DataDir: dataDir}
	revisions := make(map[string]model.GitState, len(repositories))
	for i := range repositories {
		repositoryID := repositories[i].ID
		entry := registry.Repositories[i]
		revision, e := mirror.ApprovedRevision(ctx, dataDir, entry)
		if e != nil {
			return IndexResult{}, e
		}
		if _, e = db.Discover(ctx, repositoryID, revision, fingerprint); e != nil {
			return IndexResult{}, e
		}
		queue, e := db.SelectRepairRevision(ctx, repositoryID, revision, fingerprint)
		if e != nil {
			return IndexResult{}, e
		}
		selected = append(selected, selectedRevision{repositoryID, revision, fingerprint})
		if e = db.DiscardStagedRepository(ctx, repositoryID); e != nil {
			return IndexResult{}, e
		}
		discovery, e := adapterSource.Discover(ctx, repositoryID)
		if e != nil {
			return IndexResult{}, e
		}
		changes, e := mirror.TreeChanges(ctx, mirror.MirrorPath(dataDir, repositoryID), queue.CurrentRevision, discovery.Revision)
		if e != nil {
			return IndexResult{}, e
		}
		scope := ingestrules.DecideRebuildScope(queue.CurrentRevision == "", false, changes)
		delta, e := db.RecordSourceDelta(ctx, repositoryID, queue.CurrentRevision, revision, fingerprint, scope.Kind, changes)
		if e != nil {
			return IndexResult{}, e
		}
		selections = append(selections, store.ActivationSelection{Repository: repositoryID, Revision: revision, Fingerprint: fingerprint, Delta: delta})
		repositories[i].Root = discovery.Root
		revisions[repositoryID] = model.GitState{Commit: revision, Branch: entry.Ref}
	}
	return indexMirrorRepositories(ctx, cfg, repositories, revisions, db, dataDir, indexInputs{Selections: selections})
}

func loadMirrorInputs(configPath, registryPath string) (catalog.Config, mirror.Registry, []model.Repository, error) {
	cfg, err := catalog.Load(configPath)
	if err != nil {
		return catalog.Config{}, mirror.Registry{}, nil, err
	}
	r, err := mirror.Load(registryPath)
	if err != nil {
		return catalog.Config{}, mirror.Registry{}, nil, err
	}
	repositories := cfg.SourceRepositories()
	if len(repositories) != len(r.Repositories) {
		return catalog.Config{}, mirror.Registry{}, nil, fmt.Errorf("catalog and mirror registry repository counts differ")
	}
	byID := make(map[string]mirror.Repository, len(r.Repositories))
	for _, entry := range r.Repositories {
		byID[entry.ID] = entry
	}
	ordered := make([]mirror.Repository, len(repositories))
	for i, repository := range repositories {
		entry, ok := byID[repository.ID]
		if !ok {
			return catalog.Config{}, mirror.Registry{}, nil, fmt.Errorf("repository %q is not in the approved mirror registry", repository.ID)
		}
		ordered[i] = entry
	}
	r.Repositories = ordered
	return cfg, r, repositories, nil
}

// IngestMirrorRevision compiles one already-synced approved mirror revision.
// A bootstrap is catalog-atomic: establish a complete active mirror-backed
// catalog before submitting single-repository deltas.
func IngestMirrorRevision(ctx context.Context, configPath, registryPath, dataDir, repositoryID string) (out IndexResult, retErr error) {
	cfg, r, repositories, err := loadMirrorInputs(configPath, registryPath)
	if err != nil {
		return IndexResult{}, err
	}
	var configured *int
	for i := range repositories {
		if repositories[i].ID == repositoryID {
			configured = &i
			break
		}
	}
	if configured == nil {
		return IndexResult{}, fmt.Errorf("repository %q is not in the catalog", repositoryID)
	}
	var mr *mirror.Repository
	for i := range r.Repositories {
		if r.Repositories[i].ID == repositoryID {
			mr = &r.Repositories[i]
			break
		}
	}
	if mr == nil {
		return IndexResult{}, fmt.Errorf("repository %q is not in the approved mirror registry", repositoryID)
	}
	revision, err := mirror.ApprovedRevision(ctx, dataDir, *mr)
	if err != nil {
		return IndexResult{}, err
	}
	fingerprint := mirror.Fingerprint(r)
	db, err := store.OpenWriter(dataDir)
	if err != nil {
		return IndexResult{}, err
	}
	defer db.Close()
	ctx, finishObservation := observeIngest(ctx, dataDir, repositoryID)
	defer func() { finishObservation(retErr) }()
	if err = db.ReplaceApprovedOwnership(ctx, repositories); err != nil {
		return IndexResult{}, err
	}
	if _, err = db.Discover(ctx, repositoryID, revision, fingerprint); err != nil {
		return IndexResult{}, err
	}
	q, err := db.SelectRevision(ctx, repositoryID, revision, fingerprint)
	if err != nil {
		return IndexResult{}, err
	}
	selected := true
	defer func() {
		if selected && retErr != nil {
			_ = db.FailRevision(context.Background(), repositoryID, revision, fingerprint, "mirror_revision_failed")
		}
	}()
	if err = db.DiscardStagedRepository(ctx, repositoryID); err != nil {
		return IndexResult{}, err
	}
	discovery, err := (adapter.RepositoryGit{Registry: r, DataDir: dataDir}).Discover(ctx, repositoryID)
	if err != nil {
		return IndexResult{}, err
	}
	changes, err := mirror.TreeChanges(ctx, mirror.MirrorPath(dataDir, repositoryID), q.CurrentRevision, discovery.Revision)
	if err != nil {
		return IndexResult{}, err
	}
	scope := ingestrules.DecideRebuildScope(q.CurrentRevision == "", false, changes)
	delta, err := db.RecordSourceDelta(ctx, repositoryID, q.CurrentRevision, revision, fingerprint, scope.Kind, changes)
	if err != nil {
		return IndexResult{}, err
	}
	repo := repositories[*configured]
	repo.Root = discovery.Root
	return indexMirrorRepositories(ctx, cfg, []model.Repository{repo}, map[string]model.GitState{repositoryID: {Commit: revision, Branch: mr.Ref}}, db, dataDir, indexInputs{Selections: []store.ActivationSelection{{Repository: repositoryID, Revision: revision, Fingerprint: fingerprint, Delta: delta}}})
}
