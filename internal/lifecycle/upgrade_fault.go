//go:build !completiontest

package lifecycle

import "context"

func upgradeCheckpoint(ctx context.Context, root, boundary string) error { return ctx.Err() }
