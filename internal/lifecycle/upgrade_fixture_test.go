//go:build completionfixture

package lifecycle

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"syscall"
	"testing"

	"github.com/AdamNi-7080/AIOS/internal/store"
)

// This suite requires retained actual committed A/B archives. It executes A's
// installed CLI and both real candidate validators against nonempty knowledge.
// Service callbacks prove portable package/state mechanics, never launchd.
func TestUpgradeActualProtocol1PackagesPreserveNonemptyKnowledge(t *testing.T) {
	prior, candidate := os.Getenv("AIOS_COMPLETION_PRIOR_PACKAGE"), os.Getenv("AIOS_COMPLETION_PACKAGE")
	if prior == "" || candidate == "" {
		t.Fatal("distinct retained actual A/B packages required")
	}
	a, _, e := VerifyArchive(prior)
	if e != nil {
		t.Fatal(e)
	}
	b, _, e := VerifyArchive(candidate)
	if e != nil {
		t.Fatal(e)
	}
	if a.SourceCommit == b.SourceCommit || a.Version == b.Version {
		t.Fatal("same candidate cannot prove upgrade")
	}
	for _, failure := range []bool{true, false} {
		t.Run(map[bool]string{true: "failed", false: "committed"}[failure], func(t *testing.T) {
			ctx := context.Background()
			home := canonicalTemp(t)
			// Real immutable A snapshots are read-only. Restore directory owner
			// write access only after all assertions so TempDir can remove them.
			t.Cleanup(func() {
				_ = filepath.WalkDir(home, func(path string, entry os.DirEntry, e error) error {
					if e == nil && entry.IsDir() {
						return os.Chmod(path, 0700)
					}
					return e
				})
			})
			o, e := Defaults(Options{Root: filepath.Join(home, "install"), Label: "dev.aios.fixture.actual"})
			if e != nil {
				t.Fatal(e)
			}
			o.home = home
			ops := nativeUpgradeOperations()
			ops.stop = func(context.Context, Options) error { return nil }
			ops.checkpoint = func(context.Context, string, string) error { return nil }
			ops.start = func(context.Context, Options) (State, error) {
				instance, e := store.ReadExistingInstance(o.DataDir)
				return State{Running: true, InstanceID: instance.ID}, e
			}
			v, e := installCandidate(ctx, prior, o, ops)
			if e != nil {
				t.Fatal("actual A installation", e)
			}
			o.Binary = v.Binary
			original, e := ownedFileDigest(o.Binary)
			if e != nil {
				t.Fatal(e)
			}
			source := filepath.Join(home, "source")
			if e = os.Mkdir(source, 0700); e != nil {
				t.Fatal(e)
			}
			if e = os.WriteFile(filepath.Join(source, "source.go"), []byte("package source\nfunc Publish() {}\n"), 0600); e != nil {
				t.Fatal(e)
			}
			for _, args := range [][]string{{"init", "-q", source}, {"-C", source, "add", "."}, {"-C", source, "-c", "user.name=Fixture", "-c", "user.email=fixture@example.invalid", "commit", "-qm", "fixture"}} {
				if out, e := exec.Command("git", args...).CombinedOutput(); e != nil {
					t.Fatal("Git fixture", e, string(out))
				}
			}
			config := map[string]any{"version": 1, "sources": []map[string]any{{"kind": "repository", "id": "fixture"}}}
			registry := map[string]any{"version": 1, "repositories": []map[string]any{{"id": "fixture", "path": source}}}
			for name, value := range map[string]any{"catalog.json": config, "registry.json": registry} {
				bytes, _ := json.Marshal(value)
				if e = os.WriteFile(filepath.Join(home, name), bytes, 0600); e != nil {
					t.Fatal(e)
				}
			}
			command := exec.Command(o.Binary, "local", "ingest", "--all", "--config", filepath.Join(home, "catalog.json"), "--registry", filepath.Join(home, "registry.json"), "--data-dir", o.DataDir)
			if out, e := command.CombinedOutput(); e != nil {
				t.Fatal("actual installed A ingestion", e, string(out))
			}
			// Persist real approved startup controls that B's validator must accept.
			setup := map[string]any{"mode": "local", "state": "ready", "local_repositories": registry["repositories"], "active_catalog": config, "active_mode": "local"}
			if e = writeOwnedJSON(o.DataDir, "setup.json", setup); e != nil {
				t.Fatal(e)
			}
			before, e := store.ReadUpgradeState(ctx, o.DataDir)
			if e != nil || len(before.ActiveGenerations) != 1 {
				t.Fatal("A must populate real nonempty IR", e)
			}
			id, e := store.ReadExistingInstance(o.DataDir)
			if e != nil {
				t.Fatal(e)
			}
			controls, e := os.ReadFile(filepath.Join(o.DataDir, "setup.json"))
			if e != nil {
				t.Fatal(e)
			}
			db, e := store.OpenReadOnly(o.DataDir)
			if e != nil {
				t.Fatal(e)
			}
			ids := map[string]string{}
			rows, e := db.DB().Query(`SELECT projection_kind,projection_build_id FROM active_projection_builds`)
			if e != nil {
				t.Fatal(e)
			}
			for rows.Next() {
				var kind, build string
				if e = rows.Scan(&kind, &build); e != nil {
					t.Fatal(e)
				}
				ids[kind] = build
			}
			rows.Close()
			db.Close()
			ops.launcher = func(ctx context.Context, o Options, bin string, j *upgradeJournal) (string, error) {
				return installUpgradeLauncherWithCheckpoint(ctx, o, bin, j, ops.checkpoint)
			}
			if failure {
				ops.checkpoint = func(_ context.Context, _ string, at string) error {
					if at == "health" {
						return syscall.ENOSPC
					}
					return nil
				}
			}
			result, e := applyUpgrade(ctx, candidate, o, ops)
			if failure {
				if e == nil {
					t.Fatal("actual package fault accepted")
				}
			} else if e != nil || !result.Updated || !reflect.DeepEqual(result.RebuiltProjections, []string{"lookup"}) {
				t.Fatal("actual B must rebuild only changed lookup", result, e)
			}
			after, e := store.ReadUpgradeState(ctx, o.DataDir)
			if e != nil || after.CanonicalFingerprint != before.CanonicalFingerprint || !reflect.DeepEqual(after.ActiveGenerations, before.ActiveGenerations) {
				t.Fatal("actual package canonical state changed", e)
			}
			next, e := store.ReadExistingInstance(o.DataDir)
			if e != nil || next.ID != id.ID {
				t.Fatal("actual package identity changed", e)
			}
			nextControls, e := os.ReadFile(filepath.Join(o.DataDir, "setup.json"))
			if e != nil || sha256.Sum256(nextControls) != sha256.Sum256(controls) {
				t.Fatal("actual package approvals changed", e)
			}
			installed, e := ownedFileDigest(o.Binary)
			if e != nil || (installed == original) != failure {
				t.Fatal("actual binary/state parity", e)
			}
			db, e = store.OpenReadOnly(o.DataDir)
			if e != nil {
				t.Fatal(e)
			}
			defer db.Close()
			for kind, old := range ids {
				var next string
				if e = db.DB().QueryRow(`SELECT projection_build_id FROM active_projection_builds WHERE projection_kind=?`, kind).Scan(&next); e != nil {
					t.Fatal(e)
				}
				if (next != old) != (kind == "lookup" && !failure) {
					t.Fatal("unchanged actual projection replaced", kind)
				}
			}
			if out, e := exec.Command(o.Binary, "status", "--data-dir", o.DataDir).CombinedOutput(); e != nil || !strings.Contains(string(out), "fixture") {
				t.Fatal("matching actual installed CLI cannot read restored/updated knowledge", e)
			}
		})
	}
}
