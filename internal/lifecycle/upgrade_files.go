package lifecycle

import (
	"archive/zip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

func syncUpgradeDir(path string) error {
	f, e := os.Open(path)
	if e != nil {
		return e
	}
	defer f.Close()
	return f.Sync()
}
func durableUpgradeRename(from, to string) error {
	if e := os.Rename(from, to); e != nil {
		return e
	}
	if e := syncUpgradeDir(filepath.Dir(from)); e != nil {
		return e
	}
	if filepath.Dir(from) != filepath.Dir(to) {
		return syncUpgradeDir(filepath.Dir(to))
	}
	return nil
}
func ownedFileDigest(path string) (string, error) {
	info, e := os.Lstat(path)
	if e != nil {
		return "", e
	}
	if !owned(info) || !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
		return "", fmt.Errorf("upgrade requires owned regular files")
	}
	f, e := os.Open(path)
	if e != nil {
		return "", e
	}
	defer f.Close()
	h := sha256.New()
	_, e = io.Copy(h, f)
	return hex.EncodeToString(h.Sum(nil)), e
}
func readUpgradeJSON(dir, name string, value any, limit int64) error {
	f, e := openOwnedBounded(filepath.Join(dir, name), limit)
	if e != nil {
		return e
	}
	defer f.Close()
	d := json.NewDecoder(io.LimitReader(f, limit+1))
	d.DisallowUnknownFields()
	if e = d.Decode(value); e != nil {
		return e
	}
	if d.Decode(&struct{}{}) != io.EOF {
		return fmt.Errorf("invalid trailing upgrade metadata")
	}
	return nil
}
func copyUpgradeFile(ctx context.Context, from, to string, mode os.FileMode) error {
	if e := ctx.Err(); e != nil {
		return e
	}
	info, e := os.Lstat(from)
	if e != nil {
		return e
	}
	if !owned(info) || !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("upgrade snapshot rejects foreign, linked or special files")
	}
	in, e := os.Open(from)
	if e != nil {
		return e
	}
	defer in.Close()
	opened, e := in.Stat()
	if e != nil || !os.SameFile(info, opened) {
		return fmt.Errorf("upgrade source changed during open")
	}
	out, e := os.OpenFile(to, os.O_WRONLY|os.O_CREATE|os.O_EXCL, mode)
	if e != nil {
		return e
	}
	defer out.Close()
	n, e := io.Copy(out, contextUpgradeReader{ctx, in})
	if e == nil && n != info.Size() {
		e = fmt.Errorf("upgrade snapshot file changed")
	}
	if e != nil {
		return e
	}
	last, e := in.Stat()
	if e != nil || last.Size() != info.Size() || last.ModTime() != info.ModTime() {
		return fmt.Errorf("upgrade snapshot changed during copy")
	}
	pathInfo, e := os.Lstat(from)
	if e != nil || !os.SameFile(info, pathInfo) || pathInfo.Size() != info.Size() || pathInfo.ModTime() != info.ModTime() {
		return fmt.Errorf("upgrade snapshot source path changed during copy")
	}
	if e = out.Sync(); e != nil {
		return e
	}
	return out.Close()
}

type contextUpgradeReader struct {
	ctx context.Context
	r   io.Reader
}

func (r contextUpgradeReader) Read(p []byte) (int, error) {
	if e := r.ctx.Err(); e != nil {
		return 0, e
	}
	return r.r.Read(p)
}
func upgradeTreeBytes(root string) (uint64, error) {
	var total uint64
	e := filepath.WalkDir(root, func(path string, d fs.DirEntry, e error) error {
		if e != nil {
			return e
		}
		info, e := d.Info()
		if e != nil {
			return e
		}
		if !owned(info) || info.Mode()&os.ModeSymlink != 0 || (!info.IsDir() && !info.Mode().IsRegular() && info.Mode()&os.ModeSocket == 0) {
			return fmt.Errorf("upgrade state must be owned and unlinked")
		}
		if info.Mode().IsRegular() {
			if info.Size() < 0 || uint64(info.Size()) > (100<<30)-total {
				return fmt.Errorf("upgrade state exceeds 100 GiB snapshot bound")
			}
			total += uint64(info.Size())
		}
		return nil
	})
	return total, e
}
func copyUpgradeTree(ctx context.Context, from, to string) error {
	if e := os.Mkdir(to, 0700); e != nil {
		return e
	}
	var dirs []string
	e := filepath.WalkDir(from, func(path string, d fs.DirEntry, e error) error {
		if e != nil {
			return e
		}
		if e = ctx.Err(); e != nil {
			return e
		}
		rel, e := filepath.Rel(from, path)
		if e != nil {
			return e
		}
		if rel == "." {
			dirs = append(dirs, to)
			return nil
		}
		info, e := d.Info()
		if e != nil {
			return e
		}
		if !owned(info) || info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("upgrade state contains foreign or linked assets")
		}
		// These leases and the closed control endpoint describe the old process,
		// never its durable knowledge. Do not copy lock inodes into the new slot.
		if filepath.Dir(rel) == "." && (rel == ".writer.lock" || rel == ".daemon.lock" || rel == ".instance.lock" || rel == "daemon.sock") {
			return nil
		}
		dest := filepath.Join(to, rel)
		if info.IsDir() {
			if e = os.Mkdir(dest, 0700); e == nil {
				dirs = append(dirs, dest)
			}
			return e
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("upgrade state contains special files")
		}
		return copyUpgradeFile(ctx, path, dest, info.Mode().Perm()&0700)
	})
	if e != nil {
		return e
	}
	for i := len(dirs) - 1; i >= 0; i-- {
		if e = syncUpgradeDir(dirs[i]); e != nil {
			return e
		}
	}
	return syncUpgradeDir(filepath.Dir(to))
}
func extractUpgradeArchive(ctx context.Context, reader *zip.ReadCloser, m Manifest, prefix, dest string) error {
	if e := os.Mkdir(dest, 0700); e != nil {
		return e
	}
	for _, z := range reader.File {
		if e := ctx.Err(); e != nil {
			return e
		}
		if z.Mode().IsDir() {
			continue
		}
		rel := strings.TrimPrefix(z.Name, prefix)
		if rel == "manifest.json" {
			if e := writeOwnedJSON(dest, rel, m); e != nil {
				return e
			}
			continue
		}
		path := filepath.Join(dest, filepath.FromSlash(rel))
		if e := os.MkdirAll(filepath.Dir(path), 0700); e != nil {
			return e
		}
		mode := os.FileMode(0600)
		if rel == "bin/aios" || strings.HasSuffix(rel, ".sh") {
			mode = 0700
		}
		out, e := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, mode)
		if e != nil {
			return e
		}
		in, e := z.Open()
		if e != nil {
			out.Close()
			return e
		}
		h := sha256.New()
		n, e := io.Copy(io.MultiWriter(out, h), contextUpgradeReader{ctx, io.LimitReader(in, int64(z.UncompressedSize64)+1)})
		in.Close()
		if e == nil && (n != int64(z.UncompressedSize64) || hex.EncodeToString(h.Sum(nil)) != m.SHA256[z.Name]) {
			e = fmt.Errorf("candidate changed during extraction")
		}
		if e == nil {
			e = out.Sync()
		}
		ce := out.Close()
		if e != nil {
			return e
		}
		if ce != nil {
			return ce
		}
	}
	var dirs []string
	e := filepath.WalkDir(dest, func(path string, d fs.DirEntry, e error) error {
		if e == nil && d.IsDir() {
			dirs = append(dirs, path)
		}
		return e
	})
	if e != nil {
		return e
	}
	for i := len(dirs) - 1; i >= 0; i-- {
		if e = syncUpgradeDir(dirs[i]); e != nil {
			return e
		}
	}
	return syncUpgradeDir(filepath.Dir(dest))
}

// ValidateStagedUpgradeData confines the private candidate migration command to
// an existing transaction slot. It must never create or migrate installed data.
func ValidateStagedUpgradeData(data string) error {
	if filepath.Base(data) != "new-data" || !transactionPattern.MatchString(filepath.Base(filepath.Dir(data))) {
		return fmt.Errorf("candidate validation requires a private staged transaction slot")
	}
	stage := filepath.Dir(data)
	root := filepath.Dir(stage)
	if _, e := os.Lstat(data); e != nil {
		return e
	}
	if e := PrepareDir(data); e != nil {
		return e
	}
	// Existing directories only: this private validation boundary cannot recreate
	// a missing installed or staged tree.
	if _, e := os.Lstat(filepath.Join(root, installJournalName)); e == nil {
		j, e := readInstallJournal(root)
		if e != nil {
			return e
		}
		if j.Transaction != filepath.Base(stage) || j.Phase != "preparing" {
			return fmt.Errorf("staged install authority differs")
		}
		return nil
	} else if !os.IsNotExist(e) {
		return e
	}
	j, e := readUpgradeJournal(root)
	if e != nil {
		return e
	}
	if j.Transaction != filepath.Base(stage) || j.Phase != "staged" {
		return fmt.Errorf("staged update authority differs")
	}
	return nil
}
