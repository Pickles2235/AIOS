package lifecycle

import (
	"archive/zip"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/AdamNi-7080/AIOS/internal/installstate"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"strings"
	"time"

	"github.com/AdamNi-7080/AIOS/internal/observability"
	"github.com/AdamNi-7080/AIOS/internal/store"
)

const upgradeJournalName = "upgrade.json"

var transactionPattern = regexp.MustCompile(`^\.upgrade-[a-f0-9]{24}$`)

type UpgradeInspection struct {
	Manifest
	Downloads        bool `json:"downloads"`
	PreserveIdentity bool `json:"preserve_identity"`
}
type UpgradeResult struct {
	Updated              bool              `json:"updated"`
	Recovered            bool              `json:"recovered"`
	CleanupPending       bool              `json:"cleanup_pending,omitempty"`
	InstanceID           string            `json:"instance_id"`
	ActiveGenerations    map[string]string `json:"active_generations"`
	CanonicalFingerprint string            `json:"canonical_fingerprint"`
	RebuiltProjections   []string          `json:"rebuilt_projections,omitempty"`
	Version              string            `json:"version"`
}
type upgradeJournal = installstate.UpgradeJournal
type candidateValidation struct {
	Protocol     int                `json:"update_protocol"`
	SourceCommit string             `json:"source_commit"`
	InstanceID   string             `json:"instance_id"`
	State        store.UpgradeState `json:"state"`
}
type upgradeOperations struct {
	stop       func(context.Context, Options) error
	start      func(context.Context, Options) (State, error)
	launcher   func(context.Context, Options, string, *upgradeJournal) (string, error)
	validate   func(context.Context, string, string) (candidateValidation, error)
	checkpoint func(context.Context, string, string) error
}

func nativeUpgradeOperations() upgradeOperations {
	return upgradeOperations{stop: stopDaemon, start: startDaemon, launcher: installUpgradeLauncher, validate: validateCandidateProcess, checkpoint: upgradeCheckpoint}
}
func InspectUpgrade(filename string) (UpgradeInspection, error) {
	m, _, e := VerifyArchive(filename)
	if e != nil {
		return UpgradeInspection{}, e
	}
	if m.UpdateProtocol != 1 || m.DiskSchema != 1 || m.KnowledgeIRFormat != "knowledge-ir-v10" || len(m.CompatibleIR) == 0 {
		return UpgradeInspection{}, fmt.Errorf("candidate has no supported staged update protocol")
	}
	return UpgradeInspection{Manifest: m, PreserveIdentity: true}, nil
}
func installedManifest(root string) (Manifest, error) {
	var m Manifest
	e := readUpgradeJSON(filepath.Join(root, "current"), "manifest.json", &m, 65536)
	if e != nil {
		return m, e
	}
	if m.Schema != 1 || m.Version == "" || !commitPattern.MatchString(m.SourceCommit) || m.DiskSchema != 1 {
		return m, fmt.Errorf("unsupported installed manifest")
	}
	return m, nil
}
func manifestBinary(m Manifest) (string, error) {
	var hash string
	for name, h := range m.SHA256 {
		if strings.HasSuffix(name, "/bin/aios") {
			if hash != "" || !shaPattern.MatchString(h) {
				return "", fmt.Errorf("invalid installed executable inventory")
			}
			hash = h
		}
	}
	if hash == "" {
		return "", fmt.Errorf("missing installed executable checksum")
	}
	return hash, nil
}
func lockUpgrade(root string) (*os.File, error) {
	// Inspect without creating a foreign/default installation. Start/stop must
	// acquire authority before mutating, and uninstall can delete the root.
	ancestor := root
	for {
		info, e := os.Lstat(ancestor)
		if e == nil {
			if ancestor == root && (!info.IsDir() || !owned(info) || info.Mode()&os.ModeSymlink != 0) {
				return nil, fmt.Errorf("installation root must be an owned real directory")
			}
			break
		}
		if !os.IsNotExist(e) {
			return nil, e
		}
		parent := filepath.Dir(ancestor)
		if parent == ancestor {
			return nil, e
		}
		ancestor = parent
	}
	if canonical, e := filepath.EvalSymlinks(ancestor); e != nil || canonical != ancestor {
		return nil, fmt.Errorf("installation root must use canonical parents")
	}
	// Deleting an installation must not unlink the inode providing its lease.
	// Keep this small private authority outside the removable owned root.
	// launchd and the invoking shell may have different TMPDIR values.
	dir, e := upgradeAuthorityPath(root)
	if e != nil {
		return nil, e
	}
	if e = PrepareDir(dir); e != nil {
		return nil, e
	}
	return lockOwnedFile(dir, "transaction.lock")
}
func upgradeAuthorityPath(root string) (string, error) {
	base, e := filepath.EvalSymlinks("/tmp")
	if e != nil {
		return "", e
	}
	h := sha256.Sum256([]byte(root))
	return filepath.Join(base, fmt.Sprintf("aios-transaction-%d-%x", os.Getuid(), h[:16])), nil
}
func readUpgradeJournal(root string) (upgradeJournal, error) {
	var j upgradeJournal
	if e := readUpgradeJSON(root, upgradeJournalName, &j, 65536); e != nil {
		return j, e
	}
	if e := installstate.ValidateUpgradeJournal(root, j); e != nil {
		return j, e
	}
	return j, nil
}
func setUpgradePhase(root string, j *upgradeJournal, phase string) error {
	next := *j
	next.Phase = phase
	if e := writeOwnedJSON(root, upgradeJournalName, next); e != nil {
		return e
	}
	*j = next
	return nil
}

func ApplyUpgrade(ctx context.Context, filename string, o Options) (UpgradeResult, error) {
	if e := RequireNative(); e != nil {
		return UpgradeResult{}, e
	}
	o, e := Defaults(o)
	if e != nil {
		return UpgradeResult{}, e
	}
	if _, e = os.Lstat(filepath.Join(o.Root, upgradeJournalName)); e == nil {
		if _, e = RecoverUpgrade(ctx, o); e != nil {
			return UpgradeResult{}, e
		}
	} else if !os.IsNotExist(e) {
		return UpgradeResult{}, e
	}
	return applyUpgrade(ctx, filename, o, nativeUpgradeOperations())
}
func applyUpgrade(ctx context.Context, filename string, o Options, ops upgradeOperations) (result UpgradeResult, err error) {
	reader, e := zip.OpenReader(filename)
	if e != nil {
		return result, e
	}
	defer reader.Close()
	m, prefix, e := verifyReader(reader)
	if e != nil {
		return result, e
	}
	if m.UpdateProtocol != 1 || m.DiskSchema != 1 || m.KnowledgeIRFormat != "knowledge-ir-v10" || m.Platform != runtime.GOOS || m.Architecture != runtime.GOARCH {
		return result, fmt.Errorf("candidate update protocol/platform is incompatible")
	}
	o, e = Defaults(o)
	if e != nil {
		return result, e
	}
	previous, e := readInstallation(o.Root)
	if e != nil {
		return result, e
	}
	if !previous.Installed || previous.ServiceLabel != o.Label || o.DataDir != previous.DataDir {
		return result, fmt.Errorf("update requires the exact owned installation")
	}
	o.Binary = previous.Binary
	prior, e := installedManifest(o.Root)
	if e != nil {
		return result, e
	}
	if prior.UpdateProtocol != 1 || !slices.Contains(m.CompatibleFrom, prior.DiskSchema) || !slices.Contains(m.CompatibleIR, prior.KnowledgeIRFormat) {
		return result, fmt.Errorf("candidate cannot preserve this installed state format")
	}
	expected, e := manifestBinary(prior)
	if e != nil {
		return result, e
	}
	actual, e := ownedFileDigest(previous.Binary)
	if e != nil || actual != expected {
		return result, fmt.Errorf("previous executable checksum does not match installed manifest")
	}
	candidate, e := manifestBinary(m)
	if e != nil {
		return result, e
	}
	// Space and declared compatibility rejection precede service/config changes.
	size, e := upgradeTreeBytes(o.DataDir)
	if e != nil {
		return result, e
	}
	var archiveSize uint64
	for _, z := range reader.File {
		archiveSize += z.UncompressedSize64
	}
	if e = checkUpgradeSpace(o.Root, size+archiveSize+(16<<20)); e != nil {
		return result, e
	}
	lock, e := lockUpgrade(o.Root)
	if e != nil {
		return result, fmt.Errorf("another installation transaction owns the root: %w", e)
	}
	defer lock.Close()
	if e = pendingUninstall(o.Root); e != nil {
		return result, e
	}
	if _, e = os.Lstat(filepath.Join(o.Root, installJournalName)); !os.IsNotExist(e) {
		return result, fmt.Errorf("recover the pending initial installation before updating")
	}
	if _, e = os.Lstat(filepath.Join(o.Root, upgradeJournalName)); !os.IsNotExist(e) {
		return result, fmt.Errorf("recover the pending installation transaction before another update")
	}
	running := false
	if s, e := Control(o, "status"); e == nil {
		running = s.Running
	}
	// Re-read authority under the root lease; an earlier update/uninstall may
	// have completed between inspection and lease acquisition.
	currentInstallation, e := readInstallation(o.Root)
	if e != nil || currentInstallation.Version != previous.Version || currentInstallation.RecoveryLauncher != previous.RecoveryLauncher || currentInstallation.Installed != previous.Installed {
		return result, fmt.Errorf("installation changed before transaction lease; inspect and retry")
	}
	if hash, e := ownedFileDigest(previous.Binary); e != nil || hash != actual {
		return result, fmt.Errorf("installed executable changed before transaction lease")
	}
	instance, e := store.ReadExistingInstance(o.DataDir)
	if e != nil {
		return result, e
	}
	var random [12]byte
	if _, e = rand.Read(random[:]); e != nil {
		return result, e
	}
	txn := ".upgrade-" + hex.EncodeToString(random[:])
	stage := filepath.Join(o.Root, txn)
	j := upgradeJournal{Schema: 1, Transaction: txn, Phase: "stopping", Previous: previous, PreviousBinary: actual, CandidateBinary: candidate, PreviousState: store.UpgradeState{Format: prior.KnowledgeIRFormat}, InstanceID: instance.ID, WasRunning: running, Version: m.Version}
	if previous.RecoveryLauncher {
		j.RecoveryBinary, e = ownedFileDigest(filepath.Join(o.Root, "recovery", "bin", "aios"))
		if e != nil {
			return result, e
		}
		j.RecoveryRequired = true
	}
	if e = writeOwnedJSON(o.Root, upgradeJournalName, j); e != nil {
		return result, e
	}
	upgradeObs, _ := observability.Open(o.Root)
	ctx, upgradeSpan := upgradeObs.Start(ctx, "upgrade", map[string]string{"stage": "transaction"})
	defer func() {
		observability.End(upgradeSpan, err != nil)
		if upgradeObs != nil {
			_ = upgradeObs.Close()
		}
	}()
	var lifetime *os.File
	var lease io.Closer
	var state store.UpgradeState
	var newLease io.Closer
	healthStarted := false
	releaseLeases := func() {
		if newLease != nil {
			newLease.Close()
			newLease = nil
		}
		if lifetime != nil {
			lifetime.Close()
			lifetime = nil
		}
		if lease != nil {
			lease.Close()
			lease = nil
		}
	}
	// Register release before rollback so Go's reverse defer order retains the
	// snapshot leases throughout restoration, including early error returns.
	defer releaseLeases()
	// Every observed precommit failure returns only after restoring the old pair.
	// SIGKILL/power loss bypasses this defer and is handled by the stable launcher.
	defer func() {
		if err == nil {
			return
		}
		// Committed startup permits ordinary writers. Restoring the snapshot
		// after this point would discard knowledge ingested by those writers.
		if j.Phase == "committed" {
			result = UpgradeResult{Updated: true, CleanupPending: true, InstanceID: instance.ID, Version: m.Version, ActiveGenerations: state.ActiveGenerations, CanonicalFingerprint: state.CanonicalFingerprint}
			err = fmt.Errorf("candidate %s is committed and its binary/state retained; run upgrade recover to retry daemon startup and cleanup: %w", m.Version, err)
			return
		}
		rollbackCtx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()
		if healthStarted {
			if e := ops.stop(rollbackCtx, o); e != nil {
				err = fmt.Errorf("update failed; recovery could not stop candidate: %w", e)
				return
			}
		}
		if e := restoreUpgrade(rollbackCtx, o, &j); e != nil {
			err = fmt.Errorf("update failed; matching-state recovery remains pending: %w", e)
			return
		}
		releaseLeases()
		if running {
			if _, e := ops.start(rollbackCtx, o); e != nil {
				err = fmt.Errorf("old binary/state restored; restart still required: %w", e)
			}
		}
	}()
	if e = reserveUpgradeSpace(o.Root); e != nil {
		return result, e
	}
	if e = ops.checkpoint(ctx, o.Root, "before_stop"); e != nil {
		return result, e
	}
	if e = ops.stop(ctx, o); e != nil {
		return result, e
	}
	if e = ops.checkpoint(ctx, o.Root, "after_stop"); e != nil {
		return result, e
	}
	lifetime, e = Lock(o.DataDir)
	if e != nil {
		return result, e
	}
	lease, e = store.AcquireDataLease(o.DataDir)
	if e != nil {
		return result, e
	}
	if e = validateUpgradeSourceRoots(o); e != nil {
		return result, e
	}
	state, e = store.ReadUpgradeState(ctx, o.DataDir)
	if e != nil {
		return result, e
	}
	if state.Format != prior.KnowledgeIRFormat {
		return result, fmt.Errorf("installed manifest and canonical format disagree")
	}
	j.PreviousState = state
	j.SnapshotCaptured = true
	if e = setUpgradePhase(o.Root, &j, "preparing"); e != nil {
		return result, e
	}
	if e = os.Mkdir(stage, 0700); e != nil {
		return result, e
	}
	if e = syncUpgradeDir(o.Root); e != nil {
		return result, e
	}
	if e = extractUpgradeArchive(ctx, reader, m, prefix, filepath.Join(stage, "new-current")); e != nil {
		return result, e
	}
	if e = ops.checkpoint(ctx, o.Root, "stage_binary"); e != nil {
		return result, e
	}
	if e = copyUpgradeTree(ctx, o.DataDir, filepath.Join(stage, "new-data")); e != nil {
		return result, e
	}
	if e = setUpgradePhase(o.Root, &j, "staged"); e != nil {
		return result, e
	}
	if e = ops.checkpoint(ctx, o.Root, "stage_state"); e != nil {
		return result, e
	}
	validated, e := ops.validate(ctx, filepath.Join(stage, "new-current", "bin", "aios"), filepath.Join(stage, "new-data"))
	if e != nil {
		return result, e
	}
	if validated.Protocol != 1 || validated.SourceCommit != m.SourceCommit || validated.InstanceID != instance.ID || validated.State.CanonicalFingerprint != state.CanonicalFingerprint || validated.State.Format != m.KnowledgeIRFormat {
		return result, fmt.Errorf("candidate migration changed identity, IR or source binding")
	}
	if e = setUpgradePhase(o.Root, &j, "migrated"); e != nil {
		return result, e
	}
	if e = ops.checkpoint(ctx, o.Root, "migration"); e != nil {
		return result, e
	}
	// Retain the staged lock inode across publication; locking only after the
	// rename would admit a CLI writer on the candidate during activation.
	newLease, e = store.AcquireDataLease(filepath.Join(stage, "new-data"))
	if e != nil {
		return result, e
	}
	// The recovery executable and login plan must be durable before either old
	// path moves. It survives all binary/data rename gaps and rollback.
	j.RecoveryBinary, e = ops.launcher(ctx, o, filepath.Join(stage, "new-current", "bin", "aios"), &j)
	if e != nil {
		return result, e
	}
	j.RecoveryRequired = true
	if e = setUpgradePhase(o.Root, &j, "activation"); e != nil {
		return result, e
	}
	pairs := [][2]string{{filepath.Join(o.Root, "current"), filepath.Join(stage, "old-current")}, {o.DataDir, filepath.Join(stage, "old-data")}, {filepath.Join(stage, "new-current"), filepath.Join(o.Root, "current")}, {filepath.Join(stage, "new-data"), o.DataDir}}
	for i, pair := range pairs {
		if e = ops.checkpoint(ctx, o.Root, fmt.Sprintf("before_rename_%d", i)); e != nil {
			return result, e
		}
		if e = durableUpgradeRename(pair[0], pair[1]); e != nil {
			return result, e
		}
		if e = ops.checkpoint(ctx, o.Root, fmt.Sprintf("after_rename_%d", i)); e != nil {
			return result, e
		}
	}
	next := previous
	next.Version = m.Version
	next.RecoveryLauncher = true
	if e = writeOwnedJSON(o.Root, "installation.json", next); e != nil {
		return result, e
	}
	if e = setUpgradePhase(o.Root, &j, "health"); e != nil {
		return result, e
	}
	if e = ops.checkpoint(ctx, o.Root, "activation"); e != nil {
		return result, e
	}
	healthStarted = true
	healthy, e := ops.start(ctx, o)
	if e != nil {
		return result, e
	}
	if !healthy.Running || healthy.InstanceID != instance.ID {
		return result, fmt.Errorf("activated candidate health did not retain instance identity")
	}
	if e = ops.checkpoint(ctx, o.Root, "health"); e != nil {
		return result, e
	}
	if e = ops.stop(ctx, o); e != nil {
		return result, e
	}
	healthStarted = false
	if e = ops.checkpoint(ctx, o.Root, "commit"); e != nil {
		return result, e
	}
	if e = setUpgradePhase(o.Root, &j, "committed"); e != nil {
		return result, e
	}
	newLease.Close()
	newLease = nil
	// Release old-path leases before its final cleanup, but retain the old pair
	// until the normal service has also started successfully.
	releaseLeases()
	healthStarted = true
	healthy, e = ops.start(ctx, o)
	if e != nil {
		return result, e
	}
	if !healthy.Running || healthy.InstanceID != instance.ID {
		return result, fmt.Errorf("updated daemon failed normal startup")
	}
	result = UpgradeResult{Updated: true, InstanceID: instance.ID, ActiveGenerations: validated.State.ActiveGenerations, CanonicalFingerprint: validated.State.CanonicalFingerprint, RebuiltProjections: validated.State.RebuiltProjections, Version: m.Version}
	if e = finishCommittedUpgrade(ctx, o, &j); e != nil {
		result.CleanupPending = true
	}
	return result, nil
}

func validateCandidateProcess(ctx context.Context, binary, data string) (candidateValidation, error) {
	var out candidateValidation
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, binary, "upgrade", "validate-state", "--data-dir", data, "--json")
	var buf boundedUpgradeOutput
	cmd.Stdout = &buf
	cmd.Stderr = io.Discard
	if e := cmd.Run(); e != nil {
		return out, fmt.Errorf("staged candidate migration/validation failed: %w", e)
	}
	d := json.NewDecoder(strings.NewReader(buf.String()))
	d.DisallowUnknownFields()
	if e := d.Decode(&out); e != nil {
		return out, e
	}
	if d.Decode(&struct{}{}) != io.EOF {
		return out, fmt.Errorf("invalid candidate validation result")
	}
	return out, nil
}

type boundedUpgradeOutput struct{ strings.Builder }

func (w *boundedUpgradeOutput) Write(p []byte) (int, error) {
	if w.Len()+len(p) > 65536 {
		return 0, fmt.Errorf("candidate validation output exceeds bound")
	}
	return w.Builder.Write(p)
}

func installUpgradeLauncher(ctx context.Context, o Options, candidate string, j *upgradeJournal) (string, error) {
	return installUpgradeLauncherWithCheckpoint(ctx, o, candidate, j, upgradeCheckpoint)
}
func installUpgradeLauncherWithCheckpoint(ctx context.Context, o Options, candidate string, j *upgradeJournal, checkpoint func(context.Context, string, string) error) (string, error) {
	v, e := readInstallation(o.Root)
	if e != nil {
		return "", e
	}
	recovery := filepath.Join(o.Root, "recovery", "bin", "aios")
	if !v.RecoveryLauncher {
		if _, e = os.Lstat(filepath.Join(o.Root, "recovery")); e == nil {
			return "", fmt.Errorf("recovery slot occupied without installation authority")
		} else if !os.IsNotExist(e) {
			return "", e
		}
		stage := filepath.Join(o.Root, j.Transaction, "new-recovery")
		if e = os.MkdirAll(filepath.Join(stage, "bin"), 0700); e != nil {
			return "", e
		}
		if e = copyUpgradeFile(ctx, candidate, filepath.Join(stage, "bin", "aios"), 0700); e != nil {
			return "", e
		}
		if e = syncUpgradeDir(filepath.Join(stage, "bin")); e != nil {
			return "", e
		}
		if e = syncUpgradeDir(stage); e != nil {
			return "", e
		}
		if e = syncUpgradeDir(filepath.Dir(stage)); e != nil {
			return "", e
		}
		j.RecoveryBinary, e = ownedFileDigest(filepath.Join(stage, "bin", "aios"))
		if e != nil {
			return "", e
		}
		// Authority is durable before publication, even if the updater is killed
		// between the recovery rename, metadata write, and launchd plan write.
		if e = writeOwnedJSON(o.Root, upgradeJournalName, j); e != nil {
			return "", e
		}
		if e = checkpoint(ctx, o.Root, "before_recovery_rename"); e != nil {
			return "", e
		}
		if e = durableUpgradeRename(stage, filepath.Join(o.Root, "recovery")); e != nil {
			return "", e
		}
		if e = checkpoint(ctx, o.Root, "after_recovery_rename"); e != nil {
			return "", e
		}
		v.RecoveryLauncher = true
		if e = writeOwnedJSON(o.Root, "installation.json", v); e != nil {
			return "", e
		}
		if e = checkpoint(ctx, o.Root, "after_recovery_metadata"); e != nil {
			return "", e
		}
	} else {
		j.RecoveryBinary, e = ownedFileDigest(recovery)
		if e != nil {
			return "", e
		}
		if e = writeOwnedJSON(o.Root, upgradeJournalName, j); e != nil {
			return "", e
		}
	}
	if e = writeUpgradeServicePlan(o); e != nil {
		return "", e
	}
	if e = checkpoint(ctx, o.Root, "after_recovery_plan"); e != nil {
		return "", e
	}
	return j.RecoveryBinary, nil
}
func writeUpgradeServicePlan(o Options) error {
	p, e := MakePlan(o)
	if e != nil {
		return e
	}
	if e = serviceDir(filepath.Dir(p.PlistPath), true); e != nil {
		return e
	}
	f, e := os.CreateTemp(filepath.Dir(p.PlistPath), ".aios-plist-*")
	if e != nil {
		return e
	}
	defer os.Remove(f.Name())
	if _, e = f.WriteString(p.Plist); e == nil {
		e = f.Sync()
	}
	ce := f.Close()
	if e != nil {
		return e
	}
	if ce != nil {
		return ce
	}
	return durableUpgradeRename(f.Name(), p.PlistPath)
}

func matchingPreviousState(ctx context.Context, o Options, j *upgradeJournal) error {
	hash, e := ownedFileDigest(j.Previous.Binary)
	if e != nil || hash != j.PreviousBinary {
		return fmt.Errorf("recovery refuses mismatched previous executable")
	}
	state, e := store.ReadUpgradeState(ctx, o.DataDir)
	if e != nil || (j.SnapshotCaptured && state.CanonicalFingerprint != j.PreviousState.CanonicalFingerprint) || state.Format != j.PreviousState.Format {
		return fmt.Errorf("recovery refuses mismatched previous state")
	}
	instance, e := store.ReadExistingInstance(o.DataDir)
	if e != nil || instance.ID != j.InstanceID {
		return fmt.Errorf("recovery refuses changed instance identity")
	}
	return nil
}
func restoreUpgrade(ctx context.Context, o Options, j *upgradeJournal) error {
	if e := releaseUpgradeReserve(o.Root); e != nil {
		return e
	}
	if e := setUpgradePhase(o.Root, j, "rollback"); e != nil {
		return e
	}
	stage := filepath.Join(o.Root, j.Transaction)
	for _, name := range []string{"current", "data"} {
		old := filepath.Join(stage, "old-"+name)
		if _, e := os.Lstat(old); e == nil {
			if e = PrepareDir(old); e != nil {
				return e
			}
			dest := filepath.Join(o.Root, name)
			if _, e = os.Lstat(dest); e == nil {
				failed := filepath.Join(stage, "failed-"+name)
				if _, e = os.Lstat(failed); e == nil {
					return fmt.Errorf("occupied rollback discard slot")
				} else if !os.IsNotExist(e) {
					return e
				}
				if e = durableUpgradeRename(dest, failed); e != nil {
					return e
				}
			} else if !os.IsNotExist(e) {
				return e
			}
			if e = durableUpgradeRename(old, dest); e != nil {
				return e
			}
		} else if !os.IsNotExist(e) {
			return e
		}
	}
	if e := matchingPreviousState(ctx, o, j); e != nil {
		return e
	}
	v := j.Previous
	if j.RecoveryBinary != "" {
		digest, e := ownedFileDigest(filepath.Join(o.Root, "recovery", "bin", "aios"))
		if os.IsNotExist(e) {
			if j.RecoveryRequired || j.Previous.RecoveryLauncher {
				return fmt.Errorf("missing durable recovery interpreter after activation")
			}
			staged, stageErr := ownedFileDigest(filepath.Join(stage, "new-recovery", "bin", "aios"))
			if stageErr != nil || staged != j.RecoveryBinary {
				return fmt.Errorf("missing published or staged recovery interpreter")
			}
		} else if e != nil || digest != j.RecoveryBinary {
			return fmt.Errorf("recovery interpreter checksum changed")
		} else {
			v.RecoveryLauncher = true
		}
	}
	if e := writeOwnedJSON(o.Root, "installation.json", v); e != nil {
		return e
	}
	if e := writeUpgradeServicePlan(o); e != nil {
		return e
	}
	if _, e := os.Lstat(stage); e == nil {
		if e = removeOwnedTree(ctx, stage); e != nil {
			return e
		}
	} else if !os.IsNotExist(e) {
		return e
	}
	if e := os.Remove(filepath.Join(o.Root, upgradeJournalName)); e != nil && !os.IsNotExist(e) {
		return e
	}
	return syncUpgradeDir(o.Root)
}
func finishCommittedUpgrade(ctx context.Context, o Options, j *upgradeJournal) error {
	if e := releaseUpgradeReserve(o.Root); e != nil {
		return e
	}
	hash, e := ownedFileDigest(filepath.Join(o.Root, "current", "bin", "aios"))
	if e != nil || hash != j.CandidateBinary {
		return fmt.Errorf("committed candidate checksum changed")
	}
	state, e := store.ReadUpgradeState(ctx, o.DataDir)
	if e != nil || state.Format != "knowledge-ir-v10" {
		return fmt.Errorf("committed state has an invalid format or integrity")
	}
	instance, e := store.ReadExistingInstance(o.DataDir)
	if e != nil || instance.ID != j.InstanceID {
		return fmt.Errorf("committed instance identity changed")
	}
	stage := filepath.Join(o.Root, j.Transaction)
	if _, e = os.Lstat(stage); e == nil {
		if e = removeOwnedTree(ctx, stage); e != nil {
			return e
		}
	} else if !os.IsNotExist(e) {
		return e
	}
	if e = os.Remove(filepath.Join(o.Root, upgradeJournalName)); e != nil && !os.IsNotExist(e) {
		return e
	}
	return syncUpgradeDir(o.Root)
}
func RecoverUpgrade(ctx context.Context, o Options) (UpgradeResult, error) {
	if e := RequireNative(); e != nil {
		return UpgradeResult{}, e
	}
	o, e := Defaults(o)
	if e != nil {
		return UpgradeResult{}, e
	}
	if e = pendingUninstall(o.Root); e != nil {
		return UpgradeResult{}, e
	}
	if handled, result, e := recoverPendingInstall(ctx, o); handled {
		return result, e
	}
	if _, e = os.Lstat(filepath.Join(o.Root, upgradeJournalName)); os.IsNotExist(e) {
		return UpgradeResult{}, nil
	} else if e != nil {
		return UpgradeResult{}, e
	}
	lock, e := lockUpgrade(o.Root)
	if e != nil {
		return UpgradeResult{}, e
	}
	defer lock.Close()
	j, e := readUpgradeJournal(o.Root)
	if e != nil {
		return UpgradeResult{}, e
	}
	if o.Label != j.Previous.ServiceLabel {
		return UpgradeResult{}, fmt.Errorf("recovery service label differs from owned transaction")
	}
	if e = stopDaemon(ctx, o); e != nil {
		return UpgradeResult{}, e
	}
	if e = recoverUpgradeLocked(ctx, o, &j); e != nil {
		return UpgradeResult{}, e
	}
	if j.WasRunning {
		if _, e = startDaemon(ctx, o); e != nil {
			return UpgradeResult{}, e
		}
	}
	state, e := store.ReadUpgradeState(ctx, o.DataDir)
	if e != nil {
		return UpgradeResult{}, e
	}
	version := j.Previous.Version
	if j.Phase == "committed" {
		version = j.Version
	}
	return UpgradeResult{Recovered: true, Updated: j.Phase == "committed", InstanceID: j.InstanceID, ActiveGenerations: state.ActiveGenerations, CanonicalFingerprint: state.CanonicalFingerprint, Version: version}, nil
}
func recoverUpgradeLocked(ctx context.Context, o Options, j *upgradeJournal) (retErr error) {
	recoveryObs, _ := observability.Open(o.Root)
	ctx, recoverySpan := recoveryObs.Start(ctx, "upgrade_recovery", map[string]string{"stage": "recovery"})
	defer func() {
		observability.End(recoverySpan, retErr != nil)
		if recoveryObs != nil {
			_ = recoveryObs.Close()
		}
	}()
	// A surviving foreground daemon/writer is never compatible with restoration.
	var leases []io.Closer
	defer func() {
		for i := len(leases) - 1; i >= 0; i-- {
			leases[i].Close()
		}
	}()
	for _, data := range []string{o.DataDir, filepath.Join(o.Root, j.Transaction, "old-data")} {
		if _, e := os.Lstat(data); e == nil {
			lock, e := Lock(data)
			if e != nil {
				return e
			}
			leases = append(leases, lock)
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
	if j.Phase == "committed" {
		return finishCommittedUpgrade(ctx, o, j)
	}
	return restoreUpgrade(ctx, o, j)
}

// PrepareUpgradeStartup runs in the stable launchd interpreter BEFORE it execs
// any replaceable executable or takes a data lifetime lock. Health is permitted
// only while a real updater holds the root lease at the fully activated phase.
func PrepareUpgradeStartup(ctx context.Context, o Options) (bool, error) {
	o, e := Defaults(o)
	if e != nil {
		return false, e
	}
	if e = pendingUninstall(o.Root); e != nil {
		return false, e
	}
	if handled, result, e := recoverPendingInstall(ctx, o); handled {
		if e != nil {
			return false, e
		}
		if !result.Updated {
			return false, fmt.Errorf("interrupted installation rolled back; rerun install before starting")
		}
	}
	if _, e = os.Lstat(filepath.Join(o.Root, upgradeJournalName)); os.IsNotExist(e) {
		return false, nil
	} else if e != nil {
		return false, e
	}
	lock, e := lockUpgrade(o.Root)
	if errors.Is(e, errDaemonLocked) {
		j, e := readUpgradeJournal(o.Root)
		if e != nil {
			return false, e
		}
		if j.Previous.ServiceLabel != o.Label || j.Previous.DataDir != o.DataDir {
			return false, fmt.Errorf("upgrade startup installation mismatch")
		}
		if j.Phase == "committed" {
			return false, nil
		}
		if j.Phase != "health" {
			return false, fmt.Errorf("update activation is incomplete; retry startup after recovery")
		}
		hash, e := ownedFileDigest(filepath.Join(o.Root, "current", "bin", "aios"))
		if e != nil || hash != j.CandidateBinary {
			return false, fmt.Errorf("upgrade health executable does not match transaction")
		}
		state, e := store.ReadUpgradeState(ctx, o.DataDir)
		if e != nil || state.CanonicalFingerprint != j.PreviousState.CanonicalFingerprint {
			return false, fmt.Errorf("upgrade health state does not match transaction")
		}
		return true, nil
	}
	if e != nil {
		return false, e
	}
	defer lock.Close()
	j, e := readUpgradeJournal(o.Root)
	if e != nil {
		return false, e
	}
	if j.Previous.ServiceLabel != o.Label {
		return false, fmt.Errorf("upgrade startup service mismatch")
	}
	return false, recoverUpgradeLocked(ctx, o, &j)
}
func UpgradeHealthLeaseHeld(root string) bool {
	lock, e := lockUpgrade(root)
	if e == nil {
		lock.Close()
		return false
	}
	return errors.Is(e, errDaemonLocked)
}
