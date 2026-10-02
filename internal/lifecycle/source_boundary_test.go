package lifecycle

import (
	"context"
	"encoding/json"
	"github.com/AdamNi-7080/AIOS/internal/installstate"
	"github.com/AdamNi-7080/AIOS/internal/mirror"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func persistBoundaryApproval(t *testing.T, o Options, source string) {
	t.Helper()
	if out, e := exec.Command("git", "-C", source, "init", "-q").CombinedOutput(); e != nil {
		t.Fatal(e, string(out))
	}
	b, e := json.Marshal(map[string]any{"mode": "local", "state": "ready", "local_repositories": []map[string]string{{"id": "source", "path": source}}})
	if e != nil {
		t.Fatal(e)
	}
	if e = os.WriteFile(filepath.Join(o.DataDir, "setup.json"), b, 0600); e != nil {
		t.Fatal(e)
	}
	if e = mirror.ValidateSourceBoundaries(mirror.Registry{Version: 1, Repositories: []mirror.Repository{{ID: "source", URL: source}}}, o.DataDir); e != nil {
		t.Fatal("approval should satisfy data source boundary", e)
	}
}
func TestUninstallRefreshesSourceApprovalAfterStopping(t *testing.T) {
	o, _, _ := uninstallFixture(t)
	source := filepath.Join(o.Root, "late-approved-source")
	if e := os.Mkdir(source, 0700); e != nil {
		t.Fatal(e)
	}
	sentinel := filepath.Join(source, "code.go")
	if e := os.WriteFile(sentinel, []byte("source"), 0600); e != nil {
		t.Fatal(e)
	}
	e := uninstallCandidate(context.Background(), o, false, func(context.Context, Options) error { persistBoundaryApproval(t, o, source); return nil })
	if e == nil {
		t.Fatal("uninstall deleted a source approval persisted by daemon before stop completed")
	}
	if b, e := os.ReadFile(sentinel); e != nil || string(b) != "source" {
		t.Fatal("source mutated", e)
	}
}
func TestUninstallProtectsApprovedPlistWorkspace(t *testing.T) {
	o, _, _ := uninstallFixture(t)
	p, e := MakePlan(o)
	if e != nil {
		t.Fatal(e)
	}
	source := filepath.Dir(p.PlistPath)
	persistBoundaryApproval(t, o, source)
	before, e := os.ReadFile(p.PlistPath)
	if e != nil {
		t.Fatal(e)
	}
	e = uninstallCandidate(context.Background(), o, false, func(context.Context, Options) error { return nil })
	if e == nil {
		t.Fatal("uninstall removed a file inside the approved source workspace containing its plist")
	}
	after, e := os.ReadFile(p.PlistPath)
	if e != nil || string(after) != string(before) {
		t.Fatal("source-owned plist mutated", e)
	}
}

func TestUpgradePreservesApprovedCurrentWorkspace(t *testing.T) {
	o, path, _, id := upgradeFixture(t)
	source := filepath.Join(o.Root, "current", "approved-source")
	if e := os.Mkdir(source, 0700); e != nil {
		t.Fatal(e)
	}
	sentinel := filepath.Join(source, "code.go")
	if e := os.WriteFile(sentinel, []byte("source"), 0600); e != nil {
		t.Fatal(e)
	}
	persistBoundaryApproval(t, o, source)
	ops := portableUpgradeOps(id, func(context.Context, string, string) error { return nil })
	originalValidate := ops.validate
	ops.validate = func(ctx context.Context, bin, data string) (candidateValidation, error) {
		reg := mirror.Registry{Version: 1, Repositories: []mirror.Repository{{ID: "source", URL: source}}}
		for _, target := range []string{data, o.DataDir} {
			if e := mirror.ValidateSourceBoundaries(reg, target); e != nil {
				return candidateValidation{}, e
			}
		}
		return originalValidate(ctx, bin, data)
	}
	_, e := applyUpgrade(context.Background(), path, o, ops)
	if e == nil {
		if b, e := os.ReadFile(sentinel); e != nil || string(b) != "source" {
			t.Fatal("successful upgrade removed an approved source workspace in replaced current tree", e)
		}
	}
}

func TestUninstallProtectsApprovedDurableAuthorityWorkspace(t *testing.T) {
	o, _, _ := uninstallFixture(t)
	source := installstate.Authority(o.Root)
	if e := os.Mkdir(source, 0700); e != nil {
		t.Fatal(e)
	}
	persistBoundaryApproval(t, o, source)
	sentinel := filepath.Join(source, "code.go")
	if e := os.WriteFile(sentinel, []byte("source"), 0600); e != nil {
		t.Fatal(e)
	}
	reg := mirror.Registry{Version: 1, Repositories: []mirror.Repository{{URL: source}}}
	if e := ValidateInstallationSourceBoundaries(reg, o.DataDir); e == nil {
		t.Fatal("approved durable authority workspace")
	}
	if e := uninstallCandidate(context.Background(), o, false, func(context.Context, Options) error { return nil }); e == nil {
		t.Fatal("uninstall wrote approved external authority")
	}
	if b, e := os.ReadFile(sentinel); e != nil || string(b) != "source" {
		t.Fatal("source mutated", e)
	}
	if _, e := os.Lstat(filepath.Join(source, uninstallJournalName)); !os.IsNotExist(e) {
		t.Fatal("journal written in source", e)
	}
}
