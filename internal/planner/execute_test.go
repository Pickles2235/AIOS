package planner

import (
	"testing"
	"time"
)

func TestFusePrefersExactThenStableTieBreak(t *testing.T) {
	all := []Candidate{
		{Key: "same", Repository: "b", Path: "z", Evidence: "v", SourceRank: 4, Confidence: .9},
		{Key: "same", Repository: "a", Path: "a", Evidence: "v", SourceRank: 1, Confidence: .8},
		{Key: "lexical", Repository: "a", Path: "a", Evidence: "v2", Confidence: 1},
	}
	e := Execution{Started: time.Now()}
	e.Add(SourceLexical, all, Budget{Candidates: 10, TimeMS: 1000, Entities: 1, Edges: 1, Depth: 1, Results: 1})
	e.Add(SourceExact, []Candidate{{Key: "same", Repository: "b", Path: "z", Evidence: "v", Confidence: .9}}, Budget{Candidates: 10, TimeMS: 1000, Entities: 1, Edges: 1, Depth: 1, Results: 1})
	out := Fuse(e.Candidates)
	if len(out) != 2 || out[0].Key != "same" || len(out[0].Sources) != 2 {
		t.Fatalf("%#v", out)
	}
	structural := Fuse([]Candidate{{Key: "lexical", Repository: "z", Path: "z", Sources: []Source{SourceLexical}}, {Key: "structural", Repository: "z", Path: "a", Sources: []Source{SourceStructural}}})
	if structural[0].Key != "structural" {
		t.Fatalf("structural evidence did not outrank lexical: %#v", structural)
	}
}
func TestExecutionStopsAndContinuationIsBound(t *testing.T) {
	e := Execution{Started: time.Now()}
	b := Budget{TimeMS: 1000, Candidates: 1, Entities: 1, Edges: 1, Depth: 1, Results: 1}
	e.Add(SourceExact, []Candidate{{Key: "a"}, {Key: "b"}}, b)
	if len(e.Stops) != 1 || e.Stops[0].Reason != "candidate_budget" {
		t.Fatalf("%#v", e)
	}
	v := EncodeContinuation(Continuation{Generation: "g", Family: "exact_lookup", Offset: 1})
	if c, ok := DecodeContinuation(v, "g", "exact_lookup"); !ok || c.Offset != 1 {
		t.Fatalf("%#v %v", c, ok)
	}
	if _, ok := DecodeContinuation(v, "other", "exact_lookup"); ok {
		t.Fatal("stale continuation accepted")
	}
}
