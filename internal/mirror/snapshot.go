package mirror

import (
	"archive/tar"
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/AdamNi-7080/AIOS/internal/model"
)

func SnapshotPath(dataDir, repositoryID, revision, fingerprint string) string {
	return filepath.Join(dataDir, "snapshots", repositoryID, revision, fingerprint)
}

// Full mirror capture is bounded separately from eligible source discovery.
// Its 6 GiB payload plus up to 300,000 entries and bounded tar framing fits
// the 8 GiB capture reserve.
const MaxSnapshotBytes int64 = 6 << 30
const MaxSnapshotFiles = 300_000

// Snapshot materializes a Git archive below the data directory. It is not a
// worktree: it has no .git directory and becomes read-only once complete.
func Snapshot(ctx context.Context, mirrorPath, dataDir, repositoryID, revision, fingerprint string, maxBytes int64, maxFiles int) (string, error) {
	return SnapshotWithPrepare(ctx, mirrorPath, dataDir, repositoryID, revision, fingerprint, maxBytes, maxFiles, nil)
}

// SnapshotWithPrepare durably registers its exact staging root before creation.
func SnapshotWithPrepare(ctx context.Context, mirrorPath, dataDir, repositoryID, revision, fingerprint string, maxBytes int64, maxFiles int, prepare func(context.Context, string) error) (string, error) {
	if maxBytes <= 0 || maxBytes > MaxSnapshotBytes || maxFiles <= 0 || maxFiles > MaxSnapshotFiles {
		return "", fmt.Errorf("invalid snapshot bounds")
	}
	root := SnapshotPath(dataDir, repositoryID, revision, fingerprint)
	if err := ensureOwnedSnapshotParents(dataDir, repositoryID, revision, fingerprint); err != nil {
		return "", err
	}
	if info, err := os.Lstat(root); err == nil {
		if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return "", fmt.Errorf("snapshot root is unsafe")
		}
		return root, nil
	} else if !os.IsNotExist(err) {
		return "", err
	}
	stagingParent := filepath.Join(dataDir, "snapshots", repositoryID, ".staging")
	if err := os.Mkdir(stagingParent, 0700); err != nil && !os.IsExist(err) {
		return "", err
	}
	stagingInfo, err := os.Lstat(stagingParent)
	if err != nil {
		return "", err
	}
	if !stagingInfo.IsDir() || stagingInfo.Mode()&os.ModeSymlink != 0 {
		return "", fmt.Errorf("unsafe snapshot staging parent")
	}
	temp, err := createPreparedMirrorStaging(ctx, stagingParent, prepare)
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(temp)
	processCtx, stop := context.WithCancel(ctx)
	defer stop()
	cmd, pipe, err := archiveStream(processCtx, mirrorPath, revision)
	if err != nil {
		return "", err
	}
	waited := false
	defer func() {
		if !waited {
			pipe.Close()
			stop()
			_ = cmd.Wait()
		}
	}()
	// Tar framing and PAX metadata have bounded overhead beyond extracted bytes.
	maxTransport := maxBytes + int64(maxFiles)*4096 + 1<<20
	limited := &io.LimitedReader{R: pipe, N: maxTransport + 1}
	tr := tar.NewReader(limited)
	var total int64
	count := 0
	for {
		h, e := tr.Next()
		if e == io.EOF {
			break
		}
		if e != nil {
			return "", e
		}
		path := filepath.Join(temp, filepath.FromSlash(h.Name))
		rel, relErr := filepath.Rel(temp, path)
		if filepath.IsAbs(h.Name) || relErr != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return "", fmt.Errorf("unsafe archive path %q", h.Name)
		}
		switch h.Typeflag {
		case tar.TypeXGlobalHeader:
			continue
		case tar.TypeDir:
			if e = os.MkdirAll(path, 0700); e != nil {
				return "", e
			}
		case tar.TypeReg:
			count++
			if count > maxFiles || h.Size < 0 || h.Size > maxBytes-total {
				return "", fmt.Errorf("snapshot exceeds file or byte limit")
			}
			total += h.Size
			if e = os.MkdirAll(filepath.Dir(path), 0700); e != nil {
				return "", e
			}
			f, e := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
			if e != nil {
				return "", e
			}
			_, e = io.CopyN(f, tr, h.Size)
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
	if _, err = io.Copy(io.Discard, limited); err != nil {
		return "", err
	}
	if limited.N == 0 {
		return "", fmt.Errorf("archive transport exceeds byte limit")
	}
	err = cmd.Wait()
	waited = true
	if err != nil {
		reason := "local mirror archive failed"
		if captured, ok := cmd.Stderr.(*archiveStderr); ok {
			reason = captured.reason()
		}
		return "", fmt.Errorf("%s: %w", reason, err)
	}
	if err = os.Rename(temp, root); err != nil {
		if os.IsExist(err) {
			return root, nil
		}
		return "", err
	}
	if err = makeReadOnly(root); err != nil {
		_ = filepath.Walk(root, func(path string, info os.FileInfo, walkErr error) error {
			if walkErr == nil && info.IsDir() {
				return os.Chmod(path, 0700)
			}
			return walkErr
		})
		_ = os.RemoveAll(root)
		return "", err
	}
	return root, nil
}

func createPreparedMirrorStaging(ctx context.Context, parent string, prepare func(context.Context, string) error) (string, error) {
	for attempt := 0; attempt < 8; attempt++ {
		suffix := make([]byte, 16)
		if _, err := rand.Read(suffix); err != nil {
			return "", err
		}
		path := filepath.Join(parent, ".staging-mirror-"+hex.EncodeToString(suffix))
		if _, err := os.Lstat(path); err == nil {
			continue
		} else if !os.IsNotExist(err) {
			return "", err
		}
		if prepare != nil {
			if err := prepare(ctx, path); err != nil {
				return "", err
			}
		}
		if err := os.Mkdir(path, 0700); err == nil {
			return path, nil
		} else if !os.IsExist(err) {
			return "", err
		}
	}
	return "", fmt.Errorf("could not allocate unique mirror staging root")
}

func ensureOwnedSnapshotParents(dataDir, repositoryID, revision, fingerprint string) error {
	if !safeSnapshotSegment(repositoryID) || !safeSnapshotSegment(revision) || !safeSnapshotSegment(fingerprint) {
		return fmt.Errorf("invalid owned snapshot path")
	}
	for _, dir := range []string{filepath.Join(dataDir, "snapshots"), filepath.Join(dataDir, "snapshots", repositoryID), filepath.Join(dataDir, "snapshots", repositoryID, revision)} {
		if err := os.Mkdir(dir, 0700); err != nil && !os.IsExist(err) {
			return err
		}
		info, err := os.Lstat(dir)
		if err != nil {
			return err
		}
		if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("unsafe snapshot parent")
		}
	}
	return nil
}

func safeSnapshotSegment(s string) bool {
	return s != "" && s != "." && s != ".." && !strings.ContainsAny(s, "/\\")
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
