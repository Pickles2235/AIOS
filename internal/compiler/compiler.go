// Package compiler runs optional language frontends without allowing a
// repository checkout to become an output directory.
package compiler

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/AdamNi-7080/AIOS/internal/model"
)

const version = "compiler-frontends-v1"

type Diagnostic struct {
	Language string `json:"language"`
	Code     string `json:"code"`
	Message  string `json:"message"`
	Path     string `json:"path,omitempty"`
}
type Result struct {
	Symbols     []model.Symbol
	Edges       []model.Edge
	Diagnostics []Diagnostic
	Covered     map[string]bool
}
type record struct {
	Type, Path, Name, Kind, Identity, Source, Target, SourceIdentity, TargetIdentity, Predicate, Message string
	StartByte, EndByte, StartLine, StartColumn, EndLine, EndColumn                                       int
}

func Run(ctx context.Context, repo model.Repository, files []model.File, runtime model.CompilerRuntime, dataDir string) (Result, error) {
	r := Result{Covered: map[string]bool{}}
	javaSource := javaFiles(files)
	if len(javaSource) > 0 {
		x, err := java(ctx, repo, javaSource, runtime, dataDir)
		if err != nil {
			r.Diagnostics = append(r.Diagnostics, Diagnostic{"java", "frontend_failure", bounded(err.Error()), ""})
		} else {
			merge(&r, x)
		}
	}
	ts := tsConfigs(files)
	if hasTS(files) {
		if len(ts) == 0 {
			r.Diagnostics = append(r.Diagnostics, Diagnostic{"typescript", "tsconfig_missing", "no tsconfig.json was discovered; Tree-sitter fallback retained", ""})
		} else {
			for _, c := range ts {
				x, err := typescript(ctx, repo, c, runtime, dataDir)
				if err != nil {
					r.Diagnostics = append(r.Diagnostics, Diagnostic{"typescript", "frontend_failure", bounded(err.Error()), c})
				} else {
					merge(&r, x)
				}
			}
		}
	}
	retainIndexedPaths(&r, files)
	return r, nil
}

func retainIndexedPaths(result *Result, files []model.File) {
	allowed := make(map[string]bool, len(files))
	for _, file := range files {
		allowed[file.Path] = true
	}
	skipped := map[string]bool{}
	symbols := result.Symbols[:0]
	for _, symbol := range result.Symbols {
		if allowed[symbol.Path] {
			symbols = append(symbols, symbol)
		} else {
			skipped[symbol.Path] = true
		}
	}
	result.Symbols = symbols
	edges := result.Edges[:0]
	for _, edge := range result.Edges {
		if allowed[edge.Path] {
			edges = append(edges, edge)
		} else {
			skipped[edge.Path] = true
		}
	}
	result.Edges = edges
	paths := make([]string, 0, len(skipped))
	for path := range skipped {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	for _, path := range paths {
		result.Diagnostics = append(result.Diagnostics, Diagnostic{Language: "typescript", Code: "source_not_indexed", Message: "compiler fact skipped because its source is outside the approved indexed file manifest", Path: path})
	}
}
func merge(to *Result, from Result) {
	to.Symbols = append(to.Symbols, from.Symbols...)
	to.Edges = append(to.Edges, from.Edges...)
	to.Diagnostics = append(to.Diagnostics, from.Diagnostics...)
	for k, v := range from.Covered {
		to.Covered[k] = to.Covered[k] || v
	}
}
func bounded(s string) string {
	if len(s) > 500 {
		return s[:500]
	}
	return s
}
func javaFiles(files []model.File) []model.File {
	var out []model.File
	for _, f := range files {
		if f.Language == "java" {
			out = append(out, f)
		}
	}
	return out
}
func hasTS(files []model.File) bool {
	for _, f := range files {
		if f.Language == "typescript" || f.Language == "tsx" {
			return true
		}
	}
	return false
}
func tsConfigs(files []model.File) []string {
	var out []string
	for _, f := range files {
		if filepath.Base(f.Path) == "tsconfig.json" {
			out = append(out, f.Path)
		}
	}
	sort.Strings(out)
	return out
}
func cache(dataDir, language string) (string, error) {
	d := filepath.Join(dataDir, "compiler-cache", version, language)
	return d, os.MkdirAll(d, 0700)
}
func writeHelper(dir, name, body string) (string, error) {
	p := filepath.Join(dir, name)
	want := sha256.Sum256([]byte(body))
	if b, e := os.ReadFile(p); e == nil && sha256.Sum256(b) == want {
		return p, nil
	}
	if e := os.WriteFile(p, []byte(body), 0600); e != nil {
		return "", e
	}
	return p, nil
}
func run(ctx context.Context, cmd *exec.Cmd) ([]record, error) {
	out, e := cmd.StdoutPipe()
	if e != nil {
		return nil, e
	}
	var stderr strings.Builder
	cmd.Stderr = &stderr
	if e = cmd.Start(); e != nil {
		return nil, e
	}
	var rs []record
	scanner := bufio.NewScanner(out)
	scanner.Buffer(make([]byte, 4096), 1<<20)
	for scanner.Scan() {
		var r record
		if e := json.Unmarshal(scanner.Bytes(), &r); e != nil {
			return nil, e
		}
		rs = append(rs, r)
	}
	if e = scanner.Err(); e != nil {
		return nil, e
	}
	if e = cmd.Wait(); e != nil {
		return nil, fmt.Errorf("%w: %s", e, bounded(stderr.String()))
	}
	return rs, nil
}
func toResult(language string, records []record) Result {
	r := Result{Covered: map[string]bool{language: true}}
	for _, x := range records {
		sp := model.Span{StartByte: x.StartByte, EndByte: x.EndByte, StartLine: x.StartLine, StartColumn: x.StartColumn, EndLine: x.EndLine, EndColumn: x.EndColumn}
		switch x.Type {
		case "symbol":
			r.Symbols = append(r.Symbols, model.Symbol{Path: x.Path, Name: x.Name, Kind: x.Kind, Identity: x.Identity, Span: sp, Extractor: language + "-compiler-v1", Confidence: 1})
		case "edge":
			r.Edges = append(r.Edges, model.Edge{Path: x.Path, Source: x.Source, Target: x.Target, SourceIdentity: x.SourceIdentity, TargetIdentity: x.TargetIdentity, Kind: x.Predicate, Span: sp, Resolver: language + "-compiler-v1", Derivation: "compiler_resolved", Confidence: 1})
		case "diagnostic":
			r.Diagnostics = append(r.Diagnostics, Diagnostic{language, "compiler_diagnostic", bounded(x.Message), x.Path})
		}
	}
	return r
}
func absolute(root, path string) string { return filepath.Join(root, filepath.FromSlash(path)) }
func identity(language, kind, value string) string {
	h := sha256.Sum256([]byte(language + "\x00" + kind + "\x00" + value))
	return language + ":" + kind + ":" + hex.EncodeToString(h[:16])
}
