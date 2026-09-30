package planner

import "testing"

func TestRequestNormalizesAndSelectsDeterministicPrecedence(t *testing.T) {
	r, err := (Request{Text: " Publish ", Intent: IntentLookup}).Normalized()
	if err != nil {
		t.Fatal(err)
	}
	if r.Text != "Publish" || r.Intent != IntentLookup {
		t.Fatalf("normalized request = %#v", r)
	}
	p, err := Plan(r, Budget{})
	if err != nil {
		t.Fatal(err)
	}
	if p.Strategy != StrategyExact || len(p.Stages) != 2 || p.Stages[0].Strategy != StrategyExact || p.Stages[1].Strategy != StrategyLexical {
		t.Fatalf("plan = %#v", p)
	}
	want := []Strategy{StrategyExact, StrategyLexical, StrategyGraph, StrategyStructural, StrategyPath, StrategyVector}
	if len(p.Precedence) != len(want) {
		t.Fatalf("precedence = %#v", p.Precedence)
	}
	for i := range want {
		if p.Precedence[i] != want[i] {
			t.Fatalf("precedence = %#v", p.Precedence)
		}
	}
}

func TestRequestInfersCompatibleIntentAndExplicitGraphSkipsLexicalResult(t *testing.T) {
	r, err := (Request{Text: "who consumes customer.changed event"}).Normalized()
	if err != nil || r.Intent != IntentConsumers || !r.Inferred {
		t.Fatalf("request = %#v err=%v", r, err)
	}
	p, err := Plan(Request{Text: "Publish", Intent: IntentConsumers}, Budget{})
	if err != nil {
		t.Fatal(err)
	}
	if p.Strategy != StrategyGraph || len(p.Stages) != 2 || p.Stages[0].Strategy != StrategyExact || p.Stages[1].Strategy != StrategyGraph {
		t.Fatalf("plan = %#v", p)
	}
}
