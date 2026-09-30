package benchmark

import (
	"context"
	"path/filepath"
	"testing"
)

func TestFixtureIsRunnableAndReproducible(t *testing.T) {
	fixture := filepath.Join("..", "..", "scripts", "benchmark", "fixtures", "retrieval-v1.json")
	report, err := Run(context.Background(), fixture, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Results) != 14 {
		t.Fatalf("results=%#v", report.Results)
	}
	for _, result := range report.Results {
		if !result.StateCorrect || result.Baseline.SourceReads == 0 {
			t.Fatalf("result=%#v", result)
		}
		if result.ExpectedState == "found" && (result.Revision == "" || !result.Canonical.Correct) {
			t.Fatalf("positive result=%#v", result)
		}
		for _, mode := range []string{"exact_only", "lexical_only", "structural_only", "graph_only", "hybrid"} {
			if _, ok := result.Retrievers[mode]; !ok {
				t.Fatalf("missing %s: %#v", mode, result.Retrievers)
			}
		}
		if len(result.HybridVsSingle) != 4 {
			t.Fatalf("comparison=%#v", result.HybridVsSingle)
		}
		if result.ExpectedState == "found" && (result.Context.PackageBytes == 0 || result.Context.EstimatedTokens == 0 || result.Context.SourceLines == 0 || !result.Context.ProvenanceCorrect || result.UnboundedContext.PackageBytes < result.Context.PackageBytes) {
			t.Fatalf("context=%#v unbounded=%#v", result.Context, result.UnboundedContext)
		}
	}
	if report.Quality.NegativeEvidenceAccuracy != 1 || report.Quality.UnknownRate == 0 {
		t.Fatalf("quality=%#v", report.Quality)
	}
}

func TestFixtureValidationRejectsInvalidState(t *testing.T) {
	if err := validateFixture(Fixture{Repositories: []Repository{{ID: "r"}}, Cases: []Case{{ID: "c", Query: "x", ExpectedState: "maybe"}}}); err == nil {
		t.Fatal("invalid state accepted")
	}
}

func TestCompilerFixtureMeasuresProvenanceAndCoverage(t *testing.T) {
	fixture := filepath.Join("..", "..", "scripts", "benchmark", "fixtures", "compiler-v1.json")
	report, err := Run(context.Background(), fixture, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Results) != 6 {
		t.Fatalf("results=%#v", report.Results)
	}
	for _, result := range report.Results {
		if !result.Canonical.Correct || !result.Canonical.ProvenanceCorrect || len(result.Canonical.ExtractorCoverage) == 0 {
			t.Fatalf("result=%#v", result)
		}
		want := "java-compiler-v1"
		if result.Expected["repository"] == "typescript" {
			want = "typescript-compiler-v1"
		}
		if !result.Canonical.ExtractorCoverage[want] {
			t.Fatalf("missing compiler coverage %q: %#v", want, result.Canonical.ExtractorCoverage)
		}
	}
	if report.Indexing.ColdIndexMS < 0 || report.Indexing.WarmIndexMS < 0 || report.Indexing.ChangedFiles["modified"] != 1 || !report.Indexing.RetrievalCorrectBefore || !report.Indexing.RetrievalCorrectAfter {
		t.Fatalf("incremental metrics=%#v", report.Indexing)
	}
	if len(report.Aggregates) != 6 || report.Aggregates["hybrid"].Cases != len(report.Results) || report.Quality.LLMFreeAnswerRate != 1 {
		t.Fatalf("aggregates=%#v quality=%#v", report.Aggregates, report.Quality)
	}
}
