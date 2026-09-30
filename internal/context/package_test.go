package contextpkg

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestCompilePrefersDirectDeduplicatesAndIsStable(t *testing.T) {
	in := Input{Package: Package{Status: "found", AnswerKind: "event_trace", AnswerAnchors: []string{"e1"}, Derivation: map[string]string{"source": "canonical"}}, Budget: Budget{Entities: 2, Edges: 1, Excerpts: 1, LinesPerExcerpt: 1, MaxBytes: 4096, EstimatedTokens: 1024},
		Entities:      []Entity{{Handle: "e2", Evidence: "v2", Identity: "alias", Role: "supporting", EvidenceTier: 3}, {Handle: "e1", Evidence: "v1", Identity: "anchor", Role: "answer_anchor", EvidenceTier: 0}, {Handle: "e3", Evidence: "v3", Identity: "alias", Role: "supporting", EvidenceTier: 0}},
		Relationships: []Relationship{{Handle: "c2", Subject: "e1", Predicate: "P", Object: "e2", Evidence: "v2", EvidenceTier: 3}, {Handle: "c1", Subject: "e1", Predicate: "P", Object: "e2", Evidence: "v1", EvidenceTier: 0}},
		Excerpts:      []Excerpt{{Handle: "v1", Source: "s1", Repository: "r", Path: "a", Span: Span{1, 2}, OriginalLines: 2}, {Handle: "v2", Source: "s2", Repository: "r", Path: "a", Span: Span{1, 2}, OriginalLines: 2}},
	}
	a, err := Compile(in)
	if err != nil {
		t.Fatal(err)
	}
	b, err := Compile(in)
	if err != nil {
		t.Fatal(err)
	}
	if len(a.Entities) != 2 || a.Entities[0].Handle != "e1" || a.Entities[1].Handle != "e3" {
		t.Fatalf("entities=%#v", a.Entities)
	}
	if len(a.Relationships) != 1 || a.Relationships[0].Handle != "c1" || len(a.Excerpts) != 1 || !a.Excerpts[0].Truncated {
		t.Fatalf("package=%#v", a)
	}
	if a.TokenEstimator == "" || a.EstimatedTokens != EstimateTokens(a.SerializedBytes) {
		t.Fatalf("estimate=%#v", a)
	}
	x, _ := json.Marshal(a)
	y, _ := json.Marshal(b)
	if !reflect.DeepEqual(x, y) {
		t.Fatalf("non-deterministic %s %s", x, y)
	}
}

func TestCompileRecordsBudgetOmissions(t *testing.T) {
	p, err := Compile(Input{Package: Package{Status: "found", AnswerKind: "exact_lookup", AnswerAnchors: []string{"e1"}, Derivation: map[string]string{}}, Budget: Budget{MaxBytes: 4096, EstimatedTokens: 1024, Entities: 1, Edges: 1, Excerpts: 1, LinesPerExcerpt: 1}, Entities: []Entity{{Handle: "e1", Evidence: "v1", Role: "answer_anchor"}, {Handle: "e2", Evidence: "v2"}}, Relationships: []Relationship{{Handle: "c1", Subject: "e1", Predicate: "P", Object: "e2", Evidence: "v1"}, {Handle: "c2", Subject: "e1", Predicate: "Q", Object: "e3", Evidence: "v2"}}, Excerpts: []Excerpt{{Handle: "v1", Source: "s1", Path: "a", Span: Span{StartLine: 1, EndLine: 1}, OriginalLines: 1}, {Handle: "v2", Source: "s2", Path: "b", Span: Span{StartLine: 1, EndLine: 1}, OriginalLines: 1}}})
	if err != nil || !p.Truncated || len(p.Omissions) != 3 {
		t.Fatalf("p=%#v err=%v", p, err)
	}
	for _, omission := range p.Omissions {
		if omission.Expansion.Operation == "" {
			t.Fatalf("missing expansion %#v", omission)
		}
	}
}

func TestCompileRejectsUncitedFacts(t *testing.T) {
	_, err := Compile(Input{Package: Package{Status: "found"}, Entities: []Entity{{Handle: "e1"}}})
	if err == nil {
		t.Fatal("uncited entity accepted")
	}
}

func TestCompileSupportsEveryAnswerKind(t *testing.T) {
	for _, kind := range []string{"exact_lookup", "event_trace", "explain_cause", "route_trace", "configuration_lookup", "locate_change", "impact", "negative_verification"} {
		p, err := Compile(Input{Package: Package{Status: "found", AnswerKind: kind, AnswerAnchors: []string{"e"}, Derivation: map[string]string{"source": "canonical"}}, Entities: []Entity{{Handle: "e", Evidence: "v", Role: "answer_anchor"}}, Excerpts: []Excerpt{{Handle: "v", Source: "s", OriginalLines: 1}}})
		if err != nil || p.AnswerKind != kind || len(p.Entities) != 1 {
			t.Fatalf("kind=%s package=%#v err=%v", kind, p, err)
		}
	}
}

func TestCompileRejectsPackageThatCannotContainAnAnchor(t *testing.T) {
	_, err := Compile(Input{Package: Package{Status: "found", AnswerKind: "exact_lookup", AnswerAnchors: []string{"e"}, Derivation: map[string]string{}}, Entities: []Entity{{Handle: "e", Evidence: "v", Role: "answer_anchor"}}, Budget: Budget{MaxBytes: 1, EstimatedTokens: 1, Entities: 1, Edges: 1, Excerpts: 1, LinesPerExcerpt: 1}})
	if err == nil {
		t.Fatal("impossibly small package accepted")
	}
}
