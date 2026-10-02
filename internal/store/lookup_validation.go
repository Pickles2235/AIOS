package store

import (
	"context"
	"database/sql"
	"fmt"
)

type projectionReader interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
}

func projectionBuilderVersion(kind string) string {
	if kind == "lookup" {
		return "v2"
	}
	return "v1"
}

// A ready pointer and catalog fingerprint do not prove coverage. Every active
// canonical entity must have exactly its own evidence-backed lookup record, and
// every lookup record must belong to that catalog. Verify both directions using
// the same read transaction as the exact query; corrupt derived rows return an
// error for the knowledge layer to classify as unknown, never a false absence.
func validateLookupCompleteness(ctx context.Context, reader projectionReader, build, revision string) error {
	var invalid bool
	err := reader.QueryRowContext(ctx, `SELECT
 EXISTS(SELECT 1 FROM entities e JOIN catalog_revision_members m ON m.generation_id=e.generation_id
 LEFT JOIN projection_lookup_records p ON p.projection_build_id=? AND p.generation_id=e.generation_id AND p.entity_id=e.entity_id AND p.evidence_id=e.evidence_id
 WHERE m.catalog_revision_id=? AND p.entity_id IS NULL)
 OR EXISTS(SELECT 1 FROM projection_lookup_records p
 LEFT JOIN entities e ON e.entity_id=p.entity_id AND e.generation_id=p.generation_id AND e.evidence_id=p.evidence_id
 LEFT JOIN evidence v ON v.evidence_id=p.evidence_id AND v.generation_id=p.generation_id
 WHERE p.projection_build_id=? AND (e.entity_id IS NULL OR v.evidence_id IS NULL OR NOT EXISTS
 (SELECT 1 FROM catalog_revision_members m WHERE m.catalog_revision_id=? AND m.generation_id=p.generation_id)))`, build, revision, build, revision).Scan(&invalid)
	if err != nil {
		return err
	}
	if invalid {
		return fmt.Errorf("lookup projection is incomplete or contains foreign, missing, or inactive IR references")
	}
	return nil
}
