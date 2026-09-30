package mirror

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

func archiveBytes(ctx context.Context, dir, revision string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "git", "--no-pager", "-c", "core.hooksPath=/dev/null", "-c", "credential.helper=", "-C", dir, "archive", "--format=tar", revision)
	cmd.Env = append([]string{"GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_NOSYSTEM=1", "GIT_TERMINAL_PROMPT=0", "LC_ALL=C"}, "PATH="+os.Getenv("PATH"))
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	b, e := cmd.Output()
	if e != nil {
		return nil, fmt.Errorf("git archive: %w: %s", e, stderr.String())
	}
	return b, nil
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
