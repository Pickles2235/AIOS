package discover

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/AdamNi-7080/AIOS/internal/model"
)

func TestFilesExcludesUnsafeBinaryInvalidOversizeBuildAndSymlink(t *testing.T) {
	root := canonicalTempDir(t)
	write(t, root, "src/good.ts", "export function good() {}")
	write(t, root, "README.md", "useful docs")
	write(t, root, ".env", "TOKEN=do-not-index")
	write(t, root, "config/credentials.json", "secret")
	write(t, root, "node_modules/pkg/index.ts", "dependency")
	write(t, root, "build/output.ts", "build")
	writeBytes(t, root, "binary.bin", []byte{'a', 0, 'b'})
	writeBytes(t, root, "invalid.txt", []byte{0xff, 0xfe})
	write(t, root, "large.txt", strings.Repeat("x", 65))
	if err := os.Symlink(filepath.Join(root, "src", "good.ts"), filepath.Join(root, "linked.ts")); err != nil {
		t.Fatal(err)
	}
	files, err := Files(model.Repository{ID: "repo", Root: root}, model.Limits{MaxFileBytes: 64, MaxFilesPerRepo: 20, MaxTotalBytesPerRepo: 1024, MaxResults: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 2 || files[0].Path != "README.md" || files[1].Path != "src/good.ts" {
		t.Fatalf("unexpected indexed files: %#v", files)
	}
}

func TestFilesWithReportCountsExclusionReasons(t *testing.T) {
	root := canonicalTempDir(t)
	write(t, root, "src/A.java", "class A {}")
	write(t, root, "src/ignored.go", "package ignored")
	write(t, root, "node_modules/module.ts", "export const ignored = true")
	files, report, err := FilesWithReport(model.Repository{ID: "r", Root: root, Include: []string{"**/*.java"}}, model.Limits{MaxFileBytes: 1024, MaxFilesPerRepo: 10, MaxTotalBytesPerRepo: 1024})
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 1 || report["not_included"] != 1 || report["vendor"] != 1 {
		t.Fatalf("files=%#v report=%#v", files, report)
	}
}

func TestFilesWithCoverageRetainsIncludedAndExcludedPaths(t *testing.T) {
	root := canonicalTempDir(t)
	write(t, root, "src/A.java", "class A {}")
	write(t, root, "vendor/module.go", "package module")
	write(t, root, "large.txt", strings.Repeat("x", 65))
	_, coverage, err := FilesWithCoverage(model.Repository{ID: "r", Root: root}, model.Limits{MaxFileBytes: 64, MaxFilesPerRepo: 10, MaxTotalBytesPerRepo: 1024})
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]string{}
	for _, entry := range coverage.Entries {
		got[entry.Path] = entry.Outcome + ":" + entry.Reason
	}
	if got["src/A.java"] != "included:" || got["vendor"] != "excluded:vendor" || got["large.txt"] != "excluded:file_too_large" {
		t.Fatalf("coverage=%#v", coverage)
	}
}

func TestEstateLanguagesHaveTruthfulCoverage(t *testing.T) {
	root := canonicalTempDir(t)
	for path, content := range map[string]string{
		"App.js": "export function App() {}", "View.jsx": "export const View = () => <div />",
		"Service.kt": "class Service", "Build.kts": "plugins {}", "tool.py": "def tool(): pass",
	} {
		write(t, root, path, content)
	}
	files, coverage, err := FilesWithCoverage(model.Repository{ID: "r", Root: root}, model.Limits{MaxFileBytes: 1024, MaxFilesPerRepo: 10, MaxTotalBytesPerRepo: 4096})
	if err != nil {
		t.Fatal(err)
	}
	languages, capabilities := map[string]string{}, map[string]string{}
	for _, file := range files {
		languages[file.Path] = file.Language
	}
	for _, entry := range coverage.Entries {
		capabilities[entry.Path] = entry.Capability
	}
	for path, language := range map[string]string{"App.js": "javascript", "View.jsx": "jsx", "Service.kt": "kotlin", "Build.kts": "kotlin", "tool.py": "python"} {
		if languages[path] != language {
			t.Fatalf("%s language=%q", path, languages[path])
		}
	}
	for _, path := range []string{"App.js", "View.jsx", "Service.kt", "Build.kts"} {
		if capabilities[path] != "lexical,structural" {
			t.Fatalf("%s capability=%q", path, capabilities[path])
		}
	}
	if capabilities["tool.py"] != "lexical" {
		t.Fatalf("python structural support was overstated: %q", capabilities["tool.py"])
	}
}

func TestIncludeAndExcludePatterns(t *testing.T) {
	root := canonicalTempDir(t)
	write(t, root, "root.ts", "const root = true")
	write(t, root, "src/keep.ts", "const keep = true")
	write(t, root, "src/skip.ts", "const skip = true")
	write(t, root, "src/readme.md", "ignored by include")
	write(t, root, "src/generated/nested/output.ts", "generated")
	repo := model.Repository{ID: "repo", Root: root, Include: []string{"**/*.ts"}, Exclude: []string{"src/skip.ts", "**/generated/**"}}
	files, err := Files(repo, model.Limits{MaxFileBytes: 1024, MaxFilesPerRepo: 20, MaxTotalBytesPerRepo: 4096, MaxResults: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 2 || files[0].Path != "root.ts" || files[1].Path != "src/keep.ts" {
		t.Fatalf("unexpected files: %#v", files)
	}
}

func TestReadRegularFileRejectsFinalAndParentSymlinks(t *testing.T) {
	root := canonicalTempDir(t)
	outside := canonicalTempDir(t)
	write(t, outside, "secret.txt", "must not escape")
	write(t, root, "swapped.txt", "safe")
	if _, err := os.Lstat(filepath.Join(root, "swapped.txt")); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(filepath.Join(root, "swapped.txt"), filepath.Join(root, "original.txt")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(outside, "secret.txt"), filepath.Join(root, "swapped.txt")); err != nil {
		t.Fatal(err)
	}
	write(t, root, "swapped-dir/known.txt", "safe")
	if _, err := os.Lstat(filepath.Join(root, "swapped-dir", "known.txt")); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(filepath.Join(root, "swapped-dir"), filepath.Join(root, "original-dir")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "swapped-dir")); err != nil {
		t.Fatal(err)
	}
	for _, rel := range []string{"swapped.txt", "swapped-dir/secret.txt"} {
		if content, err := readRegularFile(root, rel, 1024); err == nil {
			t.Fatalf("read through symlink %q: %q", rel, content)
		}
	}
}

func TestReadRegularFileRejectsUnsafeRelativePaths(t *testing.T) {
	root := canonicalTempDir(t)
	for _, rel := range []string{"", ".", "../escape", "/absolute"} {
		if _, err := readRegularFile(root, rel, 1024); err == nil {
			t.Fatalf("accepted unsafe path %q", rel)
		}
	}
}

func write(t *testing.T, root, rel, content string) { writeBytes(t, root, rel, []byte(content)) }

func writeBytes(t *testing.T, root, rel string, content []byte) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatal(err)
	}
}

func canonicalTempDir(t *testing.T) string {
	t.Helper()
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return dir
}
