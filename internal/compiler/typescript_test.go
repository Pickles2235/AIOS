package compiler

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/AdamNi-7080/AIOS/internal/model"
)

func TestTypeScriptCompilerFindsAliasesTSXAndCalls(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node unavailable")
	}
	module := filepath.Join("..", "..", "web", "node_modules", "typescript", "lib", "typescript.js")
	module, err = filepath.Abs(module)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = os.Stat(module); err != nil {
		t.Skip("repository TypeScript runtime unavailable")
	}
	root := t.TempDir()
	writeTS(t, root, "client/tsconfig.json", `{"compilerOptions":{"baseUrl":".","paths":{"@/*":["src/*"]},"jsx":"react-jsx"},"include":["src"]}`)
	writeTS(t, root, "client/src/util.ts", `export function target() { return 1 }`)
	writeTS(t, root, "client/src/App.tsx", `import { target as localTarget } from "@/util"; export const App = () => <div>{localTarget()}</div>;`)
	r, err := typescript(context.Background(), model.Repository{ID: "repo", Root: root}, "client/tsconfig.json", model.CompilerRuntime{Node: node, TypeScriptModule: module}, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	foundSymbol, foundCall, foundAlias := false, false, false
	for _, s := range r.Symbols {
		if s.Name == "App" && s.Identity != "" && s.Path == "client/src/App.tsx" {
			foundSymbol = true
		}
	}
	for _, e := range r.Edges {
		if e.Kind == "CALLS" && e.TargetIdentity != "" {
			foundCall = true
		}
		if e.Kind == "ALIASES" && e.TargetIdentity != "" {
			foundAlias = true
		}
	}
	if !foundSymbol || !foundCall || !foundAlias {
		t.Fatalf("symbols=%#v edges=%#v diagnostics=%#v", r.Symbols, r.Edges, r.Diagnostics)
	}
}

func TestRunRetainsFallbackWhenTypeScriptConfigurationIsMissing(t *testing.T) {
	root := t.TempDir()
	file := model.File{RepoID: "repo", Path: "src/broken.ts", Language: "typescript", Classification: "source", Content: "export const ="}
	r, err := Run(context.Background(), model.Repository{ID: "repo", Root: root}, []model.File{file}, model.CompilerRuntime{}, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Diagnostics) != 1 || r.Diagnostics[0].Code != "tsconfig_missing" || len(r.Symbols) != 0 {
		t.Fatalf("result=%#v", r)
	}
}

func TestRetainIndexedPathsRejectsCompilerFactsOutsideManifest(t *testing.T) {
	result := Result{
		Symbols: []model.Symbol{{Path: "src/kept.ts"}, {Path: "generated/out.js"}},
		Edges:   []model.Edge{{Path: "src/kept.ts"}, {Path: "../shared.ts"}},
	}
	retainIndexedPaths(&result, []model.File{{Path: "src/kept.ts"}})
	if len(result.Symbols) != 1 || len(result.Edges) != 1 || len(result.Diagnostics) != 2 {
		t.Fatalf("result=%#v", result)
	}
	for _, diagnostic := range result.Diagnostics {
		if diagnostic.Code != "source_not_indexed" {
			t.Fatalf("diagnostic=%#v", diagnostic)
		}
	}
}
func writeTS(t *testing.T, root, rel, content string) {
	t.Helper()
	p := filepath.Join(root, rel)
	if err := os.MkdirAll(filepath.Dir(p), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
}
