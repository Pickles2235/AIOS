package compiler

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/AdamNi-7080/AIOS/internal/model"
)

func TestJavaCompilerFindsDefinitionsCallsAndImplementations(t *testing.T) {
	root := t.TempDir()
	files := []model.File{
		javaFixture(t, root, "src/main/java/sample/Service.java", `package sample; interface Service { void run(); }`),
		javaFixture(t, root, "src/main/java/sample/Impl.java", `package sample; class Impl implements Service { public void run() {} }`),
		javaFixture(t, root, "src/main/java/sample/Caller.java", `package sample; class Caller { void call() { new Impl().run(); } }`),
	}
	r, err := java(context.Background(), model.Repository{ID: "repo", Root: root}, files, model.CompilerRuntime{}, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if !r.Covered["java"] {
		t.Fatalf("coverage=%#v", r.Covered)
	}
	need := map[string]bool{"CALLS": false, "IMPLEMENTS": false}
	for _, edge := range r.Edges {
		if _, ok := need[edge.Kind]; ok && edge.Derivation == "compiler_resolved" && edge.TargetIdentity != "" {
			need[edge.Kind] = true
		}
	}
	for k, ok := range need {
		if !ok {
			t.Fatalf("missing %s in %#v", k, r.Edges)
		}
	}
}
func javaFixture(t *testing.T, root, rel, content string) model.File {
	t.Helper()
	p := filepath.Join(root, rel)
	if err := os.MkdirAll(filepath.Dir(p), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
	return model.File{RepoID: "repo", Path: rel, Language: "java", Classification: "source", Content: content, Size: int64(len(content))}
}
