//go:build !windows

package lifecycle

import (
	"fmt"
	"os"
	"path/filepath"
	"syscall"
)

func Lock(data string) (*os.File, error) {
	if err := PrepareDir(data); err != nil {
		return nil, err
	}
	fd, err := syscall.Open(filepath.Join(data, ".daemon.lock"), syscall.O_CREAT|syscall.O_RDWR|syscall.O_CLOEXEC|syscall.O_NOFOLLOW, 0600)
	if err != nil {
		return nil, err
	}
	f := os.NewFile(uintptr(fd), ".daemon.lock")
	info, err := f.Stat()
	if err != nil {
		f.Close()
		return nil, err
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !info.Mode().IsRegular() || !ok || int(stat.Uid) != os.Geteuid() {
		f.Close()
		return nil, fmt.Errorf("daemon lock must be an owned regular file")
	}
	if err = syscall.Flock(fd, syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		f.Close()
		if err == syscall.EWOULDBLOCK || err == syscall.EAGAIN {
			return nil, errDaemonLocked
		}
		return nil, err
	}
	return f, nil
}

func owned(info os.FileInfo) bool {
	stat, ok := info.Sys().(*syscall.Stat_t)
	return ok && int(stat.Uid) == os.Geteuid()
}
func openOwned(path string) (*os.File, error) {
	fd, err := syscall.Open(path, syscall.O_RDONLY|syscall.O_CLOEXEC|syscall.O_NOFOLLOW, 0)
	if err != nil {
		return nil, err
	}
	f := os.NewFile(uintptr(fd), path)
	info, err := f.Stat()
	if err != nil {
		f.Close()
		return nil, err
	}
	if !info.Mode().IsRegular() || !owned(info) || info.Mode().Perm() != 0600 || info.Size() > 65536 {
		f.Close()
		return nil, fmt.Errorf("metadata must be bounded, owned and owner-only")
	}
	return f, nil
}
