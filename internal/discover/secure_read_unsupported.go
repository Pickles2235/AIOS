//go:build !darwin && !linux && !windows

package discover

import (
	"errors"
	"fmt"
)

var (
	errNotRegular = errors.New("path is not a regular file")
	errSymlink    = errors.New("path contains a symbolic link")
	errTooLarge   = errors.New("file exceeds configured size limit")
)

// Fail closed until the platform has descriptor-relative, no-follow traversal.
func readRegularFile(_, rel string, _ int64) ([]byte, error) {
	return nil, fmt.Errorf("secure file discovery is unsupported on this platform for %q", rel)
}
