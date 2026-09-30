package adapter

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/AdamNi-7080/AIOS/internal/catalog"
	"github.com/AdamNi-7080/AIOS/internal/model"
	"github.com/AdamNi-7080/AIOS/internal/provenance"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

type LocalRepository struct {
	ID   string `json:"id"`
	Path string `json:"path"`
}
type LocalRegistry struct {
	Version      int               `json:"version"`
	Repositories []LocalRepository `json:"repositories"`
}

func ValidateLocalRegistry(r LocalRegistry) error {
	if r.Version != 1 {
		return fmt.Errorf("local registry version must be 1")
	}
	cfg := catalog.Config{Version: 1, Limits: catalog.Defaults()}
	for _, entry := range r.Repositories {
		cfg.Sources = append(cfg.Sources, catalog.Source{Kind: model.SourceKindRepository, ID: entry.ID})
		if !filepath.IsAbs(entry.Path) || filepath.Clean(entry.Path) != entry.Path {
			return fmt.Errorf("local path must be canonical and absolute")
		}
	}
	return catalog.Validate(cfg)
}
func LoadLocalRegistry(path string) (LocalRegistry, error) {
	var r LocalRegistry
	f, err := os.Open(path)
	if err != nil {
		return r, err
	}
	defer f.Close()
	b, err := io.ReadAll(io.LimitReader(f, catalog.MaxCatalogBytes+1))
	if err != nil || len(b) > catalog.MaxCatalogBytes {
		return r, fmt.Errorf("local registry exceeds byte limit")
	}
	d := json.NewDecoder(strings.NewReader(string(b)))
	d.DisallowUnknownFields()
	if err = d.Decode(&r); err != nil {
		return r, err
	}
	if d.Decode(&struct{}{}) != io.EOF {
		return r, fmt.Errorf("local registry must be one JSON object")
	}
	return r, ValidateLocalRegistry(r)
}
func localCommand(ctx context.Context, root string, args ...string) *exec.Cmd {
	base := []string{"--no-pager", "--no-optional-locks", "-c", "core.fsmonitor=false", "-c", "core.hooksPath=/dev/null", "-c", "credential.helper=", "-c", "diff.external=", "-C", root}
	cmd := exec.CommandContext(ctx, "git", append(base, args...)...)
	cmd.Env = []string{"PATH=" + os.Getenv("PATH"), "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_NOSYSTEM=1", "GIT_OPTIONAL_LOCKS=0", "GIT_NO_LAZY_FETCH=1", "GIT_TERMINAL_PROMPT=0", "LC_ALL=C"}
	return cmd
}
func localGit(ctx context.Context, root string, args ...string) (string, error) {
	cmd := localCommand(ctx, root, args...)
	output, err := cmd.StdoutPipe()
	if err != nil {
		return "", err
	}
	if err = cmd.Start(); err != nil {
		return "", err
	}
	b, readErr := io.ReadAll(io.LimitReader(output, (32<<20)+1))
	if readErr != nil || len(b) > 32<<20 {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		return "", fmt.Errorf("local Git output exceeds byte limit or is unreadable")
	}
	if err = cmd.Wait(); err != nil {
		return "", fmt.Errorf("local Git read failed: %w", err)
	}
	return string(b), nil
}
func InspectLocal(ctx context.Context, entry LocalRepository, dataDir string) (model.GitState, error) {
	root, err := filepath.EvalSymlinks(entry.Path)
	if err != nil || root != entry.Path {
		return model.GitState{}, fmt.Errorf("local workspace path must not contain symlinks")
	}
	data, err := filepath.Abs(dataDir)
	if err != nil {
		return model.GitState{}, err
	}
	if real, e := filepath.EvalSymlinks(data); e == nil {
		if real != data {
			return model.GitState{}, fmt.Errorf("owned data path must not contain symlinks")
		}
		data = real
	}
	for _, pair := range [][2]string{{root, data}, {data, root}} {
		rel, e := filepath.Rel(pair[0], pair[1])
		if e == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return model.GitState{}, fmt.Errorf("owned data and local workspace must not overlap")
		}
	}
	top, err := localGit(ctx, root, "rev-parse", "--show-toplevel")
	if err != nil || strings.TrimSpace(top) != root {
		return model.GitState{}, fmt.Errorf("local path must be a Git workspace top-level directory")
	}
	if partial, e := localGit(ctx, root, "config", "--get-regexp", "extensions.partialclone|remote\\..*\\.promisor"); e == nil && strings.TrimSpace(partial) != "" {
		return model.GitState{}, fmt.Errorf("partial clones are unsupported")
	}
	state, err := provenance.Inspect(ctx, root)
	if err != nil {
		return state, err
	}
	if state.Dirty || state.UntrackedCount > 0 {
		return state, fmt.Errorf("local workspace must be clean and committed; dirty or untracked files are unsupported")
	}
	return state, nil
}

// CaptureLocal reads only the pinned commit's Git objects. Working files are never compiler inputs.
func CaptureLocal(ctx context.Context, entry LocalRepository, dataDir string, limits model.Limits) (Discovery, error) {
	state, err := InspectLocal(ctx, entry, dataDir)
	if err != nil {
		return Discovery{}, err
	}
	if err = ValidateLocalRegistry(LocalRegistry{Version: 1, Repositories: []LocalRepository{entry}}); err != nil {
		return Discovery{}, err
	}
	tree, err := localGit(ctx, entry.Path, "ls-tree", "-r", "-z", "--full-tree", state.Commit)
	if err != nil {
		return Discovery{}, err
	}
	records := strings.Split(strings.TrimSuffix(tree, "\x00"), "\x00")
	if tree == "" {
		records = nil
	}
	if len(records) > limits.MaxFilesPerRepo {
		return Discovery{}, fmt.Errorf("snapshot exceeds file count limit")
	}
	parent := filepath.Join(dataDir, "snapshots", entry.ID, state.Commit)
	if err = os.MkdirAll(parent, 0700); err != nil {
		return Discovery{}, err
	}
	temp, err := os.MkdirTemp(parent, "local-")
	if err != nil {
		return Discovery{}, err
	}
	published := false
	defer func() {
		if !published {
			_ = filepath.Walk(temp, func(path string, info os.FileInfo, e error) error {
				if e == nil && info.IsDir() {
					return os.Chmod(path, 0700)
				}
				return e
			})
			_ = os.RemoveAll(temp)
		}
	}()
	cmd := localCommand(ctx, entry.Path, "cat-file", "--batch")
	input, err := cmd.StdinPipe()
	if err != nil {
		return Discovery{}, err
	}
	output, err := cmd.StdoutPipe()
	if err != nil {
		return Discovery{}, err
	}
	if err = cmd.Start(); err != nil {
		return Discovery{}, err
	}
	waited := false
	defer func() {
		input.Close()
		if !waited {
			_ = cmd.Process.Kill()
			_ = cmd.Wait()
		}
	}()
	reader := bufio.NewReader(output)
	seen := map[string]bool{}
	var total int64
	for _, record := range records {
		pieces := strings.SplitN(record, "\t", 2)
		if len(pieces) != 2 {
			return Discovery{}, fmt.Errorf("invalid Git tree")
		}
		fields := strings.Fields(pieces[0])
		if len(fields) != 3 || (fields[0] != "100644" && fields[0] != "100755") || fields[1] != "blob" {
			return Discovery{}, fmt.Errorf("snapshot rejects symlinks, submodules and unsupported Git entries")
		}
		path := pieces[1]
		if len(path) > 4096 || strings.Contains(path, "\\") || filepath.IsAbs(path) || filepath.ToSlash(filepath.Clean(path)) != path || path == ".." || strings.HasPrefix(path, "../") {
			return Discovery{}, fmt.Errorf("unsafe snapshot path")
		}
		for _, component := range strings.Split(path, "/") {
			if strings.EqualFold(component, ".git") {
				return Discovery{}, fmt.Errorf("snapshot contains a reserved Git path")
			}
		}
		folded := strings.ToLower(path)
		if seen[folded] {
			return Discovery{}, fmt.Errorf("snapshot contains case-colliding paths")
		}
		seen[folded] = true
		if _, err = fmt.Fprintln(input, fields[2]); err != nil {
			return Discovery{}, err
		}
		header, e := reader.ReadString('\n')
		if e != nil {
			return Discovery{}, e
		}
		h := strings.Fields(header)
		if len(h) != 3 || h[0] != fields[2] || h[1] != "blob" {
			return Discovery{}, fmt.Errorf("pinned Git blob unavailable")
		}
		size, e := strconv.ParseInt(h[2], 10, 64)
		if e != nil || size < 0 || size > limits.MaxFileBytes {
			return Discovery{}, fmt.Errorf("snapshot file exceeds byte limit")
		}
		total += size
		if total > limits.MaxTotalBytesPerRepo {
			return Discovery{}, fmt.Errorf("snapshot exceeds total byte limit")
		}
		dest := filepath.Join(temp, filepath.FromSlash(path))
		if err = os.MkdirAll(filepath.Dir(dest), 0700); err != nil {
			return Discovery{}, err
		}
		f, e := os.OpenFile(dest, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
		if e != nil {
			return Discovery{}, e
		}
		_, e = io.CopyN(f, reader, size)
		ce := f.Close()
		if e != nil {
			return Discovery{}, e
		}
		if ce != nil {
			return Discovery{}, ce
		}
		separator, e := reader.ReadByte()
		if e != nil || separator != '\n' {
			return Discovery{}, fmt.Errorf("invalid Git blob framing")
		}
	}
	input.Close()
	err = cmd.Wait()
	waited = true
	if err != nil {
		return Discovery{}, err
	}
	// A second clean-state check rejects edits during capture; the pinned tree still guarantees no mixed bytes.
	after, err := provenance.Inspect(ctx, entry.Path)
	if err != nil || after != state {
		return Discovery{}, fmt.Errorf("local workspace changed during snapshot capture; retry after committing")
	}
	if err = filepath.Walk(temp, func(path string, info os.FileInfo, e error) error {
		if e != nil {
			return e
		}
		if info.IsDir() {
			return os.Chmod(path, 0500)
		}
		return os.Chmod(path, 0400)
	}); err != nil {
		return Discovery{}, err
	}
	sum := sha256.Sum256([]byte(entry.Path))
	final := filepath.Join(parent, "local-"+hex.EncodeToString(sum[:]))
	if info, e := os.Lstat(final); e == nil {
		if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return Discovery{}, fmt.Errorf("unsafe owned snapshot")
		}
		first, e := snapshotDigest(temp)
		if e != nil {
			return Discovery{}, e
		}
		second, e := snapshotDigest(final)
		if e != nil || first != second {
			return Discovery{}, fmt.Errorf("owned snapshot differs from pinned Git tree")
		}
		return Discovery{Identity: model.SourceIdentity{ID: entry.ID, Kind: model.SourceKindRepository, AdapterVersion: model.RepositoryAdapterVersion}, Revision: state.Commit, Root: final}, nil
	} else if !os.IsNotExist(e) {
		return Discovery{}, e
	}
	// Darwin requires write permission on a directory being renamed. Only the
	// owned staging root is writable during publication; no compiler receives
	// its path until the final root is sealed again. Files remain read-only.
	if err = os.Chmod(temp, 0700); err != nil {
		return Discovery{}, err
	}
	if err = os.Rename(temp, final); err != nil {
		return Discovery{}, err
	}
	temp = final // cleanup the renamed owned directory if sealing fails
	if err = os.Chmod(final, 0500); err != nil {
		return Discovery{}, err
	}
	published = true
	return Discovery{Identity: model.SourceIdentity{ID: entry.ID, Kind: model.SourceKindRepository, AdapterVersion: model.RepositoryAdapterVersion}, Revision: state.Commit, Root: final}, nil
}
func LocalFingerprint(reg LocalRegistry) string {
	reg.Repositories = append([]LocalRepository(nil), reg.Repositories...)
	sort.Slice(reg.Repositories, func(i, j int) bool { return reg.Repositories[i].ID < reg.Repositories[j].ID })
	b, _ := json.Marshal(reg)
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

func snapshotDigest(root string) (string, error) {
	hash := sha256.New()
	err := filepath.Walk(root, func(path string, info os.FileInfo, e error) error {
		if e != nil {
			return e
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("snapshot contains symlink")
		}
		if info.IsDir() {
			if info.Mode().Perm() != 0500 {
				return fmt.Errorf("snapshot directory is writable")
			}
			return nil
		}
		if !info.Mode().IsRegular() || info.Mode().Perm() != 0400 {
			return fmt.Errorf("unsafe snapshot file")
		}
		rel, e := filepath.Rel(root, path)
		if e != nil {
			return e
		}
		fmt.Fprintf(hash, "%d:%s:%d:", len(rel), rel, info.Size())
		f, e := os.Open(path)
		if e != nil {
			return e
		}
		_, e = io.Copy(hash, f)
		ce := f.Close()
		if e != nil {
			return e
		}
		return ce
	})
	return hex.EncodeToString(hash.Sum(nil)), err
}
