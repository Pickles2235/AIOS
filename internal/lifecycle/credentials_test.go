package lifecycle

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
)

func TestGitConfigPointersNeverFreezeLoginSSHAgent(t *testing.T) {
	root := canonicalTemp(t)
	config := filepath.Join(root, "external & gitconfig")
	t.Setenv("GIT_CONFIG_GLOBAL", config)
	t.Setenv("SSH_AUTH_SOCK", filepath.Join(root, "old-login-socket"))
	values, err := machineGitEnvironment()
	if err != nil {
		t.Fatal(err)
	}
	if _, saved := values["SSH_AUTH_SOCK"]; saved {
		t.Fatal("volatile installer socket was saved")
	}
	t.Setenv("SSH_AUTH_SOCK", filepath.Join(root, "new-login-socket"))
	plan, err := MakePlan(Options{Root: root, DataDir: filepath.Join(root, "data"), home: root, gitEnvironment: values})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(plan.Plist, "SSH_AUTH_SOCK") || !strings.Contains(plan.Plist, "external &amp; gitconfig") {
		t.Fatal("plist overrides login socket or lost escaped static configuration")
	}
	for _, invalid := range []map[string]string{{"GH_TOKEN": "secret"}, {"GIT_CONFIG_GLOBAL": "relative"}, {"SSH_AUTH_SOCK": filepath.Join(root, "socket")}} {
		if validateGitEnvironment(invalid) == nil {
			t.Fatal("unsafe/stale environment pointer accepted")
		}
	}
}

func TestCredentialProbeRunsOnlyOnExplicitControlAction(t *testing.T) {
	root := canonicalTemp(t)
	o := Options{Root: root, DataDir: filepath.Join(root, "data")}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	calls := 0
	if err := ServeControlWithCredentials(ctx, o, func() State { return State{Running: true} }, func() string { return "fresh" }, func() map[string]bool { calls++; return map[string]bool{"actual_probe": true} }); err != nil {
		t.Fatal(err)
	}
	for _, action := range []string{"status", "open"} {
		if _, err := Control(o, action); err != nil {
			t.Fatal(err)
		}
	}
	if calls != 0 {
		t.Fatal("startup/status/open performed a Git prerequisite probe")
	}
	state, err := Control(o, "credentials")
	if err != nil || !state.Credentials["actual_probe"] || calls != 1 {
		t.Fatal("explicit credential diagnostic did not reach daemon context", err)
	}
}
