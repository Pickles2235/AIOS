package store

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/AdamNi-7080/AIOS/internal/installstate"
)

var ErrTransactionPending = errors.New("installation transaction pending; run the installed recovery command before writing knowledge")

// Called while holding stable writer authority, before any directory creation or
// reset. Recovery acquires leases directly and can restore precommit state.
func checkTransactionFence(data string) error {
	if filepath.Base(data) != "data" {
		return nil
	}
	root := filepath.Dir(data)
	authority := installstate.Authority(root)
	if info, e := os.Lstat(authority); e == nil {
		if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm() != 0700 {
			return ErrTransactionPending
		}
	} else if !os.IsNotExist(e) {
		return ErrTransactionPending
	}
	uninstall := filepath.Join(authority, "uninstall.json")
	if _, e := os.Lstat(uninstall); e == nil {
		return ErrTransactionPending
	} else if !os.IsNotExist(e) {
		return fmt.Errorf("%w: %v", ErrTransactionPending, e)
	}
	for _, name := range []string{"upgrade.json", "install.json"} {
		path := filepath.Join(root, name)
		info, e := os.Lstat(path)
		if os.IsNotExist(e) {
			continue
		}
		if e != nil {
			return fmt.Errorf("%w: %v", ErrTransactionPending, e)
		}
		if e = validateOwnedRegularFile(path, info); e != nil || info.Mode().Perm() != 0600 || info.Size() > 65536 {
			return ErrTransactionPending
		}
		f, e := os.Open(path)
		if e != nil {
			return ErrTransactionPending
		}
		opened, e := f.Stat()
		if e != nil || !os.SameFile(info, opened) {
			f.Close()
			return ErrTransactionPending
		}
		d := json.NewDecoder(io.LimitReader(f, 65537))
		d.DisallowUnknownFields()
		binary := filepath.Join(root, "current", "bin", "aios")
		var digest, id string
		if name == "upgrade.json" {
			var j installstate.UpgradeJournal
			e = d.Decode(&j)
			if e == nil {
				e = installstate.ValidateUpgradeJournal(root, j)
			}
			if e == nil && j.Phase != "committed" {
				e = ErrTransactionPending
			}
			digest, id = j.CandidateBinary, j.InstanceID
		} else {
			var j installstate.InstallJournal
			e = d.Decode(&j)
			if e == nil {
				e = installstate.ValidateInstallJournal(root, j)
			}
			if e == nil && j.Phase != "committed" {
				e = ErrTransactionPending
			}
			digest, id = j.Binary, j.InstanceID
		}
		trailing := d.Decode(&struct{}{})
		f.Close()
		if e != nil || trailing != io.EOF {
			return ErrTransactionPending
		}
		binaryInfo, e := os.Lstat(binary)
		if e != nil || validateOwnedRegularFile(binary, binaryInfo) != nil {
			return ErrTransactionPending
		}

		bytes, e := os.ReadFile(binary)
		if e != nil || len(digest) != 64 || fmt.Sprintf("%x", sha256.Sum256(bytes)) != digest {
			return ErrTransactionPending
		}
		instance, e := ReadExistingInstance(data)
		if e != nil || instance.ID != id {
			return ErrTransactionPending
		}
	}
	return nil
}
