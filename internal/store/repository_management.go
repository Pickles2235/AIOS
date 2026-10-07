package store

import (
	"context"
	"fmt"
	"regexp"
	"time"

	"github.com/AdamNi-7080/AIOS/internal/observability"
)

var purgeID = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{0,62}$`)

// RepositoryOwnedRecords checks canonical data and retired/staged history rather
// than inferring purge completion from the active catalog alone.
func (s *Store) RepositoryOwnedRecords(ctx context.Context, repository string) (int, error) {
	if !purgeID.MatchString(repository) {
		return 0, fmt.Errorf("invalid repository ID")
	}
	queries := []string{
		`SELECT count(*) FROM ir_repositories WHERE repository_id=?`,
		`SELECT count(*) FROM ir_source_revisions WHERE repository_id=?`,
		`SELECT count(*) FROM ir_entities WHERE repository_id=?`,
		`SELECT count(*) FROM generations WHERE repo_id=?`,
		`SELECT count(*) FROM generation_staging WHERE repo_id=?`,
		`SELECT count(*) FROM ingestion_events WHERE repository_id=?`,
		`SELECT count(*) FROM ingestion_queues WHERE repository_id=?`,
		`SELECT count(*) FROM ir_delta_records WHERE repository_id=?`,
		`SELECT count(*) FROM approved_ownership WHERE repository_id=?`,
		`SELECT count(*) FROM catalog_revision_members WHERE repo_id=?`,
		`SELECT count(*) FROM source_files WHERE repo_id=?`,
		`SELECT count(*) FROM evidence WHERE repo_id=?`,
		`SELECT count(*) FROM entities WHERE repo_id=?`,
		`SELECT count(*) FROM source_fts WHERE repo_id=?`,
		`SELECT count(*) FROM search_fts WHERE repo_id=?`,
	}
	total := 0
	for _, q := range queries {
		var n int
		if err := s.db.QueryRowContext(ctx, q, repository).Scan(&n); err != nil {
			return 0, err
		}
		total += n
	}
	var diagnostics int
	if err := s.db.QueryRowContext(ctx, `SELECT count(*) FROM diagnostic_events WHERE repository_id IN (?,?)`, repository, observability.Opaque(repository)).Scan(&diagnostics); err != nil {
		return 0, err
	}
	total += diagnostics
	return total, nil
}

// PurgeRepository withdraws one repository and all canonical history in the
// same transaction as the surviving catalog/projections. It has no source path.
// Callers must stop ingestion before invoking this exclusive-writer operation.
func (s *Store) PurgeRepository(ctx context.Context, repository string) error {
	if s.readOnly || !purgeID.MatchString(repository) {
		return fmt.Errorf("invalid repository purge")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	exec := func(q string, args ...any) error { _, e := tx.ExecContext(ctx, q, args...); return e }
	if err = exec(`CREATE TEMP TABLE removal_generations(id TEXT PRIMARY KEY); CREATE TEMP TABLE removal_entities(id TEXT PRIMARY KEY); CREATE TEMP TABLE removal_facts(id TEXT PRIMARY KEY); CREATE TEMP TABLE removal_catalogs(id TEXT PRIMARY KEY)`); err != nil {
		return err
	}
	for _, q := range []string{
		`INSERT INTO removal_generations SELECT generation_id FROM generations WHERE repo_id=?`,
		`INSERT INTO removal_entities SELECT canonical_entity_id FROM ir_entities WHERE repository_id=?`,
		`INSERT INTO removal_catalogs SELECT DISTINCT catalog_revision_id FROM catalog_revision_members WHERE repo_id=?`,
	} {
		if err = exec(q, repository); err != nil {
			return err
		}
	}
	if err = exec(`INSERT INTO removal_facts SELECT fact_id FROM ir_facts WHERE subject_canonical_entity_id IN (SELECT id FROM removal_entities) OR object_canonical_entity_id IN (SELECT id FROM removal_entities)`); err != nil {
		return err
	}
	revision, err := newCatalogRevisionID()
	if err != nil {
		return err
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if err = exec(`INSERT INTO catalog_revisions VALUES(?,?,?)`, revision, now, now); err != nil {
		return err
	}
	if err = exec(`INSERT INTO catalog_revision_members SELECT ?,repo_id,generation_id FROM active_generations WHERE repo_id<>?`, revision, repository); err != nil {
		return err
	}
	if err = exec(`DELETE FROM active_projection_builds`); err != nil {
		return err
	}
	if err = linkCatalog(ctx, tx, revision); err != nil {
		return err
	}
	if err = buildProjections(ctx, tx, revision, "repository_purge"); err != nil {
		return err
	}
	if err = exec(`INSERT INTO active_catalog_revision VALUES(1,?) ON CONFLICT(singleton) DO UPDATE SET catalog_revision_id=excluded.catalog_revision_id`, revision); err != nil {
		return err
	}
	for _, q := range []string{
		`DELETE FROM projection_builds WHERE catalog_revision_id IN (SELECT id FROM removal_catalogs)`,
		`DELETE FROM catalog_revisions WHERE catalog_revision_id IN (SELECT id FROM removal_catalogs)`,
		`DELETE FROM negative_evidence`,
		`DELETE FROM ir_fact_lifecycle WHERE generation_id IN (SELECT id FROM removal_generations) OR fact_id IN (SELECT id FROM removal_facts)`,
		`DELETE FROM ir_fact_observations WHERE revision_id IN (SELECT id FROM removal_generations) OR fact_id IN (SELECT id FROM removal_facts)`,
		`DELETE FROM ir_facts WHERE fact_id IN (SELECT id FROM removal_facts)`,
		`DELETE FROM ir_entity_aliases WHERE canonical_entity_id IN (SELECT id FROM removal_entities) OR location_id IN (SELECT location_id FROM ir_locations WHERE revision_id IN (SELECT id FROM removal_generations))`,
		`DELETE FROM claims WHERE generation_id IN (SELECT id FROM removal_generations)`,
		`DELETE FROM entities WHERE generation_id IN (SELECT id FROM removal_generations)`,
		`DELETE FROM ir_locations WHERE revision_id IN (SELECT id FROM removal_generations)`,
		`DELETE FROM ir_source_revisions WHERE revision_id IN (SELECT id FROM removal_generations)`,
		`DELETE FROM ir_entities WHERE canonical_entity_id IN (SELECT id FROM removal_entities)`,
		`DELETE FROM source_fts WHERE generation_id IN (SELECT id FROM removal_generations)`,
		`DELETE FROM search_fts WHERE generation_id IN (SELECT id FROM removal_generations)`,
		`DELETE FROM vector_embedding_cache WHERE embedding_id NOT IN (SELECT embedding_id FROM projection_vector_records)`,
	} {
		if err = exec(q); err != nil {
			return err
		}
	}
	for _, q := range []string{
		`DELETE FROM active_generations WHERE repo_id=?`,
		`DELETE FROM generation_staging WHERE repo_id=?`,
		`DELETE FROM ingestion_events WHERE repository_id=?`,
		`DELETE FROM ingestion_queues WHERE repository_id=?`,
		`DELETE FROM ir_delta_records WHERE repository_id=?`,
		`DELETE FROM approved_ownership WHERE repository_id=?`,
		`DELETE FROM generations WHERE repo_id=?`,
		`DELETE FROM ir_repositories WHERE repository_id=?`,
	} {
		if err = exec(q, repository); err != nil {
			return err
		}
	}
	if err = exec(`DELETE FROM diagnostic_events WHERE repository_id IN (?,?)`, repository, observability.Opaque(repository)); err != nil {
		return err
	}
	rows, err := tx.QueryContext(ctx, `PRAGMA foreign_key_check`)
	if err != nil {
		return err
	}
	bad := rows.Next()
	scanErr := rows.Err()
	rows.Close()
	if bad || scanErr != nil {
		return fmt.Errorf("purge would leave foreign generation references")
	}
	if err = exec(`DROP TABLE removal_generations; DROP TABLE removal_entities; DROP TABLE removal_facts; DROP TABLE removal_catalogs`); err != nil {
		return err
	}
	return tx.Commit()
}

// CompactPurgedData physically drops SQLite free pages after a durable purge.
// Failure does not resurrect removed records; the caller retains a cleanup job.
func (s *Store) CompactPurgedData(ctx context.Context) error {
	if s.readOnly {
		return fmt.Errorf("store is read-only")
	}
	if _, err := s.db.ExecContext(ctx, `VACUUM`); err != nil {
		return err
	}
	return checkDatabase(ctx, s.db)
}
