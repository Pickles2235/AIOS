//go:build darwin || linux

package adapter

import (
	"golang.org/x/sys/unix"
	"os"
)

func openWorkingFile(root *os.Root, path string) (*os.File, error) {
	// A replaced FIFO must not block before the descriptor's regular-file check.
	return root.OpenFile(path, os.O_RDONLY|unix.O_NOFOLLOW|unix.O_NONBLOCK, 0)
}
