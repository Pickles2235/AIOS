//go:build darwin || linux

package discover

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/sys/unix"
)

var (
	errNotRegular = errors.New("path is not a regular file")
	errSymlink    = errors.New("path contains a symbolic link")
	errTooLarge   = errors.New("file exceeds configured size limit")
)

// readRegularFile resolves rel from an open descriptor for root. Every path
// component is opened without following symbolic links, so path replacement
// between discovery and ingestion cannot redirect the read outside root.
func readRegularFile(root, rel string, maxBytes int64) ([]byte, error) {
	parts, err := safeRelativeParts(rel)
	if err != nil {
		return nil, err
	}

	rootFD, err := unix.Open(root, unix.O_RDONLY|unix.O_CLOEXEC|unix.O_DIRECTORY|unix.O_NOFOLLOW, 0)
	if err != nil {
		return nil, classifyOpenError(root, err)
	}
	defer unix.Close(rootFD)

	dirFD := rootFD
	for _, part := range parts[:len(parts)-1] {
		nextFD, openErr := unix.Openat(dirFD, part, unix.O_RDONLY|unix.O_CLOEXEC|unix.O_DIRECTORY|unix.O_NOFOLLOW, 0)
		if dirFD != rootFD {
			_ = unix.Close(dirFD)
		}
		if openErr != nil {
			return nil, classifyOpenError(rel, openErr)
		}
		dirFD = nextFD
	}
	if dirFD != rootFD {
		defer unix.Close(dirFD)
	}

	fd, err := unix.Openat(dirFD, parts[len(parts)-1], unix.O_RDONLY|unix.O_CLOEXEC|unix.O_NOFOLLOW|unix.O_NONBLOCK, 0)
	if err != nil {
		return nil, classifyOpenError(rel, err)
	}
	file := os.NewFile(uintptr(fd), rel)
	if file == nil {
		_ = unix.Close(fd)
		return nil, fmt.Errorf("open %q: invalid file descriptor", rel)
	}
	defer file.Close()

	before, err := file.Stat()
	if err != nil {
		return nil, fmt.Errorf("stat %q: %w", rel, err)
	}
	if !before.Mode().IsRegular() {
		return nil, fmt.Errorf("%w: %q", errNotRegular, rel)
	}
	if before.Size() > maxBytes {
		return nil, fmt.Errorf("%w: %q", errTooLarge, rel)
	}

	content, err := io.ReadAll(io.LimitReader(file, maxBytes+1))
	if err != nil {
		return nil, fmt.Errorf("read %q: %w", rel, err)
	}
	after, err := file.Stat()
	if err != nil {
		return nil, fmt.Errorf("restat %q: %w", rel, err)
	}
	if !after.Mode().IsRegular() || !os.SameFile(before, after) || before.Size() != after.Size() ||
		before.Mode() != after.Mode() || !before.ModTime().Equal(after.ModTime()) || int64(len(content)) != after.Size() {
		return nil, fmt.Errorf("file %q changed while being read", rel)
	}
	return content, nil
}

func safeRelativeParts(rel string) ([]string, error) {
	if rel == "" || rel == "." || filepath.IsAbs(rel) {
		return nil, fmt.Errorf("unsafe relative path %q", rel)
	}
	for _, part := range strings.FieldsFunc(rel, func(r rune) bool { return r == '/' || r == '\\' }) {
		if part == "." || part == ".." {
			return nil, fmt.Errorf("unsafe relative path %q", rel)
		}
	}
	rel = filepath.Clean(filepath.FromSlash(rel))
	parts := strings.Split(rel, string(filepath.Separator))
	for _, part := range parts {
		if part == "" || part == "." || part == ".." {
			return nil, fmt.Errorf("unsafe relative path %q", rel)
		}
	}
	return parts, nil
}

func classifyOpenError(path string, err error) error {
	if errors.Is(err, unix.ELOOP) {
		return fmt.Errorf("%w: %q", errSymlink, path)
	}
	return fmt.Errorf("open %q: %w", path, err)
}
