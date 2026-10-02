package store

import (
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// canonicalWriterPath shares writer authority through canonical parent aliases,
// including native /var→/private/var, without following a linked data directory.
func canonicalWriterPath(data string) (string, error) {
	abs, e := filepath.Abs(data)
	if e != nil {
		return "", e
	}
	ancestor := abs
	var missing []string
	for {
		info, e := os.Lstat(ancestor)
		if e == nil {
			if ancestor == abs && info.Mode()&os.ModeSymlink != 0 {
				return "", fmt.Errorf("linked data directory forbidden")
			}
			break
		}
		if !os.IsNotExist(e) {
			return "", e
		}
		parent := filepath.Dir(ancestor)
		if parent == ancestor {
			return "", e
		}
		missing = append(missing, filepath.Base(ancestor))
		ancestor = parent
	}
	resolved, e := filepath.EvalSymlinks(ancestor)
	if e != nil {
		return "", e
	}
	for i := len(missing) - 1; i >= 0; i-- {
		resolved = filepath.Join(resolved, missing[i])
	}
	return resolved, nil
}

func acquireDataAuthority(data string) (*writerLock, error) {
	canonical, e := canonicalWriterPath(data)
	if e != nil {
		return nil, e
	}
	base := "/tmp"
	if os.PathSeparator == '\\' {
		base = os.TempDir()
	}
	base, e = filepath.EvalSymlinks(base)
	if e != nil {
		return nil, e
	}
	dir := filepath.Join(base, fmt.Sprintf("aios-writers-%d", os.Getuid()))
	if e = os.Mkdir(dir, 0700); e != nil && !os.IsExist(e) {
		return nil, e
	}
	info, e := os.Lstat(dir)
	if e != nil {
		return nil, e
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm() != 0700 {
		return nil, fmt.Errorf("writer authority must be private and unlinked")
	}
	if e = validateDataDirectory(dir); e != nil {
		return nil, e
	}
	h := sha256.Sum256([]byte(canonical))
	return acquireWriterLock(filepath.Join(dir, fmt.Sprintf("%x.lock", h)))
}

// AcquireDataAuthority excludes writers even when activation temporarily removes
// the data path. It creates no data directory, database or canonical knowledge.
func AcquireDataAuthority(data string) (io.Closer, error) {
	l, e := acquireDataAuthority(data)
	if e != nil {
		return nil, e
	}
	return dataLease{l}, nil
}

func acquireDataWriterLock(data string) (*writerLock, error) {
	guard, e := acquireDataAuthority(data)
	if e != nil {
		return nil, e
	}
	lock, e := acquireWriterLock(filepath.Join(data, writerLockName))
	if e != nil {
		guard.close()
		return nil, e
	}
	lock.guard = guard
	return lock, nil
}
