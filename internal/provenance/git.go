package provenance

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/AdamNi-7080/AIOS/internal/model"
)

func Inspect(ctx context.Context, root string) (model.GitState, error) {
	commit, err := git(ctx, root, "rev-parse", "HEAD")
	if err != nil {
		return model.GitState{}, fmt.Errorf("git commit: %w", err)
	}
	branch, err := git(ctx, root, "symbolic-ref", "--short", "-q", "HEAD")
	if err != nil {
		branch = "DETACHED"
	}
	status, err := gitBytes(ctx, root, "status", "--porcelain=v1", "-z", "--untracked-files=all")
	if err != nil {
		return model.GitState{}, fmt.Errorf("git status: %w", err)
	}
	untracked := 0
	for _, entry := range bytes.Split(status, []byte{0}) {
		if len(entry) >= 2 && string(entry[:2]) == "??" {
			untracked++
		}
	}
	return model.GitState{
		Commit:         strings.TrimSpace(commit),
		Branch:         strings.TrimSpace(branch),
		Dirty:          len(status) > 0,
		UntrackedCount: untracked,
	}, nil
}

func git(ctx context.Context, root string, args ...string) (string, error) {
	b, err := gitBytes(ctx, root, args...)
	return string(b), err
}

func gitBytes(ctx context.Context, root string, args ...string) ([]byte, error) {
	base := []string{
		"--no-pager",
		"--literal-pathspecs",
		"-c", "core.fsmonitor=false",
		"-c", "core.untrackedCache=false",
		"-c", "core.hooksPath=/dev/null",
		"-c", "credential.helper=",
		"-c", "core.askPass=",
		"-c", "diff.external=",
		"-C", root,
	}
	cmd := exec.CommandContext(ctx, "git", append(base, args...)...)
	cmd.Env = gitEnvironment()
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("%v: %w: %s", args, err, strings.TrimSpace(stderr.String()))
	}
	return out, nil
}

func gitEnvironment() []string {
	env := []string{
		"GIT_ASKPASS=/usr/bin/false",
		"GIT_CONFIG_GLOBAL=/dev/null",
		"GIT_CONFIG_NOSYSTEM=1",
		"GIT_OPTIONAL_LOCKS=0",
		"GIT_TERMINAL_PROMPT=0",
		"GCM_INTERACTIVE=never",
		"HOME=/dev/null",
		"LC_ALL=C",
		"SSH_ASKPASS=/usr/bin/false",
		"XDG_CONFIG_HOME=/dev/null",
	}
	if path := os.Getenv("PATH"); path != "" {
		env = append(env, "PATH="+path)
	}
	return env
}
