package app

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/AdamNi-7080/AIOS/internal/snapshotlease"
	"github.com/AdamNi-7080/AIOS/internal/store"
)

// reclaimPrunedSnapshots removes only roots named by committed retention,
// including durable candidates left by interrupted cleanup. It rechecks
// published and staged references while holding the repository lease.
// It never accepts arbitrary repository paths as deletion targets.
func reclaimPrunedSnapshots(ctx context.Context, db *store.Store, dataDir string, roots []string) error {
	base, err := filepath.Abs(filepath.Join(dataDir, "snapshots"))
	if err != nil {
		return err
	}
	if err := verifyDirectoryAncestors(dataDir, base); err != nil {
		return err
	}
	baseInfo, err := os.Lstat(base)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if !baseInfo.IsDir() || baseInfo.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("owned snapshot root is unsafe")
	}
	for _, root := range roots {
		if err = ctx.Err(); err != nil {
			return err
		}
		candidate, e := filepath.Abs(root)
		if e != nil {
			return e
		}
		rel, e := filepath.Rel(base, candidate)
		if e != nil || rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			continue
		}
		parts := strings.Split(rel, string(filepath.Separator))
		if len(parts) != 3 || parts[0] == "" || parts[1] == "" || parts[2] == "" || strings.HasPrefix(parts[0], ".") {
			continue
		}
		current := base
		safe := true
		missing := false
		for _, part := range parts {
			current = filepath.Join(current, part)
			info, statErr := os.Lstat(current)
			if os.IsNotExist(statErr) {
				missing = true
				safe = false
				break
			}
			if statErr != nil {
				return statErr
			}
			if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
				safe = false
				break
			}
		}
		if !safe && !missing {
			continue
		}
		if missing {
			lease, acquired, e := snapshotlease.TryExclusive(dataDir, candidate)
			if e != nil {
				return e
			}
			if !acquired {
				continue
			}
			referenced, e := db.SnapshotRootReferenced(ctx, candidate)
			if e == nil && !referenced {
				e = db.CompleteSnapshotGC(ctx, candidate)
			}
			_ = lease.Close()
			if e != nil {
				return e
			}
			continue
		}
		lease, acquired, e := snapshotlease.TryExclusive(dataDir, candidate)
		if e != nil {
			return e
		}
		if !acquired {
			continue
		}
		removed, e := reclaimOneSnapshot(ctx, db, candidate)
		if e == nil && removed {
			e = db.CompleteSnapshotGC(ctx, candidate)
		}
		_ = lease.Close()
		if e != nil {
			return e
		}
	}
	return nil
}

func reclaimOneSnapshot(ctx context.Context, db *store.Store, candidate string) (bool, error) {
	referenced, err := db.SnapshotRootReferenced(ctx, candidate)
	if err != nil || referenced {
		return false, err
	}
	if err = filepath.Walk(candidate, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if info.IsDir() {
			return os.Chmod(path, 0700)
		}
		return nil
	}); err != nil {
		return false, fmt.Errorf("owned snapshot cleanup unavailable: %w", err)
	}
	if err = os.RemoveAll(candidate); err != nil {
		return false, fmt.Errorf("owned snapshot cleanup unavailable: %w", err)
	}
	// The repository lease prevents concurrent capture while removing empty owned parents.
	revisionParent := filepath.Dir(candidate)
	if err = os.Remove(revisionParent); err != nil && !os.IsNotExist(err) && !isNotEmpty(err) {
		return false, err
	}
	if err == nil {
		repoParent := filepath.Dir(revisionParent)
		if e := os.Remove(repoParent); e != nil && !os.IsNotExist(e) && !isNotEmpty(e) {
			return false, e
		}
	}
	return true, nil
}

func verifyDirectoryAncestors(dataDir, path string) error {
	absData, err := filepath.Abs(dataDir)
	if err != nil {
		return err
	}
	rel, err := filepath.Rel(absData, path)
	if err != nil || rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return fmt.Errorf("owned snapshot base escapes data directory")
	}
	current := absData
	for _, part := range strings.Split(rel, string(filepath.Separator)) {
		current = filepath.Join(current, part)
		info, e := os.Lstat(current)
		if os.IsNotExist(e) && current == path {
			return nil
		}
		if e != nil {
			return e
		}
		if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("owned snapshot ancestor is unsafe")
		}
	}
	return nil
}

func isNotEmpty(err error) bool {
	return errors.Is(err, syscall.ENOTEMPTY) || errors.Is(err, syscall.EEXIST)
}

// ReclaimPendingSnapshots is a bounded write preflight for maintenance work.
// Read-only resource/status/health paths never invoke it.
func ReclaimPendingSnapshots(ctx context.Context, dataDir string) error {
	db, err := store.OpenWriter(dataDir)
	if err != nil {
		return err
	}
	defer db.Close()
	roots, err := db.ReclaimableSnapshotGCCandidates(ctx, 4)
	if err != nil {
		return err
	}
	return reclaimPrunedSnapshots(ctx, db, dataDir, roots)
}
