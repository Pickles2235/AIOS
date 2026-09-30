package catalog

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/AdamNi-7080/AIOS/internal/model"
)

type Config struct {
	Version              int                   `json:"version"`
	Sources              []Source              `json:"sources"`
	Limits               model.Limits          `json:"limits"`
	CompilerRuntime      model.CompilerRuntime `json:"compiler_runtime,omitempty"`
	RetentionGenerations int                   `json:"retention_generations,omitempty"`
	Vector               VectorConfig          `json:"vector,omitempty"`
	Features             map[string]bool       `json:"features,omitempty"`
	// Repositories exists only for in-process unit fixtures. It is never read
	// from configuration and is not a supported product configuration surface.
	Repositories []model.Repository `json:"-"`
}

// Source is deliberately no broader than the repository adapter needs in V1.
type Source struct {
	Kind      string            `json:"kind"`
	ID        string            `json:"id"`
	Include   []string          `json:"include,omitempty"`
	Exclude   []string          `json:"exclude,omitempty"`
	Ownership []model.Ownership `json:"ownership,omitempty"`
}

func (c Config) SourceRepositories() []model.Repository {
	if len(c.Sources) == 0 {
		return c.Repositories
	}
	out := make([]model.Repository, 0, len(c.Sources))
	for _, source := range c.Sources {
		out = append(out, model.Repository{ID: source.ID, Include: source.Include, Exclude: source.Exclude, Ownership: source.Ownership})
	}
	return out
}

// VectorConfig is intentionally small: models are bundled by the executable,
// never fetched from a catalog or a remote endpoint.
type VectorConfig struct {
	Enabled    bool `json:"enabled,omitempty"`
	Dimensions int  `json:"dimensions,omitempty"`
}

var idPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{0,62}$`)

const (
	MaxCatalogBytes    = 1 << 20
	MaxRepositories    = 100
	MaxPatternsPerRepo = 64
	MaxPatternBytes    = 256
)

func Defaults() model.Limits {
	return model.Limits{
		MaxFileBytes:         1 << 20,
		MaxFilesPerRepo:      20_000,
		MaxTotalBytesPerRepo: 256 << 20,
		MaxResults:           100,
	}
}

func Load(path string) (Config, error) {
	file, err := os.Open(path)
	if err != nil {
		return Config{}, fmt.Errorf("read catalog: %w", err)
	}
	defer file.Close()
	b, err := io.ReadAll(io.LimitReader(file, MaxCatalogBytes+1))
	if err != nil {
		return Config{}, fmt.Errorf("read catalog: %w", err)
	}
	if len(b) > MaxCatalogBytes {
		return Config{}, fmt.Errorf("catalog exceeds %d bytes", MaxCatalogBytes)
	}
	var cfg Config
	dec := json.NewDecoder(strings.NewReader(string(b)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&cfg); err != nil {
		return Config{}, fmt.Errorf("decode catalog: %w", err)
	}
	if err := dec.Decode(&struct{}{}); err != io.EOF {
		return Config{}, errors.New("decode catalog: expected exactly one JSON object")
	}
	if cfg.Limits == (model.Limits{}) {
		cfg.Limits = Defaults()
	}
	if cfg.RetentionGenerations == 0 {
		cfg.RetentionGenerations = 3
	}
	if err := Validate(cfg); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func Validate(cfg Config) error {
	if cfg.Version != 1 {
		return fmt.Errorf("catalog version %d is unsupported: AIOS is repository-only V1; use version 1 sources", cfg.Version)
	}
	if len(cfg.SourceRepositories()) == 0 {
		return fmt.Errorf("V1 catalog must declare between 1 and %d repository sources", MaxRepositories)
	}
	for feature, enabled := range cfg.Features {
		if enabled {
			return fmt.Errorf("feature %q is not available in V1; future-release capabilities are disabled", feature)
		}
	}
	if cfg.RetentionGenerations < 0 || cfg.RetentionGenerations > 100 {
		return errors.New("retention_generations must be between 0 and 100")
	}
	if cfg.Vector.Dimensions != 0 && (cfg.Vector.Dimensions < 8 || cfg.Vector.Dimensions > 4096) {
		return errors.New("vector.dimensions must be between 8 and 4096")
	}
	repositories := cfg.SourceRepositories()
	if len(repositories) > MaxRepositories {
		return fmt.Errorf("catalog must not declare more than %d repositories", MaxRepositories)
	}
	if cfg.Limits.MaxFileBytes <= 0 || cfg.Limits.MaxFileBytes > 16<<20 {
		return errors.New("max_file_bytes must be between 1 and 16777216")
	}
	if cfg.Limits.MaxFilesPerRepo <= 0 || cfg.Limits.MaxFilesPerRepo > 200_000 {
		return errors.New("max_files_per_repo must be between 1 and 200000")
	}
	if cfg.Limits.MaxTotalBytesPerRepo <= 0 || cfg.Limits.MaxTotalBytesPerRepo > 2<<30 {
		return errors.New("max_total_bytes_per_repo must be between 1 and 2147483648")
	}
	if cfg.Limits.MaxResults <= 0 || cfg.Limits.MaxResults > 500 {
		return errors.New("max_results must be between 1 and 500")
	}
	seen := make(map[string]bool)
	for i, repo := range repositories {
		if len(cfg.Sources) > 0 && cfg.Sources[i].Kind != model.SourceKindRepository {
			return fmt.Errorf("sources[%d].kind %q is unsupported in V1; only %q is enabled", i, cfg.Sources[i].Kind, model.SourceKindRepository)
		}
		if !idPattern.MatchString(repo.ID) {
			return fmt.Errorf("repositories[%d].id is invalid", i)
		}
		if seen[repo.ID] {
			return fmt.Errorf("duplicate repository id %q", repo.ID)
		}
		seen[repo.ID] = true
		if repo.Root != "" {
			if !filepath.IsAbs(repo.Root) || filepath.Clean(repo.Root) != repo.Root {
				return fmt.Errorf("repository %q root must be a canonical absolute path", repo.ID)
			}
			info, err := os.Lstat(repo.Root)
			if err != nil {
				return fmt.Errorf("repository %q root: %w", repo.ID, err)
			}
			if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
				return fmt.Errorf("repository %q root must be a real directory, not a symlink", repo.ID)
			}
			resolved, err := filepath.EvalSymlinks(repo.Root)
			if err != nil || resolved != repo.Root {
				return fmt.Errorf("repository %q root contains a symlink or is not canonical", repo.ID)
			}
		}
		patterns := append(append([]string{}, repo.Include...), repo.Exclude...)
		if len(patterns) > MaxPatternsPerRepo {
			return fmt.Errorf("repository %q must not declare more than %d include/exclude patterns", repo.ID, MaxPatternsPerRepo)
		}
		for _, pattern := range patterns {
			if len(pattern) > MaxPatternBytes {
				return fmt.Errorf("repository %q pattern exceeds %d bytes", repo.ID, MaxPatternBytes)
			}
			if filepath.IsAbs(pattern) || strings.Contains(pattern, "..") {
				return fmt.Errorf("repository %q has unsafe pattern %q", repo.ID, pattern)
			}
		}
		seenOwnership := map[string]bool{}
		for _, ownership := range repo.Ownership {
			if ownership.Coordinate == "" || len(ownership.Coordinate) > 256 || strings.ContainsAny(ownership.Coordinate, "\n\r") {
				return fmt.Errorf("repository %q has invalid ownership coordinate", repo.ID)
			}
			if ownership.Owner == "" || len(ownership.Owner) > 256 || strings.ContainsAny(ownership.Owner, "\n\r") {
				return fmt.Errorf("repository %q has invalid ownership owner", repo.ID)
			}
			if seenOwnership[ownership.Coordinate] {
				return fmt.Errorf("repository %q has conflicting ownership coordinate %q", repo.ID, ownership.Coordinate)
			}
			seenOwnership[ownership.Coordinate] = true
		}
	}
	for name, path := range map[string]string{"java_home": cfg.CompilerRuntime.JavaHome, "node": cfg.CompilerRuntime.Node, "typescript_module": cfg.CompilerRuntime.TypeScriptModule} {
		if path == "" {
			continue
		}
		if !filepath.IsAbs(path) || filepath.Clean(path) != path {
			return fmt.Errorf("compiler_runtime.%s must be a canonical absolute path", name)
		}
		info, err := os.Stat(path)
		if err != nil {
			return fmt.Errorf("compiler_runtime.%s: %w", name, err)
		}
		if name == "java_home" && !info.IsDir() {
			return fmt.Errorf("compiler_runtime.%s must be a directory", name)
		}
		if name != "java_home" && info.IsDir() {
			return fmt.Errorf("compiler_runtime.%s must be a file", name)
		}
		for _, repo := range repositories {
			if repo.Root == "" {
				continue
			}
			if rel, err := filepath.Rel(repo.Root, path); err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
				return fmt.Errorf("compiler_runtime.%s must be agent-owned, not inside repository %q", name, repo.ID)
			}
		}
	}
	return nil
}

func Select(cfg Config, id string) ([]model.Repository, error) {
	repositories := cfg.SourceRepositories()
	if id == "" {
		return repositories, nil
	}
	for _, repo := range repositories {
		if repo.ID == id {
			return []model.Repository{repo}, nil
		}
	}
	return nil, fmt.Errorf("repository %q is not in the catalog", id)
}
