//go:build !windows

package lifecycle

import (
	"fmt"
	"golang.org/x/sys/unix"
)

func checkUpgradeSpace(path string, required uint64) error {
	var s unix.Statfs_t
	if e := unix.Statfs(path, &s); e != nil {
		return e
	}
	available := uint64(s.Bavail) * uint64(s.Bsize)
	if available < required {
		return fmt.Errorf("insufficient space for verified staged update and rollback; free space and retry")
	}
	return nil
}
