package lifecycle

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/AdamNi-7080/AIOS/internal/store"
)

func initialInstallFixture(t *testing.T, preserved bool) (Options, string, store.UpgradeState, string) {
	t.Helper()
	o, path, state, id := upgradeFixture(t)
	if preserved {
		v, e := readInstallation(o.Root)
		if e != nil {
			t.Fatal(e)
		}
		v.Installed = false
		if e = removeOwnedTree(context.Background(), filepath.Join(o.Root, "current")); e != nil {
			t.Fatal(e)
		}
		if e = writeOwnedJSON(o.Root, "installation.json", v); e != nil {
			t.Fatal(e)
		}
	} else {
		if e := removeOwnedTree(context.Background(), o.Root); e != nil {
			t.Fatal(e)
		}
	}
	return o, path, state, id
}

func assertInitialRollback(t *testing.T, o Options, preserved bool, state store.UpgradeState, id string) {
	t.Helper()
	if _, e := os.Lstat(filepath.Join(o.Root, "current")); !os.IsNotExist(e) {
		t.Fatal("partial executable retained", e)
	}
	if _, e := os.Lstat(filepath.Join(o.Root, installJournalName)); !os.IsNotExist(e) {
		t.Fatal("install intent retained", e)
	}
	if preserved {
		s, e := store.ReadUpgradeState(context.Background(), o.DataDir)
		if e != nil || s.CanonicalFingerprint != state.CanonicalFingerprint || !reflect.DeepEqual(s.ActiveGenerations, state.ActiveGenerations) {
			t.Fatal("preserved data changed", e)
		}
		i, e := store.ReadExistingInstance(o.DataDir)
		if e != nil || i.ID != id {
			t.Fatal("identity changed", e)
		}
		v, e := readInstallation(o.Root)
		if e != nil || v.Installed {
			t.Fatal("preserved metadata changed", e)
		}
	} else {
		entries, e := os.ReadDir(o.Root)
		if e != nil || len(entries) != 0 {
			t.Fatal("fresh partial install remains", entries, e)
		}
	}
}

func initialInstallBoundaries(preserved bool) []string {
	boundaries := []string{"install_binary", "install_state", "install_migration", "install_metadata"}
	for i := 0; i < 2; i++ {
		boundaries = append(boundaries, "install_before_rename_"+string(rune('0'+i)), "install_after_rename_"+string(rune('0'+i)))
	}
	if preserved {
		boundaries = append(boundaries, "install_before_rename_2", "install_after_rename_2")
	}
	return boundaries
}

func TestInitialInstallFaultsRestoreFreshOrPreservedState(t *testing.T) {
	for _, preserved := range []bool{false, true} {
		for _, boundary := range initialInstallBoundaries(preserved) {
			t.Run(boundary+map[bool]string{true: "_preserved", false: "_fresh"}[preserved], func(t *testing.T) {
				o, path, state, id := initialInstallFixture(t, preserved)
				ops := portableUpgradeOps(id, func(_ context.Context, _ string, at string) error {
					if at == boundary {
						return syscall.ENOSPC
					}
					return nil
				})
				if _, e := installCandidate(context.Background(), path, o, ops); e == nil {
					t.Fatal("fault accepted")
				}
				assertInitialRollback(t, o, preserved, state, id)
				v, e := installCandidate(context.Background(), path, o, portableUpgradeOps(id, func(context.Context, string, string) error { return nil }))
				if e != nil || !v.Installed {
					t.Fatal("retry failed", e)
				}
				if preserved {
					s, e := store.ReadUpgradeState(context.Background(), o.DataDir)
					if e != nil || s.CanonicalFingerprint != state.CanonicalFingerprint {
						t.Fatal("retry lost canonical data", e)
					}
				}
			})
		}
	}
}

func TestInitialInstallKilledProcessRecoversBeforeStartup(t *testing.T) {
	if os.Getenv("AIOS_INSTALL_TEST_CHILD") == "1" {
		var o Options
		if e := json.Unmarshal([]byte(os.Getenv("AIOS_UPGRADE_TEST_OPTIONS")), &o); e != nil {
			t.Fatal(e)
		}
		o.home = os.Getenv("AIOS_UPGRADE_TEST_HOME")
		ops := portableUpgradeOps("", func(_ context.Context, root, at string) error {
			if at == os.Getenv("AIOS_UPGRADE_TEST_BOUNDARY") {
				if e := os.WriteFile(filepath.Join(root, "paused"), []byte(at), 0600); e != nil {
					return e
				}
				select {}
			}
			return nil
		})
		_, e := installCandidate(context.Background(), os.Getenv("AIOS_UPGRADE_TEST_ARCHIVE"), o, ops)
		t.Fatalf("child escaped pause: %v", e)
	}
	for _, preserved := range []bool{false, true} {
		for _, boundary := range initialInstallBoundaries(preserved) {
			t.Run(boundary+map[bool]string{true: "_preserved", false: "_fresh"}[preserved], func(t *testing.T) {
				o, path, state, id := initialInstallFixture(t, preserved)
				b, _ := json.Marshal(o)
				cmd := exec.Command(os.Args[0], "-test.run=^TestInitialInstallKilledProcessRecoversBeforeStartup$")
				cmd.Env = append(os.Environ(), "AIOS_INSTALL_TEST_CHILD=1", "AIOS_UPGRADE_TEST_OPTIONS="+string(b), "AIOS_UPGRADE_TEST_HOME="+o.home, "AIOS_UPGRADE_TEST_ARCHIVE="+path, "AIOS_UPGRADE_TEST_BOUNDARY="+boundary)
				logPath := filepath.Join(o.home, "install-child.log")
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
				cmd.Wait()
				if db, e := store.OpenWriter(o.DataDir); e != store.ErrTransactionPending {
					if db != nil {
						db.Close()
					}
					t.Fatal("interrupted install admitted writer", e)
				}
				if e = store.ResetDerivedData(o.DataDir); e != store.ErrTransactionPending {
					t.Fatal("interrupted install admitted reset", e)
				}
				if _, e = PrepareUpgradeStartup(context.Background(), o); e == nil {
					t.Fatal("startup allowed partial installation")
				}
				os.Remove(filepath.Join(o.Root, "paused"))
				assertInitialRollback(t, o, preserved, state, id)
			})
		}
	}
}

func TestInitialInstallPublicationRetainsWriterLease(t *testing.T) {
	for _, preserved := range []bool{false, true} {
		t.Run(map[bool]string{true: "preserved", false: "fresh"}[preserved], func(t *testing.T) {
			o, path, _, id := initialInstallFixture(t, preserved)
			boundary := "install_after_rename_1"
			if preserved {
				boundary = "install_after_rename_2"
			}
			ops := portableUpgradeOps(id, func(_ context.Context, _ string, at string) error {
				if at == boundary || (preserved && at == "install_after_rename_0") || (!preserved && at == "install_before_rename_1") {
					db, e := store.OpenWriter(o.DataDir)
					if e != store.ErrWriterLocked {
						if db != nil {
							db.Close()
						}
						t.Fatal("initial publication admitted writer", e)
					}
				}
				return nil
			})
			if _, e := installCandidate(context.Background(), path, o, ops); e != nil {
				t.Fatal(e)
			}
		})
	}
}

func TestInitialInstallRecoveryCleansPartiallyDeletedDiscard(t *testing.T) {
	o, _, before, id := initialInstallFixture(t, true)
	// Reproduce the durable post-rename recovery state with a discard whose
	// executable was already deleted before an interruption in tree cleanup.
	stage := filepath.Join(o.Root, ".upgrade-012345678901234567890123")
	if e := os.MkdirAll(filepath.Join(stage, "failed-current", "bin"), 0700); e != nil {
		t.Fatal(e)
	}
	v, e := readInstallation(o.Root)
	if e != nil {
		t.Fatal(e)
	}
	next := v
	next.Installed = true
	next.Version = "B"
	j := installJournal{Schema: 1, Transaction: filepath.Base(stage), Phase: "rollback", Installation: next, Previous: &v, HadData: true, Binary: strings.Repeat("a", 64), OriginalState: before, OriginalInstanceID: id}
	if e = writeOwnedJSON(o.Root, installJournalName, j); e != nil {
		t.Fatal(e)
	}
	if _, _, e = recoverPendingInstall(context.Background(), o); e != nil {
		t.Fatal("partial-discard recovery failed", e)
	}
	assertInitialRollback(t, o, true, before, id)
}
