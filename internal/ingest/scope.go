// Package ingest contains deterministic, source-evidence-only ingestion rules.
package ingest

import (
	"path/filepath"
	"strings"

	"github.com/AdamNi-7080/AIOS/internal/model"
)

type RebuildScope struct {
	Kind    string
	Reasons []string
}

// DecideRebuildScope makes full rebuild fallbacks explicit and narrow. The
// caller records the decision in the immutable delta before activation.
func DecideRebuildScope(bootstrap, extractorChanged bool, changes []model.FileChange) RebuildScope {
	if bootstrap {
		return RebuildScope{Kind: "full_repository", Reasons: []string{"bootstrap"}}
	}
	if extractorChanged {
		return RebuildScope{Kind: "full_repository", Reasons: []string{"extractor_wide_invalidation"}}
	}
	for _, c := range changes {
		base := strings.ToLower(filepath.Base(c.Path))
		if base == "tsconfig.json" || base == "package.json" || base == "pom.xml" || strings.HasSuffix(base, ".gradle") {
			return RebuildScope{Kind: "full_repository", Reasons: []string{"framework_or_contract_configuration"}}
		}
	}
	return RebuildScope{Kind: "affected_files_and_cross_identities", Reasons: []string{"git_tree_delta"}}
}
