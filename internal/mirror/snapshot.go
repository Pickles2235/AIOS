package mirror

import (
	"archive/tar"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/AdamNi-7080/AIOS/internal/model"
)

// Snapshot materializes a Git archive below the data directory. It is not a
// worktree: it has no .git directory and becomes read-only once complete.
func Snapshot(ctx context.Context, mirrorPath, dataDir, repositoryID, revision, fingerprint string) (string, error) {
	root := filepath.Join(dataDir, "snapshots", repositoryID, revision, fingerprint)
	if info, err := os.Stat(root); err == nil && info.IsDir() {
		return root, nil
	} else if err != nil && !os.IsNotExist(err) {
		return "", err
	}
	if err := os.MkdirAll(filepath.Join(dataDir, "snapshots"), 0700); err != nil {
		return "", err
	}
	temp, err := os.MkdirTemp(filepath.Join(dataDir, "snapshots"), ".staging-")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(temp)
	cmdOut, err := gitArchive(ctx, mirrorPath, revision)
	if err != nil {
		return "", err
	}
	tr := tar.NewReader(strings.NewReader(string(cmdOut)))
	for {
		h, e := tr.Next()
		if e == io.EOF {
			break
		}
		if e != nil {
			return "", e
		}
		if filepath.IsAbs(h.Name) || strings.Contains(filepath.ToSlash(h.Name), "../") {
			return "", fmt.Errorf("unsafe archive path %q", h.Name)
		}
		path := filepath.Join(temp, filepath.FromSlash(h.Name))
		switch h.Typeflag {
		case tar.TypeXGlobalHeader:
			continue
		case tar.TypeDir:
			if e = os.MkdirAll(path, 0700); e != nil {
				return "", e
			}
		case tar.TypeReg:
			if e = os.MkdirAll(filepath.Dir(path), 0700); e != nil {
				return "", e
			}
			f, e := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
			if e != nil {
				return "", e
			}
			_, e = io.Copy(f, tr)
			closeErr := f.Close()
			if e != nil {
				return "", e
			}
			if closeErr != nil {
				return "", closeErr
			}
		default:
			return "", fmt.Errorf("archive contains unsupported entry %q", h.Name)
		}
	}
	if err = os.MkdirAll(filepath.Dir(root), 0700); err != nil {
		return "", err
	}
	if err = os.Rename(temp, root); err != nil {
		if os.IsExist(err) {
			return root, nil
		}
		return "", err
	}
	if err = makeReadOnly(root); err != nil {
		return "", err
	}
	return root, nil
}

func gitArchive(ctx context.Context, mirrorPath, revision string) ([]byte, error) {
	// git() returns textual output, while tar can include NUL. Invoke through a
	// temporary file-free pipe with the same constrained command settings.
	return archiveBytes(ctx, mirrorPath, revision)
}

func TreeChanges(ctx context.Context, mirrorPath, source, target string) ([]model.FileChange, error) {
	if source == "" {
		return nil, nil
	}
	out, err := git(ctx, mirrorPath, "diff", "--name-status", "-z", "--find-renames=100%", source, target)
	if err != nil {
		return nil, err
	}
	parts := strings.Split(strings.TrimSuffix(out, "\x00"), "\x00")
	var changes []model.FileChange
	for i := 0; i < len(parts); {
		kind := parts[i]
		i++
		if i >= len(parts) {
			break
		}
		path := parts[i]
		i++
		switch {
		case strings.HasPrefix(kind, "A"):
			changes = append(changes, model.FileChange{Kind: "added", Path: path})
		case strings.HasPrefix(kind, "M"):
			changes = append(changes, model.FileChange{Kind: "modified", Path: path})
		case strings.HasPrefix(kind, "D"):
			changes = append(changes, model.FileChange{Kind: "removed", Path: path})
		case strings.HasPrefix(kind, "R") && i < len(parts):
			old := path
			path = parts[i]
			i++
			changes = append(changes, model.FileChange{Kind: "renamed", Path: path, OldPath: old})
		}
	}
	return changes, nil
}
