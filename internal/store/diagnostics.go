package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/AdamNi-7080/AIOS/internal/model"
	"github.com/AdamNi-7080/AIOS/internal/observability"
)

func safeDiagnostic(event model.DiagnosticEvent) (model.DiagnosticEvent, error) {
	if event.Code == "" || event.Severity == "" {
		return event, fmt.Errorf("diagnostic code and severity are required")
	}
	if event.Severity != model.DiagnosticInfo && event.Severity != model.DiagnosticWarning && event.Severity != model.DiagnosticError {
		return event, fmt.Errorf("invalid diagnostic severity")
	}
	code := event.Code
	switch code {
	case DiagnosticIngestionFailure, DiagnosticExtractionFailure, DiagnosticUnsupportedAnalysis, DiagnosticProjectionFailure,
		DiagnosticProjectionMismatch, DiagnosticDanglingProvenance, DiagnosticCacheMismatch, DiagnosticPlannerFallback, DiagnosticPlannerBudgetExceeded:
	default:
		code = "OPERATIONAL_DIAGNOSTIC"
	}
	event.Code = code
	event.Scope.Repository = observability.OpaqueIfNeeded(event.Scope.Repository)
	event.Scope.Revision = observability.OpaqueIfNeeded(event.Scope.Revision)
	event.Scope.Generation = observability.OpaqueIfNeeded(event.Scope.Generation)
	event.Scope.Path = observability.OpaqueIfNeeded(event.Scope.Path)
	event.Scope.Query = observability.OpaqueIfNeeded(event.Scope.Query)
	event.Scope.Handle = observability.OpaqueIfNeeded(event.Scope.Handle)
	if event.Scope.Projection != "" {
		event.Scope.Projection = safeDiagnosticValue(event.Scope.Projection)
	}
	event.Remediation = "inspect local knowledge health and retry the operation"
	if event.Resolution != "" {
		event.Resolution = "resolved"
	}
	if event.ID != "" && !strings.HasPrefix(event.ID, "diag-h_") {
		event.ID = "diag-" + observability.Opaque(event.ID)
	}
	if _, err := time.Parse(time.RFC3339Nano, event.Timestamp); err != nil {
		event.Timestamp = time.Now().UTC().Format(time.RFC3339Nano)
	}
	if event.ResolvedAt != "" {
		if _, err := time.Parse(time.RFC3339Nano, event.ResolvedAt); err != nil {
			event.ResolvedAt = time.Now().UTC().Format(time.RFC3339Nano)
		}
	}
	clean := map[string]string{}
	for key, value := range event.Metadata {
		if key == "" || len(key) > 64 || len(value) > 256 || strings.ContainsAny(value, "\n\r") {
			return event, fmt.Errorf("diagnostic metadata must be bounded single-line fields")
		}
		lower := strings.ToLower(key)
		if strings.Contains(lower, "token") || strings.Contains(lower, "secret") || strings.Contains(lower, "password") || strings.Contains(lower, "excerpt") || strings.Contains(lower, "query") || strings.Contains(lower, "content") {
			return event, fmt.Errorf("diagnostic metadata field %q is not safe", key)
		}
		switch key {
		case "language":
			switch value {
			case "go", "java", "kotlin", "javascript", "typescript", "python", "tsx", "jsx":
				clean[key] = value
			default:
				clean[key] = "other"
			}
		case "operation":
			if value == "operator_rebuild" {
				clean[key] = value
			} else {
				clean[key] = "other"
			}
		case "state":
			switch value {
			case "ready", "staged", "unavailable", "failed", "retired", "disabled":
				clean[key] = value
			default:
				clean[key] = "other"
			}
		case "extractor_code", "strategy", "reason":
			clean[key] = observability.Opaque(value)
		case "build":
			clean[key] = observability.Opaque(value)
		}
	}
	event.Metadata = clean
	return event, nil
}
func safeDiagnosticValue(value string) string {
	switch value {
	case "lookup", "lexical", "graph", "path", "ui", "cache", "vector", "landmarks":
		return value
	}
	return "other"
}

// RecordDiagnostic writes only allowlisted, source-content-free operational
// context.  Callers supply query fingerprints rather than query text.
func (s *Store) RecordDiagnostic(ctx context.Context, event model.DiagnosticEvent) (model.DiagnosticEvent, error) {
	if s.readOnly {
		return model.DiagnosticEvent{}, fmt.Errorf("store is read-only")
	}
	var err error
	event, err = safeDiagnostic(event)
	if err != nil {
		return model.DiagnosticEvent{}, err
	}
	if event.ID == "" {
		event.ID = "diag-" + digest(event.Code, string(event.Severity), event.Scope.Repository, event.Scope.Revision, event.Scope.Generation, event.Scope.Path, event.Scope.Projection, event.Scope.Query, event.Scope.Handle)
	}
	if event.Timestamp == "" {
		event.Timestamp = time.Now().UTC().Format(time.RFC3339Nano)
	}
	metadata, err := diagnosticMetadata(event.Metadata)
	if err != nil {
		return model.DiagnosticEvent{}, err
	}
	_, err = s.db.ExecContext(ctx, `INSERT OR IGNORE INTO diagnostic_events VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`, event.ID, event.Code, event.Severity, event.Timestamp, event.Scope.Repository, event.Scope.Revision, event.Scope.Generation, event.Scope.Path, event.Scope.Projection, event.Scope.Query, event.Scope.Handle, event.Remediation, metadata, event.ResolvedAt, event.Resolution)
	if err == nil {
		err = retainDiagnostics(ctx, s.db)
	}
	return event, err
}

func diagnosticMetadata(metadata map[string]string) (string, error) {
	if len(metadata) == 0 {
		return "{}", nil
	}
	clean := make(map[string]string, len(metadata))
	keys := make([]string, 0, len(metadata))
	for key := range metadata {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		value := metadata[key]
		if key == "" || len(key) > 64 || len(value) > 256 || strings.ContainsAny(value, "\n\r") {
			return "", fmt.Errorf("diagnostic metadata must be bounded single-line fields")
		}
		lower := strings.ToLower(key)
		if strings.Contains(lower, "token") || strings.Contains(lower, "secret") || strings.Contains(lower, "password") || strings.Contains(lower, "excerpt") || strings.Contains(lower, "query") || strings.Contains(lower, "content") {
			return "", fmt.Errorf("diagnostic metadata field %q is not safe", key)
		}
		clean[key] = value
	}
	encoded, err := json.Marshal(clean)
	return string(encoded), err
}

func (s *Store) ResolveDiagnostic(ctx context.Context, id, resolution string) error {
	if s.readOnly {
		return fmt.Errorf("store is read-only")
	}
	if id == "" || len(resolution) > 256 || strings.ContainsAny(resolution, "\n\r") {
		return fmt.Errorf("invalid diagnostic resolution")
	}
	_, err := s.db.ExecContext(ctx, `UPDATE diagnostic_events SET resolved_at=?,resolution=? WHERE diagnostic_id=? AND resolved_at=''`, time.Now().UTC().Format(time.RFC3339Nano), "resolved", id)
	return err
}

func (s *Store) DiagnosticEvents(ctx context.Context, repository string, includeResolved bool, limit int) ([]model.DiagnosticEvent, error) {
	if limit < 1 || limit > 100 {
		limit = 50
	}
	q := `SELECT diagnostic_id,code,severity,timestamp,repository_id,revision_id,generation_id,path,projection_kind,query_fingerprint,handle,remediation,metadata_json,resolved_at,resolution FROM diagnostic_events`
	args := []any{}
	where := []string{}
	if repository != "" {
		where = append(where, "repository_id=?")
		args = append(args, observability.Opaque(repository))
	}
	if !includeResolved {
		where = append(where, "resolved_at=''")
	}
	if len(where) > 0 {
		q += " WHERE " + strings.Join(where, " AND ")
	}
	q += " ORDER BY timestamp DESC,diagnostic_id DESC LIMIT ?"
	args = append(args, limit)
	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []model.DiagnosticEvent
	for rows.Next() {
		var event model.DiagnosticEvent
		var severity string
		var metadata string
		if err := rows.Scan(&event.ID, &event.Code, &severity, &event.Timestamp, &event.Scope.Repository, &event.Scope.Revision, &event.Scope.Generation, &event.Scope.Path, &event.Scope.Projection, &event.Scope.Query, &event.Scope.Handle, &event.Remediation, &metadata, &event.ResolvedAt, &event.Resolution); err != nil {
			return nil, err
		}
		event.Severity = model.DiagnosticSeverity(severity)
		_ = json.Unmarshal([]byte(metadata), &event.Metadata)
		out = append(out, event)
	}
	return out, rows.Err()
}

func recordDiagnosticTx(ctx context.Context, tx *sql.Tx, event model.DiagnosticEvent) error {
	var err error
	event, err = safeDiagnostic(event)
	if err != nil {
		return err
	}
	metadata, err := diagnosticMetadata(event.Metadata)
	if err != nil {
		return err
	}
	if event.ID == "" {
		event.ID = "diag-" + digest(event.Code, string(event.Severity), event.Scope.Repository, event.Scope.Revision, event.Scope.Generation, event.Scope.Path, event.Scope.Projection, event.Scope.Query, event.Scope.Handle)
	}
	if event.Timestamp == "" {
		event.Timestamp = time.Now().UTC().Format(time.RFC3339Nano)
	}
	_, err = tx.ExecContext(ctx, `INSERT OR IGNORE INTO diagnostic_events VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`, event.ID, event.Code, event.Severity, event.Timestamp, event.Scope.Repository, event.Scope.Revision, event.Scope.Generation, event.Scope.Path, event.Scope.Projection, event.Scope.Query, event.Scope.Handle, event.Remediation, metadata, event.ResolvedAt, event.Resolution)
	if err == nil {
		_, err = tx.ExecContext(ctx, `DELETE FROM diagnostic_events WHERE diagnostic_id NOT IN (SELECT diagnostic_id FROM diagnostic_events ORDER BY timestamp DESC,diagnostic_id DESC LIMIT 1000)`)
	}
	return err
}
