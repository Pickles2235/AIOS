package lifecycle

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
)

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
