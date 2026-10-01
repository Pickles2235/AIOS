package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

var errUnsupportedCanonicalFormat = errors.New("unsupported canonical format")

// The writer lease covers backup and the transaction. Readers never migrate.
// v10 permits multiple capture epochs with the same bytes, preserving each
// generation's provenance, coverage and immutable observation history.
func upgradeMaintainedFormat(ctx context.Context, db *sql.DB, path string) error {
	var tables int
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM sqlite_master WHERE type='table' AND name='schema_metadata'`).Scan(&tables); err != nil {
		return err
	}
	if tables != 1 {
		return errUnsupportedCanonicalFormat
	}
	var got string
	if err := db.QueryRowContext(ctx, `SELECT value FROM schema_metadata WHERE key='format'`).Scan(&got); err != nil {
		return err
	}
	if got == format {
		return verifyFormat(ctx, db)
	}
	if got != "knowledge-ir-v9" {
		return errUnsupportedCanonicalFormat
	}
	if err := checkDatabase(ctx, db); err != nil {
		return err
	}
	if err := backupMaintainedFormat(ctx, db, path); err != nil {
		return err
	}
	if _, err := db.ExecContext(ctx, `PRAGMA foreign_keys=OFF`); err != nil {
		return err
	}
	defer db.Exec(`PRAGMA foreign_keys=ON`)
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	_, err = tx.ExecContext(ctx, `CREATE TABLE ir_source_revisions_next (revision_id TEXT PRIMARY KEY, repository_id TEXT NOT NULL REFERENCES ir_repositories(repository_id), source_kind TEXT NOT NULL, source_adapter_version TEXT NOT NULL, git_commit TEXT NOT NULL, content_hash TEXT NOT NULL, extractor_versions TEXT NOT NULL, indexed_at TEXT NOT NULL);
INSERT INTO ir_source_revisions_next SELECT * FROM ir_source_revisions;
DROP TABLE ir_source_revisions;
ALTER TABLE ir_source_revisions_next RENAME TO ir_source_revisions;
UPDATE schema_metadata SET value='knowledge-ir-v10' WHERE key='format';`)
	if err != nil {
		return fmt.Errorf("canonical format transaction failed: %w", err)
	}
	rows, err := tx.QueryContext(ctx, `PRAGMA foreign_key_check`)
	if err != nil {
		return err
	}
	violations := rows.Next()
	scanErr := rows.Err()
	rows.Close()
	if violations || scanErr != nil {
		return fmt.Errorf("canonical format failed foreign-key validation")
	}
	var integrity string
	if err = tx.QueryRowContext(ctx, `PRAGMA quick_check`).Scan(&integrity); err != nil || integrity != "ok" {
		return fmt.Errorf("canonical format failed integrity validation")
	}
	return tx.Commit()
}

func checkDatabase(ctx context.Context, db *sql.DB) error {
	var integrity string
	if err := db.QueryRowContext(ctx, `PRAGMA quick_check`).Scan(&integrity); err != nil || integrity != "ok" {
		return fmt.Errorf("canonical database integrity failed")
	}
	return nil
}

func backupMaintainedFormat(ctx context.Context, db *sql.DB, path string) error {
	f, err := os.CreateTemp(filepath.Dir(path), ".ir-9-backup-*")
	if err != nil {
		return err
	}
	temporary := f.Name()
	if err = f.Close(); err != nil {
		return err
	}
	defer os.Remove(temporary)
	if err = os.Remove(temporary); err != nil {
		return err
	}
	if _, err = db.ExecContext(ctx, `VACUUM INTO '`+strings.ReplaceAll(temporary, "'", "''")+`'`); err != nil {
		return fmt.Errorf("canonical backup failed: %w", err)
	}
	f, err = os.OpenFile(temporary, os.O_RDWR, 0)
	if err != nil {
		return err
	}
	if err = f.Chmod(0600); err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	copy, err := sql.Open("sqlite", "file:"+url.PathEscape(temporary)+"?mode=ro")
	if err != nil {
		return err
	}
	err = checkDatabase(ctx, copy)
	var backupFormat string
	if err == nil {
		err = copy.QueryRowContext(ctx, `SELECT value FROM schema_metadata WHERE key='format'`).Scan(&backupFormat)
	}
	copy.Close()
	if err != nil || backupFormat != "knowledge-ir-v9" {
		return fmt.Errorf("canonical backup verification failed")
	}
	// Exclusive publication preserves any older backup. Names carry no sources.
	backup := filepath.Join(filepath.Dir(path), "index.ir-9-backup-"+strings.TrimPrefix(filepath.Base(temporary), ".ir-9-backup-")+".db")
	if err = os.Link(temporary, backup); err != nil {
		return err
	}
	dir, err := os.Open(filepath.Dir(path))
	if err != nil {
		return err
	}
	defer dir.Close()
	return dir.Sync()
}
