package store

import (
	"context"
	"database/sql"
	"strings"
	"testing"
	"time"

	"github.com/AdamNi-7080/AIOS/internal/model"
	"github.com/AdamNi-7080/AIOS/internal/observability"
)

func TestLegacyOperationalLedgerScrubPreservesCanonicalEvidence(t *testing.T) {
	data := t.TempDir()
	db, err := OpenWriter(data)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	secret := "sk-test-PLANTED_SECRET_936"
	file := model.File{RepoID: "repo", Path: "src/Worker.java", SHA256: "fixture", Size: int64(len(secret)), Language: "java", Classification: "source", Content: secret}
	snapshot := model.Snapshot{RepoID: "repo", Root: "/Users/planted-private/source", Git: model.GitState{Commit: "commit"}, ContentHash: "fixture", FileCount: 1, TotalBytes: file.Size, IndexedAt: time.Now(), ExtractorVersions: "fixture", CompilerDiagnostics: []model.CompilerDiagnostic{{Language: "java", Code: "legacy", Message: "source diagnosis", Path: file.Path}}}
	coverage := model.CoverageReport{Entries: []model.CoverageEntry{{Path: file.Path, Language: "java", Classification: "source", Outcome: "included", Capability: "lexical"}}}
	g, err := db.StageGenerationWithCoverage(ctx, snapshot, []model.File{file}, nil, nil, coverage)
	if err != nil {
		t.Fatal(err)
	}
	if err = db.ActivateGeneration(ctx, g.ID); err != nil {
		t.Fatal(err)
	}
	_, err = db.DB().Exec(`INSERT INTO diagnostic_events VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`, "diag-h_"+secret, "EXTRACTION_FAILURE", "warning", time.Now().UTC().Format(time.RFC3339Nano), "repo", "revision", "generation", "/Users/planted-private/source", "lookup", "query-text", secret, "repair "+secret, `{"language":"java","build":"`+secret+`"}`, "", "resolved "+secret)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.DB().Exec(`INSERT INTO ingestion_queues(repository_id,state,failure_diagnostic) VALUES('repo','failed',?)`, secret)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.DB().Exec(`INSERT INTO ingestion_events(event_id,repository_id,event_type,manifest_fingerprint,timestamp,attempt,status,failure_diagnostic) VALUES('legacy-event','repo','failed','manifest',?,1,'failed',?)`, time.Now().UTC().Format(time.RFC3339Nano), secret)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.DB().Exec(`DELETE FROM schema_metadata WHERE key='diagnostic_policy'`)
	if err != nil {
		t.Fatal(err)
	}
	if err = db.Close(); err != nil {
		t.Fatal(err)
	}
	before, err := ReadUpgradeState(ctx, data)
	if err != nil {
		t.Fatal(err)
	}
	db, err = OpenWriter(data)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	after, err := ReadUpgradeState(ctx, data)
	if err != nil {
		t.Fatal(err)
	}
	if before.CanonicalFingerprint != after.CanonicalFingerprint {
		t.Fatal("canonical fingerprint changed during operational cleanup")
	}
	for _, table := range []string{"source_files", "compiler_diagnostics", "coverage_runs", "coverage_entries"} {
		var count int
		if err = db.DB().QueryRow(`SELECT count(*) FROM ` + table).Scan(&count); err != nil || count == 0 {
			t.Fatalf("canonical %s lost: %d %v", table, count, err)
		}
	}
	events, err := db.DiagnosticEvents(ctx, "repo", true, 10)
	if err != nil || len(events) < 2 || events[0].Scope.Repository != observability.Opaque("repo") {
		t.Fatalf("events=%+v err=%v", events, err)
	}
	var ledger string
	if err = db.DB().QueryRow(`SELECT diagnostic_id||code||repository_id||path||query_fingerprint||metadata_json||remediation||resolution FROM diagnostic_events LIMIT 1`).Scan(&ledger); err != nil {
		t.Fatal(err)
	}
	var failure string
	if err = db.DB().QueryRow(`SELECT failure_diagnostic FROM ingestion_queues LIMIT 1`).Scan(&failure); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(ledger, secret) || strings.Contains(ledger, "/Users/planted") || strings.Contains(failure, secret) || failure != "operation_failed" {
		t.Fatal("legacy operational content remained")
	}
}

func TestFreshOperationalIDThatLooksOpaqueIsHashed(t *testing.T) {
	db, err := OpenWriter(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	secret := "sk-test-PLANTED_ID_SECRET_836"
	provided := "diag-h_" + secret
	event, err := db.RecordDiagnostic(context.Background(), model.DiagnosticEvent{
		ID: provided, Code: DiagnosticIngestionFailure, Severity: model.DiagnosticWarning,
	})
	if err != nil {
		t.Fatal(err)
	}
	if event.ID != "diag-"+observability.Opaque(provided) {
		t.Fatalf("untrusted ID was not hashed: %q", event.ID)
	}
	var persisted string
	if err := db.DB().QueryRow(`SELECT diagnostic_id FROM diagnostic_events WHERE diagnostic_id=?`, event.ID).Scan(&persisted); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(persisted, secret) {
		t.Fatal("planted ID secret persisted")
	}
}

func TestBusyOperationalCheckpointKeepsPendingRetry(t *testing.T) {
	data := t.TempDir()
	db, err := OpenWriter(data)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ctx := context.Background()
	var mode string
	if err = db.DB().QueryRow(`PRAGMA journal_mode=WAL`).Scan(&mode); err != nil || mode != "wal" {
		t.Fatalf("WAL unavailable: %q %v", mode, err)
	}
	reader, err := sql.Open("sqlite", sqliteDSN(db.Path(), true))
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	readTx, err := reader.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	var count int
	if err = readTx.QueryRowContext(ctx, `SELECT count(*) FROM schema_metadata`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if _, err = db.DB().Exec(`DELETE FROM schema_metadata WHERE key='diagnostic_policy'`); err != nil {
		t.Fatal(err)
	}
	err = scrubOperationalLedger(ctx, db.DB())
	if err == nil {
		t.Fatal("busy checkpoint was marked current")
	}
	var policy string
	if err = db.DB().QueryRow(`SELECT value FROM schema_metadata WHERE key='diagnostic_policy'`).Scan(&policy); err != nil || policy != "pending" {
		t.Fatalf("policy=%q err=%v", policy, err)
	}
	if err = readTx.Rollback(); err != nil {
		t.Fatal(err)
	}
	if err = scrubOperationalLedger(ctx, db.DB()); err != nil {
		t.Fatal(err)
	}
	if err = db.DB().QueryRow(`SELECT value FROM schema_metadata WHERE key='diagnostic_policy'`).Scan(&policy); err != nil || policy != "redacted_v1" {
		t.Fatalf("policy=%q err=%v", policy, err)
	}
}
