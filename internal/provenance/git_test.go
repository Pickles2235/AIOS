package provenance

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestInspectIgnoresGitExecutionConfig(t *testing.T) {
	tests := map[string]func(*testing.T, string, string){
		"repository": func(t *testing.T, repo, hook string) {
			runGit(t, repo, "config", "core.fsmonitor", hook)
		},
		"global": func(t *testing.T, _, hook string) {
			home := t.TempDir()
			if err := os.WriteFile(filepath.Join(home, ".gitconfig"), []byte("[core]\n\tfsmonitor = "+hook+"\n"), 0o600); err != nil {
				t.Fatal(err)
			}
			t.Setenv("HOME", home)
		},
		"injected environment": func(t *testing.T, _, hook string) {
			t.Setenv("GIT_CONFIG_COUNT", "1")
			t.Setenv("GIT_CONFIG_KEY_0", "core.fsmonitor")
			t.Setenv("GIT_CONFIG_VALUE_0", hook)
		},
	}
	for name, poison := range tests {
		t.Run(name, func(t *testing.T) {
			repo := initRepository(t)
			marker := filepath.Join(t.TempDir(), "executed")
			hook := filepath.Join(t.TempDir(), "fsmonitor")
			writeExecutable(t, hook, "#!/bin/sh\nprintf executed > \""+marker+"\"\nexit 0\n")
			poison(t, repo, hook)

			before, err := os.ReadFile(filepath.Join(repo, "tracked.txt"))
			if err != nil {
				t.Fatal(err)
			}
			state, err := Inspect(context.Background(), repo)
			if err != nil {
				t.Fatal(err)
			}
			if state.Commit == "" || state.Branch == "" || state.Dirty {
				t.Fatalf("unexpected state: %#v", state)
			}
			if _, err := os.Stat(marker); !os.IsNotExist(err) {
				t.Fatalf("Git configuration executed fsmonitor hook: %v", err)
			}
			after, err := os.ReadFile(filepath.Join(repo, "tracked.txt"))
			if err != nil {
				t.Fatal(err)
			}
			if string(after) != string(before) {
				t.Fatalf("Inspect changed tracked content: before=%q after=%q", before, after)
			}
		})
	}
}

func TestGitEnvironmentIsAllowlisted(t *testing.T) {
	t.Setenv("GIT_CONFIG_COUNT", "1")
	t.Setenv("GIT_CONFIG_KEY_0", "core.fsmonitor")
	t.Setenv("GIT_CONFIG_VALUE_0", "/tmp/hostile")
	t.Setenv("GIT_SSH_COMMAND", "/tmp/hostile")
	for _, entry := range gitEnvironment() {
		if strings.HasPrefix(entry, "GIT_CONFIG_COUNT=") || strings.HasPrefix(entry, "GIT_CONFIG_KEY_") ||
			strings.HasPrefix(entry, "GIT_CONFIG_VALUE_") || strings.HasPrefix(entry, "GIT_SSH_COMMAND=") {
			t.Fatalf("inherited unsafe environment entry %q", entry)
		}
	}
}

func initRepository(t *testing.T) string {
	t.Helper()
	repo := t.TempDir()
	runGit(t, repo, "init", "-q")
	if err := os.WriteFile(filepath.Join(repo, "tracked.txt"), []byte("tracked\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	runGit(t, repo, "add", "tracked.txt")
	runGit(t, repo, "-c", "user.name=Test", "-c", "user.email=test@example.invalid", "commit", "-qm", "initial")
	return repo
}

func runGit(t *testing.T, repo string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", repo}, args...)...)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v: %s", args, err, output)
	}
}

func writeExecutable(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o700); err != nil {
		t.Fatal(err)
	}
}
