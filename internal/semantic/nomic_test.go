package semantic

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestBundledNomicEmbedsLocally(t *testing.T) {
	if available, reason := RuntimeAvailability(); !available {
		t.Skip(reason)
	}
	n := New(t.TempDir())
	v, err := n.Embed("customer changed event")
	if err != nil {
		t.Fatal(err)
	}
	if len(v) != Dimensions {
		t.Fatalf("dimensions=%d", len(v))
	}
	for path, mode := range map[string]os.FileMode{filepath.Dir(n.model): 0700, n.model: 0600, n.runtime: 0700} {
		info, err := os.Stat(path)
		if err != nil || info.Mode().Perm() != mode {
			t.Fatalf("runtime permissions for %s: %v", path, err)
		}
	}
	if ModelSHA256() != "f7af6f66802f4df86eda10fe9bbcfc75c39562bed48ef6ace719a251cf1c2fdb" {
		t.Fatal("unexpected bundled model")
	}
}

func TestCancelledEmbeddingDoesNotMaterializeOrLaunchRuntime(t *testing.T) {
	dir := t.TempDir()
	n := New(dir)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := n.EmbedContext(ctx, "query"); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancel: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "vector-runtime")); !os.IsNotExist(err) {
		t.Fatal("cancelled call prepared runtime assets")
	}
}
