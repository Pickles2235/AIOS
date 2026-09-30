package ingest

import (
	"github.com/AdamNi-7080/AIOS/internal/model"
	"testing"
)

func TestScopeUsesFullFallbackOnlyForExplicitConditions(t *testing.T) {
	if got := DecideRebuildScope(false, false, []model.FileChange{{Kind: "modified", Path: "src/a.ts"}}); got.Kind != "affected_files_and_cross_identities" {
		t.Fatal(got)
	}
	if got := DecideRebuildScope(false, false, []model.FileChange{{Kind: "modified", Path: "tsconfig.json"}}); got.Kind != "full_repository" {
		t.Fatal(got)
	}
	if got := DecideRebuildScope(true, false, nil); got.Kind != "full_repository" {
		t.Fatal(got)
	}
}
