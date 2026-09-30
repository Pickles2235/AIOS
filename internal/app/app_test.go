package app

import (
	"testing"

	"github.com/AdamNi-7080/AIOS/internal/model"
)

func TestWithinRejectsRepositoryDescendants(t *testing.T) {
	if !within("/repo", "/repo/data") || within("/repo", "/other/data") {
		t.Fatal("repository containment check is unsafe")
	}
}

func TestAffectedPathsDropsDeletedSourceAndDependants(t *testing.T) {
	changes := []model.FileChange{{Kind: "removed", Path: "src/Deleted.java"}}
	symbols := []model.Symbol{{Path: "src/Deleted.java", Identity: "java:Deleted"}}
	edges := []model.Edge{{Path: "src/Caller.java", TargetIdentity: "java:Deleted"}}
	affected := affectedPaths(changes, symbols, edges)
	if !affected["src/Deleted.java"] || !affected["src/Caller.java"] {
		t.Fatalf("affected=%v", affected)
	}
}

func TestAffectedPathsInvalidatesBothSidesOfRename(t *testing.T) {
	affected := affectedPaths([]model.FileChange{{Kind: "renamed", OldPath: "src/Old.ts", Path: "src/New.ts"}}, []model.Symbol{{Path: "src/Old.ts", Identity: "typescript:Old"}}, []model.Edge{{Path: "src/Consumer.ts", TargetIdentity: "typescript:Old"}})
	for _, path := range []string{"src/Old.ts", "src/New.ts", "src/Consumer.ts"} {
		if !affected[path] {
			t.Fatalf("%s missing from affected=%v", path, affected)
		}
	}
}
