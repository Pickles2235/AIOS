package vector

import "testing"

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
