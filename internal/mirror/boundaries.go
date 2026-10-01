package mirror

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

// ValidateSourceBoundaries keeps local Git remotes outside owned data, including
// canonical aliases and either direction of nesting. A local Mirror remote is
// source authority just like a Direct workspace; purge may never delete it.
func ValidateSourceBoundaries(reg Registry, dataDir string) error {
	data, err := canonicalBoundaryPath(dataDir)
	if err != nil {
		return err
	}
	for _, entry := range reg.Repositories {
		local := entry.URL
		if !filepath.IsAbs(local) {
			u, err := url.Parse(local)
			if err != nil {
				return fmt.Errorf("invalid remote boundary")
			}
			if u.Scheme != "file" {
				continue
			}
			local = u.Path
		}
		root, err := canonicalBoundaryPath(local)
		if err != nil {
			return fmt.Errorf("local remote boundary is unavailable")
		}
		for _, pair := range [][2]string{{root, data}, {data, root}} {
			rel, err := filepath.Rel(pair[0], pair[1])
			if err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
				return fmt.Errorf("owned data and local Git remote must not overlap")
			}
		}
	}
	return nil
}

// ValidatePurgeSourceBoundaries also protects legacy local approvals. An invalid
// dependent can be withdrawn first when its own assets do not contain any
// approved source; withdrawing its provider must fail before recording intent.
func ValidatePurgeSourceBoundaries(reg Registry, dataDir, id string) error {
	if !idPattern.MatchString(id) {
		return fmt.Errorf("invalid purge ID")
	}
	paths := []string{filepath.Join(dataDir, "snapshots", id), MirrorPath(dataDir, id), filepath.Join(dataDir, "compiler-cache"), filepath.Join(dataDir, "cache.db"), filepath.Join(dataDir, "cache.db-wal"), filepath.Join(dataDir, "cache.db-shm")}
	for _, pattern := range []string{"index.ir-9-backup-*.db", ".ir-9-backup-*", ".setup-*", ".maintenance-*", ".removal-*", filepath.Join("snapshots", ".staging-*")} {
		matches, err := filepath.Glob(filepath.Join(dataDir, pattern))
		if err != nil {
			return err
		}
		paths = append(paths, matches...)
	}
	for _, entry := range reg.Repositories {
		local := entry.URL
		if !filepath.IsAbs(local) {
			u, err := url.Parse(local)
			if err != nil {
				return err
			}
			if u.Scheme != "file" {
				continue
			}
			local = u.Path
		}
		source, err := canonicalBoundaryPath(local)
		if err != nil {
			return err
		}
		for _, path := range paths {
			target, err := canonicalBoundaryPath(path)
			if err != nil {
				return err
			}
			for _, pair := range [][2]string{{source, target}, {target, source}} {
				rel, err := filepath.Rel(pair[0], pair[1])
				if err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
					return fmt.Errorf("repository purge would overlap an approved local remote")
				}
			}
		}
	}
	return nil
}

// Resolve existing ancestors so an unavailable remote can still be approved for
// bounded retry without permitting a missing child under owned data or a link.
func canonicalBoundaryPath(path string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	ancestor := abs
	for {
		if _, err = os.Lstat(ancestor); err == nil {
			break
		}
		if !os.IsNotExist(err) {
			return "", err
		}
		parent := filepath.Dir(ancestor)
		if parent == ancestor {
			return "", fmt.Errorf("remote has no canonical ancestor")
		}
		ancestor = parent
	}
	real, err := filepath.EvalSymlinks(ancestor)
	if err != nil {
		return "", err
	}
	rel, err := filepath.Rel(ancestor, abs)
	if err != nil {
		return "", err
	}
	return filepath.Join(real, rel), nil
}
