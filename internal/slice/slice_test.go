package slice

import "testing"

func TestSelectorAndBudgetNormalize(t *testing.T) {
	s, err := (Selector{Kind: EventFlow, EntityHandles: []string{"b", "a"}}).Normalized()
	if err != nil || s.EntityHandles[0] != "a" {
		t.Fatalf("selector=%#v err=%v", s, err)
	}
	if _, err := (Selector{Kind: "dump", Anchor: "x"}).Normalized(); err == nil {
		t.Fatal("accepted arbitrary graph kind")
	}
	b, err := (Budget{}).Normalized()
	if err != nil || b.Depth != DefaultDepth {
		t.Fatalf("budget=%#v err=%v", b, err)
	}
	if _, err := (Budget{Fanout: MaxFanout + 1}).Normalized(); err == nil {
		t.Fatal("accepted unbounded fanout")
	}
}

func TestKindProfilesAreFixed(t *testing.T) {
	for _, k := range []Kind{RepositoryServiceOverview, ServiceBoundary, ContractSurface, EventFlow, HTTPFlow, Configuration, Dependency, TestFeature, Impact, Neighbourhood} {
		if !k.Valid() {
			t.Fatalf("invalid kind %q", k)
		}
	}
	p, d := Predicates(EventFlow)
	if len(p) == 0 || d != "both" {
		t.Fatalf("event profile=%v %s", p, d)
	}
	if p, d := Predicates(Impact); p != nil || d != "in" {
		t.Fatalf("impact profile=%v %s", p, d)
	}
}
