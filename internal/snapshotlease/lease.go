// Package snapshotlease coordinates immutable owned snapshot readers and GC
// across the daemon and local CLI processes.
package snapshotlease

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
)

type Lease struct {
	file *os.File
	once sync.Once
}

func lockFile(dataDir, root string) (*os.File, error) {
	base, err := filepath.Abs(filepath.Join(dataDir, "snapshots"))
	if err != nil {
		return nil, err
	}
	candidate, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	rel, err := filepath.Rel(base, candidate)
	if err != nil {
		return nil, err
	}
	parts := strings.Split(rel, string(filepath.Separator))
	if len(parts) != 3 || parts[0] == "" || parts[0] == "." || parts[0] == ".." || strings.HasPrefix(parts[0], ".") {
		return nil, errors.New("unsafe snapshot lease root")
	}
	if err := os.MkdirAll(base, 0700); err != nil {
		return nil, err
	}
	info, err := os.Lstat(base)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return nil, errors.New("unsafe snapshot base")
	}
	dir := filepath.Join(base, ".leases")
	if err := os.Mkdir(dir, 0700); err != nil && !os.IsExist(err) {
		return nil, err
	}
	info, err = os.Lstat(dir)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm()&0077 != 0 {
		return nil, errors.New("unsafe snapshot lease directory")
	}
	sum := sha256.Sum256([]byte(parts[0]))
	path := filepath.Join(dir, hex.EncodeToString(sum[:])+".lock")
	if info, err := os.Lstat(path); err == nil {
		if !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 {
			return nil, errors.New("unsafe snapshot lease file")
		}
	} else if !os.IsNotExist(err) {
		return nil, err
	}
	return os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0600)
}
func Acquire(dataDir, root string) (*Lease, error) {
	f, err := lockFile(dataDir, root)
	if err != nil {
		return nil, err
	}
	if err = syscall.Flock(int(f.Fd()), syscall.LOCK_SH); err != nil {
		f.Close()
		return nil, err
	}
	return &Lease{file: f}, nil
}
func TryExclusive(dataDir, root string) (*Lease, bool, error) {
	f, err := lockFile(dataDir, root)
	if err != nil {
		return nil, false, err
	}
	err = syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
	if errors.Is(err, syscall.EWOULDBLOCK) {
		f.Close()
		return nil, false, nil
	}
	if err != nil {
		f.Close()
		return nil, false, err
	}
	return &Lease{file: f}, true, nil
}
func (l *Lease) Close() error {
	if l == nil {
		return nil
	}
	var err error
	l.once.Do(func() {
		err = syscall.Flock(int(l.file.Fd()), syscall.LOCK_UN)
		closeErr := l.file.Close()
		if err == nil {
			err = closeErr
		}
	})
	return err
}
