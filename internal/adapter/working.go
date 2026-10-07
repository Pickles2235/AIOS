package adapter

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"

	"github.com/AdamNi-7080/AIOS/internal/catalog"
	"github.com/AdamNi-7080/AIOS/internal/discover"
	"github.com/AdamNi-7080/AIOS/internal/lifecycle"
	"github.com/AdamNi-7080/AIOS/internal/model"
	"github.com/AdamNi-7080/AIOS/internal/snapshotlease"
)

type capturedFile struct {
	path   string
	info   os.FileInfo
	change string
	digest string
}

// Internal checkpoint permits deterministic fault injection at a real capture
// boundary; it is never configured from source or HTTP input.
type captureCheckpointKey struct{}

// ctime detects writes that restore the original bytes, size and modification
// time. Reading may change atime, which deliberately is not compared.
func changeStamp(info os.FileInfo) string {
	v := reflect.ValueOf(info.Sys())
	if v.Kind() != reflect.Pointer || v.IsNil() {
		return ""
	}
	v = v.Elem()
	if v.Kind() != reflect.Struct {
		return ""
	}
	for _, name := range []string{"Ctim", "Ctimespec"} {
		field := v.FieldByName(name)
		if field.IsValid() && field.CanInterface() {
			return fmt.Sprint(field.Interface())
		}
	}
	return ""
}

func sameCaptureFile(a, b os.FileInfo, change string) bool {
	return os.SameFile(a, b) && a.Mode() == b.Mode() && a.Size() == b.Size() && a.ModTime() == b.ModTime() && change == changeStamp(b)
}

func workingPaths(ctx context.Context, root string, limits model.Limits) ([]string, error) {
	listed, err := localGit(ctx, root, "ls-files", "--cached", "--others", "--exclude-standard", "-z")
	if err != nil {
		return nil, err
	}
	seen := map[string]string{}
	for _, path := range strings.Split(listed, "\x00") {
		if path == "" {
			continue
		}
		if len(path) > 4096 || strings.Contains(path, "\\") || filepath.IsAbs(path) || filepath.ToSlash(filepath.Clean(path)) != path || path == ".." || strings.HasPrefix(path, "../") {
			return nil, fmt.Errorf("unsafe working-tree path")
		}
		for _, part := range strings.Split(path, "/") {
			if strings.EqualFold(part, ".git") {
				return nil, fmt.Errorf("reserved working-tree path")
			}
		}
		folded := strings.ToLower(path)
		if previous, exists := seen[folded]; exists && previous != path {
			return nil, fmt.Errorf("case-colliding working-tree paths")
		}
		seen[folded] = path
		if len(seen) > 100000 {
			return nil, fmt.Errorf("working file listing exceeds bound")
		}
	}
	paths := make([]string, 0, len(seen))
	for _, path := range seen {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	return paths, nil
}

func readWorkingFile(root *os.Root, path string, max int64) ([]byte, os.FileInfo, error) {
	// Reject symlinks in every component; Root additionally confines a raced
	// replacement to the approved workspace instead of following it outside.
	parts := strings.Split(path, "/")
	for i := range parts {
		info, err := root.Lstat(strings.Join(parts[:i+1], "/"))
		if err != nil {
			return nil, nil, err
		}
		if info.Mode()&os.ModeSymlink != 0 || (i < len(parts)-1 && !info.IsDir()) {
			return nil, nil, fmt.Errorf("snapshot rejects symlinks and unsupported entries")
		}
	}
	before, err := root.Lstat(path)
	if err != nil {
		return nil, nil, err
	}
	if !before.Mode().IsRegular() || before.Size() > max {
		return nil, nil, fmt.Errorf("snapshot rejects nonregular or oversized file")
	}
	f, err := openWorkingFile(root, path)
	if err != nil {
		return nil, nil, err
	}
	defer f.Close()
	opened, err := f.Stat()
	if err != nil || !sameCaptureFile(before, opened, changeStamp(before)) {
		return nil, nil, fmt.Errorf("working file changed while opening")
	}
	content, err := io.ReadAll(io.LimitReader(f, max+1))
	if err != nil {
		return nil, nil, err
	}
	after, err := f.Stat()
	if err != nil || int64(len(content)) > max || !sameCaptureFile(opened, after, changeStamp(opened)) {
		return nil, nil, fmt.Errorf("working file changed while reading")
	}
	current, err := root.Lstat(path)
	if err != nil || !sameCaptureFile(opened, current, changeStamp(opened)) {
		return nil, nil, fmt.Errorf("working file replaced while reading")
	}
	return content, opened, nil
}

// CaptureLocal freezes eligible current working bytes, including tracked edits
// and untracked files. It does not checkout, reset, stash or write the workspace.
// Two complete reads plus metadata/list/Git checks reject mixed capture epochs.
func CaptureLocal(ctx context.Context, entry LocalRepository, dataDir string, limits model.Limits) (Discovery, error) {
	return CaptureLocalScoped(ctx, entry, dataDir, limits, model.Repository{ID: entry.ID})
}

func CaptureLocalScoped(ctx context.Context, entry LocalRepository, dataDir string, limits model.Limits, scope model.Repository) (Discovery, error) {
	return CaptureLocalScopedWithPrepare(ctx, entry, dataDir, limits, scope, nil)
}

// CaptureLocalScopedWithPrepare registers the known final root before publication.
func CaptureLocalScopedWithPrepare(ctx context.Context, entry LocalRepository, dataDir string, limits model.Limits, scope model.Repository, beforePublish func(context.Context, string) error) (Discovery, error) {
	if err := catalog.Validate(catalog.Config{Version: 1, Limits: limits, Sources: []catalog.Source{{Kind: model.SourceKindRepository, ID: entry.ID}}}); err != nil {
		return Discovery{}, err
	}
	state, err := InspectLocal(ctx, entry, dataDir)
	if err != nil {
		return Discovery{}, err
	}
	if err = ValidateLocalRegistry(LocalRegistry{Version: 1, Repositories: []LocalRepository{entry}}); err != nil {
		return Discovery{}, err
	}
	paths, err := workingPaths(ctx, entry.Path, limits)
	if err != nil {
		return Discovery{}, err
	}
	status, err := localGit(ctx, entry.Path, "status", "--porcelain=v1", "-z", "--untracked-files=all")
	if err != nil {
		return Discovery{}, err
	}
	root, err := os.OpenRoot(entry.Path)
	if err != nil {
		return Discovery{}, err
	}
	defer root.Close()
	rootInfo, err := root.Stat(".")
	if err != nil {
		return Discovery{}, err
	}
	if err = lifecycle.PrepareDir(dataDir); err != nil {
		return Discovery{}, err
	}
	// Repository lease protects the entire staging lifetime, before temp creation.
	leaseRoot := filepath.Join(dataDir, "snapshots", entry.ID, "capture", "lease")
	lease, err := snapshotlease.Acquire(dataDir, leaseRoot)
	if err != nil {
		return Discovery{}, err
	}
	keepLease := false
	defer func() {
		if !keepLease {
			_ = lease.Close()
		}
	}()
	parent := filepath.Join(dataDir, "snapshots", entry.ID, state.Commit)
	if err = lifecycle.PrepareDir(parent); err != nil {
		return Discovery{}, err
	}
	stagingParent := filepath.Join(dataDir, "snapshots", entry.ID, ".staging")
	if err = lifecycle.PrepareDir(stagingParent); err != nil {
		return Discovery{}, err
	}
	temp, err := createPreparedLocalStaging(ctx, stagingParent, beforePublish)
	if err != nil {
		return Discovery{}, err
	}
	published := false
	defer func() {
		if !published {
			_ = filepath.Walk(temp, func(path string, info os.FileInfo, e error) error {
				if e == nil && info.IsDir() {
					return os.Chmod(path, 0700)
				}
				return e
			})
			_ = os.RemoveAll(temp)
		}
	}()
	var total int64
	files := make([]capturedFile, 0, len(paths))
	coverage := model.CoverageReport{}
	missing := map[string]bool{}
	hash := sha256.New()
	fmt.Fprintf(hash, "%s\x00", state.Commit)
	for _, path := range paths {
		if err = ctx.Err(); err != nil {
			return Discovery{}, err
		}
		if reason := discover.PathExclusionReason(scope, path); reason != "" {
			coverage.Entries = append(coverage.Entries, model.CoverageEntry{Path: path, Outcome: "excluded", Reason: reason})
			continue
		}
		content, info, e := readWorkingFile(root, path, limits.MaxFileBytes)
		if os.IsNotExist(e) {
			missing[path] = true
			continue
		}
		if e != nil {
			return Discovery{}, e
		}
		total += int64(len(content))
		if total > limits.MaxTotalBytesPerRepo {
			return Discovery{}, fmt.Errorf("snapshot exceeds total byte limit")
		}
		digest := sha256.Sum256(content)
		sum := hex.EncodeToString(digest[:])
		fmt.Fprintf(hash, "%d:%s:%s\x00", len(path), path, sum)
		files = append(files, capturedFile{path, info, changeStamp(info), sum})
		if len(files) > limits.MaxFilesPerRepo {
			return Discovery{}, fmt.Errorf("snapshot exceeds eligible file count limit")
		}
		dest := filepath.Join(temp, filepath.FromSlash(path))
		if err = os.MkdirAll(filepath.Dir(dest), 0700); err != nil {
			return Discovery{}, err
		}
		f, e := os.OpenFile(dest, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0400)
		if e != nil {
			return Discovery{}, e
		}
		_, err = f.Write(content)
		if err == nil {
			err = f.Sync()
		}
		closeErr := f.Close()
		if err == nil {
			err = closeErr
		}
		if err != nil {
			return Discovery{}, err
		}
	}
	if checkpoint, ok := ctx.Value(captureCheckpointKey{}).(func()); ok {
		checkpoint()
	}
	for _, file := range files {
		if err = ctx.Err(); err != nil {
			return Discovery{}, err
		}
		content, info, e := readWorkingFile(root, file.path, limits.MaxFileBytes)
		if e != nil {
			return Discovery{}, fmt.Errorf("working tree changed during capture")
		}
		digest := sha256.Sum256(content)
		if !sameCaptureFile(file.info, info, file.change) || hex.EncodeToString(digest[:]) != file.digest {
			return Discovery{}, fmt.Errorf("working tree changed during capture")
		}
	}
	for _, file := range files {
		info, e := root.Lstat(file.path)
		if e != nil || !sameCaptureFile(file.info, info, file.change) {
			return Discovery{}, fmt.Errorf("working tree changed during validation")
		}
	}
	for path := range missing {
		if _, e := root.Lstat(path); !os.IsNotExist(e) {
			return Discovery{}, fmt.Errorf("working tree changed during capture")
		}
	}
	afterPaths, e := workingPaths(ctx, entry.Path, limits)
	if e != nil || !reflect.DeepEqual(paths, afterPaths) {
		return Discovery{}, fmt.Errorf("working-tree file set changed during capture")
	}
	after, e := InspectLocal(ctx, entry, dataDir)
	if e != nil || after != state {
		return Discovery{}, fmt.Errorf("working-tree Git state changed during capture")
	}
	afterStatus, e := localGit(ctx, entry.Path, "status", "--porcelain=v1", "-z", "--untracked-files=all")
	if e != nil || afterStatus != status {
		return Discovery{}, fmt.Errorf("working-tree index changed during capture")
	}
	currentRoot, e := os.Stat(entry.Path)
	if e != nil || !os.SameFile(rootInfo, currentRoot) {
		return Discovery{}, fmt.Errorf("working-tree root changed during capture")
	}
	revision := state.Commit
	if state.Dirty || state.UntrackedCount > 0 {
		revision += "-working-" + hex.EncodeToString(hash.Sum(nil))
	}
	pathHash := sha256.Sum256([]byte(entry.Path))
	scopeBytes, _ := json.Marshal(struct{ Include, Exclude []string }{scope.Include, scope.Exclude})
	revisionHash := sha256.Sum256(append([]byte(revision+"\x00"+hex.EncodeToString(hash.Sum(nil))), scopeBytes...))
	final := filepath.Join(parent, "local-"+hex.EncodeToString(pathHash[:])+"-"+hex.EncodeToString(revisionHash[:]))
	if beforePublish != nil {
		if err = beforePublish(ctx, final); err != nil {
			return Discovery{}, err
		}
	}
	if err = filepath.Walk(temp, func(path string, info os.FileInfo, e error) error {
		if e != nil {
			return e
		}
		if info.IsDir() {
			dir, err := os.Open(path)
			if err != nil {
				return err
			}
			err = dir.Sync()
			dir.Close()
			if err != nil {
				return err
			}
			return os.Chmod(path, 0500)
		}
		return nil
	}); err != nil {
		return Discovery{}, err
	}
	discovery := Discovery{Identity: model.SourceIdentity{ID: entry.ID, Kind: model.SourceKindRepository, AdapterVersion: model.RepositoryAdapterVersion}, Revision: revision, Root: final, Git: state, Coverage: coverage, Lease: lease}
	if _, e = os.Lstat(final); e == nil {
		one, e := snapshotDigest(temp)
		if e != nil {
			return Discovery{}, e
		}
		two, e := snapshotDigest(final)
		if e != nil || one != two {
			return Discovery{}, fmt.Errorf("owned snapshot differs from validated working tree")
		}
		keepLease = true
		return discovery, nil
	} else if !os.IsNotExist(e) {
		return Discovery{}, e
	}
	if err = os.Chmod(temp, 0700); err != nil {
		return Discovery{}, err
	}
	if err = os.Rename(temp, final); err != nil {
		return Discovery{}, err
	}
	temp = final
	if err = os.Chmod(final, 0500); err != nil {
		return Discovery{}, err
	}
	dir, err := os.Open(parent)
	if err != nil {
		return Discovery{}, err
	}
	err = dir.Sync()
	dir.Close()
	if err != nil {
		return Discovery{}, err
	}
	published = true
	keepLease = true
	return discovery, nil
}

func createPreparedLocalStaging(ctx context.Context, parent string, prepare func(context.Context, string) error) (string, error) {
	for attempt := 0; attempt < 8; attempt++ {
		suffix := make([]byte, 16)
		if _, err := rand.Read(suffix); err != nil {
			return "", err
		}
		path := filepath.Join(parent, ".staging-local-"+hex.EncodeToString(suffix))
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
	return "", fmt.Errorf("could not allocate unique local staging root")
}
