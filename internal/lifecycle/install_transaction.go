package lifecycle

import (
	"archive/zip"
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"github.com/AdamNi-7080/AIOS/internal/installstate"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"github.com/AdamNi-7080/AIOS/internal/store"
)

const installJournalName = "install.json"

type installJournal = installstate.InstallJournal

func readInstallJournal(root string) (installJournal, error) {
	var j installJournal
	if e := readUpgradeJSON(root, installJournalName, &j, 65536); e != nil {
		return j, e
	}
	if e := installstate.ValidateInstallJournal(root, j); e != nil {
		return j, e
	}
	return j, nil
}

func setInstallPhase(root string, j *installJournal, phase string) error {
	next := *j
	next.Phase = phase
	if e := writeOwnedJSON(root, installJournalName, next); e != nil {
		return e
	}
	*j = next
	return nil
}

func installCandidate(ctx context.Context, filename string, o Options, ops upgradeOperations) (result Installation, err error) {
	reader, e := zip.OpenReader(filename)
	if e != nil {
		return result, e
	}
	defer reader.Close()
	m, prefix, e := verifyReader(reader)
	if e != nil {
		return result, e
	}
	if m.Platform != runtime.GOOS || m.Architecture != runtime.GOARCH || m.UpdateProtocol != 1 || m.DiskSchema != 1 || m.KnowledgeIRFormat != "knowledge-ir-v10" {
		return result, fmt.Errorf("candidate install protocol/platform is incompatible")
	}
	o, e = Defaults(o)
	if e != nil {
		return result, e
	}
	if o.DataDir != filepath.Join(o.Root, "data") {
		return result, fmt.Errorf("installation requires its fixed owned data path")
	}
	// An empty root, a preserve-data uninstall, or our durable install journal
	// provide authority. An arbitrary occupied directory does not.
	if entries, e := os.ReadDir(o.Root); e == nil && len(entries) > 0 {
		if _, je := readInstallJournal(o.Root); je != nil {
			v, ve := readInstallation(o.Root)
			if ve != nil || v.Installed {
				return result, fmt.Errorf("installation root occupied; use explicit upgrade")
			}
		}
	} else if e != nil && !os.IsNotExist(e) {
		return result, e
	}
	lease, e := lockUpgrade(o.Root)
	if e != nil {
		return result, e
	}
	defer lease.Close()
	if e = pendingUninstall(o.Root); e != nil {
		return result, e
	}
	if e = PrepareDir(o.Root); e != nil {
		return result, e
	}
	if _, e = os.Lstat(filepath.Join(o.Root, upgradeJournalName)); !os.IsNotExist(e) {
		return result, fmt.Errorf("recover the pending update before reinstalling")
	}
	if _, e = os.Lstat(filepath.Join(o.Root, installJournalName)); e == nil {
		j, e := readInstallJournal(o.Root)
		if e != nil {
			return result, e
		}
		if j.Installation.ServiceLabel != o.Label {
			return result, fmt.Errorf("pending installation label differs")
		}
		if e = recoverInstallLocked(ctx, o, &j); e != nil {
			return result, e
		}
		if j.Phase == "committed" {
			return j.Installation, nil
		}
	} else if !os.IsNotExist(e) {
		return result, e
	}
	var previous *Installation
	if v, e := readInstallation(o.Root); e == nil {
		if v.Installed || v.ServiceLabel != o.Label || v.RecoveryLauncher {
			return result, fmt.Errorf("installation became occupied")
		}
		previous = &v
	} else if !os.IsNotExist(e) {
		return result, e
	}
	if _, e = os.Lstat(filepath.Join(o.Root, "current")); !os.IsNotExist(e) {
		return result, fmt.Errorf("binary slot occupied without pending installation")
	}
	if _, e = os.Lstat(filepath.Join(o.Root, "recovery")); !os.IsNotExist(e) {
		return result, fmt.Errorf("recovery slot occupied without installation")
	}
	v := Installation{Scope: "user", Binary: filepath.Join(o.Root, "current", "bin", "aios"), DataDir: o.DataDir, ServiceLabel: o.Label, Version: m.Version, Installed: true}
	v.GitEnvironment, e = machineGitEnvironment()
	if e != nil {
		return result, e
	}
	var locks []io.Closer
	closeLocks := func() {
		for i := len(locks) - 1; i >= 0; i-- {
			locks[i].Close()
		}
		locks = nil
	}
	defer closeLocks()
	var original store.UpgradeState
	var originalInstanceID string
	hadData := false
	if _, e = os.Lstat(o.DataDir); e == nil {
		hadData = true
		if previous == nil {
			return result, fmt.Errorf("preserved data requires previous uninstall metadata")
		}
		l, e := Lock(o.DataDir)
		if e != nil {
			return result, e
		}
		locks = append(locks, l)
		l2, e := store.AcquireDataLease(o.DataDir)
		if e != nil {
			return result, e
		}
		locks = append(locks, l2)
		original, e = store.ReadUpgradeState(ctx, o.DataDir)
		if e != nil {
			return result, e
		}
		instance, e := store.ReadExistingInstance(o.DataDir)
		if e != nil {
			return result, e
		}
		originalInstanceID = instance.ID
	} else if !os.IsNotExist(e) {
		return result, e
	} else {
		authority, e := store.AcquireDataAuthority(o.DataDir)
		if e != nil {
			return result, e
		}
		locks = append(locks, authority)
	}
	var random [12]byte
	if _, e = rand.Read(random[:]); e != nil {
		return result, e
	}
	hash, e := manifestBinary(m)
	if e != nil {
		return result, e
	}
	j := installJournal{Schema: 1, Transaction: ".upgrade-" + hex.EncodeToString(random[:]), Phase: "preparing", Installation: v, Previous: previous, HadData: hadData, Binary: hash, OriginalState: original, OriginalInstanceID: originalInstanceID}
	if e = writeOwnedJSON(o.Root, installJournalName, j); e != nil {
		return result, e
	}
	defer func() {
		if err != nil && j.Phase != "committed" {
			rollbackCtx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
			defer cancel()
			if e := recoverInstallWithLeases(rollbackCtx, o, &j, false); e != nil {
				err = fmt.Errorf("installation failed; owned recovery remains pending: %w", e)
			}
		}
	}()
	if e = reserveUpgradeSpace(o.Root); e != nil {
		return result, e
	}
	stage := filepath.Join(o.Root, j.Transaction)
	if e = os.Mkdir(stage, 0700); e != nil {
		return result, e
	}
	if e = syncUpgradeDir(o.Root); e != nil {
		return result, e
	}
	if e = extractUpgradeArchive(ctx, reader, m, prefix, filepath.Join(stage, "new-current")); e != nil {
		return result, e
	}
	if e = ops.checkpoint(ctx, o.Root, "install_binary"); e != nil {
		return result, e
	}
	stagedData := filepath.Join(stage, "new-data")
	if hadData {
		e = copyUpgradeTree(ctx, o.DataDir, stagedData)
	} else {
		e = PrepareDir(stagedData)
		if e == nil {
			db, openErr := store.OpenWriter(stagedData)
			e = openErr
			if e == nil {
				e = db.Close()
			}
		}
		if e == nil {
			_, e = store.LoadInstance(stagedData)
		}
	}
	if e != nil {
		return result, e
	}
	if e = ops.checkpoint(ctx, o.Root, "install_state"); e != nil {
		return result, e
	}
	validated, e := ops.validate(ctx, filepath.Join(stage, "new-current", "bin", "aios"), stagedData)
	if e != nil {
		return result, e
	}
	instance, e := store.ReadExistingInstance(stagedData)
	if e != nil {
		return result, e
	}
	if validated.Protocol != 1 || validated.SourceCommit != m.SourceCommit || validated.InstanceID != instance.ID || validated.State.Format != m.KnowledgeIRFormat || (hadData && validated.State.CanonicalFingerprint != original.CanonicalFingerprint) {
		return result, fmt.Errorf("staged installation failed identity/canonical validation")
	}
	j.InstanceID = instance.ID
	j.State = validated.State
	stagedLease, e := store.AcquireDataLease(stagedData)
	if e != nil {
		return result, e
	}
	locks = append(locks, stagedLease)
	if e = ops.checkpoint(ctx, o.Root, "install_migration"); e != nil {
		return result, e
	}
	if e = setInstallPhase(o.Root, &j, "activation"); e != nil {
		return result, e
	}
	pairs := [][2]string{}
	if hadData {
		pairs = append(pairs, [2]string{o.DataDir, filepath.Join(stage, "old-data")})
	}
	pairs = append(pairs, [2]string{filepath.Join(stage, "new-current"), filepath.Join(o.Root, "current")}, [2]string{stagedData, o.DataDir})
	for i, pair := range pairs {
		if e = ops.checkpoint(ctx, o.Root, fmt.Sprintf("install_before_rename_%d", i)); e != nil {
			return result, e
		}
		if e = durableUpgradeRename(pair[0], pair[1]); e != nil {
			return result, e
		}
		if e = ops.checkpoint(ctx, o.Root, fmt.Sprintf("install_after_rename_%d", i)); e != nil {
			return result, e
		}
	}
	if e = writeOwnedJSON(o.Root, "installation.json", v); e != nil {
		return result, e
	}
	if e = ops.checkpoint(ctx, o.Root, "install_metadata"); e != nil {
		return result, e
	}
	if e = setInstallPhase(o.Root, &j, "committed"); e != nil {
		return result, e
	}
	closeLocks()
	if e = recoverInstallLocked(ctx, o, &j); e != nil {
		return v, fmt.Errorf("installation committed; rerun install for pending cleanup: %w", e)
	}
	return v, nil
}

// No service is started until Install returns. This durable initial transaction
// can therefore restore preserved data, or remove its own partial fresh install,
// without stopping a daemon that could have accepted user mutations.
func recoverInstallLocked(ctx context.Context, o Options, j *installJournal) error {
	return recoverInstallWithLeases(ctx, o, j, true)
}
func recoverInstallWithLeases(ctx context.Context, o Options, j *installJournal, acquire bool) error {
	var leases []io.Closer
	defer func() {
		for i := len(leases) - 1; i >= 0; i-- {
			leases[i].Close()
		}
	}()
	if acquire {
		for _, data := range []string{o.DataDir, filepath.Join(o.Root, j.Transaction, "old-data")} {
			if _, e := os.Lstat(data); e == nil {
				l, e := Lock(data)
				if e != nil {
					return e
				}
				leases = append(leases, l)
				writer, e := store.AcquireDataLease(data)
				if e != nil {
					return e
				}
				leases = append(leases, writer)
			} else if !os.IsNotExist(e) {
				return e
			} else if data == o.DataDir {
				authority, e := store.AcquireDataAuthority(data)
				if e != nil {
					return e
				}
				leases = append(leases, authority)
			}
		}
	}
	if e := releaseUpgradeReserve(o.Root); e != nil {
		return e
	}
	stage := filepath.Join(o.Root, j.Transaction)
	if j.Phase == "committed" {
		h, e := ownedFileDigest(j.Installation.Binary)
		if e != nil || h != j.Binary {
			return fmt.Errorf("installed candidate checksum changed")
		}
		i, e := store.ReadExistingInstance(o.DataDir)
		if e != nil || i.ID != j.InstanceID {
			return fmt.Errorf("installed candidate identity changed")
		}
		s, e := store.ReadUpgradeState(ctx, o.DataDir)
		if e != nil || s.Format != j.State.Format {
			return fmt.Errorf("installed state integrity changed")
		}
	} else {
		if e := setInstallPhase(o.Root, j, "rollback"); e != nil {
			return e
		}
		current := filepath.Join(o.Root, "current")
		if _, e := os.Lstat(current); e == nil {
			h, e := ownedFileDigest(filepath.Join(current, "bin", "aios"))
			if e != nil || h != j.Binary {
				return fmt.Errorf("partial install executable differs from journal")
			}
			if e = durableUpgradeRename(current, filepath.Join(stage, "failed-current")); e != nil {
				return e
			}
		} else if !os.IsNotExist(e) {
			return e
		}
		old := filepath.Join(stage, "old-data")
		if _, e := os.Lstat(old); e == nil {
			if _, e = os.Lstat(o.DataDir); e == nil {
				if e = durableUpgradeRename(o.DataDir, filepath.Join(stage, "failed-data")); e != nil {
					return e
				}
			} else if !os.IsNotExist(e) {
				return e
			}
			if e = durableUpgradeRename(old, o.DataDir); e != nil {
				return e
			}
		} else if !os.IsNotExist(e) {
			return e
		} else if !j.HadData {
			if _, e = os.Lstat(o.DataDir); e == nil {
				if e = durableUpgradeRename(o.DataDir, filepath.Join(stage, "failed-data")); e != nil {
					return e
				}
			} else if !os.IsNotExist(e) {
				return e
			}
		}
		if j.HadData {
			s, e := store.ReadUpgradeState(ctx, o.DataDir)
			if e != nil || s.Format != j.OriginalState.Format || s.CanonicalFingerprint != j.OriginalState.CanonicalFingerprint {
				return fmt.Errorf("preserved original state does not match installation journal")
			}
			i, e := store.ReadExistingInstance(o.DataDir)
			if e != nil || i.ID != j.OriginalInstanceID {
				return fmt.Errorf("preserved original identity changed")
			}
		}
		if j.Previous != nil {
			if e := writeOwnedJSON(o.Root, "installation.json", j.Previous); e != nil {
				return e
			}
		} else if e := os.Remove(filepath.Join(o.Root, "installation.json")); e != nil && !os.IsNotExist(e) {
			return e
		}
	}
	if _, e := os.Lstat(stage); e == nil {
		if e = removeOwnedTree(ctx, stage); e != nil {
			return e
		}
	} else if !os.IsNotExist(e) {
		return e
	}
	if e := os.Remove(filepath.Join(o.Root, installJournalName)); e != nil && !os.IsNotExist(e) {
		return e
	}
	return syncUpgradeDir(o.Root)
}

func recoverPendingInstall(ctx context.Context, o Options) (bool, UpgradeResult, error) {
	if _, e := os.Lstat(filepath.Join(o.Root, installJournalName)); os.IsNotExist(e) {
		return false, UpgradeResult{}, nil
	} else if e != nil {
		return true, UpgradeResult{}, e
	}
	lock, e := lockUpgrade(o.Root)
	if e != nil {
		return true, UpgradeResult{}, e
	}
	defer lock.Close()
	j, e := readInstallJournal(o.Root)
	if e != nil {
		return true, UpgradeResult{}, e
	}
	if j.Installation.ServiceLabel != o.Label || j.Installation.DataDir != o.DataDir {
		return true, UpgradeResult{}, fmt.Errorf("pending installation authority mismatch")
	}
	committed := j.Phase == "committed"
	if e = recoverInstallLocked(ctx, o, &j); e != nil {
		return true, UpgradeResult{}, e
	}
	result := UpgradeResult{Recovered: true, Updated: committed}
	if committed {
		result.Version = j.Installation.Version
		result.InstanceID = j.InstanceID
		result.ActiveGenerations = j.State.ActiveGenerations
		result.CanonicalFingerprint = j.State.CanonicalFingerprint
	}
	return true, result, nil
}
