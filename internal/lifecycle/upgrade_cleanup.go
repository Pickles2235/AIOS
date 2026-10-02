package lifecycle

import (
	"context"
	"os"
	"path/filepath"
)

func removeUpgradeRecovery(ctx context.Context, root string) error {
	path := filepath.Join(root, "recovery")
	if _, e := os.Lstat(path); os.IsNotExist(e) {
		return nil
	} else if e != nil {
		return e
	}
	return removeOwnedTree(ctx, path)
}
