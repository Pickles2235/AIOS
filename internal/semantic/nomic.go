// Package semantic owns the self-contained, local Nomic embedding runtime.
package semantic

import (
	"context"
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
)

const ModelIdentity = "nomic-embed-text-v1.5.f16:sha256:f7af6f66802f4df86eda10fe9bbcfc75c39562bed48ef6ace719a251cf1c2fdb"
const Dimensions = 768

//go:embed assets/nomic-embed-text-v1.5.f16.gguf
var model []byte

//go:embed assets/llama-embedding-darwin-arm64
var darwinARM64 []byte

type Nomic struct {
	dataDir        string
	once           sync.Once
	runtime, model string
	err            error
}

func New(dataDir string) *Nomic   { return &Nomic{dataDir: dataDir} }
func (n *Nomic) Identity() string { return ModelIdentity }
func (n *Nomic) Dimensions() int  { return Dimensions }

func RuntimeAvailability() (bool, string) {
	if runtime.GOOS == "darwin" && runtime.GOARCH == "arm64" {
		return true, "bundled_nomic"
	}
	return false, fmt.Sprintf("bundled Nomic runtime is unavailable for %s/%s", runtime.GOOS, runtime.GOARCH)
}

func (n *Nomic) prepare() {
	n.once.Do(func() {
		if available, reason := RuntimeAvailability(); !available {
			n.err = errors.New(reason)
			return
		}
		d := filepath.Join(n.dataDir, "vector-runtime", "nomic-embed-text-v1.5")
		if err := os.MkdirAll(d, 0700); err != nil {
			n.err = err
			return
		}
		_ = os.Chmod(d, 0700)
		n.model = filepath.Join(d, "model.gguf")
		n.runtime = filepath.Join(d, "llama-embedding")
		if n.err = materialize(n.model, model, 0600); n.err != nil {
			return
		}
		n.err = materialize(n.runtime, darwinARM64, 0700)
	})
}
func materialize(path string, data []byte, mode os.FileMode) error {
	if b, err := os.ReadFile(path); err == nil {
		h := sha256.Sum256(b)
		want := sha256.Sum256(data)
		if h == want {
			info, err := os.Stat(path)
			if err != nil {
				return err
			}
			if info.Mode().Perm() == mode {
				return nil
			}
			return os.Chmod(path, mode)
		}
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, mode); err != nil {
		return err
	}
	if err := os.Chmod(tmp, mode); err != nil {
		return err
	}
	return replaceFile(tmp, path)
}
func (n *Nomic) Embed(text string) ([]float64, error) {
	return n.EmbedContext(context.Background(), text)
}

// Prepare verifies/materializes bundled assets once during owned runtime setup.
func (n *Nomic) Prepare(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	n.prepare()
	if err := ctx.Err(); err != nil {
		return err
	}
	return n.err
}

// EmbedContext keeps the bundled helper within the caller's query/build budget.
func (n *Nomic) EmbedContext(ctx context.Context, text string) ([]float64, error) {
	if err := n.Prepare(ctx); err != nil {
		return nil, err
	}
	// Avoid per-process Metal shader/warmup costs for short bounded queries.
	// The pinned model and mean pooling stay unchanged; CPU execution uses two threads.
	cmd := exec.CommandContext(ctx, n.runtime, "-m", n.model, "--gpu-layers", "0", "--no-warmup", "--threads", "2", "--pooling", "mean", "--embd-output-format", "array", "-p", text)
	b, err := cmd.CombinedOutput()
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, fmt.Errorf("local Nomic embedding: %w: %s", err, strings.TrimSpace(string(b)))
	}
	start, end := strings.IndexByte(string(b), '['), strings.LastIndexByte(string(b), ']')
	if start < 0 || end <= start {
		return nil, fmt.Errorf("local Nomic embedding returned no vector")
	}
	var rows [][]float64
	if err = json.Unmarshal(b[start:end+1], &rows); err != nil {
		return nil, err
	}
	if len(rows) != 1 || len(rows[0]) != Dimensions {
		return nil, fmt.Errorf("local Nomic returned %d vectors of %d dimensions", len(rows), func() int {
			if len(rows) == 0 {
				return 0
			}
			return len(rows[0])
		}())
	}
	return rows[0], nil
}
func ModelSHA256() string { h := sha256.Sum256(model); return hex.EncodeToString(h[:]) }
