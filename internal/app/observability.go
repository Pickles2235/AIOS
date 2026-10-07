package app

import (
	"context"

	"github.com/AdamNi-7080/AIOS/internal/observability"
)

func observeIngest(ctx context.Context, dataDir, repository string) (context.Context, func(error)) {
	c := observability.From(ctx)
	owned := false
	if c == nil {
		c, _ = observability.Open(dataDir)
		owned = c != nil
	}
	ctx, span := c.Start(ctx, "ingest", map[string]string{"repository": repository})
	return ctx, func(err error) {
		observability.End(span, err != nil)
		if owned {
			_ = c.Close()
		}
	}
}
