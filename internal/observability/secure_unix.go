//go:build !windows

package observability

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"syscall"

	"golang.org/x/sys/unix"
)

type secureRoot struct{ fd int }

func openRoot(dataDir string) (*secureRoot, error) {
	return openRootMode(dataDir, true)
}
func openExistingRoot(dataDir string) (*secureRoot, error) {
	return openRootMode(dataDir, false)
}
func openRootMode(dataDir string, create bool) (*secureRoot, error) {
	parent, err := unix.Open(dataDir, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		return nil, err
	}
	defer unix.Close(parent)
	if err = ownedDir(parent); err != nil {
		return nil, err
	}
	if create {
		if err = unix.Mkdirat(parent, "diagnostics", 0700); err != nil && err != syscall.EEXIST {
			return nil, err
		}
	}
	fd, err := unix.Openat(parent, "diagnostics", unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		return nil, err
	}
	if err = ownedDir(fd); err != nil {
		unix.Close(fd)
		return nil, err
	}
	return &secureRoot{fd: fd}, nil
}

func ownedDir(fd int) error {
	var stat unix.Stat_t
	if err := unix.Fstat(fd, &stat); err != nil {
		return err
	}
	if stat.Mode&unix.S_IFMT != unix.S_IFDIR || stat.Uid != uint32(os.Getuid()) || stat.Mode&0022 != 0 {
		return fmt.Errorf("diagnostic directory is not privately owned")
	}
	return nil
}
func (r *secureRoot) close() error { return unix.Close(r.fd) }

func checkedFile(fd int) error {
	var stat unix.Stat_t
	if err := unix.Fstat(fd, &stat); err != nil {
		return err
	}
	if stat.Mode&unix.S_IFMT != unix.S_IFREG || stat.Uid != uint32(os.Getuid()) || stat.Nlink != 1 || stat.Mode&0077 != 0 {
		return fmt.Errorf("diagnostic sink must be a private single-link regular file")
	}
	return nil
}
func (r *secureRoot) open(name string, create bool) (int, error) {
	flags := unix.O_RDONLY | unix.O_NOFOLLOW | unix.O_NONBLOCK | unix.O_CLOEXEC
	if create {
		flags = unix.O_WRONLY | unix.O_APPEND | unix.O_CREAT | unix.O_NOFOLLOW | unix.O_NONBLOCK | unix.O_CLOEXEC
	}
	fd, err := unix.Openat(r.fd, name, flags, 0600)
	if err != nil {
		return -1, err
	}
	if err = checkedFile(fd); err != nil {
		unix.Close(fd)
		return -1, err
	}
	return fd, nil
}
func (r *secureRoot) rotate(files int) error {
	for i := files - 1; i >= 1; i-- {
		old := fmt.Sprintf("events-%d.jsonl", i-1)
		newName := fmt.Sprintf("events-%d.jsonl", i)
		for _, name := range []string{old, newName} {
			fd, err := r.open(name, false)
			if err == nil {
				var stat unix.Stat_t
				if e := unix.Fstat(fd, &stat); e != nil || stat.Size > maxFile {
					unix.Close(fd)
					return fmt.Errorf("diagnostic rotation file exceeds bound")
				}
				unix.Close(fd)
			} else if err != syscall.ENOENT {
				return err
			}
		}
		if i == files-1 {
			if err := unix.Unlinkat(r.fd, newName, 0); err != nil && err != syscall.ENOENT {
				return err
			}
		}
		if err := unix.Renameat(r.fd, old, r.fd, newName); err != nil && err != syscall.ENOENT {
			return err
		}
	}
	return unix.Fsync(r.fd)
}
func (r *secureRoot) append(b []byte, limit, files int) error {
	fd, err := r.open("events-0.jsonl", true)
	if err != nil {
		return err
	}
	var stat unix.Stat_t
	err = unix.Fstat(fd, &stat)
	if err == nil && stat.Size > int64(limit) {
		unix.Close(fd)
		return fmt.Errorf("diagnostic sink exceeds bound")
	}
	if err == nil && stat.Size+int64(len(b)) > int64(limit) {
		unix.Close(fd)
		if err = r.rotate(files); err != nil {
			return err
		}
		fd, err = r.open("events-0.jsonl", true)
		if err != nil {
			return err
		}
	}
	defer unix.Close(fd)
	if err != nil {
		return err
	}
	for len(b) > 0 {
		n, e := unix.Write(fd, b)
		if e != nil {
			return e
		}
		if n == 0 {
			return fmt.Errorf("short diagnostic write")
		}
		b = b[n:]
	}
	return unix.Fsync(fd)
}
func (r *secureRoot) read(limit, files int) ([]byte, error) {
	var out []byte
	for i := files - 1; i >= 0; i-- {
		fd, err := r.open(fmt.Sprintf("events-%d.jsonl", i), false)
		if err == syscall.ENOENT {
			continue
		}
		if err != nil {
			return nil, err
		}
		var stat unix.Stat_t
		if err = unix.Fstat(fd, &stat); err != nil || stat.Size > int64(limit) {
			unix.Close(fd)
			return nil, fmt.Errorf("diagnostic file exceeds bound")
		}
		file := os.NewFile(uintptr(fd), filepath.Base(fmt.Sprintf("events-%d.jsonl", i)))
		buf := make([]byte, stat.Size)
		_, err = file.ReadAt(buf, 0)
		file.Close()
		if err != nil && !errors.Is(err, io.EOF) {
			return nil, err
		}
		out = append(out, buf...)
	}
	return out, nil
}
