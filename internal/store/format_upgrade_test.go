package store

import (
	"context"
	"database/sql"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func populatedV9(t *testing.T) (string, Generation) {
	t.Helper()
	data := t.TempDir()
	if e := os.Chmod(data, 0700); e != nil {
		t.Fatal(e)
	}
	path := filepath.Join(data, DatabaseName)
	if e := prepareDatabaseFile(path); e != nil {
		t.Fatal(e)
	}
	db, e := sql.Open("sqlite", path)
	if e != nil {
		t.Fatal(e)
	}
	oldSchema := strings.Replace(schema, "knowledge-ir-v10", "knowledge-ir-v9", 1)
	oldSchema = strings.Replace(oldSchema, "extractor_versions TEXT NOT NULL, indexed_at TEXT NOT NULL);", "extractor_versions TEXT NOT NULL, indexed_at TEXT NOT NULL, UNIQUE(repository_id,content_hash,extractor_versions));", 1)
	if _, e = db.Exec(oldSchema); e != nil {
		t.Fatal(e)
	}
	if _, e = db.Exec(`PRAGMA foreign_keys=ON`); e != nil {
		t.Fatal(e)
	}
	s := &Store{db: db, path: path}
	g := canonicalFixture(t, s, "original")
	s.Close()
	return data, g
}

func TestMaintainedV9UpgradePreservesEvidenceAndVerifiedOwnerOnlyBackup(t *testing.T) {
	data, g := populatedV9(t)
	db, e := OpenWriter(data)
	if e != nil {
		t.Fatal(e)
	}
	defer db.Close()
	if e = verifyFormat(context.Background(), db.DB()); e != nil {
		t.Fatal(e)
	}
	active, e := db.ActiveGeneration(context.Background(), "repo")
	if e != nil || active.ID != g.ID {
		t.Fatal("migration replaced prior active generation")
	}
	var count int
	if e = db.DB().QueryRow(`SELECT count(*) FROM ir_fact_observations WHERE revision_id=?`, g.ID).Scan(&count); e != nil || count < 1 {
		t.Fatal("migration lost canonical facts")
	}
	backups, e := filepath.Glob(filepath.Join(data, "index.ir-9-backup-*.db"))
	if e != nil || len(backups) != 1 {
		t.Fatal("verified backup absent")
	}
	info, e := os.Stat(backups[0])
	if e != nil || info.Mode().Perm() != 0600 {
		t.Fatal("backup not owner only")
	}
	backup, e := sql.Open("sqlite", "file:"+url.PathEscape(backups[0])+"?mode=ro")
	if e != nil {
		t.Fatal(e)
	}
	defer backup.Close()
	var f, id string
	if e = backup.QueryRow(`SELECT value FROM schema_metadata WHERE key='format'`).Scan(&f); e != nil || f != "knowledge-ir-v9" {
		t.Fatal("backup is not prior format")
	}
	if e = backup.QueryRow(`SELECT generation_id FROM active_generations WHERE repo_id='repo'`).Scan(&id); e != nil || id != g.ID {
		t.Fatal("backup lost active knowledge")
	}
	second := canonicalFixture(t, db, "original")
	if second.ID == g.ID {
		t.Fatal("identical content did not retain distinct capture epoch")
	}
}

func TestMaintainedV9MigrationFailureRollsBackWithoutRetractingKnowledge(t *testing.T) {
	data, g := populatedV9(t)
	path := filepath.Join(data, DatabaseName)
	sqlDB, e := sql.Open("sqlite", path)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = sqlDB.Exec(`CREATE TRIGGER reject_format BEFORE UPDATE ON schema_metadata BEGIN SELECT RAISE(ABORT,'injected migration fault'); END;`); e != nil {
		t.Fatal(e)
	}
	sqlDB.Close()
	if db, e := OpenWriter(data); e == nil {
		db.Close()
		t.Fatal("failed migration reported success")
	}
	old, e := sql.Open("sqlite", "file:"+url.PathEscape(path)+"?mode=ro")
	if e != nil {
		t.Fatal(e)
	}
	defer old.Close()
	var f, id string
	if e = old.QueryRow(`SELECT value FROM schema_metadata WHERE key='format'`).Scan(&f); e != nil || f != "knowledge-ir-v9" {
		t.Fatal("migration failure changed source format")
	}
	if e = old.QueryRow(`SELECT generation_id FROM active_generations WHERE repo_id='repo'`).Scan(&id); e != nil || id != g.ID {
		t.Fatal("migration failure retracted last-good knowledge")
	}
	var extras int
	if e = old.QueryRow(`SELECT count(*) FROM sqlite_master WHERE name='ir_source_revisions_next'`).Scan(&extras); e != nil || extras != 0 {
		t.Fatal("migration transaction leaked temporary table")
	}
	backups, e := filepath.Glob(filepath.Join(data, "index.ir-9-backup-*.db"))
	if e != nil || len(backups) != 1 {
		t.Fatal("failure did not preserve recovery backup")
	}
}
