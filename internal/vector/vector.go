// Package vector contains replaceable local-only embedding primitives.  It
// deliberately has no network, model-download, or source-reading capability.
package vector

import (
	"crypto/sha256"
	"math"
	"regexp"
	"strings"

	"github.com/AdamNi-7080/AIOS/internal/semantic"
)

// Embedder is the narrow boundary used by the vector projection.  A bundled
// native model can replace Local without changing IR or MCP contracts.
type Embedder interface {
	Identity() string
	Dimensions() int
	Embed(string) ([]float64, error)
}

type Local struct {
	identity   string
	dimensions int
}

// NewBundled returns the pinned local Nomic provider. The executable and model
// are embedded in the Go binary and materialized only under the agent data dir.
func NewBundled(dataDir string) Embedder { return semantic.New(dataDir) }

func NewLocal(identity string, dimensions int) Local {
	if identity == "" {
		identity = "local-token-vector-v1"
	}
	if dimensions < 8 {
		dimensions = 8
	}
	return Local{identity: identity, dimensions: dimensions}
}
func (e Local) Identity() string { return e.identity }
func (e Local) Dimensions() int  { return e.dimensions }

var words = regexp.MustCompile(`[[:alnum:]_./:-]+`)

// Embed is deterministic feature hashing. It is intentionally a safe local
// fallback while the optional bundled model provider is unavailable.
func (e Local) Embed(text string) ([]float64, error) {
	v := make([]float64, e.dimensions)
	for _, token := range words.FindAllString(strings.ToLower(text), -1) {
		h := sha256.Sum256([]byte(token))
		i := int(h[0])<<8 | int(h[1])
		i %= len(v)
		if h[2]&1 == 0 {
			v[i]++
		} else {
			v[i]--
		}
	}
	normalize(v)
	return v, nil
}
func normalize(v []float64) {
	var n float64
	for _, x := range v {
		n += x * x
	}
	if n == 0 {
		return
	}
	n = math.Sqrt(n)
	for i := range v {
		v[i] /= n
	}
}
func Cosine(a, b []float64) float64 {
	if len(a) != len(b) {
		return 0
	}
	var n float64
	for i := range a {
		n += a[i] * b[i]
	}
	return n
}
func Equal(a, b []float64) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
