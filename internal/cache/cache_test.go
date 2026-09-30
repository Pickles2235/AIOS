package cache

import (
	"context"
	"sync"
	"testing"
)

func meta() Metadata {
	return Metadata{Kind: Candidate, RequestFingerprint: "request", SourceScope: []string{"repo"}, GenerationFingerprint: "gen", ProjectionFingerprints: map[string]string{"lexical": "p"}, BudgetFingerprint: "budget", OutputShape: "candidate_handles"}
}
func TestKeyDeterministicAndPayloadSafe(t *testing.T) {
	a := meta()
	a.SourceScope = []string{"b", "a"}
	b := meta()
	b.SourceScope = []string{"a", "b"}
	if Key(a) != Key(b) {
		t.Fatal("key order is unstable")
	}
	s, e := Open(t.TempDir())
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	if e = s.Put(context.Background(), meta(), []byte(`{"content":"source"}`)); e == nil {
		t.Fatal("raw source field accepted")
	}
}
func TestCoalescesAndRejectsStale(t *testing.T) {
	s, e := Open(t.TempDir())
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	var n int
	var wg sync.WaitGroup
	for range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _, e := s.Do(context.Background(), meta(), func(Metadata) bool { return true }, func(context.Context) ([]byte, error) { n++; return []byte(`{"handles":["kb.e2.x"]}`), nil })
			if e != nil {
				t.Error(e)
			}
		}()
	}
	wg.Wait()
	if n != 1 {
		t.Fatalf("builds=%d", n)
	}
	if _, ok := s.Get(context.Background(), meta(), func(Metadata) bool { return false }); ok {
		t.Fatal("stale entry accepted")
	}
}
func TestScopedInvalidation(t *testing.T) {
	s, e := Open(t.TempDir())
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	a := meta()
	b := meta()
	b.SourceScope = []string{"other"}
	if e = s.Put(context.Background(), a, []byte(`{"handles":[]}`)); e != nil {
		t.Fatal(e)
	}
	if e = s.Put(context.Background(), b, []byte(`{"handles":[]}`)); e != nil {
		t.Fatal(e)
	}
	if e = s.Invalidate(context.Background(), "delta", []string{"repo"}); e != nil {
		t.Fatal(e)
	}
	if _, ok := s.Get(context.Background(), a, func(Metadata) bool { return true }); ok {
		t.Fatal("affected entry retained")
	}
	if _, ok := s.Get(context.Background(), b, func(Metadata) bool { return true }); !ok {
		t.Fatal("unrelated entry removed")
	}
}

func TestNegativeEvidenceRejectsChangedCoverage(t *testing.T) {
	s, e := Open(t.TempDir())
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	m := meta()
	m.Kind, m.CoverageFingerprint = NegativeEvidence, "coverage-one"
	if e = s.Put(context.Background(), m, []byte(`{"state":"not_found"}`)); e != nil {
		t.Fatal(e)
	}
	if _, ok := s.Get(context.Background(), m, func(got Metadata) bool { return got.CoverageFingerprint == "coverage-two" }); ok {
		t.Fatal("negative evidence survived changed coverage")
	}
}
