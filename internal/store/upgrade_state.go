package store

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/AdamNi-7080/AIOS/internal/installstate"
	"io"
	"path/filepath"
	"sort"
	"strings"
)

// AcquireDataLease serializes an owned state snapshot without opening or
// migrating its database. The service must already have stopped.
func AcquireDataLease(data string) (io.Closer, error) {
	abs, err := filepath.Abs(data)
	if err != nil {
		return nil, err
	}
	if err = validateDataDirectory(abs); err != nil {
		return nil, err
	}
	l, err := acquireDataWriterLock(abs)
	if err != nil {
		return nil, err
	}
	return dataLease{l}, nil
}

type dataLease struct{ lock *writerLock }

func (l dataLease) Close() error { return l.lock.close() }

type UpgradeState = installstate.UpgradeState

// ReadUpgradeState is a nonmutating compatibility read of the two actually
// supported formats. It fingerprints full canonical rows, including source
// locations/evidence, without returning or persisting their contents.
func ReadUpgradeState(ctx context.Context, data string) (UpgradeState, error) {
	var out UpgradeState
	p, err := validateReadOnlyDatabase(data)
	if err != nil {
		return out, err
	}
	db, err := sql.Open("sqlite", sqliteDSN(p, true))
	if err != nil {
		return out, err
	}
	defer func() {
		if db != nil {
			db.Close()
		}
	}()
	db.SetMaxOpenConns(1)
	tx, err := db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return out, err
	}
	defer tx.Rollback()
	if err = tx.QueryRowContext(ctx, `SELECT value FROM schema_metadata WHERE key='format'`).Scan(&out.Format); err != nil {
		return out, err
	}
	if out.Format != "knowledge-ir-v9" && out.Format != format {
		return out, fmt.Errorf("unsupported upgrade canonical format")
	}
	var integrity string
	if err = tx.QueryRowContext(ctx, `PRAGMA quick_check`).Scan(&integrity); err != nil || integrity != "ok" {
		return out, fmt.Errorf("upgrade canonical integrity check failed")
	}
	violations, err := tx.QueryContext(ctx, `PRAGMA foreign_key_check`)
	if err != nil {
		return out, err
	}
	bad := violations.Next()
	err = violations.Err()
	violations.Close()
	if bad || err != nil {
		return out, fmt.Errorf("upgrade canonical foreign-key check failed")
	}
	tables := []string{"generations", "active_generations", "catalog_revisions", "catalog_revision_members", "active_catalog_revision", "source_files", "entities", "evidence", "claims", "canonical_identities", "identity_memberships", "cross_claims", "coverage_runs", "coverage_entries", "compiler_diagnostics"}
	rows, err := tx.QueryContext(ctx, `SELECT name FROM sqlite_master WHERE type='table' AND name GLOB 'ir_*' ORDER BY name`)
	if err != nil {
		return out, err
	}
	for rows.Next() {
		var name string
		if err = rows.Scan(&name); err != nil {
			rows.Close()
			return out, err
		}
		tables = append(tables, name)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return out, err
	}
	sort.Strings(tables)
	h := sha256.New()
	encoder := json.NewEncoder(h)
	for _, name := range tables {
		// Names come only from sqlite_master and the fixed inventory; SQL quoting
		// preserves even an adversarial name without executing it as SQL.
		quote := func(v string) string { return `"` + strings.ReplaceAll(v, `"`, `""`) + `"` }
		rs, e := tx.QueryContext(ctx, "SELECT * FROM "+quote(name)+" LIMIT 0")
		if e != nil {
			return out, e
		}
		columns, e := rs.Columns()
		rs.Close()
		if e != nil {
			return out, e
		}
		ordered := make([]string, len(columns))
		for i, c := range columns {
			ordered[i] = quote(c)
		}
		rs, e = tx.QueryContext(ctx, "SELECT * FROM "+quote(name)+" ORDER BY "+strings.Join(ordered, ","))
		if e != nil {
			return out, e
		}
		if e = encoder.Encode(name); e != nil {
			rs.Close()
			return out, e
		}
		for rs.Next() {
			values := make([]any, len(columns))
			dest := make([]any, len(columns))
			for i := range values {
				dest[i] = &values[i]
			}
			if e = rs.Scan(dest...); e != nil {
				rs.Close()
				return out, e
			}
			if e = encoder.Encode(values); e != nil {
				rs.Close()
				return out, e
			}
		}
		e = rs.Err()
		rs.Close()
		if e != nil {
			return out, e
		}
	}
	out.CanonicalFingerprint = hex.EncodeToString(h.Sum(nil))
	out.ActiveGenerations = map[string]string{}
	rows, err = tx.QueryContext(ctx, `SELECT repo_id,generation_id FROM active_generations ORDER BY repo_id`)
	if err != nil {
		return out, err
	}
	for rows.Next() {
		var id, g string
		if err = rows.Scan(&id, &g); err != nil {
			rows.Close()
			return out, err
		}
		out.ActiveGenerations[id] = g
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return out, err
	}
	return out, tx.Commit()
}

// MigrateUpgradeState operates only on a private staged copy. Current builders
// are retained when their version, schema and provenance remain compatible.
func MigrateUpgradeState(ctx context.Context, data string) (UpgradeState, error) {
	before, err := ReadUpgradeState(ctx, data)
	if err != nil {
		return before, err
	}
	db, err := OpenWriter(data)
	if err != nil {
		return before, err
	}
	defer func() {
		if db != nil {
			db.Close()
		}
	}()
	var changed []string
	var catalogs int
	if err = db.db.QueryRowContext(ctx, `SELECT count(*) FROM active_catalog_revision`).Scan(&catalogs); err != nil {
		return before, err
	}
	if catalogs > 0 {
		for _, kind := range requiredProjectionKinds {
			var count int
			if err = db.db.QueryRowContext(ctx, `SELECT count(*) FROM active_projection_builds a JOIN projection_builds p ON p.projection_build_id=a.projection_build_id JOIN active_catalog_revision c ON c.catalog_revision_id=p.catalog_revision_id WHERE a.projection_kind=? AND p.state='ready' AND p.builder_version=? AND p.projection_schema_version='v1'`, kind, projectionBuilderVersion(kind)).Scan(&count); err != nil {
				return before, err
			}
			if count != 1 {
				changed = append(changed, kind)
			}
		}
		if len(changed) > 0 {
			if err = db.RebuildProjections(ctx, changed); err != nil {
				return before, err
			}
		}
		problems, e := db.ValidateActiveProjectionProvenance(ctx)
		if e != nil {
			return before, e
		}
		if len(problems) > 0 {
			return before, fmt.Errorf("staged upgrade projections failed provenance validation")
		}
	}
	if err = db.Close(); err != nil {
		return before, err
	}
	db = nil
	after, err := ReadUpgradeState(ctx, data)
	if err != nil {
		return after, err
	}
	if after.CanonicalFingerprint != before.CanonicalFingerprint {
		return after, fmt.Errorf("migration changed canonical identities or evidence")
	}
	after.RebuiltProjections = changed
	return after, nil
}
