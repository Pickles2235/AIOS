package app

import "context"

// Progress describes completed boundaries, never evidence eligible for queries.
type Progress struct {
	Stage      string `json:"stage"`
	Repository string `json:"repository"`
	Generation string `json:"generation,omitempty"`
	Files      int    `json:"files"`
}
type progressKey struct{}

func WithProgress(ctx context.Context, observe func(Progress)) context.Context {
	return context.WithValue(ctx, progressKey{}, observe)
}
func progress(ctx context.Context, p Progress) {
	if observe, ok := ctx.Value(progressKey{}).(func(Progress)); ok && observe != nil {
		observe(p)
	}
}
