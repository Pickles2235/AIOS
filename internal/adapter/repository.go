// Package adapter defines the narrow source-to-canonical-IR boundary.
package adapter

import (
	"context"
	"fmt"

	"github.com/AdamNi-7080/AIOS/internal/catalog"
	"github.com/AdamNi-7080/AIOS/internal/mirror"
	"github.com/AdamNi-7080/AIOS/internal/model"
	"github.com/AdamNi-7080/AIOS/internal/snapshotlease"
)

// Discovery is an immutable, local source revision ready for deterministic
// extraction. It never represents a remote client, credential, or connector.
type Discovery struct {
	Identity model.SourceIdentity
	Revision string
	Root     string
	Changes  []model.FileChange
	Git      model.GitState
	Coverage model.CoverageReport
	Lease    *snapshotlease.Lease
}

func (d Discovery) Close() error { return d.Lease.Close() }

// SourceAdapter discovers a fixed approved revision. Extraction remains in
// the compiler pipeline so every adapter produces the same canonical IR.
type SourceAdapter interface {
	Kind() string
	Version() string
	Discover(context.Context, string) (Discovery, error)
}

// RepositoryGit adapts an approved, already-synchronised Git mirror. It never
// contacts a remote; `mirrors sync` is the sole network boundary.
type RepositoryGit struct {
	Registry mirror.Registry
	DataDir  string
	Limits   model.Limits
}

func (RepositoryGit) Kind() string    { return model.SourceKindRepository }
func (RepositoryGit) Version() string { return model.RepositoryAdapterVersion }

func (a RepositoryGit) Discover(ctx context.Context, id string) (Discovery, error) {
	var entry *mirror.Repository
	for i := range a.Registry.Repositories {
		if a.Registry.Repositories[i].ID == id {
			entry = &a.Registry.Repositories[i]
			break
		}
	}
	if entry == nil {
		return Discovery{}, fmt.Errorf("repository source %q is not in the approved mirror registry", id)
	}
	revision, err := mirror.ApprovedRevision(ctx, a.DataDir, *entry)
	if err != nil {
		return Discovery{}, err
	}
	fingerprint := mirror.Fingerprint(a.Registry)
	limits := a.Limits
	if limits == (model.Limits{}) {
		limits = catalog.Defaults()
	}
	expected := mirror.SnapshotPath(a.DataDir, id, revision, fingerprint)
	lease, err := snapshotlease.Acquire(a.DataDir, expected)
	if err != nil {
		return Discovery{}, err
	}
	root, err := mirror.Snapshot(ctx, mirror.MirrorPath(a.DataDir, id), a.DataDir, id, revision, fingerprint, limits.MaxTotalBytesPerRepo, limits.MaxFilesPerRepo)
	if err != nil {
		lease.Close()
		return Discovery{}, err
	}
	return Discovery{Identity: model.SourceIdentity{ID: id, Kind: a.Kind(), AdapterVersion: a.Version()}, Revision: revision, Root: root, Lease: lease}, nil
}
