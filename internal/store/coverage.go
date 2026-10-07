package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"github.com/AdamNi-7080/AIOS/internal/model"
	"sort"
	"strings"
	"time"
)

// ActiveInputMetadata lets ingestion compare canonical coverage/provenance
// even when source bytes are unchanged. It never reads a source workspace.
func (s *Store) ActiveInputMetadata(ctx context.Context, repo string) (model.Snapshot, model.CoverageReport, error) {
	snapshots, err := s.Status(ctx, repo)
	if err != nil {
		return model.Snapshot{}, model.CoverageReport{}, err
	}
	if len(snapshots) != 1 {
		return model.Snapshot{}, model.CoverageReport{}, sql.ErrNoRows
	}
	snapshot := snapshots[0]
	rows, err := s.db.QueryContext(ctx, `SELECT e.path,e.language,e.classification,e.outcome,e.reason,e.capability,e.diagnostic FROM coverage_entries e JOIN coverage_runs c ON c.coverage_id=e.coverage_id JOIN active_generations a ON a.generation_id=c.generation_id WHERE a.repo_id=? ORDER BY e.path,e.outcome,e.reason`, repo)
	if err != nil {
		return snapshot, model.CoverageReport{}, err
	}
	report := model.CoverageReport{}
	for rows.Next() {
		var entry model.CoverageEntry
		if err = rows.Scan(&entry.Path, &entry.Language, &entry.Classification, &entry.Outcome, &entry.Reason, &entry.Capability, &entry.Diagnostic); err != nil {
			rows.Close()
			return snapshot, report, err
		}
		report.Entries = append(report.Entries, entry)
		if len(report.Entries) > 100000 {
			rows.Close()
			return snapshot, report, fmt.Errorf("canonical coverage exceeds maintenance bound")
		}
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return snapshot, report, err
	}
	rows, err = s.db.QueryContext(ctx, `SELECT language,code,message,path FROM compiler_diagnostics d JOIN active_generations a ON a.generation_id=d.generation_id WHERE a.repo_id=?`, repo)
	if err != nil {
		return snapshot, report, err
	}
	defer rows.Close()
	for rows.Next() {
		var d model.CompilerDiagnostic
		if err = rows.Scan(&d.Language, &d.Code, &d.Message, &d.Path); err != nil {
			return snapshot, report, err
		}
		snapshot.CompilerDiagnostics = append(snapshot.CompilerDiagnostics, d)
		if len(snapshot.CompilerDiagnostics) > 20000 {
			return snapshot, report, fmt.Errorf("canonical diagnostics exceed maintenance bound")
		}
	}
	return snapshot, report, rows.Err()
}

// CoverageBasis is a bounded, source-content-free explanation of whether an
// absence assertion is safe for a particular canonical capability.
type CoverageBasis struct {
	Complete     bool     `json:"complete"`
	Capability   string   `json:"capability"`
	Repositories []string `json:"repositories"`
	Generations  []string `json:"generations"`
	CoverageIDs  []string `json:"coverage_records"`
	Indexes      []string `json:"indexes"`
	Exclusions   []string `json:"exclusions,omitempty"`
	Uncertainty  []string `json:"uncertainty,omitempty"`
}

func (s *Store) Coverage(ctx context.Context, repository, capability string, fields []string) (CoverageBasis, error) {
	b := CoverageBasis{Complete: true, Capability: capability, Indexes: []string{"entities", "evidence", "claims"}}
	if capability == "exact" {
		b.Indexes = append(b.Indexes, "lookup")
	} else if capability == "structural" || capability == "path" {
		b.Indexes = append(b.Indexes, "graph")
	} else {
		b.Indexes = append(b.Indexes, "search_fts")
	}
	q := `SELECT g.repo_id,g.generation_id,COALESCE(c.coverage_id,''),COALESCE(c.status,'') FROM generations g JOIN active_generations a ON a.generation_id=g.generation_id LEFT JOIN coverage_runs c ON c.generation_id=g.generation_id`
	args := []any{}
	if repository != "" {
		q += ` WHERE g.repo_id=?`
		args = append(args, repository)
	}
	q += ` ORDER BY g.repo_id`
	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return b, err
	}
	defer rows.Close()
	type activeCoverage struct{ repo, generation, coverage, status string }
	var active []activeCoverage
	for rows.Next() {
		var repo, generation, coverage, status string
		if err := rows.Scan(&repo, &generation, &coverage, &status); err != nil {
			return b, err
		}
		active = append(active, activeCoverage{repo, generation, coverage, status})
	}
	if err := rows.Err(); err != nil {
		return b, err
	}
	rows.Close()
	for _, current := range active {
		repo, generation, coverage, status := current.repo, current.generation, current.coverage, current.status
		b.Repositories, b.Generations = append(b.Repositories, repo), append(b.Generations, generation)
		if coverage == "" {
			b.Complete = false
			b.Uncertainty = append(b.Uncertainty, "missing_coverage_record:"+repo)
			continue
		}
		b.CoverageIDs = append(b.CoverageIDs, coverage)
		if status != "complete" {
			b.Complete = false
		}
		erows, err := s.db.QueryContext(ctx, `SELECT path,outcome,reason,capability FROM coverage_entries WHERE coverage_id=? AND outcome<>'included' ORDER BY path,outcome,reason LIMIT 100`, coverage)
		if err != nil {
			return b, err
		}
		for erows.Next() {
			var path, outcome, reason, supported string
			if err := erows.Scan(&path, &outcome, &reason, &supported); err != nil {
				erows.Close()
				return b, err
			}
			b.Complete = false
			b.Exclusions = append(b.Exclusions, outcome+":"+reason+":"+path)
		}
		erows.Close()
		if capability == "structural" || capability == "path" {
			missingRows, err := s.db.QueryContext(ctx, `SELECT path FROM coverage_entries WHERE coverage_id=? AND outcome='included' AND classification='source' AND language<>'configuration' AND (language<>'text' OR path GLOB '*.*') AND instr(',' || capability || ',', ',structural,')=0 ORDER BY path LIMIT 100`, coverage)
			if err != nil {
				return b, err
			}
			for missingRows.Next() {
				var path string
				if err := missingRows.Scan(&path); err != nil {
					missingRows.Close()
					return b, err
				}
				b.Complete = false
				b.Uncertainty = append(b.Uncertainty, "unsupported_structural_extractor:"+repo+":"+path)
			}
			if err := missingRows.Err(); err != nil {
				missingRows.Close()
				return b, err
			}
			missingRows.Close()
			var diagnostics int
			if err := s.db.QueryRowContext(ctx, `SELECT count(*) FROM compiler_diagnostics WHERE generation_id=?`, generation).Scan(&diagnostics); err != nil {
				return b, err
			}
			if diagnostics > 0 {
				b.Complete = false
				b.Uncertainty = append(b.Uncertainty, "extractor_diagnostics:"+repo)
			}
		}
	}
	if len(b.Repositories) == 0 {
		b.Complete = false
		b.Uncertainty = append(b.Uncertainty, "no_active_generation")
	}
	projection := "lexical"
	if capability == "exact" {
		projection = "lookup"
	}
	if capability == "structural" || capability == "path" {
		projection = "graph"
	}
	var available int
	if err := s.db.QueryRowContext(ctx, `SELECT count(*) FROM active_projection_builds a JOIN projection_builds p ON p.projection_build_id=a.projection_build_id JOIN active_catalog_revision r ON r.catalog_revision_id=p.catalog_revision_id WHERE a.projection_kind=? AND p.state='ready'`, projection).Scan(&available); err != nil {
		return b, err
	}
	if available != 1 {
		b.Complete = false
		b.Uncertainty = append(b.Uncertainty, "index_unavailable:"+projection)
	}
	if len(fields) > 0 {
		b.Indexes = append(b.Indexes, "fields:"+strings.Join(fields, ","))
	}
	sort.Strings(b.Exclusions)
	sort.Strings(b.Uncertainty)
	return b, nil
}

type NegativeEvidence struct {
	ID, AssertionType, NormalizedTarget, SearchPlanFingerprint string
	SearchedScope                                              map[string]any
	CreatedAt                                                  string
}

func (s *Store) RecordNegative(ctx context.Context, assertionType, target, plan string, scope map[string]any, basis CoverageBasis) (NegativeEvidence, error) {
	if s.readOnly {
		return NegativeEvidence{}, fmt.Errorf("store is read-only")
	}
	encoded, err := json.Marshal(scope)
	if err != nil {
		return NegativeEvidence{}, err
	}
	id := "neg-" + digest(assertionType, strings.ToLower(strings.TrimSpace(target)), plan, strings.Join(basis.Generations, "/"), strings.Join(basis.CoverageIDs, "/"))
	now := time.Now().UTC().Format(time.RFC3339Nano)
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return NegativeEvidence{}, err
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, `INSERT OR IGNORE INTO negative_evidence VALUES(?,?,?,?,?,?,?,'','')`, id, assertionType, strings.ToLower(strings.TrimSpace(target)), "not_found", plan, string(encoded), now); err != nil {
		return NegativeEvidence{}, err
	}
	for i, generation := range basis.Generations {
		if _, err = tx.ExecContext(ctx, `INSERT OR REPLACE INTO negative_evidence_dependencies VALUES(?,?,?,?)`, id, "generation", generation, generation); err != nil {
			return NegativeEvidence{}, err
		}
		if i < len(basis.CoverageIDs) {
			if _, err = tx.ExecContext(ctx, `INSERT OR REPLACE INTO negative_evidence_dependencies VALUES(?,?,?,?)`, id, "coverage", basis.CoverageIDs[i], generation); err != nil {
				return NegativeEvidence{}, err
			}
		}
	}
	for _, index := range basis.Indexes {
		if _, err = tx.ExecContext(ctx, `INSERT OR REPLACE INTO negative_evidence_dependencies VALUES(?,?,?,?)`, id, "index", index, strings.Join(basis.Generations, "/")); err != nil {
			return NegativeEvidence{}, err
		}
	}
	if err = tx.Commit(); err != nil {
		return NegativeEvidence{}, err
	}
	return NegativeEvidence{ID: id, AssertionType: assertionType, NormalizedTarget: strings.ToLower(strings.TrimSpace(target)), SearchPlanFingerprint: plan, SearchedScope: scope, CreatedAt: now}, nil
}

// invalidateNegativeEvidenceTx only invalidates records whose own scoped
// generation ceased to be active. A delta in another repository therefore
// preserves a repository-scoped absence conclusion.
func invalidateNegativeEvidenceTx(ctx context.Context, tx *sql.Tx) error {
	_, err := tx.ExecContext(ctx, `UPDATE negative_evidence SET invalidated_at=?,invalidation_reason='dependent_generation_changed'
		WHERE invalidated_at='' AND EXISTS (
			SELECT 1 FROM negative_evidence_dependencies d
			WHERE d.negative_id=negative_evidence.negative_id AND d.dependency_kind='generation'
			AND d.dependency_id NOT IN (SELECT generation_id FROM active_generations))`, time.Now().UTC().Format(time.RFC3339Nano))
	return err
}
