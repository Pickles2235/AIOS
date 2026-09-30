// Package app owns deterministic repository compilation.
package app

import (
	"context"
	"fmt"
	cachepkg "github.com/AdamNi-7080/AIOS/internal/cache"
	"github.com/AdamNi-7080/AIOS/internal/catalog"
	"github.com/AdamNi-7080/AIOS/internal/compiler"
	"github.com/AdamNi-7080/AIOS/internal/discover"
	"github.com/AdamNi-7080/AIOS/internal/extract"
	"github.com/AdamNi-7080/AIOS/internal/model"
	"github.com/AdamNi-7080/AIOS/internal/provenance"
	"github.com/AdamNi-7080/AIOS/internal/store"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

type IndexResult struct {
	Database   string                    `json:"database"`
	Snapshots  []model.Snapshot          `json:"snapshots"`
	Changes    map[string]int            `json:"changes,omitempty"`
	Exclusions map[string]map[string]int `json:"exclusions,omitempty"`
}

func index(ctx context.Context, cfg catalog.Config, repos []model.Repository, fixedGit map[string]model.GitState, db *store.Store, dataDir string) (IndexResult, error) {
	result := IndexResult{Database: db.Path(), Changes: map[string]int{}, Exclusions: map[string]map[string]int{}}
	staged := make([]string, 0, len(repos))
	changedCatalog := false
	changedRepositories := []string{}
	for _, repo := range repos {
		before, fixed := fixedGit[repo.ID]
		var err error
		if !fixed {
			before, err = provenance.Inspect(ctx, repo.Root)
			if err != nil {
				return result, fmt.Errorf("inspect %s: %w", repo.ID, err)
			}
		}
		files, coverage, err := discover.FilesWithCoverage(repo, cfg.Limits)
		if err != nil {
			return result, fmt.Errorf("discover %s: %w", repo.ID, err)
		}
		result.Exclusions[repo.ID] = map[string]int{}
		for _, entry := range coverage.Entries {
			if entry.Outcome != "included" {
				result.Exclusions[repo.ID][entry.Reason]++
			}
		}
		allFiles := files
		previous, previousErr := db.ActiveFiles(ctx, repo.ID)
		changes := discover.Diff(previous, allFiles)
		for _, change := range changes {
			result.Changes[change.Kind]++
		}
		var bytes int64
		for _, file := range allFiles {
			bytes += file.Size
		}
		snapshot := model.Snapshot{RepoID: repo.ID, Source: model.SourceIdentity{ID: repo.ID, Kind: model.SourceKindRepository, AdapterVersion: model.RepositoryAdapterVersion}, Root: repo.Root, Git: before, ContentHash: store.ContentSnapshot(allFiles), FileCount: len(allFiles), TotalBytes: bytes, IndexedAt: time.Now().UTC(), ExtractorVersions: model.ExtractorVersion}
		if previousErr == nil && len(changes) == 0 {
			generation, activeErr := db.ActiveGeneration(ctx, repo.ID)
			if activeErr != nil {
				return result, fmt.Errorf("read active generation %s: %w", repo.ID, activeErr)
			}
			staged = append(staged, generation.ID)
			result.Snapshots = append(result.Snapshots, snapshot)
			continue
		}
		changedCatalog = true
		changedRepositories = append(changedRepositories, repo.ID)
		var symbols []model.Symbol
		var edges []model.Edge
		if previousErr == nil {
			oldSymbols, oldEdges, inputErr := db.PreviousInputs(ctx, repo.ID)
			if inputErr != nil {
				return result, fmt.Errorf("read active inputs %s: %w", repo.ID, inputErr)
			}
			affected := affectedPaths(changes, oldSymbols, oldEdges)
			for _, x := range oldSymbols {
				if !affected[x.Path] {
					symbols = append(symbols, x)
				}
			}
			for _, x := range oldEdges {
				if !affected[x.Path] {
					edges = append(edges, x)
				}
			}
			files = selectAffected(allFiles, affected)
		}
		for _, file := range files {
			s, e, parseErr := extract.Parse(file)
			if parseErr != nil {
				return result, fmt.Errorf("extract %s/%s: %w", repo.ID, file.Path, parseErr)
			}
			symbols = append(symbols, s...)
			edges = append(edges, e...)
		}
		compiled, compileErr := compiler.Run(ctx, repo, files, cfg.CompilerRuntime, dataDir)
		if compileErr != nil {
			return result, fmt.Errorf("compile %s: %w", repo.ID, compileErr)
		}
		symbols = append(symbols, compiled.Symbols...)
		edges = append(edges, compiled.Edges...)
		if fixed {
			afterFiles, _, inspectErr := discover.FilesWithCoverage(repo, cfg.Limits)
			if inspectErr != nil || store.ContentSnapshot(afterFiles) != store.ContentSnapshot(allFiles) {
				return result, fmt.Errorf("repository snapshot %s changed while indexing", repo.ID)
			}
		} else {
			after, inspectErr := provenance.Inspect(ctx, repo.Root)
			if inspectErr != nil || before != after {
				return result, fmt.Errorf("repository %s changed while indexing", repo.ID)
			}
		}
		diagnostics := make([]model.CompilerDiagnostic, 0, len(compiled.Diagnostics))
		for _, d := range compiled.Diagnostics {
			diagnostics = append(diagnostics, model.CompilerDiagnostic{Language: d.Language, Code: d.Code, Message: d.Message, Path: d.Path})
		}
		snapshot.CompilerDiagnostics = diagnostics
		generation, err := db.StageGenerationWithCoverage(ctx, snapshot, allFiles, symbols, edges, coverage)
		if err != nil {
			return result, fmt.Errorf("index %s: %w", repo.ID, err)
		}
		if err := db.StageInvalidations(ctx, generation.ID, changes); err != nil {
			return result, fmt.Errorf("stage invalidations %s: %w", repo.ID, err)
		}
		staged = append(staged, generation.ID)
		result.Snapshots = append(result.Snapshots, snapshot)
	}
	if !changedCatalog {
		return result, nil
	}
	// A repository-scoped ingestion still activates a coherent catalog. Reuse
	// only already-active immutable generations for every untouched repository.
	stagedRepos := map[string]bool{}
	for _, id := range staged {
		g, e := db.GenerationByID(ctx, id)
		if e != nil {
			return result, e
		}
		stagedRepos[g.RepoID] = true
	}
	for _, configured := range cfg.SourceRepositories() {
		if stagedRepos[configured.ID] {
			continue
		}
		g, e := db.ActiveGeneration(ctx, configured.ID)
		if e != nil {
			return result, fmt.Errorf("active generation required for untouched repository %s: %w", configured.ID, e)
		}
		staged = append(staged, g.ID)
	}
	if err := db.ActivateCatalog(ctx, staged); err != nil {
		return result, fmt.Errorf("activate catalog: %w", err)
	}
	// Vector indexing is optional derived work. Its failure must never retract a
	// complete canonical activation; vector queries will report it unavailable.
	if cfg.Vector.Enabled {
		_ = db.RebuildVectorProjection(ctx, store.VectorOptions{Enabled: true, Dimensions: cfg.Vector.Dimensions, DataDir: dataDir}, "catalog_activation")
	}
	// Cache invalidation is best-effort derived work. Read-time generation and
	// projection checks still reject stale entries if this cache database is
	// unavailable while the catalog activation succeeds.
	if c, err := cachepkg.Open(dataDir); err == nil {
		_ = c.Invalidate(ctx, "active_generation_changed", changedRepositories)
		_ = c.Close()
	}
	return result, nil
}

// IndexRepository compiles exactly one immutable agent-owned source root while
// retaining all other active catalog members at activation time.
func IndexRepository(ctx context.Context, cfg catalog.Config, repo model.Repository, db *store.Store, dataDir string) (IndexResult, error) {
	return index(ctx, cfg, []model.Repository{repo}, nil, db, dataDir)
}

func indexMirrorRepositories(ctx context.Context, cfg catalog.Config, repositories []model.Repository, revisions map[string]model.GitState, db *store.Store, dataDir string) (IndexResult, error) {
	return index(ctx, cfg, repositories, revisions, db, dataDir)
}

func selectAffected(files []model.File, affected map[string]bool) []model.File {
	out := make([]model.File, 0)
	for _, f := range files {
		if affected[f.Path] {
			out = append(out, f)
		}
	}
	return out
}

func affectedPaths(changes []model.FileChange, symbols []model.Symbol, edges []model.Edge) map[string]bool {
	affected := map[string]bool{}
	identities := map[string]bool{}
	for _, c := range changes {
		affected[c.Path] = true
		if c.OldPath != "" {
			affected[c.OldPath] = true
		}
		for _, s := range symbols {
			if s.Path == c.Path || s.Path == c.OldPath {
				identities[s.Identity] = true
			}
		}
	}
	for _, e := range edges {
		if identities[e.SourceIdentity] || identities[e.TargetIdentity] {
			affected[e.Path] = true
		}
	}
	return affected
}

type DoctorResult struct {
	OK           bool   `json:"ok"`
	GoVersion    string `json:"go_version"`
	OS           string `json:"os"`
	Architecture string `json:"architecture"`
	GitPath      string `json:"git_path"`
	Catalog      string `json:"catalog"`
	Database     string `json:"database,omitempty"`
}

func Doctor(configPath, dataDir string) (DoctorResult, error) {
	r := DoctorResult{GoVersion: runtime.Version(), OS: runtime.GOOS, Architecture: runtime.GOARCH, Catalog: configPath}
	git, err := exec.LookPath("git")
	if err != nil {
		return r, fmt.Errorf("native git is required: %w", err)
	}
	r.GitPath = git
	if _, err = catalog.Load(configPath); err != nil {
		return r, err
	}
	if dataDir != "" {
		db, e := store.OpenReadOnly(dataDir)
		if e != nil {
			return r, e
		}
		r.Database = db.Path()
		if e = db.Close(); e != nil {
			return r, e
		}
	}
	r.OK = true
	return r, nil
}
func within(root, candidate string) bool {
	rel, err := filepath.Rel(root, candidate)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}
