package catalog

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/AdamNi-7080/AIOS/internal/model"
)

func TestValidateCanonicalUniqueAndSymlink(t *testing.T) {
	root := canonicalTempDir(t)
	cfg := Config{Version: 1, Repositories: []model.Repository{{ID: "repo", Root: root}}, Limits: Defaults()}
	if err := Validate(cfg); err != nil {
		t.Fatalf("valid catalog: %v", err)
	}

	duplicate := cfg
	duplicate.Repositories = append(duplicate.Repositories, duplicate.Repositories[0])
	if err := Validate(duplicate); err == nil {
		t.Fatal("expected duplicate id rejection")
	}

	link := filepath.Join(filepath.Dir(root), filepath.Base(root)+"-link")
	if err := os.Symlink(root, link); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Remove(link) })
	cfg.Repositories[0].Root = link
	if err := Validate(cfg); err == nil {
		t.Fatal("expected symlink root rejection")
	}

	cfg.Repositories[0].Root = root + string(filepath.Separator) + ".." + string(filepath.Separator) + filepath.Base(root)
	if err := Validate(cfg); err == nil {
		t.Fatal("expected non-canonical root rejection")
	}
}

func TestValidateRequiresV1RepositorySourcesAndRejectsFutureScope(t *testing.T) {
	if err := Validate(Config{Version: 1, Limits: Defaults()}); err == nil || !strings.Contains(err.Error(), "between 1 and 100") {
		t.Fatalf("missing V1 estate rejection: %v", err)
	}
	if err := Validate(Config{Version: 2, Limits: Defaults()}); err == nil || !strings.Contains(err.Error(), "repository-only V1") {
		t.Fatalf("legacy version accepted: %v", err)
	}
	sources := make([]Source, 25)
	for i := range sources {
		sources[i] = Source{Kind: model.SourceKindRepository, ID: fmt.Sprintf("repo-%02d", i)}
	}
	if err := Validate(Config{Version: 1, Sources: sources, Limits: Defaults()}); err != nil {
		t.Fatal(err)
	}
	validSources := append([]Source(nil), sources...)
	sources[0].Kind = "jira"
	if err := Validate(Config{Version: 1, Sources: sources, Limits: Defaults()}); err == nil || !strings.Contains(err.Error(), "unsupported") {
		t.Fatalf("jira source accepted: %v", err)
	}
	if err := Validate(Config{Version: 1, Sources: validSources, Features: map[string]bool{"planning": true}, Limits: Defaults()}); err == nil || !strings.Contains(err.Error(), "future-release") {
		t.Fatalf("future feature accepted: %v", err)
	}
}

func TestValidateOwnershipMetadataIsExplicitAndUnambiguous(t *testing.T) {
	root := canonicalTempDir(t)
	cfg := Config{Version: 1, Repositories: []model.Repository{{ID: "repo", Root: root, Ownership: []model.Ownership{{Coordinate: "repository", Owner: "platform"}}}}, Limits: Defaults()}
	if err := Validate(cfg); err != nil {
		t.Fatal(err)
	}
	cfg.Repositories[0].Ownership = append(cfg.Repositories[0].Ownership, model.Ownership{Coordinate: "repository", Owner: "another"})
	if err := Validate(cfg); err == nil {
		t.Fatal("conflicting ownership accepted")
	}
}

func TestValidateHardCatalogAndPatternLimits(t *testing.T) {
	root := canonicalTempDir(t)
	cfg := Config{Version: 1, Limits: Defaults()}
	for i := 0; i <= MaxRepositories; i++ {
		cfg.Repositories = append(cfg.Repositories, model.Repository{ID: "repo-" + strings.Repeat("x", i%20) + string(rune('a'+i%26)), Root: root})
	}
	// Ensure unique IDs without relying on filesystem fixtures.
	for i := range cfg.Repositories {
		cfg.Repositories[i].ID = "repo-" + fmt.Sprintf("%03d", i)
	}
	if err := Validate(cfg); err == nil || !strings.Contains(err.Error(), "more than") {
		t.Fatalf("expected repository cap rejection, got %v", err)
	}

	cfg.Repositories = cfg.Repositories[:1]
	cfg.Repositories[0].Include = make([]string, MaxPatternsPerRepo+1)
	for i := range cfg.Repositories[0].Include {
		cfg.Repositories[0].Include[i] = "*.ts"
	}
	if err := Validate(cfg); err == nil || !strings.Contains(err.Error(), "patterns") {
		t.Fatalf("expected pattern count rejection, got %v", err)
	}

	cfg.Repositories[0].Include = []string{strings.Repeat("x", MaxPatternBytes+1)}
	if err := Validate(cfg); err == nil || !strings.Contains(err.Error(), "exceeds") {
		t.Fatalf("expected pattern byte cap rejection, got %v", err)
	}
}

func TestLoadRejectsOversizedCatalog(t *testing.T) {
	path := filepath.Join(t.TempDir(), "catalog.json")
	if err := os.WriteFile(path, []byte(strings.Repeat(" ", MaxCatalogBytes+1)), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(path); err == nil || !strings.Contains(err.Error(), "exceeds") {
		t.Fatalf("expected catalog size rejection, got %v", err)
	}
}

func TestLoadRejectsTrailingJSON(t *testing.T) {
	path := filepath.Join(t.TempDir(), "catalog.json")
	if err := os.WriteFile(path, []byte(`{"version":1} {"version":1}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(path); err == nil || !strings.Contains(err.Error(), "exactly one") {
		t.Fatalf("expected trailing JSON rejection, got %v", err)
	}
}

func canonicalTempDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	resolved, err := filepath.EvalSymlinks(dir)
	if err != nil {
		t.Fatal(err)
	}
	return resolved
}

func TestVariableSourceEstate(t *testing.T) {
	for _, n := range []int{1, 3, 25, MaxRepositories, MaxRepositories + 1} {
		cfg := Config{Version: 1, Limits: Defaults()}
		for i := 0; i < n; i++ {
			cfg.Sources = append(cfg.Sources, Source{Kind: model.SourceKindRepository, ID: fmt.Sprintf("repo-%03d", i)})
		}
		err := Validate(cfg)
		if (err == nil) != (n <= MaxRepositories) {
			t.Fatalf("size %d: %v", n, err)
		}
	}
}
