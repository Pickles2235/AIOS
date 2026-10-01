// Package mirror owns the approved, agent-managed Git mirror boundary.
package mirror

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
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
		if !validURL(x.URL) {
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
	if err := ValidateSourceBoundaries(r, dataDir); err != nil {
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
			if _, e = git(ctx, "", "clone", "--mirror", "--no-hardlinks", "--", x.URL, path); e != nil {
				return nil, fmt.Errorf("clone %s: %w", x.ID, e)
			}
		} else if e != nil {
			return nil, e
		} else {
			remote, e := git(ctx, path, "remote", "get-url", "origin")
			if e != nil || strings.TrimSpace(remote) != x.URL {
				return nil, fmt.Errorf("approved remote for %s differs from the owned mirror; use a new repository ID", x.ID)
			}
			if _, e = git(ctx, path, "fetch", "--prune", "origin"); e != nil {
				return nil, fmt.Errorf("fetch %s: %w", x.ID, e)
			}
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
	base := []string{"--no-pager", "-c", "core.hooksPath=/dev/null", "-c", "protocol.ext.allow=never", "-c", "core.askPass="}
	if dir != "" {
		base = append(base, "-C", dir)
	}
	cmd := exec.CommandContext(ctx, "git", append(base, args...)...)
	boundGitProcess(cmd)
	cmd.Env = CredentialEnvironment()
	var output boundedGitOutput
	cmd.Stdout = &output
	cmd.Stderr = &output
	err := cmd.Run()
	cleanupGitProcess(cmd)
	if err != nil {
		kind := "unavailable"
		lower := strings.ToLower(output.String())
		for _, marker := range []string{"authentication failed", "permission denied", "could not read username", "terminal prompts disabled", "credential", "401", "403"} {
			if strings.Contains(lower, marker) {
				kind = "authentication"
				break
			}
		}
		return "", &GitError{Kind: kind}
	}
	if output.overflow {
		return "", &GitError{Kind: "output_limit"}
	}
	return output.String(), nil
}

// ReadOnlyGit inspects a local source without optional Git writes or prompts.
// Its process group and inherited pipes obey the same deadline as remote Git.
func ReadOnlyGit(ctx context.Context, root string, args ...string) (string, error) {
	base := []string{"--no-pager", "--no-optional-locks", "-c", "core.fsmonitor=false", "-c", "core.hooksPath=/dev/null", "-c", "credential.helper=", "-c", "diff.external=", "-C", root}
	cmd := exec.CommandContext(ctx, "git", append(base, args...)...)
	cmd.Env = append(CredentialEnvironment(), "GIT_OPTIONAL_LOCKS=0", "GIT_NO_LAZY_FETCH=1")
	boundGitProcess(cmd)
	var out boundedGitOutput
	cmd.Stdout = &out
	err := cmd.Run()
	cleanupGitProcess(cmd)
	if err != nil || out.overflow {
		return "", fmt.Errorf("local Git read failed or exceeded output bound")
	}
	return out.String(), nil
}

// Configuration/helpers are the user's trusted machine setup. No credential
// response or raw stderr is persisted or returned through the product API.
func CredentialEnvironment() []string {
	blocked := map[string]bool{"GIT_DIR": true, "GIT_WORK_TREE": true, "GIT_INDEX_FILE": true, "GIT_OBJECT_DIRECTORY": true, "GIT_COMMON_DIR": true, "GIT_ALTERNATE_OBJECT_DIRECTORIES": true, "GIT_ASKPASS": true, "GIT_TERMINAL_PROMPT": true, "LC_ALL": true, "SSH_ASKPASS": true, "SSH_ASKPASS_REQUIRE": true}
	out := []string{}
	for _, v := range os.Environ() {
		key, _, _ := strings.Cut(v, "=")
		if !blocked[key] {
			out = append(out, v)
		}
	}
	return append(out, "GIT_ASKPASS=/usr/bin/false", "SSH_ASKPASS=/usr/bin/false", "SSH_ASKPASS_REQUIRE=never", "GIT_TERMINAL_PROMPT=0", "LC_ALL=C")
}
func CredentialStatus(ctx context.Context, login bool) map[string]bool {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", "config", "--get-all", "credential.helper")
	boundGitProcess(cmd)
	cmd.Env = CredentialEnvironment()
	var out boundedGitOutput
	cmd.Stdout = &out
	_ = cmd.Run()
	cleanupGitProcess(cmd)
	return map[string]bool{"credential_helpers_enabled": true, "default_helper_configured": strings.TrimSpace(out.String()) != "", "ssh_agent_environment_present": os.Getenv("SSH_AUTH_SOCK") != "", "login_context": login, "terminal_prompt_enabled": false, "product_token_store": false}
}

type GitError struct{ Kind string }

func (e *GitError) Error() string { return "Git source operation failed (" + e.Kind + ")" }
func (e *GitError) Remediation() string {
	if e.Kind == "authentication" {
		return "Git authentication failed in the daemon login environment. Configure your machine Git/SSH credential helper outside AgentOS, then retry. You may explicitly choose Direct mode for local workspaces; your selected mode has not changed."
	}
	return "Git source is unavailable. Check network access, approved remote/ref and machine credentials, then retry. You may explicitly choose Direct mode; your selected mode has not changed."
}

type boundedGitOutput struct {
	bytes.Buffer
	overflow bool
}

func (b *boundedGitOutput) Write(p []byte) (int, error) {
	n := len(p)
	remain := (32 << 20) - b.Len()
	if remain < n {
		b.overflow = true
		if remain > 0 {
			_, _ = b.Buffer.Write(p[:remain])
		}
		return n, nil
	}
	_, _ = b.Buffer.Write(p)
	return n, nil
}

func validURL(value string) bool {
	if value == "" || len(value) > 4096 || strings.ContainsAny(value, "\n\r\x00") {
		return false
	}
	if filepath.IsAbs(value) {
		return filepath.Clean(value) == value
	}
	u, err := url.Parse(value)
	if err != nil {
		return false
	}
	if u.Scheme == "file" {
		return u.Host == "" && filepath.IsAbs(u.Path) && filepath.Clean(u.Path) == u.Path && u.RawQuery == "" && u.Fragment == ""
	}
	if u.Scheme != "https" && u.Scheme != "ssh" {
		return false
	}
	if u.Hostname() == "" || u.Path == "" || u.RawQuery != "" || u.Fragment != "" {
		return false
	}
	if u.User != nil {
		if _, password := u.User.Password(); password || u.Scheme != "ssh" {
			return false
		}
	}
	return true
}
