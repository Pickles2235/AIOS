//go:build !windows

package mirror

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestPipeHoldingHelperCannotDefeatGitDeadline(t *testing.T) {
	root := t.TempDir()
	pid := filepath.Join(root, "child.pid")
	helper := filepath.Join(root, "helper")
	body := "#!/bin/sh\nsleep 120 &\nprintf '%s' $! > '" + pid + "'\nwait\n"
	if err := os.WriteFile(helper, []byte(body), 0700); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", "-c", "credential.helper=!"+helper, "credential", "fill")
	cmd.Env = CredentialEnvironment()
	cmd.Stdin = strings.NewReader("protocol=https\nhost=fixture.invalid\n\n")
	var output bytes.Buffer
	cmd.Stdout = &output
	cmd.Stderr = &output
	boundGitProcess(cmd)
	start := time.Now()
	err := cmd.Run()
	cleanupGitProcess(cmd)
	if err == nil || time.Since(start) > 3*time.Second {
		t.Fatal("credential descendant defeated bounded cancellation")
	}
	child, err := os.ReadFile(pid)
	if err != nil {
		t.Fatal("actual pipe-holding helper did not run", err)
	}
	deadline := time.Now().Add(time.Second)
	for {
		state, _ := exec.Command("ps", "-o", "stat=", "-p", string(child)).Output()
		if len(bytes.TrimSpace(state)) == 0 || strings.HasPrefix(strings.TrimSpace(string(state)), "Z") {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("cancel left owned helper descendant running")
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestExitedGitWithInheritedPipeIsBounded(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "sh", "-c", "sleep 120 & exit 0")
	var output bytes.Buffer
	cmd.Stdout = &output
	cmd.Stderr = &output
	boundGitProcess(cmd)
	start := time.Now()
	err := cmd.Run()
	cleanupGitProcess(cmd)
	if err != exec.ErrWaitDelay || time.Since(start) > 3*time.Second {
		t.Fatal("exited parent left pipe wait unbounded", err)
	}
}
