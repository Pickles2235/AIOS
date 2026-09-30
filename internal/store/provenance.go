package store

import (
	"context"
	"database/sql"
	"sort"

	"github.com/AdamNi-7080/AIOS/internal/model"
)

// ValidateActiveProjectionProvenance replays activation's referential checks.
// The events are reproducibly derived from immutable IR and projection rows,
// allowing status calls to remain read-only.
func (s *Store) ValidateActiveProjectionProvenance(ctx context.Context) ([]model.DiagnosticEvent, error) {
	revision, err := s.ActiveCatalogRevision(ctx)
	if err != nil {
		return nil, nil
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	rows, err := tx.QueryContext(ctx, `SELECT a.projection_kind,a.projection_build_id FROM active_projection_builds a JOIN projection_builds p ON p.projection_build_id=a.projection_build_id WHERE p.catalog_revision_id=? ORDER BY a.projection_kind`, revision.ID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var events []model.DiagnosticEvent
	for rows.Next() {
		var kind, build string
		if err = rows.Scan(&kind, &build); err != nil {
			return nil, err
		}
		if err = validateProjectionRows(ctx, tx, kind, build, revision.ID); err != nil {
			events = append(events, model.DiagnosticEvent{ID: "derived-" + digest(DiagnosticDanglingProvenance, kind, build), Code: DiagnosticDanglingProvenance, Severity: model.DiagnosticError, Scope: model.DiagnosticScope{Projection: kind}, Remediation: "rebuild the projection; if it persists, reindex canonical IR", Metadata: map[string]string{"build": build}})
		}
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}
	sort.Slice(events, func(i, j int) bool { return events[i].ID < events[j].ID })
	return events, nil
}
