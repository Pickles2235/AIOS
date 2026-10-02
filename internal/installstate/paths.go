// Package installstate defines persistent installation authority paths without
// importing the store or service manager.
package installstate

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
)

// Authority survives reboot temporary-directory cleanup and removal of root.
// It remains on the installation filesystem and never contains source content.
func Authority(root string) string {
	h := sha256.Sum256([]byte(filepath.Clean(root)))
	return filepath.Join(filepath.Dir(root), fmt.Sprintf(".aios-transactions-%d-%x", os.Getuid(), h[:16]))
}
