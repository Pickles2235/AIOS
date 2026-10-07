package lifecycle

import (
	"archive/zip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/AdamNi-7080/AIOS/internal/model"
	"github.com/AdamNi-7080/AIOS/internal/observability"
	"github.com/AdamNi-7080/AIOS/internal/store"
)

// These tests exercise the real portable filesystem/state transaction. Service
// callbacks stand in for launchd; native installed-package proof is separate.
func upgradeFixture(t *testing.T) (Options, string, store.UpgradeState, string) {
	t.Helper()
	home := canonicalTemp(t)
	o, e := Defaults(Options{home: home, Root: filepath.Join(home, "owned"), Label: "dev.aios.completion.upgrade"})
	if e != nil {
		t.Fatal(e)
	}
	if e = PrepareDir(o.Root); e != nil {
		t.Fatal(e)
	}
	if e = os.MkdirAll(filepath.Join(o.Root, "current", "bin"), 0700); e != nil {
		t.Fatal(e)
	}
	o.Binary = filepath.Join(o.Root, "current", "bin", "aios")
	if e = os.WriteFile(o.Binary, []byte("prior protocol1 portable fixture"), 0700); e != nil {
		t.Fatal(e)
	}
	hash, e := ownedFileDigest(o.Binary)
	if e != nil {
		t.Fatal(e)
	}
	prior := Manifest{Schema: 1, Version: "A", SourceCommit: strings.Repeat("a", 40), Platform: runtime.GOOS, Architecture: runtime.GOARCH, DiskSchema: 1, KnowledgeIRFormat: "knowledge-ir-v10", UpdateProtocol: 1, SHA256: map[string]string{"candidate/bin/aios": hash}}
	if e = writeOwnedJSON(filepath.Join(o.Root, "current"), "manifest.json", prior); e != nil {
		t.Fatal(e)
	}
	v := Installation{Scope: "user", Binary: o.Binary, DataDir: o.DataDir, ServiceLabel: o.Label, Version: "A", Installed: true}
	if e = writeOwnedJSON(o.Root, "installation.json", v); e != nil {
		t.Fatal(e)
	}
	db, e := store.OpenWriter(o.DataDir)
	if e != nil {
		t.Fatal(e)
	}
	file := model.File{RepoID: "repo", Path: "service.go", SHA256: "fixture", Content: "func Publish() {}", Size: 17, Language: "go", Classification: "source"}
	g, e := db.StageGeneration(context.Background(), model.Snapshot{RepoID: "repo", Root: "/approved/source", ContentHash: "fixture", FileCount: 1, TotalBytes: 17, IndexedAt: time.Unix(1, 0), ExtractorVersions: model.ExtractorVersion}, []model.File{file}, []model.Symbol{{RepoID: "repo", Path: file.Path, Name: "Publish", Kind: "function", Span: model.Span{StartLine: 1, EndLine: 1, StartColumn: 1, EndColumn: 18, EndByte: 17}, Extractor: "fixture", Confidence: 1}}, nil)
	if e == nil {
		e = db.ActivateGeneration(context.Background(), g.ID)
	}
	db.Close()
	if e != nil {
		t.Fatal(e)
	}
	instance, e := store.LoadInstance(o.DataDir)
	if e != nil {
		t.Fatal(e)
	}
	if e = os.WriteFile(filepath.Join(o.DataDir, "approved-config.json"), []byte(`{"source":"preserved","namespace":"fixture"}`), 0600); e != nil {
		t.Fatal(e)
	}
	state, e := store.ReadUpgradeState(context.Background(), o.DataDir)
	if e != nil {
		t.Fatal(e)
	}
	path := filepath.Join(home, "candidate.zip")
	f, e := os.Create(path)
	if e != nil {
		t.Fatal(e)
	}
	z := zip.NewWriter(f)
	m := prior
	m.Version = "B"
	m.SourceCommit = strings.Repeat("b", 40)
	m.CompatibleFrom = []int{1}
	m.CompatibleIR = []string{"knowledge-ir-v10"}
	m.SHA256 = map[string]string{}
	for _, n := range []string{"bin/aios", "install.sh", "uninstall.sh", "update.sh", "LICENSES.txt", "USER-GUIDE.md", "RELEASE-NOTES.md"} {
		body := []byte("candidate portable fixture " + n)
		h := sha256.Sum256(body)
		m.SHA256["candidate/"+n] = hex.EncodeToString(h[:])
		w, e := z.Create("candidate/" + n)
		if e != nil {
			t.Fatal(e)
		}
		if _, e = w.Write(body); e != nil {
			t.Fatal(e)
		}
	}
	b, _ := json.Marshal(m)
	w, e := z.Create("candidate/manifest.json")
	if e != nil {
		t.Fatal(e)
	}
	w.Write(b)
	if e = z.Close(); e != nil {
		t.Fatal(e)
	}
	if e = f.Close(); e != nil {
		t.Fatal(e)
	}
	return o, path, state, instance.ID
}

func portableUpgradeOps(id string, checkpoint func(context.Context, string, string) error) upgradeOperations {
	ops := upgradeOperations{stop: func(context.Context, Options) error { return nil }, start: func(context.Context, Options) (State, error) { return State{Running: true, InstanceID: id}, nil }, checkpoint: checkpoint}
	ops.validate = func(ctx context.Context, _ string, data string) (candidateValidation, error) {
		s, e := store.MigrateUpgradeState(ctx, data)
		if e != nil {
			return candidateValidation{}, e
		}
		instance, e := store.ReadExistingInstance(data)
		return candidateValidation{Protocol: 1, SourceCommit: strings.Repeat("b", 40), InstanceID: instance.ID, State: s}, e
	}
	ops.launcher = func(ctx context.Context, o Options, b string, j *upgradeJournal) (string, error) {
		return installUpgradeLauncherWithCheckpoint(ctx, o, b, j, checkpoint)
	}
	return ops
}

func assertPriorUpgrade(t *testing.T, o Options, before store.UpgradeState, id string) {
	t.Helper()
	b, e := os.ReadFile(o.Binary)
	if e != nil || string(b) != "prior protocol1 portable fixture" {
		t.Fatalf("old executable not restored: %v", e)
	}
	s, e := store.ReadUpgradeState(context.Background(), o.DataDir)
	if e != nil || s.CanonicalFingerprint != before.CanonicalFingerprint || !reflect.DeepEqual(s.ActiveGenerations, before.ActiveGenerations) {
		t.Fatalf("old canonical state not restored: %+v %v", s, e)
	}
	i, e := store.ReadExistingInstance(o.DataDir)
	if e != nil || i.ID != id {
		t.Fatal("identity changed", e)
	}
	b, e = os.ReadFile(filepath.Join(o.DataDir, "approved-config.json"))
	if e != nil || string(b) != `{"source":"preserved","namespace":"fixture"}` {
		t.Fatal("configuration changed", e)
	}
	if _, e = os.Lstat(filepath.Join(o.Root, upgradeJournalName)); !os.IsNotExist(e) {
		t.Fatal("recovery journal remains", e)
	}
	entries, e := os.ReadDir(o.Root)
	if e != nil {
		t.Fatal(e)
	}
	for _, entry := range entries {
		if transactionPattern.MatchString(entry.Name()) || entry.Name() == upgradeReserveName {
			t.Fatal("transaction garbage remains", entry.Name())
		}
	}
}

var upgradeFaultBoundaries = []string{"before_stop", "after_stop", "stage_binary", "stage_state", "migration", "before_recovery_rename", "after_recovery_rename", "after_recovery_metadata", "after_recovery_plan", "before_rename_0", "after_rename_0", "before_rename_1", "after_rename_1", "before_rename_2", "after_rename_2", "before_rename_3", "after_rename_3", "activation", "health", "commit"}

func TestUpgradeRootTelemetryRecordsOutcomeWithoutTransactionContent(t *testing.T) {
	o, path, _, id := upgradeFixture(t)
	ops := portableUpgradeOps(id, func(context.Context, string, string) error { return nil })
	result, err := applyUpgrade(context.Background(), path, o, ops)
	if err != nil || !result.Updated {
		t.Fatalf("upgrade result=%+v err=%v", result, err)
	}
	status, records, err := observability.ReadExisting(o.Root)
	if err != nil || !status.Available {
		t.Fatalf("root diagnostics status=%+v err=%v", status, err)
	}
	found := false
	for _, row := range records {
		if row.Operation == "upgrade" && row.Outcome == "ok" {
			found = true
		}
		if strings.Contains(fmt.Sprint(row), path) || strings.Contains(fmt.Sprint(row), id) {
			t.Fatal("root telemetry exposed transaction input")
		}
	}
	if !found {
		t.Fatalf("missing root upgrade span: %+v", records)
	}
}

func TestUpgradeEveryPrecommitBoundaryRestoresMatchingPair(t *testing.T) {
	for _, boundary := range upgradeFaultBoundaries {
		t.Run(boundary, func(t *testing.T) {
			o, path, before, id := upgradeFixture(t)
			ops := portableUpgradeOps(id, func(_ context.Context, _ string, at string) error {
				if at == boundary {
					return syscall.ENOSPC
				}
				return nil
			})
			if _, e := applyUpgrade(context.Background(), path, o, ops); e == nil {
				t.Fatal("fault was accepted")
			}
			assertPriorUpgrade(t, o, before, id)
		})
	}
}

func TestUpgradePostcommitStartFailureRetainsCandidate(t *testing.T) {
	o, path, before, id := upgradeFixture(t)
	ops := portableUpgradeOps(id, func(context.Context, string, string) error { return nil })
	starts := 0
	ops.start = func(context.Context, Options) (State, error) {
		starts++
		if starts == 2 {
			return State{}, fmt.Errorf("normal startup failed")
		}
		return State{Running: true, InstanceID: id}, nil
	}
	result, e := applyUpgrade(context.Background(), path, o, ops)
	if e == nil || !result.Updated || !result.CleanupPending || !strings.Contains(e.Error(), "committed") {
		t.Fatalf("postcommit result %+v %v", result, e)
	}
	j, e := readUpgradeJournal(o.Root)
	if e != nil || j.Phase != "committed" {
		t.Fatal("commit lost", e)
	}
	if b, e := os.ReadFile(o.Binary); e != nil || !strings.HasPrefix(string(b), "candidate") {
		t.Fatal("committed executable rolled back", e)
	}
	db, e := store.OpenWriter(o.DataDir)
	if e != nil {
		t.Fatal("committed transaction rejected writer", e)
	}
	file := model.File{RepoID: "repo", Path: "later.go", SHA256: "later", Content: "package later", Size: 13, Language: "go", Classification: "source"}
	generation, e := db.StageGeneration(context.Background(), model.Snapshot{RepoID: "repo", Root: "/approved/source", ContentHash: "later", FileCount: 1, TotalBytes: 13, IndexedAt: time.Unix(2, 0), ExtractorVersions: model.ExtractorVersion}, []model.File{file}, nil, nil)
	if e == nil {
		e = db.ActivateGeneration(context.Background(), generation.ID)
	}
	db.Close()
	if e != nil {
		t.Fatal("postcommit ingestion", e)
	}
	after, e := store.ReadUpgradeState(context.Background(), o.DataDir)
	if e != nil || after.CanonicalFingerprint == before.CanonicalFingerprint {
		t.Fatal("fixture must change canonical facts", e)
	}
	if health, e := PrepareUpgradeStartup(context.Background(), o); e != nil || health {
		t.Fatal("committed cleanup failed", e)
	}
	s, e := store.ReadUpgradeState(context.Background(), o.DataDir)
	if e != nil || s.CanonicalFingerprint != after.CanonicalFingerprint {
		t.Fatal("postcommit canonical state lost", e)
	}
}

func TestUpgradeKilledProcessRecoversBeforeNextStartup(t *testing.T) {
	if os.Getenv("AIOS_UPGRADE_TEST_CHILD") == "1" {
		var o Options
		if e := json.Unmarshal([]byte(os.Getenv("AIOS_UPGRADE_TEST_OPTIONS")), &o); e != nil {
			t.Fatal(e)
		}
		o.home = os.Getenv("AIOS_UPGRADE_TEST_HOME")
		ops := portableUpgradeOps(os.Getenv("AIOS_UPGRADE_TEST_ID"), func(_ context.Context, root, at string) error {
			if at == os.Getenv("AIOS_UPGRADE_TEST_BOUNDARY") {
				if e := os.WriteFile(filepath.Join(root, "paused"), []byte(at), 0600); e != nil {
					return e
				}
				select {}
			}
			return nil
		})
		_, e := applyUpgrade(context.Background(), os.Getenv("AIOS_UPGRADE_TEST_ARCHIVE"), o, ops)
		t.Fatalf("child escaped pause: %v", e)
	}
	for _, boundary := range upgradeFaultBoundaries {
		t.Run(boundary, func(t *testing.T) {
			o, path, before, id := upgradeFixture(t)
			b, _ := json.Marshal(o)
			cmd := exec.Command(os.Args[0], "-test.run=^TestUpgradeKilledProcessRecoversBeforeNextStartup$")
			cmd.Env = append(os.Environ(), "AIOS_UPGRADE_TEST_CHILD=1", "AIOS_UPGRADE_TEST_OPTIONS="+string(b), "AIOS_UPGRADE_TEST_HOME="+o.home, "AIOS_UPGRADE_TEST_ID="+id, "AIOS_UPGRADE_TEST_ARCHIVE="+path, "AIOS_UPGRADE_TEST_BOUNDARY="+boundary)
			logPath := filepath.Join(o.home, "child.log")
			log, e := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY, 0600)
			if e != nil {
				t.Fatal(e)
			}
			defer log.Close()
			cmd.Stdout = log
			cmd.Stderr = log
			if e = cmd.Start(); e != nil {
				t.Fatal(e)
			}
			defer func() { cmd.Process.Kill(); cmd.Wait() }()
			deadline := time.Now().Add(15 * time.Second)
			for {
				if _, e = os.Stat(filepath.Join(o.Root, "paused")); e == nil {
					break
				}
				if time.Now().After(deadline) {
					body, _ := os.ReadFile(logPath)
					t.Fatalf("child failed to reach %s: %s", boundary, body)
				}
				time.Sleep(10 * time.Millisecond)
			}
			if e = cmd.Process.Kill(); e != nil {
				t.Fatal(e)
			}
			if e = cmd.Wait(); e == nil {
				t.Fatal("process was not killed")
			}
			_, beforeStat := os.Lstat(o.DataDir)
			if db, e := store.OpenWriter(o.DataDir); e != store.ErrTransactionPending {
				if db != nil {
					db.Close()
				}
				t.Fatal("killed precommit owner admitted writer", e)
			}
			if e := store.ResetDerivedData(o.DataDir); e != store.ErrTransactionPending {
				t.Fatal("killed owner admitted reset", e)
			}
			if _, e := os.Lstat(o.DataDir); os.IsNotExist(beforeStat) && !os.IsNotExist(e) {
				t.Fatal("writer recreated missing activation data")
			}
			if health, e := PrepareUpgradeStartup(context.Background(), o); e != nil || health {
				t.Fatalf("startup recovery: %v", e)
			}
			assertPriorUpgrade(t, o, before, id)
			if db, e := store.OpenWriter(o.DataDir); e != nil {
				t.Fatal("recovered state rejects writer", e)
			} else {
				db.Close()
			}
		})
	}
}

func TestUpgradeLeaseSurvivesRootDeletion(t *testing.T) {
	o, _, _, _ := upgradeFixture(t)
	lease, e := lockUpgrade(o.Root)
	if e != nil {
		t.Fatal(e)
	}
	defer lease.Close()
	if e = removeOwnedTree(context.Background(), o.Root); e != nil {
		t.Fatal(e)
	}
	if other, e := lockUpgrade(o.Root); e == nil {
		other.Close()
		t.Fatal("delete/reinstall admitted a second transaction")
	}
}

func TestUpgradePublicationRetainsWriterLeaseAndBindsHealthLabel(t *testing.T) {
	o, path, _, id := upgradeFixture(t)
	ops := portableUpgradeOps(id, func(ctx context.Context, _ string, at string) error {
		if at == "after_rename_1" || at == "after_rename_2" || at == "after_rename_3" || at == "activation" {
			if db, e := store.OpenWriter(o.DataDir); e != store.ErrWriterLocked {
				if db != nil {
					db.Close()
				}
				t.Fatalf("publication admitted writer: %v", e)
			}
			if at == "after_rename_1" || at == "after_rename_2" {
				if _, e := os.Lstat(o.DataDir); !os.IsNotExist(e) {
					t.Fatal("rejected writer recreated absent data", e)
				}
			}
		}
		if at == "activation" {
			wrong := o
			wrong.Label = "dev.aios.completion.wrong"
			if _, e := PrepareUpgradeStartup(ctx, wrong); e == nil {
				t.Fatal("foreign-label health admitted")
			}
			health, e := PrepareUpgradeStartup(ctx, o)
			if e != nil || !health {
				t.Fatal("owned health rejected", e)
			}
		}
		return nil
	})
	starts := 0
	ops.start = func(context.Context, Options) (State, error) {
		starts++
		db, e := store.OpenWriter(o.DataDir)
		if starts == 1 {
			if e != store.ErrWriterLocked {
				if db != nil {
					db.Close()
				}
				t.Fatal("health writer admitted", e)
			}
		} else {
			if e != nil {
				t.Fatal("committed writer rejected", e)
			}
			db.Close()
		}
		return State{Running: true, InstanceID: id}, nil
	}
	if _, e := applyUpgrade(context.Background(), path, o, ops); e != nil {
		t.Fatal(e)
	}
}

func TestUpgradeTransactionLeaseIgnoresTMPDIR(t *testing.T) {
	o, _, _, _ := upgradeFixture(t)
	t.Setenv("TMPDIR", canonicalTemp(t))
	first, e := lockUpgrade(o.Root)
	if e != nil {
		t.Fatal(e)
	}
	defer first.Close()
	t.Setenv("TMPDIR", canonicalTemp(t))
	if second, e := lockUpgrade(o.Root); e == nil {
		second.Close()
		t.Fatal("different process environment admitted second transaction")
	}
}

func TestUpgradeSubsequentFailureRetainsExistingRecoveryInterpreter(t *testing.T) {
	o, path, before, id := upgradeFixture(t)
	if _, e := applyUpgrade(context.Background(), path, o, portableUpgradeOps(id, func(context.Context, string, string) error { return nil })); e != nil {
		t.Fatal(e)
	}
	prior, e := ownedFileDigest(o.Binary)
	if e != nil {
		t.Fatal(e)
	}
	recovery, e := ownedFileDigest(filepath.Join(o.Root, "recovery", "bin", "aios"))
	if e != nil {
		t.Fatal(e)
	}
	for _, boundary := range []string{"before_stop", "after_recovery_plan", "after_rename_3", "health"} {
		ops := portableUpgradeOps(id, func(_ context.Context, _ string, at string) error {
			if at == boundary {
				return syscall.ENOSPC
			}
			return nil
		})
		if _, e := applyUpgrade(context.Background(), path, o, ops); e == nil {
			t.Fatal("subsequent fault accepted")
		}
		if h, e := ownedFileDigest(o.Binary); e != nil || h != prior {
			t.Fatal("previous committed executable lost", e)
		}
		if h, e := ownedFileDigest(filepath.Join(o.Root, "recovery", "bin", "aios")); e != nil || h != recovery {
			t.Fatal("stable recovery interpreter replaced", e)
		}
		if s, e := store.ReadUpgradeState(context.Background(), o.DataDir); e != nil || s.CanonicalFingerprint != before.CanonicalFingerprint {
			t.Fatal("canonical rows lost", e)
		}
	}
}

func TestUpgradeRejectsProtocolZeroBeforeStoppingOrStateMutation(t *testing.T) {
	o, path, _, id := upgradeFixture(t)
	m, e := installedManifest(o.Root)
	if e != nil {
		t.Fatal(e)
	}
	m.UpdateProtocol = 0
	if e = writeOwnedJSON(filepath.Join(o.Root, "current"), "manifest.json", m); e != nil {
		t.Fatal(e)
	}
	before, e := os.ReadFile(filepath.Join(o.DataDir, store.DatabaseName))
	if e != nil {
		t.Fatal(e)
	}
	ops := portableUpgradeOps(id, func(context.Context, string, string) error {
		t.Fatal("incompatible update reached mutation checkpoint")
		return nil
	})
	ops.stop = func(context.Context, Options) error { t.Fatal("incompatible update stopped service"); return nil }
	if _, e = applyUpgrade(context.Background(), path, o, ops); e == nil {
		t.Fatal("legacy protocol accepted")
	}
	after, e := os.ReadFile(filepath.Join(o.DataDir, store.DatabaseName))
	if e != nil || sha256.Sum256(before) != sha256.Sum256(after) {
		t.Fatal("incompatible update mutated state", e)
	}
	if _, e = os.Lstat(filepath.Join(o.Root, upgradeJournalName)); !os.IsNotExist(e) {
		t.Fatal("incompatible update created journal", e)
	}
}

func TestUpgradeCorruptCommittedIntentRejectsEveryWriter(t *testing.T) {
	for _, name := range []string{upgradeJournalName, installJournalName} {
		t.Run(name, func(t *testing.T) {
			o, _, _, id := upgradeFixture(t)
			digest, e := ownedFileDigest(o.Binary)
			if e != nil {
				t.Fatal(e)
			}
			incomplete := map[string]any{"schema_version": 1, "transaction": ".upgrade-012345678901234567890123", "phase": "committed", "instance_id": id}
			if name == upgradeJournalName {
				incomplete["candidate_binary_sha256"] = digest
				incomplete["previous"] = map[string]any{"binary": o.Binary, "data_dir": o.DataDir}
			} else {
				incomplete["binary_sha256"] = digest
				incomplete["installation"] = map[string]any{"binary": o.Binary, "data_dir": o.DataDir, "installed": true}
			}
			if e = writeOwnedJSON(o.Root, name, incomplete); e != nil {
				t.Fatal(e)
			}
			if db, e := store.OpenWriter(o.DataDir); e != store.ErrTransactionPending {
				if db != nil {
					db.Close()
				}
				t.Fatal("incomplete committed intent admitted writer", e)
			}
			if e = store.ResetDerivedData(o.DataDir); e != store.ErrTransactionPending {
				t.Fatal("incomplete committed intent admitted reset", e)
			}
		})
	}
}
