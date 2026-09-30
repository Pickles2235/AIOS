package planner

import "testing"

func TestAlgebraValidationIsBounded(t *testing.T) {
	b, err := (Budget{}).Normalized()
	if err != nil || b.TimeMS != DefaultTimeMS || b.Results != DefaultResult {
		t.Fatalf("budget=%#v err=%v", b, err)
	}
	n := Node{Operator: Fetch, Inputs: []*Node{
		{Operator: Rank, Inputs: []*Node{
			{Operator: Union, Inputs: []*Node{
				{Operator: Exact, Text: "UserDirectory"},
				{Operator: Lexical, Text: "UserDirectory", Fields: []string{"source"}},
			}},
		}},
	}}
	if err := n.Validate(b); err != nil {
		t.Fatal(err)
	}
	for _, bad := range []Node{{Operator: "sql"}, {Operator: Traverse, Direction: "all", Depth: 1}, {Operator: Traverse, Direction: "out", Depth: 5}, {Operator: Lexical, Fields: []string{"unsafe"}}, {Operator: Filter, Predicates: []string{"x; drop"}}} {
		if err := bad.Validate(b); err == nil {
			t.Fatalf("accepted %#v", bad)
		}
	}
}

func TestBudgetRejectsOverLimit(t *testing.T) {
	if _, err := (Budget{TimeMS: MaxTimeMS + 1}).Normalized(); err == nil {
		t.Fatal("accepted time limit")
	}
	if _, err := (Budget{Candidates: MaxItems + 1}).Normalized(); err == nil {
		t.Fatal("accepted candidate limit")
	}
}
