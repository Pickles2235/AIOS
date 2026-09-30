package semantic

import "testing"

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
	if ModelSHA256() != "f7af6f66802f4df86eda10fe9bbcfc75c39562bed48ef6ace719a251cf1c2fdb" {
		t.Fatal("unexpected bundled model")
	}
}
