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
	return s.selectRevision(ctx, repo, revision, fingerprint, false)
}

// SelectRepairRevision is reserved for the serialized maintained-repository
// writer. Its exclusive writer lease proves an interrupted pending job has no
// concurrent owner. Supersession is durable and never changes active knowledge.
func (s *Store) SelectRepairRevision(ctx context.Context, repo, revision, fingerprint string) (IngestionQueueStatus, error) {
	return s.selectRevision(ctx, repo, revision, fingerprint, true)
}

func (s *Store) selectRevision(ctx context.Context, repo, revision, fingerprint string, repair bool) (IngestionQueueStatus, error) {
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
		if q.PendingRevision == revision && q.ManifestFingerprint == fingerprint && !repair {
			return q, tx.Commit()
		}
		if q.PendingRevision != "" {
			if !repair {
				return q, fmt.Errorf("out-of-order revision for %s: pending %s", repo, q.PendingRevision)
			}
			if q.PendingRevision != revision || q.ManifestFingerprint != fingerprint {
				e := IngestionEvent{RepositoryID: repo, Type: "revision_superseded", SourceRevision: q.PendingRevision, TargetRevision: revision, ManifestFingerprint: fingerprint, Attempt: q.Attempt, Status: "superseded"}
				if err = recordEventTx(ctx, tx, &e); err != nil {
					return q, err
				}
			}
		}
		if q.CurrentRevision == revision && !repair {
			return q, fmt.Errorf("stale revision for %s: %s is already active", repo, revision)
		}
	} else if err != sql.ErrNoRows {
		return q, err
	}
	source := q.CurrentRevision
	if source == "" {
		if e := tx.QueryRowContext(ctx, `SELECT g.git_commit FROM active_generations a JOIN generations g ON g.generation_id=a.generation_id WHERE a.repo_id=?`, repo).Scan(&source); e != nil && e != sql.ErrNoRows {
			return q, e
		}
	}
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
	e.FailureDiagnostic = safeFailureDiagnostic(e.FailureDiagnostic)
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
	if err = completeRevisionTx(ctx, tx, repo, revision, fingerprint); err != nil {
		return err
	}
	return tx.Commit()
}

func completeRevisionTx(ctx context.Context, tx *sql.Tx, repo, revision, fingerprint string) error {
	var attempt int
	var source string
	if err := tx.QueryRowContext(ctx, `SELECT current_revision,attempt FROM ingestion_queues WHERE repository_id=? AND pending_revision=? AND manifest_fingerprint=?`, repo, revision, fingerprint).Scan(&source, &attempt); err != nil {
		return fmt.Errorf("complete revision: %w", err)
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err := tx.ExecContext(ctx, `UPDATE ingestion_queues SET current_revision=?,pending_revision='',state='completed',failure_diagnostic='',activated_at=?,completed_at=? WHERE repository_id=?`, revision, now, now, repo); err != nil {
		return err
	}
	for _, e := range []IngestionEvent{
		{RepositoryID: repo, Type: EventIRGenerationActivated, SourceRevision: source, TargetRevision: revision, ManifestFingerprint: fingerprint, Attempt: attempt, Status: "activated"},
		{RepositoryID: repo, Type: EventProjectionRequested, SourceRevision: source, TargetRevision: revision, ManifestFingerprint: fingerprint, Attempt: attempt, Status: "requested"},
		{RepositoryID: repo, Type: EventProjectionCompleted, SourceRevision: source, TargetRevision: revision, ManifestFingerprint: fingerprint, Attempt: attempt, Status: "completed"},
	} {
		if err := recordEventTx(ctx, tx, &e); err != nil {
			return err
		}
	}
	return nil
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
	diagnostic = safeFailureDiagnostic(diagnostic)
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

func safeFailureDiagnostic(value string) string {
	switch value {
	case "", "mirror_revision_failed", "local_revision_failed", "maintained_revision_failed":
		return value
	}
	return "operation_failed"
}

// RecordSourceDelta writes immutable Git-tree evidence before compilation.
func (s *Store) RecordSourceDelta(ctx context.Context, repo, source, target, fingerprint, scope string, changes []model.FileChange) (string, error) {
	if s.readOnly {
		return "", fmt.Errorf("store is read-only")
	}
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
	var sourceGeneration string
	if e := tx.QueryRowContext(ctx, `SELECT generation_id FROM active_generations WHERE repo_id=?`, repo).Scan(&sourceGeneration); e != nil && e != sql.ErrNoRows {
		return "", e
	}
	var attempt int
	var selectedAt string
	if e := tx.QueryRowContext(ctx, `SELECT attempt, selected_at FROM ingestion_queues WHERE repository_id=?`, repo).Scan(&attempt, &selectedAt); e != nil && e != sql.ErrNoRows {
		return "", e
	}
	// A content revision may be revisited after a different canonical epoch.
	// Keep each selected attempt's evidence separate and immutable.
	id := eventID(IngestionEvent{RepositoryID: repo, Type: EventSourceDeltaCalculated, SourceRevision: source + "\x00" + sourceGeneration + "\x00" + selectedAt, TargetRevision: target, ManifestFingerprint: fingerprint, Attempt: attempt})
	_, err = tx.ExecContext(ctx, `INSERT OR IGNORE INTO ir_delta_records(delta_id,repository_id,source_generation_id,target_generation_id,source_revision,target_revision,manifest_fingerprint,created_at,activated_at,invalidation_scope,counts_json) VALUES(?,?,?,?,?,?,?,?,?,?,?)`, id, repo, sourceGeneration, "", source, target, fingerprint, now, "", scope, encoded)
	if err != nil {
		return "", err
	}
	for _, c := range changes {
		if _, err = tx.ExecContext(ctx, `INSERT OR IGNORE INTO ir_delta_changes(delta_id,change_kind,canonical_id,derivation,invalidation_reason) VALUES(?,?,?,?,?)`, id, c.Kind, c.Path, "git_tree", "source_"+c.Kind); err != nil {
			return "", err
		}
	}
	e := IngestionEvent{RepositoryID: repo, Type: EventSourceDeltaCalculated, SourceRevision: source, TargetRevision: target, ManifestFingerprint: fingerprint, Attempt: attempt, Status: "calculated"}
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
	return s.finalizeDelta(ctx, "WHERE repository_id=? AND target_revision=? AND manifest_fingerprint=? ORDER BY rowid DESC LIMIT 1", []any{repo, target, fingerprint}, generation)
}

// FinalizeRecordedDelta binds maintained jobs to the exact recorded source
// epoch, including repeated visits to the same working revision.
func (s *Store) FinalizeRecordedDelta(ctx context.Context, delta, generation string) error {
	return s.finalizeDelta(ctx, "WHERE delta_id=?", []any{delta}, generation)
}

func (s *Store) finalizeDelta(ctx context.Context, where string, args []any, generation string) error {
	if s.readOnly {
		return fmt.Errorf("store is read-only")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err = finalizeDeltaTx(ctx, tx, where, args, generation); err != nil {
		return err
	}
	return tx.Commit()
}

func finalizeDeltaTx(ctx context.Context, tx *sql.Tx, where string, args []any, generation string) error {
	var delta, source, repository, priorTarget, priorActivation string
	if err := tx.QueryRowContext(ctx, `SELECT delta_id,source_generation_id,repository_id,target_generation_id,activated_at FROM ir_delta_records `+where, args...).Scan(&delta, &source, &repository, &priorTarget, &priorActivation); err != nil {
		return err
	}
	var generationRepository string
	if err := tx.QueryRowContext(ctx, `SELECT repo_id FROM generations WHERE generation_id=?`, generation).Scan(&generationRepository); err != nil {
		return err
	}
	if generationRepository != repository {
		return fmt.Errorf("delta generation belongs to another repository")
	}
	if priorActivation != "" {
		if priorTarget != generation {
			return fmt.Errorf("activated delta is immutable")
		}
		return nil
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err := tx.ExecContext(ctx, `UPDATE ir_delta_records SET target_generation_id=?,activated_at=? WHERE delta_id=? AND activated_at=''`, generation, now, delta); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO ir_fact_lifecycle(fact_id,generation_id,state,reason,delta_id) SELECT DISTINCT fact_id,?,'active','observed',? FROM ir_fact_observations WHERE revision_id=?`, generation, delta, generation); err != nil {
		return err
	}
	if source != "" {
		if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO ir_fact_lifecycle(fact_id,generation_id,state,reason,delta_id)
 SELECT DISTINCT o.fact_id,?,'inactive','source_deleted',? FROM ir_fact_observations o JOIN evidence e ON e.evidence_id=o.evidence_id
 JOIN ir_delta_changes d ON d.delta_id=? AND d.change_kind='removed' AND d.canonical_id=e.path
	 WHERE o.revision_id=?
	 AND NOT EXISTS (SELECT 1 FROM ir_fact_observations n WHERE n.revision_id=? AND n.fact_id=o.fact_id)`, generation, delta, delta, source, generation); err != nil {
			return err
		}
	}
	return nil
}

type ActivationSelection struct{ Repository, Revision, Fingerprint, Delta string }

func completeSelectionsTx(ctx context.Context, tx *sql.Tx, selections []ActivationSelection) error {
	for _, selection := range selections {
		var generation string
		if err := tx.QueryRowContext(ctx, `SELECT generation_id FROM active_generations WHERE repo_id=?`, selection.Repository).Scan(&generation); err != nil {
			return err
		}
		where, args := "WHERE delta_id=?", []any{selection.Delta}
		if selection.Delta == "" {
			where = "WHERE repository_id=? AND target_revision=? AND manifest_fingerprint=? ORDER BY rowid DESC LIMIT 1"
			args = []any{selection.Repository, selection.Revision, selection.Fingerprint}
		}
		var repo, target, fingerprint string
		if err := tx.QueryRowContext(ctx, `SELECT repository_id,target_revision,manifest_fingerprint FROM ir_delta_records `+where, args...).Scan(&repo, &target, &fingerprint); err != nil {
			return err
		}
		if repo != selection.Repository || target != selection.Revision || fingerprint != selection.Fingerprint {
			return fmt.Errorf("activation selection does not match recorded delta")
		}
		if err := finalizeDeltaTx(ctx, tx, where, args, generation); err != nil {
			return err
		}
		if err := completeRevisionTx(ctx, tx, repo, target, fingerprint); err != nil {
			return err
		}
	}
	return nil
}

// CompleteUnchangedSelections commits queue/delta freshness together while
// retaining the canonical catalog when validated input is entirely unchanged.
func (s *Store) CompleteUnchangedSelections(ctx context.Context, selections []ActivationSelection) error {
	if s.readOnly {
		return fmt.Errorf("store is read-only")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err = completeSelectionsTx(ctx, tx, selections); err != nil {
		return err
	}
	return tx.Commit()
}
