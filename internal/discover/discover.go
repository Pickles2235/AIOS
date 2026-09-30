package discover

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/AdamNi-7080/AIOS/internal/model"
)

var blockedSegments = map[string]bool{
	".git": true, ".gradle": true, "node_modules": true, "vendor": true,
	"build": true, "dist": true, "target": true, "dependency": true,
	"dependencies": true, "env": true,
}

func Files(repo model.Repository, limits model.Limits) ([]model.File, error) {
	files, _, err := FilesWithReport(repo, limits)
	return files, err
}

// FilesWithReport returns bounded reason counts for excluded catalogue entries.
// Counts describe encountered filesystem entries; an excluded directory is one
// entry and its unread descendants are intentionally not traversed.
func FilesWithReport(repo model.Repository, limits model.Limits) ([]model.File, map[string]int, error) {
	files, coverage, err := FilesWithCoverage(repo, limits)
	counts := map[string]int{}
	for _, entry := range coverage.Entries {
		if entry.Outcome != "included" {
			counts[entry.Reason]++
		}
	}
	return files, counts, err
}

// FilesWithCoverage records per-path outcomes without retaining excluded file
// contents. It is the discovery input to canonical coverage accounting.
func FilesWithCoverage(repo model.Repository, limits model.Limits) ([]model.File, model.CoverageReport, error) {
	var files []model.File
	var total int64
	report := model.CoverageReport{}
	add := func(path, language, classification, outcome, reason, capability string) {
		report.Entries = append(report.Entries, model.CoverageEntry{Path: path, Language: language, Classification: classification, Outcome: outcome, Reason: reason, Capability: capability})
	}
	err := filepath.WalkDir(repo.Root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			if rel, e := filepath.Rel(repo.Root, path); e == nil && rel != "." {
				add(filepath.ToSlash(rel), "", "", "unreadable", "walk_error", "")
			}
			return nil
		}
		if path == repo.Root {
			return nil
		}
		rel, err := filepath.Rel(repo.Root, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		info, err := os.Lstat(path)
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			add(rel, "", "", "excluded", "symlink", "")
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if alwaysBlocked(rel) {
			reason := "built_in_policy"
			lower := strings.ToLower(rel)
			if lower == "vendor" || lower == "node_modules" || strings.Contains(lower, "vendor/") || strings.Contains(lower, "node_modules/") {
				reason = "vendor"
			}
			if strings.Contains(lower, "build/") || strings.Contains(lower, "dist/") || strings.Contains(lower, "target/") {
				reason = "generated"
			}
			add(rel, "", "", "excluded", reason, "")
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if matchesAny(repo.Exclude, rel) {
			add(rel, "", "", "excluded", "catalog_pattern", "")
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if entry.IsDir() {
			return nil
		}
		if len(repo.Include) > 0 && !matchesAny(repo.Include, rel) {
			add(rel, "", "", "excluded", "not_included", "")
			return nil
		}
		if info.Size() > limits.MaxFileBytes {
			add(rel, language(rel), classification(rel), "excluded", "file_too_large", "")
			return nil
		}
		content, err := readRegularFile(repo.Root, rel, limits.MaxFileBytes)
		if err != nil {
			if errors.Is(err, errNotRegular) || errors.Is(err, errSymlink) || errors.Is(err, errTooLarge) {
				add(rel, "", "", "unreadable", "unsafe_or_non_regular", "")
				return nil
			}
			add(rel, "", "", "unreadable", "read_error", "")
			return nil
		}
		if int64(len(content)) > limits.MaxFileBytes {
			add(rel, language(rel), classification(rel), "excluded", "file_too_large", "")
			return nil
		}
		if bytes.IndexByte(content, 0) >= 0 || !utf8.Valid(content) {
			add(rel, "", "", "unsupported", "binary_or_invalid_utf8", "")
			return nil
		}
		if len(files)+1 > limits.MaxFilesPerRepo {
			return fmt.Errorf("repository %q exceeds max_files_per_repo", repo.ID)
		}
		total += int64(len(content))
		if total > limits.MaxTotalBytesPerRepo {
			return fmt.Errorf("repository %q exceeds max_total_bytes_per_repo", repo.ID)
		}
		sum := sha256.Sum256(content)
		files = append(files, model.File{
			RepoID: repo.ID, Path: rel, SHA256: hex.EncodeToString(sum[:]),
			Size: int64(len(content)), Language: language(rel),
			Classification: classification(rel), Content: string(content),
		})
		capability := "lexical"
		if l := language(rel); l == "java" || l == "typescript" || l == "tsx" || l == "javascript" || l == "jsx" || l == "kotlin" {
			capability = "lexical,structural"
		}
		add(rel, language(rel), classification(rel), "included", "", capability)
		return nil
	})
	if err != nil {
		return nil, report, err
	}
	sort.Slice(files, func(i, j int) bool { return files[i].Path < files[j].Path })
	sort.Slice(report.Entries, func(i, j int) bool { return report.Entries[i].Path < report.Entries[j].Path })
	return files, report, nil
}

func alwaysBlocked(rel string) bool {
	parts := strings.Split(strings.ToLower(rel), "/")
	for _, part := range parts {
		if blockedSegments[part] || part == ".env" || strings.HasPrefix(part, ".env.") ||
			strings.Contains(part, "secret") || strings.Contains(part, "credential") ||
			strings.Contains(part, "keystore") || strings.Contains(part, "private-key") ||
			strings.Contains(part, "private_key") {
			return true
		}
	}
	return false
}

func matchesAny(patterns []string, rel string) bool {
	for _, pattern := range patterns {
		if glob(pattern, rel) {
			return true
		}
	}
	return false
}

func glob(pattern, rel string) bool {
	pattern = filepath.ToSlash(pattern)
	patternParts, relParts := strings.Split(pattern, "/"), strings.Split(rel, "/")
	var match func(int, int) bool
	match = func(patternIndex, relIndex int) bool {
		if patternIndex == len(patternParts) {
			return relIndex == len(relParts)
		}
		if patternParts[patternIndex] == "**" {
			return match(patternIndex+1, relIndex) || relIndex < len(relParts) && match(patternIndex, relIndex+1)
		}
		if relIndex == len(relParts) {
			return false
		}
		matched, err := filepath.Match(patternParts[patternIndex], relParts[relIndex])
		return err == nil && matched && match(patternIndex+1, relIndex+1)
	}
	return match(0, 0)
}

func language(path string) string {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".ts":
		return "typescript"
	case ".tsx":
		return "tsx"
	case ".js":
		return "javascript"
	case ".jsx":
		return "jsx"
	case ".java":
		return "java"
	case ".kt", ".kts":
		return "kotlin"
	case ".py":
		return "python"
	case ".md", ".adoc", ".txt", ".rst":
		return "documentation"
	case ".json", ".yaml", ".yml", ".toml", ".properties", ".gradle":
		return "configuration"
	default:
		return "text"
	}
}

func classification(path string) string {
	ext := strings.ToLower(filepath.Ext(path))
	if ext == ".md" || ext == ".adoc" || ext == ".txt" || ext == ".rst" {
		return "documentation"
	}
	return "source"
}
