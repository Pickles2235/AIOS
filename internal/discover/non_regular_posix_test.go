//go:build darwin || linux

package discover

import (
	"path/filepath"
	"syscall"
	"testing"

	"github.com/AdamNi-7080/AIOS/internal/model"
)

func TestFilesRejectsFIFOWithoutBlocking(t *testing.T) {
	root := canonicalTempDir(t)
	write(t, root, "safe.txt", "safe")
	fifo := filepath.Join(root, "pipe.txt")
	if err := syscall.Mkfifo(fifo, 0o600); err != nil {
		t.Fatal(err)
	}
	files, err := Files(model.Repository{ID: "repo", Root: root}, model.Limits{MaxFileBytes: 1024, MaxFilesPerRepo: 20, MaxTotalBytesPerRepo: 4096})
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 1 || files[0].Path != "safe.txt" {
		t.Fatalf("unexpected indexed files: %#v", files)
	}
}
