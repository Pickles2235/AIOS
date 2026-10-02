//go:build completiontest

package lifecycle

import (
	"context"
	"fmt"
	"os"
	"syscall"
	"time"
)

func upgradeCheckpoint(ctx context.Context, root, boundary string) error {
	if os.Getenv("AIOS_COMPLETION_FAULT") == boundary {
		return fmt.Errorf("injected update boundary: %s", boundary)
	}
	if os.Getenv("AIOS_COMPLETION_ENOSPC") == boundary {
		return syscall.ENOSPC
	}
	if os.Getenv("AIOS_COMPLETION_PAUSE") == boundary {
		if e := writeOwnedJSON(root, "completion-pause.json", map[string]string{"boundary": boundary}); e != nil {
			return e
		}
		for {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(100 * time.Millisecond):
			}
		}
	}
	return ctx.Err()
}
