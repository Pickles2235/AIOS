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
// The (build, entity) primary key guarantees uniqueness. Exact coverage of every
// expected entity plus equal row counts therefore also excludes all extra rows,
// without a second evidence/entity join over the entire derived catalog.
func validateLookupCompleteness(ctx context.Context, reader projectionReader, build, revision string) error {
	var invalid bool
	err := reader.QueryRowContext(ctx, `SELECT
 EXISTS(SELECT 1 FROM entities e JOIN catalog_revision_members m ON m.generation_id=e.generation_id
 LEFT JOIN projection_lookup_records p ON p.projection_build_id=? AND p.generation_id=e.generation_id AND p.entity_id=e.entity_id AND p.evidence_id=e.evidence_id
 LEFT JOIN evidence v ON v.evidence_id=e.evidence_id AND v.generation_id=e.generation_id
 WHERE m.catalog_revision_id=? AND (p.entity_id IS NULL OR v.evidence_id IS NULL))
 OR (SELECT count(*) FROM projection_lookup_records WHERE projection_build_id=?)
 <> (SELECT count(*) FROM entities e JOIN catalog_revision_members m ON m.generation_id=e.generation_id WHERE m.catalog_revision_id=?)`, build, revision, build, revision).Scan(&invalid)
	if err != nil {
		return err
	}
	if invalid {
		return fmt.Errorf("lookup projection is incomplete or contains foreign, missing, or inactive IR references")
	}
	return nil
}
