package lifecycle

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/AdamNi-7080/AIOS/internal/store"
)

func uninstallFixture(t *testing.T) (Options, store.UpgradeState, string) {
	t.Helper()
	o, _, state, id := upgradeFixture(t)
	p, e := MakePlan(o)
	if e != nil {
		t.Fatal(e)
	}
	if e = os.MkdirAll(filepath.Dir(p.PlistPath), 0700); e != nil {
		t.Fatal(e)
	}
	if e = os.WriteFile(p.PlistPath, []byte(p.Plist), 0600); e != nil {
		t.Fatal(e)
	}
	if e = os.WriteFile(filepath.Join(o.home, "source-sentinel"), []byte("source must survive"), 0600); e != nil {
		t.Fatal(e)
	}
	return o, state, id
}

func assertUninstalled(t *testing.T, o Options, preserve bool, state store.UpgradeState, id string) {
	t.Helper()
	if _, e := readUninstallJournal(o.Root); !os.IsNotExist(e) {
		t.Fatal("uninstall intent remains", e)
	}
	if _, e := os.Lstat(filepath.Join(o.Root, "current")); !os.IsNotExist(e) {
		t.Fatal("binary remains", e)
	}
	if preserve {
		s, e := store.ReadUpgradeState(context.Background(), o.DataDir)
		if e != nil || s.CanonicalFingerprint != state.CanonicalFingerprint || s.Format != state.Format {
			t.Fatal("preserved canonical knowledge changed", e)
		}
		i, e := store.ReadExistingInstance(o.DataDir)
		if e != nil || i.ID != id {
			t.Fatal("preserved identity changed", e)
		}
		v, e := readInstallation(o.Root)
		if e != nil || v.Installed || v.RecoveryLauncher {
			t.Fatal("preserve metadata not retired", e)
		}
	} else {
		if _, e := os.Lstat(o.Root); !os.IsNotExist(e) {
			t.Fatal("owned root remains", e)
		}
	}
	if b, e := os.ReadFile(filepath.Join(o.home, "source-sentinel")); e != nil || string(b) != "source must survive" {
		t.Fatal("source sibling changed", e)
	}
	if _, e := os.Lstat(filepath.Join(o.home, "Library", "LaunchAgents", o.Label+".plist")); !os.IsNotExist(e) {
		t.Fatal("login service remains", e)
	}
}

func uninstallBoundaries(preserve bool) []string {
	out := []string{"uninstall_intent", "uninstall_stopped", "uninstall_service", "uninstall_removed"}
	if preserve {
		out = append(out, "uninstall_current", "uninstall_recovery")
	}
	return out
}

func TestUninstallFaultsKeepDurableChoiceAndRetryOwnedCleanup(t *testing.T) {
	for _, preserve := range []bool{false, true} {
		for _, boundary := range uninstallBoundaries(preserve) {
			t.Run(boundary+map[bool]string{true: "_preserve", false: "_delete"}[preserve], func(t *testing.T) {
				o, state, id := uninstallFixture(t)
				stops := 0
				stop := func(context.Context, Options) error { stops++; return nil }
				checkpoint := func(_ context.Context, _ string, at string) error {
					if at == "uninstall_service" || at == "uninstall_removed" {
						if db, e := store.OpenWriter(o.DataDir); e != store.ErrWriterLocked {
							if db != nil {
								db.Close()
							}
							t.Fatal("uninstall admitted writer", e)
						}
					}
					if at == boundary {
						return syscall.ENOSPC
					}
					return nil
				}
				if e := uninstallWithCheckpoint(context.Background(), o, preserve, stop, checkpoint); e == nil {
					t.Fatal("uninstall fault accepted")
				}
				if e := uninstallCandidate(context.Background(), o, !preserve, stop); e == nil {
					t.Fatal("durable uninstall choice changed")
				}
				if _, e := PrepareUpgradeStartup(context.Background(), o); e == nil {
					t.Fatal("startup admitted during uninstall")
				}
				if e := uninstallCandidate(context.Background(), o, preserve, stop); e != nil {
					t.Fatal("uninstall retry failed", e)
				}
				if stops != 1 {
					t.Fatal("owned service stop did not occur exactly once", stops)
				}
				assertUninstalled(t, o, preserve, state, id)
			})
		}
	}
}

func TestUninstallKilledProcessRetainsOwnedCleanupAuthority(t *testing.T) {
	if os.Getenv("AIOS_UNINSTALL_TEST_CHILD") == "1" {
		var o Options
		if e := json.Unmarshal([]byte(os.Getenv("AIOS_UPGRADE_TEST_OPTIONS")), &o); e != nil {
			t.Fatal(e)
		}
		o.home = os.Getenv("AIOS_UPGRADE_TEST_HOME")
		preserve := os.Getenv("AIOS_UNINSTALL_PRESERVE") == "1"
		checkpoint := func(_ context.Context, _ string, at string) error {
			if at == os.Getenv("AIOS_UPGRADE_TEST_BOUNDARY") {
				if e := os.WriteFile(filepath.Join(o.home, "paused"), []byte(at), 0600); e != nil {
					return e
				}
				select {}
			}
			return nil
		}
		e := uninstallWithCheckpoint(context.Background(), o, preserve, func(context.Context, Options) error { return nil }, checkpoint)
		t.Fatalf("child escaped pause: %v", e)
	}
	for _, preserve := range []bool{false, true} {
		for _, boundary := range uninstallBoundaries(preserve) {
			t.Run(boundary+map[bool]string{true: "_preserve", false: "_delete"}[preserve], func(t *testing.T) {
				o, state, id := uninstallFixture(t)
				b, _ := json.Marshal(o)
				cmd := exec.Command(os.Args[0], "-test.run=^TestUninstallKilledProcessRetainsOwnedCleanupAuthority$")
				pv := "0"
				if preserve {
					pv = "1"
				}
				cmd.Env = append(os.Environ(), "AIOS_UNINSTALL_TEST_CHILD=1", "AIOS_UNINSTALL_PRESERVE="+pv, "AIOS_UPGRADE_TEST_OPTIONS="+string(b), "AIOS_UPGRADE_TEST_HOME="+o.home, "AIOS_UPGRADE_TEST_BOUNDARY="+boundary)
				logPath := filepath.Join(o.home, "uninstall-child.log")
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
					if _, e = os.Stat(filepath.Join(o.home, "paused")); e == nil {
						break
					}
					if time.Now().After(deadline) {
						body, _ := os.ReadFile(logPath)
						t.Fatalf("child failed at %s: %s", boundary, body)
					}
					time.Sleep(10 * time.Millisecond)
				}
				if e = cmd.Process.Kill(); e != nil {
					t.Fatal(e)
				}
				cmd.Wait()
				if e = uninstallCandidate(context.Background(), o, preserve, func(context.Context, Options) error { return nil }); e != nil {
					t.Fatal("killed uninstall did not recover", e)
				}
				assertUninstalled(t, o, preserve, state, id)
			})
		}
	}
}

func TestUninstallCompletesPartialOwnedRootDeletion(t *testing.T) {
	o, state, id := uninstallFixture(t)
	e := uninstallWithCheckpoint(context.Background(), o, false, func(context.Context, Options) error { return nil }, func(_ context.Context, _ string, at string) error {
		if at == "uninstall_service" {
			return syscall.ENOSPC
		}
		return nil
	})
	if e == nil {
		t.Fatal("fault accepted")
	}
	// An actual recursive unlink may have removed metadata and executable
	// before process termination; its durable authority is outside that tree.
	if e = os.Remove(filepath.Join(o.Root, "installation.json")); e != nil {
		t.Fatal(e)
	}
	if e = os.Remove(o.Binary); e != nil {
		t.Fatal(e)
	}
	transient, e := upgradeAuthorityPath(o.Root)
	if e != nil {
		t.Fatal(e)
	}
	if e = os.RemoveAll(transient); e != nil {
		t.Fatal(e)
	}
	if _, e = readUninstallJournal(o.Root); e != nil {
		t.Fatal("temporary-directory cleanup lost durable intent", e)
	}
	if e = uninstallCandidate(context.Background(), o, false, func(context.Context, Options) error {
		t.Fatal("stopped service reinterpreted after metadata deletion")
		return nil
	}); e != nil {
		t.Fatal("partial-delete recovery failed", e)
	}
	assertUninstalled(t, o, false, state, id)
}
