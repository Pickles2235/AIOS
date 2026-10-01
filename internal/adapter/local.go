package adapter

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/AdamNi-7080/AIOS/internal/catalog"
	"github.com/AdamNi-7080/AIOS/internal/mirror"
	"github.com/AdamNi-7080/AIOS/internal/model"
	"io"
	"os"
	"path/filepath"
	"sort"
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
func localGit(ctx context.Context, root string, args ...string) (string, error) {
	return mirror.ReadOnlyGit(ctx, root, args...)
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
	commit, err := localGit(ctx, root, "rev-parse", "HEAD")
	if err != nil {
		return model.GitState{}, err
	}
	branch, e := localGit(ctx, root, "symbolic-ref", "--short", "-q", "HEAD")
	if e != nil {
		branch = "DETACHED"
	}
	status, err := localGit(ctx, root, "status", "--porcelain=v1", "-z", "--untracked-files=all")
	if err != nil {
		return model.GitState{}, err
	}
	untracked := 0
	for _, entry := range strings.Split(status, "\x00") {
		if strings.HasPrefix(entry, "??") {
			untracked++
		}
	}
	return model.GitState{Commit: strings.TrimSpace(commit), Branch: strings.TrimSpace(branch), Dirty: status != "", UntrackedCount: untracked}, nil
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
