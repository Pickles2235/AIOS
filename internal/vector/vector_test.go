package vector

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestLocalEmbedderIsDeterministicAndNormalised(t *testing.T) {
	e := NewLocal("local-token-vector-v1", 16)
	a, err := e.Embed("Customer changed event")
	if err != nil {
		t.Fatal(err)
	}
	b, err := e.Embed("Customer changed event")
	if err != nil {
		t.Fatal(err)
	}
	if len(a) != 16 || len(b) != 16 || !Equal(a, b) {
		t.Fatalf("vectors are not deterministic: %#v %#v", a, b)
	}
	if score := Cosine(a, a); score < .999 || score > 1.001 {
		t.Fatalf("self similarity=%v", score)
	}
}

type cancelledNativeFixture struct{ Local }

func (cancelledNativeFixture) Embed(string) ([]float64, error) { panic("used uncancellable path") }
func (cancelledNativeFixture) EmbedContext(ctx context.Context, text string) ([]float64, error) {
	<-ctx.Done()
	return nil, ctx.Err()
}
func TestEmbeddingForwardsDeadlineToNativeProvider(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if _, err := Embed(ctx, cancelledNativeFixture{NewLocal("fixture", 16)}, "query"); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("deadline: %v", err)
	}
}
