package mirror

import (
	"context"
	"encoding/pem"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestActualMachineCredentialHelperAndSanitizedAuthFailure(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "source")
	if e := os.Mkdir(source, 0700); e != nil {
		t.Fatal(e)
	}
	run(t, source, "init", "-q", "--initial-branch=main")
	os.WriteFile(filepath.Join(source, "Readme.txt"), []byte("fixture\n"), 0600)
	run(t, source, "add", ".")
	run(t, source, "-c", "user.name=fixture", "-c", "user.email=fixture@example.test", "commit", "-qm", "fixture")
	remote := filepath.Join(root, "repo.git")
	b, e := exec.Command("git", "clone", "--bare", source, remote).CombinedOutput()
	if e != nil {
		t.Fatalf("fixture clone: %v %s", e, b)
	}
	run(t, remote, "update-server-info")
	var valid atomic.Bool
	valid.Store(true)
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, pass, ok := r.BasicAuth()
		if !valid.Load() || !ok || user != "fixture" || pass != "fixture-test-password" {
			w.Header().Set("WWW-Authenticate", `Basic realm="fixture"`)
			w.WriteHeader(401)
			return
		}
		http.FileServer(http.Dir(root)).ServeHTTP(w, r)
	}))
	defer server.Close()
	certificate := filepath.Join(root, "fixture-ca.pem")
	if e = os.WriteFile(certificate, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: server.Certificate().Raw}), 0600); e != nil {
		t.Fatal(e)
	}
	marker := filepath.Join(root, "helper-called")
	helper := filepath.Join(root, "helper.sh")
	profile := filepath.Join(root, "git-config")
	quote := func(s string) string { return "'" + strings.ReplaceAll(s, "'", "'\"'\"'") + "'" }
	if e = os.WriteFile(helper, []byte("#!/bin/sh\nset -eu\ncat >/dev/null\nif [ \"$1\" = get ]; then\n printf called > "+quote(marker)+"\n printf 'username=fixture\\npassword=fixture-test-password\\n'\nfi\n"), 0700); e != nil {
		t.Fatal(e)
	}
	b, e = exec.Command("git", "config", "--file", profile, "credential.helper", "!"+quote(helper)).CombinedOutput()
	if e != nil {
		t.Fatalf("fixture profile: %v %s", e, b)
	}
	t.Setenv("GIT_CONFIG_GLOBAL", profile)
	t.Setenv("GIT_SSL_CAINFO", certificate)
	reg := Registry{Version: 1, Repositories: []Repository{{ID: "fixture", URL: server.URL + "/repo.git", Ref: "refs/heads/main"}}}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if _, e = Sync(ctx, reg, filepath.Join(root, "data")); e != nil {
		t.Fatalf("actual helper failed: %v", e)
	}
	if _, e = os.Stat(marker); e != nil {
		t.Fatal("machine credential helper was never executed")
	}
	status := CredentialStatus(ctx, false)
	if !status["credential_helpers_enabled"] || !status["default_helper_configured"] || status["login_context"] || status["product_token_store"] {
		t.Fatal("credential context dishonest")
	}
	valid.Store(false)
	_, e = Sync(ctx, reg, filepath.Join(root, "failure"))
	var auth *GitError
	if e == nil || !errors.As(e, &auth) || auth.Kind != "authentication" {
		t.Fatalf("auth failure misclassified: %v", e)
	}
	for _, forbidden := range []string{server.URL, "fixture-test-password", profile, source} {
		if strings.Contains(e.Error(), forbidden) || strings.Contains(auth.Remediation(), forbidden) {
			t.Fatal("raw authentication input escaped")
		}
	}
	if !strings.Contains(auth.Remediation(), "Direct") || !strings.Contains(auth.Remediation(), "has not changed") {
		t.Fatal("auth remediation silently changes source mode")
	}
	b, e = exec.Command("git", "-C", source, "status", "--porcelain=v1").Output()
	if e != nil || len(b) != 0 {
		t.Fatal("source mutated")
	}
}
