// Package mirror owns the approved, agent-managed Git mirror boundary.
package mirror

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/AdamNi-7080/AIOS/internal/catalog"
)

const RegistryVersion = 1

type Registry struct {
	Version      int          `json:"version"`
	Repositories []Repository `json:"repositories"`
}

type Repository struct {
	ID  string `json:"id"`
	URL string `json:"url"`
	Ref string `json:"ref"`
}

type Synced struct {
	ID, Revision, Mirror string
	UpdatedAt            time.Time
}

var idPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{0,62}$`)

func Load(path string) (Registry, error) {
	f, err := os.Open(path)
	if err != nil {
		return Registry{}, fmt.Errorf("read mirror registry: %w", err)
	}
	defer f.Close()
	b, err := io.ReadAll(io.LimitReader(f, 1<<20+1))
	if err != nil {
		return Registry{}, err
	}
	if len(b) > 1<<20 {
		return Registry{}, fmt.Errorf("mirror registry exceeds %d bytes", 1<<20)
	}
	var r Registry
	d := json.NewDecoder(strings.NewReader(string(b)))
	d.DisallowUnknownFields()
	if err = d.Decode(&r); err != nil {
		return Registry{}, fmt.Errorf("decode mirror registry: %w", err)
	}
	if err = d.Decode(&struct{}{}); err != io.EOF {
		return Registry{}, fmt.Errorf("decode mirror registry: expected exactly one JSON object")
	}
	return r, Validate(r)
}

func Validate(r Registry) error {
	if r.Version != RegistryVersion {
		return fmt.Errorf("mirror registry version must be %d", RegistryVersion)
	}
	if len(r.Repositories) < 1 || len(r.Repositories) > catalog.MaxRepositories {
		return fmt.Errorf("mirror registry must declare between 1 and %d repositories, got %d", catalog.MaxRepositories, len(r.Repositories))
	}
	seen := map[string]bool{}
	for i, x := range r.Repositories {
		if !idPattern.MatchString(x.ID) || seen[x.ID] {
			return fmt.Errorf("repositories[%d].id is invalid or duplicated", i)
		}
		seen[x.ID] = true
		if strings.TrimSpace(x.URL) == "" || strings.ContainsAny(x.URL, "\n\r") {
			return fmt.Errorf("repository %q has an invalid URL", x.ID)
		}
		if !strings.HasPrefix(x.Ref, "refs/") || strings.ContainsAny(x.Ref, " \t\n\r") {
			return fmt.Errorf("repository %q ref must be a full refs/ name", x.ID)
		}
	}
	return nil
}

// Fingerprint is stable across JSON formatting and registry ordering.
func Fingerprint(r Registry) string {
	x := append([]Repository(nil), r.Repositories...)
	sort.Slice(x, func(i, j int) bool { return x[i].ID < x[j].ID })
	b, _ := json.Marshal(struct {
		Version      int          `json:"version"`
		Repositories []Repository `json:"repositories"`
	}{r.Version, x})
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}

func MirrorPath(dataDir, id string) string { return filepath.Join(dataDir, "mirrors", id+".git") }

// ApprovedRevision reads the configured ref from an existing managed mirror;
// ingestion deliberately does not contact a remote.
func ApprovedRevision(ctx context.Context, dataDir string, repo Repository) (string, error) {
	path := MirrorPath(dataDir, repo.ID)
	info, err := os.Stat(path)
	if err != nil || !info.IsDir() {
		return "", fmt.Errorf("managed mirror for %s is unavailable; run mirrors sync", repo.ID)
	}
	rev, err := git(ctx, path, "rev-parse", "--verify", repo.Ref+"^{commit}")
	if err != nil {
		return "", fmt.Errorf("approved ref for %s: %w", repo.ID, err)
	}
	return strings.TrimSpace(rev), nil
}

// Sync clones or fetches only the registered remote into the agent-owned data
// directory. It never receives a user checkout path.
func Sync(ctx context.Context, r Registry, dataDir string) ([]Synced, error) {
	if err := Validate(r); err != nil {
		return nil, err
	}
	root, err := filepath.Abs(dataDir)
	if err != nil {
		return nil, err
	}
	if err = os.MkdirAll(root, 0700); err != nil {
		return nil, err
	}
	if err = os.Chmod(root, 0700); err != nil {
		return nil, err
	}
	dir := filepath.Join(root, "mirrors")
	if err = os.MkdirAll(dir, 0700); err != nil {
		return nil, err
	}
	var out []Synced
	for _, x := range r.Repositories {
		path := MirrorPath(root, x.ID)
		if _, e := os.Lstat(path); os.IsNotExist(e) {
			if _, e = git(ctx, "", "clone", "--mirror", x.URL, path); e != nil {
				return nil, fmt.Errorf("clone %s: %w", x.ID, e)
			}
		} else if e != nil {
			return nil, e
		} else if _, e = git(ctx, path, "fetch", "--prune", "origin"); e != nil {
			return nil, fmt.Errorf("fetch %s: %w", x.ID, e)
		}
		rev, e := git(ctx, path, "rev-parse", "--verify", x.Ref+"^{commit}")
		if e != nil {
			return nil, fmt.Errorf("approved ref for %s: %w", x.ID, e)
		}
		out = append(out, Synced{ID: x.ID, Revision: strings.TrimSpace(rev), Mirror: path, UpdatedAt: time.Now().UTC()})
	}
	return out, nil
}

func git(ctx context.Context, dir string, args ...string) (string, error) {
	base := []string{"--no-pager", "-c", "core.hooksPath=/dev/null", "-c", "credential.helper=", "-c", "core.askPass="}
	if dir != "" {
		base = append(base, "-C", dir)
	}
	cmd := exec.CommandContext(ctx, "git", append(base, args...)...)
	cmd.Env = append([]string{"GIT_ASKPASS=/usr/bin/false", "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_NOSYSTEM=1", "GIT_TERMINAL_PROMPT=0", "LC_ALL=C"}, "PATH="+os.Getenv("PATH"))
	b, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("git %v: %w: %s", args, err, strings.TrimSpace(string(b)))
	}
	return string(b), nil
}
