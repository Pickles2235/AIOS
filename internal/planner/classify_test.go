package planner

import "testing"

func TestClassifyAndCompileFamilies(t *testing.T) {
	for _, tc := range []struct {
		text string
		want Family
	}{
		{"find \"PING_ONE_MFA_ENV_ID\" configuration", Configuration},
		{"who consumes customer.changed event", CallerReference},
		{"trace POST /api/customers route", RouteTrace},
		{"why does publishCustomerChanged fail", ExplainCause},
		{"where update UserDirectory", LocateChange},
		{"what is the impact of AccountService", Impact},
		{"does any caller of MissingService exist", Negative},
		{"UserDirectory", ExactLookup},
	} {
		got, _, plan, err := Compile(tc.text, Budget{})
		if err != nil || got != tc.want || plan.Operator != Explain {
			t.Fatalf("%q: family=%q plan=%#v err=%v", tc.text, got, plan, err)
		}
	}
}

func TestClassifyRecognizesPathsAndAmbiguity(t *testing.T) {
	f := Classify("find src/api/CustomerController.java")
	if !f.Path {
		t.Fatal("path not recognized")
	}
	f = Classify("customer")
	if f.Identifier || f.Path || f.Event || f.Route || f.Configuration {
		t.Fatalf("ambiguous term over-classified: %#v", f)
	}
}

func TestSliceShapeSelectionIsFixed(t *testing.T) {
	for text, want := range map[string]string{"show event flow": "event_topic_flow", "show API surface": "contract_api_surface", "show dependency boundary": "dependency_boundary"} {
		got, ok := SliceKind(text)
		if !ok || got != want {
			t.Fatalf("%q = %q %v", text, got, ok)
		}
	}
	if _, ok := SliceKind("dump the whole graph"); ok {
		t.Fatal("accepted unrestricted graph dump")
	}
}
