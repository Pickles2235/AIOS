//go:build unix

package store

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
)

const writerLockName = ".writer.lock"

var ErrWriterLocked = errors.New("index already has an active writer")

type writerLock struct {
	file *os.File
}

func acquireWriterLock(path string) (*writerLock, error) {
	fd, err := syscall.Open(path, syscall.O_CREAT|syscall.O_RDWR|syscall.O_CLOEXEC|syscall.O_NOFOLLOW, 0o600)
	if err != nil {
		return nil, fmt.Errorf("open writer lock: %w", err)
	}
	file := os.NewFile(uintptr(fd), path)
	info, statErr := file.Stat()
	if statErr != nil {
		file.Close()
		return nil, fmt.Errorf("inspect writer lock: %w", statErr)
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !info.Mode().IsRegular() || !ok || int(stat.Uid) != os.Geteuid() {
		file.Close()
		return nil, fmt.Errorf("writer lock must be a regular file owned by the current user: %s", path)
	}
	if err := file.Chmod(0o600); err != nil {
		file.Close()
		return nil, fmt.Errorf("secure writer lock permissions: %w", err)
	}
	if err := syscall.Flock(fd, syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		file.Close()
		if errors.Is(err, syscall.EWOULDBLOCK) {
			return nil, ErrWriterLocked
		}
		return nil, fmt.Errorf("acquire writer lock: %w", err)
	}
	return &writerLock{file: file}, nil
}

func (l *writerLock) close() error {
	if l == nil || l.file == nil {
		return nil
	}
	unlockErr := syscall.Flock(int(l.file.Fd()), syscall.LOCK_UN)
	closeErr := l.file.Close()
	if unlockErr != nil {
		return fmt.Errorf("release writer lock: %w", unlockErr)
	}
	return closeErr
}

func validateDataDirectory(path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		return fmt.Errorf("inspect data directory: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return fmt.Errorf("data directory must be a real directory: %s", path)
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok || int(stat.Uid) != os.Geteuid() {
		return fmt.Errorf("data directory must be owned by the current user: %s", path)
	}
	if err := os.Chmod(path, 0o700); err != nil {
		return fmt.Errorf("secure data directory permissions: %w", err)
	}
	return nil
}

// validateReadOnlyDatabase validates without changing anything or taking the
// writer lock. Read-only callers must not silently repair an unsafe store.
func validateReadOnlyDatabase(dataDir string) (string, error) {
	abs, err := filepath.Abs(dataDir)
	if err != nil {
		return "", err
	}
	canonical, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return "", fmt.Errorf("resolve data directory: %w", err)
	}
	info, err := os.Lstat(canonical)
	if err != nil {
		return "", fmt.Errorf("inspect data directory: %w", err)
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() || !ok || int(stat.Uid) != os.Geteuid() || info.Mode().Perm() != 0o700 {
		return "", fmt.Errorf("data directory must be an owner-only real directory: %s", canonical)
	}
	path := filepath.Join(canonical, DatabaseName)
	fd, err := syscall.Open(path, syscall.O_RDONLY|syscall.O_CLOEXEC|syscall.O_NOFOLLOW, 0)
	if err != nil {
		return "", fmt.Errorf("open index safely: %w", err)
	}
	file := os.NewFile(uintptr(fd), path)
	defer file.Close()
	info, err = file.Stat()
	if err != nil {
		return "", fmt.Errorf("inspect index: %w", err)
	}
	stat, ok = info.Sys().(*syscall.Stat_t)
	if !info.Mode().IsRegular() || !ok || int(stat.Uid) != os.Geteuid() || info.Mode().Perm() != 0o600 {
		return "", fmt.Errorf("index must be an owner-only regular file: %s", path)
	}
	return path, nil
}

func prepareDatabaseFile(path string) error {
	fd, err := syscall.Open(path, syscall.O_CREAT|syscall.O_RDWR|syscall.O_CLOEXEC|syscall.O_NOFOLLOW, 0o600)
	if err != nil {
		return fmt.Errorf("open database safely: %w", err)
	}
	file := os.NewFile(uintptr(fd), path)
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return fmt.Errorf("inspect database: %w", err)
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("database must be a regular file: %s", path)
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok || int(stat.Uid) != os.Geteuid() {
		return fmt.Errorf("database must be owned by the current user: %s", path)
	}
	if err := file.Chmod(0o600); err != nil {
		return fmt.Errorf("secure database permissions: %w", err)
	}
	return nil
}

func enforceDatabasePermissions(path string) error {
	for _, candidate := range []string{path, path + "-journal", path + "-shm", path + "-wal"} {
		info, err := os.Lstat(candidate)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return fmt.Errorf("inspect database file %s: %w", filepath.Base(candidate), err)
		}
		if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
			return fmt.Errorf("database file must be regular and not a symlink: %s", candidate)
		}
		stat, ok := info.Sys().(*syscall.Stat_t)
		if !ok || int(stat.Uid) != os.Geteuid() {
			return fmt.Errorf("database file must be owned by the current user: %s", candidate)
		}
		if err := os.Chmod(candidate, 0o600); err != nil {
			return fmt.Errorf("secure database file permissions: %w", err)
		}
	}
	return nil
}

func validateOwnedRegularFile(path string, info os.FileInfo) error {
	stat, ok := info.Sys().(*syscall.Stat_t)
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() || !ok || int(stat.Uid) != os.Geteuid() {
		return fmt.Errorf("file must be regular, not a symlink, and owned by the current user: %s", path)
	}
	return nil
}

/* Legacy migration backup support is intentionally removed: V1 rejects old
derived databases and requires a clean reindex.
func backupBeforeUpgrade(ctx context.Context, db *sql.DB, databasePath string, available []migration) error {
	var current int
	if err := db.QueryRowContext(ctx, `PRAGMA user_version`).Scan(&current); err != nil {
		return fmt.Errorf("read schema version for backup: %w", err)
	}
	latest := 0
	if len(available) > 0 {
		latest = available[len(available)-1].version
	}
	if current == 0 || current >= latest {
		return nil
	}
	backupPath := fmt.Sprintf("%s.v%d.bak", databasePath, current)
	if info, err := os.Lstat(backupPath); err == nil {
		if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
			return fmt.Errorf("database backup must be regular and not a symlink: %s", backupPath)
		}
		stat, ok := info.Sys().(*syscall.Stat_t)
		if !ok || int(stat.Uid) != os.Geteuid() {
			return fmt.Errorf("database backup must be owned by the current user: %s", backupPath)
		}
		return os.Chmod(backupPath, 0o600)
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("inspect database backup: %w", err)
	}
	temp, err := os.CreateTemp(filepath.Dir(databasePath), ".index-backup-*")
	if err != nil {
		return fmt.Errorf("create database backup path: %w", err)
	}
	tempPath := temp.Name()
	if err := temp.Close(); err != nil {
		return err
	}
	if err := os.Remove(tempPath); err != nil {
		return err
	}
	defer os.Remove(tempPath)
	escaped := strings.ReplaceAll(tempPath, "'", "''")
	if _, err := db.ExecContext(ctx, `VACUUM INTO '`+escaped+`'`); err != nil {
		return fmt.Errorf("create consistent database backup: %w", err)
	}
	backup, err := os.OpenFile(tempPath, os.O_RDWR, 0)
	if err != nil {
		return fmt.Errorf("open database backup: %w", err)
	}
	if err := backup.Chmod(0o600); err != nil {
		backup.Close()
		return err
	}
	if err := backup.Sync(); err != nil {
		backup.Close()
		return fmt.Errorf("sync database backup: %w", err)
	}
	if err := backup.Close(); err != nil {
		return err
	}
	if err := os.Rename(tempPath, backupPath); err != nil {
		return fmt.Errorf("publish database backup: %w", err)
	}
	dir, err := os.Open(filepath.Dir(databasePath))
	if err != nil {
		return err
	}
	defer dir.Close()
	if err := dir.Sync(); err != nil {
		return fmt.Errorf("sync database backup directory: %w", err)
	}
	return nil
}
*/
