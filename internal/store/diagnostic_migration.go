package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/AdamNi-7080/AIOS/internal/model"
)

// scrubOperationalLedger upgrades only the operational ledger. Canonical
// compiler diagnostics, coverage and source evidence are never removed.
// A pending marker is deliberately retried after a busy WAL checkpoint.
func scrubOperationalLedger(ctx context.Context, db *sql.DB) error {
	var policy string
	err := db.QueryRowContext(ctx, `SELECT value FROM schema_metadata WHERE key='diagnostic_policy'`).Scan(&policy)
	if err != nil && err != sql.ErrNoRows {
		return err
	}
	if policy == "redacted_v1" {
		return nil
	}
	if _, err = db.ExecContext(ctx, `PRAGMA secure_delete=ON`); err != nil {
		return err
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, `INSERT INTO schema_metadata(key,value) VALUES('diagnostic_policy','pending') ON CONFLICT(key) DO UPDATE SET value='pending'`); err != nil {
		return err
	}
	rows, err := tx.QueryContext(ctx, `SELECT diagnostic_id,code,severity,timestamp,repository_id,revision_id,generation_id,path,projection_kind,query_fingerprint,handle,remediation,metadata_json,resolved_at,resolution FROM diagnostic_events ORDER BY timestamp DESC,diagnostic_id DESC LIMIT 1000`)
	if err != nil {
		return err
	}
	var events []model.DiagnosticEvent
	for rows.Next() {
		var event model.DiagnosticEvent
		var severity, metadata string
		if err = rows.Scan(&event.ID, &event.Code, &severity, &event.Timestamp, &event.Scope.Repository, &event.Scope.Revision, &event.Scope.Generation, &event.Scope.Path, &event.Scope.Projection, &event.Scope.Query, &event.Scope.Handle, &event.Remediation, &metadata, &event.ResolvedAt, &event.Resolution); err != nil {
			rows.Close()
			return err
		}
		event.Severity = model.DiagnosticSeverity(severity)
		_ = json.Unmarshal([]byte(metadata), &event.Metadata)
		clean, cleanErr := safeDiagnostic(event)
		if cleanErr != nil {
			event.Metadata = nil
			clean, cleanErr = safeDiagnostic(event)
		}
		if cleanErr != nil {
			rows.Close()
			return cleanErr
		}
		events = append(events, clean)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `DELETE FROM diagnostic_events`); err != nil {
		return err
	}
	for _, event := range events {
		if err = recordSanitizedDiagnosticTx(ctx, tx, event); err != nil {
			return err
		}
	}
	// Failure strings are operational status, not canonical observations.
	if _, err = tx.ExecContext(ctx, `UPDATE ingestion_events SET failure_diagnostic='operation_failed' WHERE failure_diagnostic<>'' AND failure_diagnostic NOT IN ('mirror_revision_failed','local_revision_failed','maintained_revision_failed')`); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `UPDATE ingestion_queues SET failure_diagnostic='operation_failed' WHERE failure_diagnostic<>'' AND failure_diagnostic NOT IN ('mirror_revision_failed','local_revision_failed','maintained_revision_failed')`); err != nil {
		return err
	}
	if err = tx.Commit(); err != nil {
		return err
	}
	// VACUUM rewrites free pages; checkpoint must report busy=0 before claiming
	// the policy current. No canonical table is included in the cleanup.
	if err = checkpointDiagnosticWAL(ctx, db); err != nil {
		return err
	}
	if _, err = db.ExecContext(ctx, `VACUUM`); err != nil {
		return err
	}
	if err = checkpointDiagnosticWAL(ctx, db); err != nil {
		return err
	}
	_, err = db.ExecContext(ctx, `UPDATE schema_metadata SET value='redacted_v1' WHERE key='diagnostic_policy'`)
	return err
}

func recordSanitizedDiagnosticTx(ctx context.Context, tx *sql.Tx, event model.DiagnosticEvent) error {
	metadata, err := json.Marshal(event.Metadata)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `INSERT OR IGNORE INTO diagnostic_events VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`, event.ID, event.Code, event.Severity, event.Timestamp, event.Scope.Repository, event.Scope.Revision, event.Scope.Generation, event.Scope.Path, event.Scope.Projection, event.Scope.Query, event.Scope.Handle, event.Remediation, string(metadata), event.ResolvedAt, event.Resolution)
	return err
}

func checkpointDiagnosticWAL(ctx context.Context, db *sql.DB) error {
	var busy, pages, checkpointed int
	if err := db.QueryRowContext(ctx, `PRAGMA wal_checkpoint(TRUNCATE)`).Scan(&busy, &pages, &checkpointed); err != nil {
		return err
	}
	if busy != 0 || (pages > 0 && checkpointed < pages) {
		return fmt.Errorf("diagnostic cleanup checkpoint busy; retry writer opening")
	}
	return nil
}

func retainDiagnostics(ctx context.Context, db *sql.DB) error {
	cutoff := time.Now().UTC().Add(-30 * 24 * time.Hour).Format(time.RFC3339Nano)
	_, err := db.ExecContext(ctx, `DELETE FROM diagnostic_events WHERE timestamp<? OR diagnostic_id NOT IN (SELECT diagnostic_id FROM diagnostic_events ORDER BY timestamp DESC,diagnostic_id DESC LIMIT 1000)`, cutoff)
	return err
}
