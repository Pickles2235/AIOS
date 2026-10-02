package lifecycle

import (
	"fmt"
	"os"
	"path/filepath"
)

const upgradeReserveName = ".upgrade-recovery-space"
const upgradeReserveBytes = 8 << 20

// Real allocated blocks, rather than a sparse truncate, reserve metadata space
// before stopping or activating. Recovery releases them before any journal write.
func reserveUpgradeSpace(root string) error {
	f, e := os.OpenFile(filepath.Join(root, upgradeReserveName), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if e != nil {
		return e
	}
	defer f.Close()
	zeros := make([]byte, 64<<10)
	for n := 0; n < upgradeReserveBytes; n += len(zeros) {
		if _, e = f.Write(zeros); e != nil {
			return e
		}
	}
	if e = f.Sync(); e != nil {
		return e
	}
	if e = f.Close(); e != nil {
		return e
	}
	return syncUpgradeDir(root)
}
func releaseUpgradeReserve(root string) error {
	p := filepath.Join(root, upgradeReserveName)
	i, e := os.Lstat(p)
	if os.IsNotExist(e) {
		return nil
	}
	if e != nil {
		return e
	}
	if !owned(i) || !i.Mode().IsRegular() || i.Mode().Perm() != 0600 || i.Size() > upgradeReserveBytes {
		return fmt.Errorf("invalid owned recovery reserve")
	}
	if e = os.Remove(p); e != nil {
		return e
	}
	return syncUpgradeDir(root)
}
