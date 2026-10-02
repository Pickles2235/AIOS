package lifecycle

import (
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/AdamNi-7080/AIOS/internal/installstate"
	"github.com/AdamNi-7080/AIOS/internal/store"
)

const uninstallJournalName = "uninstall.json"

type uninstallJournal struct {
	Schema       int          `json:"schema_version"`
	Root         string       `json:"root"`
	Phase        string       `json:"phase"`
	Installation Installation `json:"installation"`
	Preserve     bool         `json:"preserve_data"`
	PlistSHA256  string       `json:"plist_sha256"`
	SourceRoots  []string     `json:"source_roots,omitempty"`
}

// Intent lives in persistent sibling authority, outside the tree being deleted.
// A killed recursive deletion therefore retains its exact owned-root authority.
func readUninstallJournal(root string) (uninstallJournal, error) {
	var j uninstallJournal
	dir := installstate.Authority(root)
	info, e := os.Lstat(dir)
	if e != nil {
		return j, e
	}
	if !info.IsDir() || !owned(info) || info.Mode().Perm() != 0700 || info.Mode()&os.ModeSymlink != 0 {
		return j, fmt.Errorf("invalid uninstall authority directory")
	}
	if e = readUpgradeJSON(dir, uninstallJournalName, &j, 1<<20); e != nil {
		return j, e
	}
	v := j.Installation
	if j.Schema != 1 || j.Root != root || (j.Phase != "stopping" && j.Phase != "stopped") || v.Scope != "user" || v.Binary != filepath.Join(root, "current", "bin", "aios") || v.DataDir != filepath.Join(root, "data") || !labelPattern.MatchString(v.ServiceLabel) || !shaPattern.MatchString(j.PlistSHA256) {
		return j, fmt.Errorf("invalid uninstall intent")
	}
	if e = validateGitEnvironment(v.GitEnvironment); e != nil {
		return j, e
	}
	if len(j.SourceRoots) > 200 {
		return j, fmt.Errorf("invalid uninstall source bound")
	}
	for _, source := range j.SourceRoots {
		if !filepath.IsAbs(source) || len(source) > 4096 {
			return j, fmt.Errorf("invalid uninstall source authority")
		}
	}
	return j, nil
}

func pendingUninstall(root string) error {
	if _, e := readUninstallJournal(root); os.IsNotExist(e) {
		return nil
	} else if e != nil {
		return e
	}
	return fmt.Errorf("uninstall is pending; repeat uninstall with its original preserve/delete choice")
}

func uninstallPlan(o Options, v Installation) (Plan, error) {
	o.Binary = v.Binary
	o.gitEnvironment = v.GitEnvironment
	o.recoveryProgram = &v.RecoveryLauncher
	return MakePlan(o)
}

func uninstallCandidate(ctx context.Context, o Options, preserve bool, stop func(context.Context, Options) error) error {
	return uninstallWithCheckpoint(ctx, o, preserve, stop, upgradeCheckpoint)
}
func uninstallWithCheckpoint(ctx context.Context, o Options, preserve bool, stop func(context.Context, Options) error, checkpoint func(context.Context, string, string) error) error {
	o, e := Defaults(o)
	if e != nil {
		return e
	}
	transaction, e := lockUpgrade(o.Root)
	if e != nil {
		return e
	}
	defer transaction.Close()
	dir := installstate.Authority(o.Root)
	j, e := readUninstallJournal(o.Root)
	if os.IsNotExist(e) {
		if _, e = os.Lstat(o.Root); os.IsNotExist(e) {
			return nil
		} else if e != nil {
			return e
		}
		if e = PrepareDir(o.Root); e != nil {
			return e
		}
		if _, e = os.Lstat(filepath.Join(o.Root, installJournalName)); e == nil {
			in, e := readInstallJournal(o.Root)
			if e != nil {
				return e
			}
			if in.Installation.ServiceLabel != o.Label {
				return fmt.Errorf("uninstall label differs from pending installation")
			}
			if e = stop(ctx, o); e != nil {
				return e
			}
			if e = recoverInstallLocked(ctx, o, &in); e != nil {
				return e
			}
		} else if !os.IsNotExist(e) {
			return e
		}
		if entries, e := os.ReadDir(o.Root); e == nil && len(entries) == 0 {
			if e = os.Remove(o.Root); e != nil {
				return e
			}
			return syncUpgradeDir(filepath.Dir(o.Root))
		} else if e != nil {
			return e
		}
		if _, e = os.Lstat(filepath.Join(o.Root, upgradeJournalName)); e == nil {
			in, e := readUpgradeJournal(o.Root)
			if e != nil {
				return e
			}
			if in.Previous.ServiceLabel != o.Label {
				return fmt.Errorf("uninstall label differs from pending update")
			}
			if e = stop(ctx, o); e != nil {
				return e
			}
			if e = recoverUpgradeLocked(ctx, o, &in); e != nil {
				return e
			}
		} else if !os.IsNotExist(e) {
			return e
		}
		v, e := readInstallation(o.Root)
		if e != nil {
			return e
		}
		if v.ServiceLabel != o.Label {
			return fmt.Errorf("uninstall label differs from owned installation")
		}
		roots, e := uninstallSourceRoots(v.DataDir)
		if e != nil {
			return e
		}
		if e = validateUninstallSourceRoots(o, preserve, roots); e != nil {
			return e
		}
		p, e := uninstallPlan(o, v)
		if e != nil {
			return e
		}
		if e = checkServiceFile(o, p); e != nil {
			return e
		}
		j = uninstallJournal{Schema: 1, Root: o.Root, Phase: "stopping", Installation: v, Preserve: preserve, SourceRoots: roots, PlistSHA256: fmt.Sprintf("%x", sha256.Sum256([]byte(p.Plist)))}
		if e = PrepareDir(dir); e != nil {
			return e
		}
		if e = syncUpgradeDir(filepath.Dir(dir)); e != nil {
			return e
		}
		if e = writeOwnedJSON(dir, uninstallJournalName, j); e != nil {
			return e
		}
	} else if e != nil {
		return e
	}
	if j.Installation.ServiceLabel != o.Label || j.Preserve != preserve {
		return fmt.Errorf("repeat the original uninstall label and preserve/delete choice")
	}
	if e = validateUninstallSourceRoots(o, preserve, j.SourceRoots); e != nil {
		return e
	}
	p, e := uninstallPlan(o, j.Installation)
	if e != nil {
		return e
	}
	if fmt.Sprintf("%x", sha256.Sum256([]byte(p.Plist))) != j.PlistSHA256 {
		return fmt.Errorf("uninstall service authority changed")
	}
	if e = checkpoint(ctx, o.Root, "uninstall_intent"); e != nil {
		return e
	}
	if j.Phase == "stopping" {
		if e = stop(ctx, o); e != nil {
			return e
		}
		next := j
		next.Phase = "stopped"
		if e = writeOwnedJSON(dir, uninstallJournalName, next); e != nil {
			return e
		}
		j = next
	}
	if e = checkpoint(ctx, o.Root, "uninstall_stopped"); e != nil {
		return e
	}
	var leases []io.Closer
	defer func() {
		for i := len(leases) - 1; i >= 0; i-- {
			leases[i].Close()
		}
	}()
	if _, e = os.Lstat(j.Installation.DataDir); e == nil {
		l, e := Lock(j.Installation.DataDir)
		if e != nil {
			return e
		}
		leases = append(leases, l)
		writer, e := store.AcquireDataLease(j.Installation.DataDir)
		if e != nil {
			return e
		}
		leases = append(leases, writer)
	} else if !os.IsNotExist(e) {
		return e
	} else {
		writer, e := store.AcquireDataAuthority(j.Installation.DataDir)
		if e != nil {
			return e
		}
		leases = append(leases, writer)
	}
	if roots, readErr := uninstallSourceRoots(j.Installation.DataDir); readErr != nil {
		return readErr
	} else if roots != nil {
		next := j
		next.SourceRoots = roots
		if e = validateUninstallSourceRoots(o, preserve, roots); e != nil {
			return e
		}
		if e = writeOwnedJSON(dir, uninstallJournalName, next); e != nil {
			return e
		}
		j = next
	}
	if e = validateUninstallSourceRoots(o, preserve, j.SourceRoots); e != nil {
		return e
	}
	if e = cleanupControl(j.Installation.DataDir); e != nil {
		return e
	}
	if e = checkServiceFile(o, p); e != nil {
		return e
	}
	if e = os.Remove(p.PlistPath); e == nil {
		if e = syncUpgradeDir(filepath.Dir(p.PlistPath)); e != nil {
			return e
		}
	} else if !os.IsNotExist(e) {
		return e
	}
	if e = checkpoint(ctx, o.Root, "uninstall_service"); e != nil {
		return e
	}
	if e = validateUninstallSourceRoots(o, preserve, j.SourceRoots); e != nil {
		return e
	}
	if preserve {
		for _, name := range []string{"current", "recovery"} {
			path := filepath.Join(o.Root, name)
			if _, e = os.Lstat(path); e == nil {
				if e = removeOwnedTree(ctx, path); e != nil {
					return e
				}
			} else if !os.IsNotExist(e) {
				return e
			}
			if e = checkpoint(ctx, o.Root, "uninstall_"+name); e != nil {
				return e
			}
		}
		v := j.Installation
		v.Installed = false
		v.RecoveryLauncher = false
		if e = writeOwnedJSON(o.Root, "installation.json", v); e != nil {
			return e
		}
	} else {
		if _, e = os.Lstat(o.Root); e == nil {
			if e = removeOwnedTree(ctx, o.Root); e != nil {
				return e
			}
		} else if !os.IsNotExist(e) {
			return e
		}
		if e = syncUpgradeDir(filepath.Dir(o.Root)); e != nil {
			return e
		}
	}
	if e = checkpoint(ctx, o.Root, "uninstall_removed"); e != nil {
		return e
	}
	if e = os.Remove(filepath.Join(dir, uninstallJournalName)); e != nil && !os.IsNotExist(e) {
		return e
	}
	return syncUpgradeDir(dir)
}
