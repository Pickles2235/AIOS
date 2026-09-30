package store

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"

	"github.com/AdamNi-7080/AIOS/internal/model"
)

// Discover records a mirror revision once per repository/revision/manifest.
func (s *Store) Discover(ctx context.Context, repo, revision, fingerprint string) (IngestionEvent, error) {
	return s.recordEvent(ctx, IngestionEvent{RepositoryID: repo, Type: EventMirrorRevisionDiscovered, TargetRevision: revision, ManifestFingerprint: fingerprint, Attempt: 0, Status: "discovered"})
}

// SelectRevision creates or resumes a single serialized repository job. The
// caller must have already verified the revision against the approved mirror.
func (s *Store) SelectRevision(ctx context.Context, repo, revision, fingerprint string) (IngestionQueueStatus, error) {
	if s.readOnly {
		return IngestionQueueStatus{}, fmt.Errorf("store is read-only")
	}
	if repo == "" || revision == "" || fingerprint == "" {
		return IngestionQueueStatus{}, fmt.Errorf("repository, revision, and manifest fingerprint are required")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return IngestionQueueStatus{}, err
	}
	defer tx.Rollback()
	var q IngestionQueueStatus
	err = tx.QueryRowContext(ctx, `SELECT repository_id,current_revision,pending_revision,manifest_fingerprint,state,attempt,failure_diagnostic,selected_at,activated_at,completed_at FROM ingestion_queues WHERE repository_id=?`, repo).Scan(&q.RepositoryID, &q.CurrentRevision, &q.PendingRevision, &q.ManifestFingerprint, &q.State, &q.Attempt, &q.FailureDiagnostic, &q.SelectedAt, &q.ActivatedAt, &q.CompletedAt)
	if err == nil {
		if q.PendingRevision == revision && q.ManifestFingerprint == fingerprint {
			return q, tx.Commit()
		}
		if q.PendingRevision != "" {
			return q, fmt.Errorf("out-of-order revision for %s: pending %s", repo, q.PendingRevision)
		}
		if q.CurrentRevision == revision {
			return q, fmt.Errorf("stale revision for %s: %s is already active", repo, revision)
		}
	} else if err != sql.ErrNoRows {
		return q, err
	}
	var source string
	_ = tx.QueryRowContext(ctx, `SELECT g.git_commit FROM active_generations a JOIN generations g ON g.generation_id=a.generation_id WHERE a.repo_id=?`, repo).Scan(&source)
	now := time.Now().UTC().Format(time.RFC3339Nano)
	attempt := 1
	if q.RepositoryID != "" {
		attempt = q.Attempt + 1
	}
	q = IngestionQueueStatus{RepositoryID: repo, CurrentRevision: source, PendingRevision: revision, ManifestFingerprint: fingerprint, State: "selected", Attempt: attempt, SelectedAt: now}
	_, err = tx.ExecContext(ctx, `INSERT INTO ingestion_queues(repository_id,current_revision,pending_revision,manifest_fingerprint,state,attempt,failure_diagnostic,selected_at,activated_at,completed_at) VALUES(?,?,?,?,?,?,?,?,?,?) ON CONFLICT(repository_id) DO UPDATE SET current_revision=excluded.current_revision,pending_revision=excluded.pending_revision,manifest_fingerprint=excluded.manifest_fingerprint,state=excluded.state,attempt=excluded.attempt,failure_diagnostic='',selected_at=excluded.selected_at`, q.RepositoryID, q.CurrentRevision, q.PendingRevision, q.ManifestFingerprint, q.State, q.Attempt, "", q.SelectedAt, "", "")
	if err != nil {
		return q, err
	}
	e := IngestionEvent{RepositoryID: repo, Type: EventRevisionSelected, SourceRevision: source, TargetRevision: revision, ManifestFingerprint: fingerprint, Attempt: attempt, Status: "selected"}
	if err = recordEventTx(ctx, tx, &e); err != nil {
		return q, err
	}
	return q, tx.Commit()
}

func (s *Store) recordEvent(ctx context.Context, e IngestionEvent) (IngestionEvent, error) {
	if s.readOnly {
		return e, fmt.Errorf("store is read-only")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return e, err
	}
	defer tx.Rollback()
	if err = recordEventTx(ctx, tx, &e); err != nil {
		return e, err
	}
	return e, tx.Commit()
}
func recordEventTx(ctx context.Context, tx *sql.Tx, e *IngestionEvent) error {
	if e.RepositoryID == "" || e.Type == "" || e.ManifestFingerprint == "" {
		return fmt.Errorf("event repository, type, and manifest fingerprint are required")
	}
	if e.Timestamp == "" {
		e.Timestamp = time.Now().UTC().Format(time.RFC3339Nano)
	}
	if e.Status == "" {
		e.Status = "recorded"
	}
	if e.EventID == "" {
		e.EventID = eventID(*e)
	}
	_, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO ingestion_events(event_id,repository_id,event_type,source_revision,target_revision,manifest_fingerprint,timestamp,attempt,status,failure_diagnostic) VALUES(?,?,?,?,?,?,?,?,?,?)`, e.EventID, e.RepositoryID, e.Type, e.SourceRevision, e.TargetRevision, e.ManifestFingerprint, e.Timestamp, e.Attempt, e.Status, e.FailureDiagnostic)
	return err
}
func eventID(e IngestionEvent) string {
	h := sha256.Sum256([]byte(e.RepositoryID + "\x00" + e.Type + "\x00" + e.SourceRevision + "\x00" + e.TargetRevision + "\x00" + e.ManifestFingerprint + "\x00" + fmt.Sprint(e.Attempt)))
	return hex.EncodeToString(h[:])
}

func (s *Store) IngestionStatus(ctx context.Context, repo string) ([]IngestionQueueStatus, error) {
	q := `SELECT repository_id,current_revision,pending_revision,manifest_fingerprint,state,attempt,failure_diagnostic,selected_at,activated_at,completed_at FROM ingestion_queues`
	args := []any{}
	if repo != "" {
		q += " WHERE repository_id=?"
		args = append(args, repo)
	}
	q += " ORDER BY repository_id"
	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []IngestionQueueStatus
	for rows.Next() {
		var x IngestionQueueStatus
		if err = rows.Scan(&x.RepositoryID, &x.CurrentRevision, &x.PendingRevision, &x.ManifestFingerprint, &x.State, &x.Attempt, &x.FailureDiagnostic, &x.SelectedAt, &x.ActivatedAt, &x.CompletedAt); err != nil {
			return nil, err
		}
		out = append(out, x)
	}
	return out, rows.Err()
}

// IngestionEvents returns the bounded durable audit trail newest first.
func (s *Store) IngestionEvents(ctx context.Context, repo string) ([]IngestionEvent, error) {
	q := `SELECT event_id,repository_id,event_type,source_revision,target_revision,manifest_fingerprint,timestamp,attempt,status,failure_diagnostic FROM ingestion_events`
	args := []any{}
	if repo != "" {
		q += " WHERE repository_id=?"
		args = append(args, repo)
	}
	q += " ORDER BY timestamp DESC,event_id DESC LIMIT 100"
	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []IngestionEvent
	for rows.Next() {
		var e IngestionEvent
		if err = rows.Scan(&e.EventID, &e.RepositoryID, &e.Type, &e.SourceRevision, &e.TargetRevision, &e.ManifestFingerprint, &e.Timestamp, &e.Attempt, &e.Status, &e.FailureDiagnostic); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// CompleteRevision advances only the selected matching queue after a catalog
// activation. It preserves a durable audit of IR and derived-work completion.
func (s *Store) CompleteRevision(ctx context.Context, repo, revision, fingerprint string) error {
	if s.readOnly {
		return fmt.Errorf("store is read-only")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var attempt int
	var source string
	if err = tx.QueryRowContext(ctx, `SELECT current_revision,attempt FROM ingestion_queues WHERE repository_id=? AND pending_revision=? AND manifest_fingerprint=?`, repo, revision, fingerprint).Scan(&source, &attempt); err != nil {
		return fmt.Errorf("complete revision: %w", err)
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err = tx.ExecContext(ctx, `UPDATE ingestion_queues SET current_revision=?,pending_revision='',state='completed',failure_diagnostic='',activated_at=?,completed_at=? WHERE repository_id=?`, revision, now, now, repo); err != nil {
		return err
	}
	for _, e := range []IngestionEvent{
		{RepositoryID: repo, Type: EventIRGenerationActivated, SourceRevision: source, TargetRevision: revision, ManifestFingerprint: fingerprint, Attempt: attempt, Status: "activated"},
		{RepositoryID: repo, Type: EventProjectionRequested, SourceRevision: source, TargetRevision: revision, ManifestFingerprint: fingerprint, Attempt: attempt, Status: "requested"},
		{RepositoryID: repo, Type: EventProjectionCompleted, SourceRevision: source, TargetRevision: revision, ManifestFingerprint: fingerprint, Attempt: attempt, Status: "completed"},
	} {
		if err = recordEventTx(ctx, tx, &e); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// FailRevision is restart-safe: it retains the selected revision and records
// a retryable failure without altering active knowledge.
func (s *Store) FailRevision(ctx context.Context, repo, revision, fingerprint, diagnostic string) error {
	if s.readOnly {
		return fmt.Errorf("store is read-only")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var source string
	var attempt int
	if err = tx.QueryRowContext(ctx, `SELECT current_revision,attempt FROM ingestion_queues WHERE repository_id=? AND pending_revision=? AND manifest_fingerprint=?`, repo, revision, fingerprint).Scan(&source, &attempt); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `UPDATE ingestion_queues SET state='failed',failure_diagnostic=? WHERE repository_id=?`, diagnostic, repo); err != nil {
		return err
	}
	for _, typ := range []string{EventProjectionFailed} {
		e := IngestionEvent{RepositoryID: repo, Type: typ, SourceRevision: source, TargetRevision: revision, ManifestFingerprint: fingerprint, Attempt: attempt, Status: "failed", FailureDiagnostic: diagnostic}
		if err = recordEventTx(ctx, tx, &e); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// RecordSourceDelta writes immutable Git-tree evidence before compilation.
func (s *Store) RecordSourceDelta(ctx context.Context, repo, source, target, fingerprint, scope string, changes []model.FileChange) (string, error) {
	if s.readOnly {
		return "", fmt.Errorf("store is read-only")
	}
	id := eventID(IngestionEvent{RepositoryID: repo, Type: EventSourceDeltaCalculated, SourceRevision: source, TargetRevision: target, ManifestFingerprint: fingerprint})
	counts := map[string]int{}
	for _, c := range changes {
		counts[c.Kind]++
	}
	encoded := "{}"
	if len(counts) > 0 {
		b, _ := json.Marshal(counts)
		encoded = string(b)
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return "", err
	}
	defer tx.Rollback()
	now := time.Now().UTC().Format(time.RFC3339Nano)
	_, err = tx.ExecContext(ctx, `INSERT OR IGNORE INTO ir_delta_records(delta_id,repository_id,source_generation_id,target_generation_id,source_revision,target_revision,manifest_fingerprint,created_at,activated_at,invalidation_scope,counts_json) VALUES(?,?,?,?,?,?,?,?,?,?,?)`, id, repo, "", "", source, target, fingerprint, now, "", scope, encoded)
	if err != nil {
		return "", err
	}
	for _, c := range changes {
		if _, err = tx.ExecContext(ctx, `INSERT OR IGNORE INTO ir_delta_changes(delta_id,change_kind,canonical_id,derivation,invalidation_reason) VALUES(?,?,?,?,?)`, id, c.Kind, c.Path, "git_tree", "source_"+c.Kind); err != nil {
			return "", err
		}
	}
	e := IngestionEvent{RepositoryID: repo, Type: EventSourceDeltaCalculated, SourceRevision: source, TargetRevision: target, ManifestFingerprint: fingerprint, Status: "calculated"}
	if err = recordEventTx(ctx, tx, &e); err != nil {
		return "", err
	}
	e.Type, e.Status = EventIRDeltaStaged, "staged"
	if err = recordEventTx(ctx, tx, &e); err != nil {
		return "", err
	}
	return id, tx.Commit()
}

// FinalizeDelta attaches lifecycle state to the generation that was atomically
// activated by the catalog writer. A fact absent only because its source file
// was deleted is retained canonically and marked inactive with its reason.
func (s *Store) FinalizeDelta(ctx context.Context, repo, target, fingerprint, generation string) error {
	if s.readOnly {
		return fmt.Errorf("store is read-only")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var delta, source string
	if err = tx.QueryRowContext(ctx, `SELECT delta_id,source_revision FROM ir_delta_records WHERE repository_id=? AND target_revision=? AND manifest_fingerprint=?`, repo, target, fingerprint).Scan(&delta, &source); err != nil {
		return err
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err = tx.ExecContext(ctx, `UPDATE ir_delta_records SET target_generation_id=?,activated_at=? WHERE delta_id=?`, generation, now, delta); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `INSERT OR IGNORE INTO ir_fact_lifecycle(fact_id,generation_id,state,reason,delta_id) SELECT DISTINCT fact_id,?,'active','observed',? FROM ir_fact_observations WHERE revision_id=?`, generation, delta, generation); err != nil {
		return err
	}
	if source != "" {
		if _, err = tx.ExecContext(ctx, `INSERT OR IGNORE INTO ir_fact_lifecycle(fact_id,generation_id,state,reason,delta_id)
 SELECT DISTINCT o.fact_id,?,'inactive','source_deleted',? FROM ir_fact_observations o JOIN evidence e ON e.evidence_id=o.evidence_id
 JOIN ir_delta_changes d ON d.delta_id=? AND d.change_kind='removed' AND d.canonical_id=e.path
 WHERE o.revision_id=(SELECT generation_id FROM generations WHERE repo_id=? AND git_commit=? ORDER BY activated_at DESC LIMIT 1)
 AND NOT EXISTS (SELECT 1 FROM ir_fact_observations n WHERE n.revision_id=? AND n.fact_id=o.fact_id)`, generation, delta, delta, repo, source, generation); err != nil {
			return err
		}
	}
	return tx.Commit()
}
