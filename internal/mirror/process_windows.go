//go:build windows

package mirror

import (
	"os/exec"
	"time"
)

func boundGitProcess(cmd *exec.Cmd)   { cmd.WaitDelay = time.Second }
func cleanupGitProcess(cmd *exec.Cmd) {}
