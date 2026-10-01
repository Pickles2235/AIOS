package lifecycle

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
)

var repositoryAssetID = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{0,62}$`)

// PurgeRepositoryAssets accepts only an owned data root and an ID, never a
// source path. Shared derived caches/backups may contain removed references and
// are discarded; the remaining canonical repositories are preserved.
func PurgeRepositoryAssets(ctx context.Context, dataDir, id string) error {
	if !repositoryAssetID.MatchString(id) {
		return fmt.Errorf("invalid repository asset ID")
	}
	if err := PrepareDir(dataDir); err != nil {
		return err
	}
	paths := []string{filepath.Join("snapshots", id), filepath.Join("mirrors", id+".git"), "compiler-cache", "cache.db", "cache.db-wal", "cache.db-shm"}
	for _, pattern := range []string{"index.ir-9-backup-*.db", ".ir-9-backup-*", ".setup-*", ".maintenance-*", ".removal-*", filepath.Join("snapshots", ".staging-*")} {
		matches, err := filepath.Glob(filepath.Join(dataDir, pattern))
		if err != nil {
			return err
		}
		for _, path := range matches {
			rel, err := filepath.Rel(dataDir, path)
			if err != nil {
				return err
			}
			paths = append(paths, rel)
		}
	}
	root, err := os.OpenRoot(dataDir)
	if err != nil {
		return err
	}
	defer root.Close()
	for _, rel := range paths {
		info, err := root.Lstat(rel)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return err
		}
		// Resolve parents without following links before any walk/chmod. A
		// same-user process cannot redirect this request outside the owned root.
		path := filepath.Join(dataDir, rel)
		real, err := filepath.EvalSymlinks(path)
		if err != nil || real != path || !owned(info) || info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("refusing linked or foreign repository assets")
		}
		if info.IsDir() {
			if err = removeOwnedRootTree(ctx, root, rel); err != nil {
				return err
			}
		} else {
			if !info.Mode().IsRegular() {
				return fmt.Errorf("invalid owned asset")
			}
			if err = root.Remove(rel); err != nil {
				return err
			}
		}
		parent, err := root.Open(filepath.Dir(rel))
		if err != nil {
			return err
		}
		err = parent.Sync()
		ce := parent.Close()
		if err != nil {
			return err
		}
		if ce != nil {
			return ce
		}
	}
	f, err := os.Open(dataDir)
	if err != nil {
		return err
	}
	defer f.Close()
	return f.Sync()
}

func removeOwnedRootTree(ctx context.Context, root *os.Root, rel string) error {
	check := func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		info, err := root.Lstat(path)
		if err != nil {
			return err
		}
		if !owned(info) || info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("refusing foreign or linked owned assets")
		}
		return nil
	}
	if err := fs.WalkDir(root.FS(), rel, check); err != nil {
		return err
	}
	if err := fs.WalkDir(root.FS(), rel, func(path string, d fs.DirEntry, err error) error {
		if err = check(path, d, err); err != nil {
			return err
		}
		if d.IsDir() {
			return root.Chmod(path, 0700)
		}
		return nil
	}); err != nil {
		return err
	}
	return root.RemoveAll(rel)
}

// Stop the writer/service before calling. Snapshot directories are deliberately
// sealed against compiler writes; only owned directories are unsealed for delete.
func removeOwnedTree(ctx context.Context, root string) error {
	check := func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if err = ctx.Err(); err != nil {
			return err
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		if !owned(info) || info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("refusing foreign or linked installation state")
		}
		return nil
	}
	// Preflight the entire tree before changing any permissions.
	if err := filepath.WalkDir(root, check); err != nil {
		return err
	}
	if err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err = check(path, d, err); err != nil {
			return err
		}
		if d.IsDir() {
			return os.Chmod(path, 0700)
		}
		return nil
	}); err != nil {
		return err
	}
	return os.RemoveAll(root)
}
