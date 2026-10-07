package mirror

import (
	"context"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// archiveStream starts a local git archive with bounded inherited environment.
// The caller must drain or cancel stdout and wait for the process.
func archiveStream(ctx context.Context, dir, revision string) (*exec.Cmd, io.ReadCloser, error) {
	cmd := exec.CommandContext(ctx, "git", "--no-pager", "-c", "core.hooksPath=/dev/null", "-c", "credential.helper=", "-C", dir, "archive", "--format=tar", revision)
	cmd.Env = append([]string{"GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_NOSYSTEM=1", "GIT_TERMINAL_PROMPT=0", "LC_ALL=C"}, "PATH="+os.Getenv("PATH"))
	cmd.Stderr = &archiveStderr{}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, nil, err
	}
	if err = cmd.Start(); err != nil {
		stdout.Close()
		return nil, nil, err
	}
	return cmd, stdout, nil
}

// Git messages can contain local paths; retain only a small buffer and expose
// stable remediation categories rather than raw stderr.
type archiveStderr struct{ data []byte }

func (b *archiveStderr) Write(p []byte) (int, error) {
	n := len(p)
	if len(b.data) < 4096 {
		keep := 4096 - len(b.data)
		if len(p) < keep {
			keep = len(p)
		}
		b.data = append(b.data, p[:keep]...)
	}
	return n, nil
}
func (b *archiveStderr) reason() string {
	s := strings.ToLower(string(b.data))
	switch {
	case strings.Contains(s, "not a valid object"), strings.Contains(s, "not a tree object"), strings.Contains(s, "ambiguous argument"):
		return "approved revision is unavailable in the local mirror"
	case strings.Contains(s, "permission denied"), strings.Contains(s, "cannot change to"):
		return "local mirror is inaccessible"
	default:
		return "local mirror archive failed"
	}
}
func makeReadOnly(root string) error {
	return filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return os.Chmod(path, 0500)
		}
		return os.Chmod(path, 0400)
	})
}
