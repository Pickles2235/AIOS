package knowledge

import (
	"context"
	"strings"
	"testing"
)

func TestInvestigationCommandsAndCanonicalCopy(t *testing.T) {
	if term := SearchTerm("question", "Where is loadOrdersRecord defined?"); term != "loadOrdersRecord" {
		t.Fatalf("question term=%q", term)
	}
	for _, tc := range []struct {
		input, intent, repo, query string
		ok                         bool
	}{
		{"@repo /symbol Publish", "symbol", "repo", "Publish", true},
		{"/event @repo Event", "event", "repo", "Event", true},
		{"/route POST /api/orders", "route", "", "POST /api/orders", true},
		{"/bogus Publish", "unsupported", "", "", false},
		{"@repo @other Publish", "unsupported", "", "", false},
	} {
		intent, repo, query, ok := ParseInvestigation(tc.input)
		if intent != tc.intent || repo != tc.repo || query != tc.query || ok != tc.ok {
			t.Fatalf("%q: %q %q %q %v", tc.input, intent, repo, query, ok)
		}
	}
	db, service := fixture(t)
	defer db.Close()
	result, err := service.Investigation(context.Background(), "@repo /symbol Publish")
	if err != nil || result.SchemaVersion != 1 || result.Status != "found" || len(result.Findings) == 0 || len(result.CanonicalEvidence) != len(result.Findings) || result.Coverage == nil || len(result.Generations) == 0 {
		t.Fatalf("investigation=%+v err=%v", result, err)
	}
	for i, finding := range result.Findings {
		excerpt := result.CanonicalEvidence[i]
		if excerpt.Evidence != finding.Evidence || excerpt.Generation != finding.Entity.Generation || excerpt.Repository != finding.Entity.Repository || excerpt.Path != finding.Entity.Path || excerpt.StartLine < 1 || len(excerpt.Lines) == 0 {
			t.Fatalf("copy source mismatch: %+v %+v", finding, excerpt)
		}
	}
	negative, err := service.Investigation(context.Background(), "/symbol @repo __absent__")
	if err != nil || negative.Status != "not_found" || len(negative.Findings) != 0 || !negative.Coverage.Complete {
		t.Fatalf("negative=%+v err=%v", negative, err)
	}
	unsupported, err := service.Investigation(context.Background(), strings.Repeat("x", 257))
	if err != nil || unsupported.Status != "unknown" || len(unsupported.Unknowns) == 0 {
		t.Fatalf("unsupported=%+v err=%v", unsupported, err)
	}
	if _, err := db.DB().ExecContext(context.Background(), `UPDATE coverage_runs SET status='incomplete'`); err != nil {
		t.Fatal(err)
	}
	event, err := service.Investigation(context.Background(), "/event @repo __unresolved_event__")
	if err != nil || event.Status != "unknown" || event.Coverage == nil || event.Coverage.Capability != "structural" {
		t.Fatalf("event gap=%+v err=%v", event, err)
	}
	question, err := service.Investigation(context.Background(), "Why does the system fail here?")
	if err != nil || question.Status != "unknown" || len(question.Unknowns) == 0 {
		t.Fatalf("unsupported depth=%+v err=%v", question, err)
	}
}

func TestInvestigationGenerationComparison(t *testing.T) {
	a := Status{Repositories: []RepositoryStatus{{ID: "repo", Active: true, Generation: "one"}}}
	b := Status{Repositories: []RepositoryStatus{{ID: "repo", Active: true, Generation: "two"}}}
	if !sameActiveGenerations(a, a) || sameActiveGenerations(a, b) {
		t.Fatal("generation comparison lost promotion")
	}
}
