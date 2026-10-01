package app

import "context"

type rebuildKey struct{}

// WithFullRebuild requests fresh extraction from every selected immutable file.
// Last-good canonical generations remain active until validation and promotion.
func WithFullRebuild(ctx context.Context) context.Context {
	return context.WithValue(ctx, rebuildKey{}, true)
}

func fullRebuild(ctx context.Context) bool {
	v, _ := ctx.Value(rebuildKey{}).(bool)
	return v
}
