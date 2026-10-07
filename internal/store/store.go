package store

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/AdamNi-7080/AIOS/internal/model"
	"github.com/AdamNi-7080/AIOS/internal/semantic"
	_ "modernc.org/sqlite"
)

const DatabaseName = "index.db"
const format = "knowledge-ir-v10"

// Every pooled connection waits for short canonical read/write leases. SQLite's
// busy handler may finish after context cancellation; read waits are only 100ms
// to preserve query budgets, while writers may wait up to five seconds.
func sqliteDSN(path string, readOnly bool) string {
	query := url.Values{"_pragma": {"busy_timeout(5000)"}}
	if readOnly {
		query.Set("mode", "ro")
		query.Set("_pragma", "busy_timeout(100)")
	}
	return "file:" + url.PathEscape(path) + "?" + query.Encode()
}

//go:embed schema.sql
var schema string

type Store struct {
	db              *sql.DB
	readOnly        bool
	path            string
	lock            *writerLock
	stageFault      func() error // package-local write-boundary fault seam
	activationFault func() error
}
type Generation struct {
	ID, RepoID, ContentHash string
	CreatedAt, ActivatedAt  time.Time
}

func (s *Store) GenerationByID(ctx context.Context, id string) (Generation, error) {
	var g Generation
	var created, activated string
	err := s.db.QueryRowContext(ctx, `SELECT generation_id,repo_id,content_hash,created_at,activated_at FROM generations WHERE generation_id=? UNION ALL SELECT generation_id,repo_id,content_hash,created_at,'' FROM generation_staging WHERE generation_id=?`, id, id).Scan(&g.ID, &g.RepoID, &g.ContentHash, &created, &activated)
	if err != nil {
		return g, err
	}
	g.CreatedAt, _ = time.Parse(time.RFC3339Nano, created)
	g.ActivatedAt, _ = time.Parse(time.RFC3339Nano, activated)
	return g, nil
}

type CatalogRevision struct {
	ID                     string
	CreatedAt, ActivatedAt time.Time
}
type Entity struct{ ID, GenerationID, RepoID, Kind, Label, Identity, Path, Language, Extractor, EvidenceID string }
type Source struct {
	ID, GenerationID, RepoID, Path, SHA256, Language, Classification, Content string
	Size                                                                      int64
}
type Evidence struct {
	ID, GenerationID, SourceID, RepoID, Path, SHA256, Excerpt string
	SourceKind, SourceAdapterVersion                          string
	Span                                                      model.Span
}
type Claim struct {
	ID, GenerationID, SubjectID, Predicate, ObjectID, EvidenceID, Extractor, Derivation string
	Confidence                                                                          float64
}
type CoverageRun struct {
	ID, GenerationID, RepositoryID, SourceRevision, SourceFingerprint, ExtractorVersions, Status, CompletedAt, Diagnostic string
}
type CoverageEntry struct {
	Path, Language, Classification, Outcome, Reason, Capability, Diagnostic string
}
type Status struct {
	ActiveCatalog        string                  `json:"active_catalog_revision,omitempty"`
	ActiveSourceKinds    []string                `json:"active_source_kinds"`
	SupportedAdapters    map[string]string       `json:"supported_source_adapters"`
	DisabledCapabilities map[string]string       `json:"disabled_capabilities"`
	Snapshots            []model.Snapshot        `json:"snapshots"`
	PublishedGenerations int                     `json:"published_generations"`
	StagedGenerations    int                     `json:"staged_generations"`
	IndexBytes           int64                   `json:"index_bytes"`
	Invalidations        map[string]int          `json:"invalidation_counts,omitempty"`
	Projections          []ProjectionStatus      `json:"projections,omitempty"`
	Ingestion            []IngestionQueueStatus  `json:"ingestion,omitempty"`
	IngestionEvents      []IngestionEvent        `json:"ingestion_events,omitempty"`
	Cache                any                     `json:"cache,omitempty"`
	Planner              map[string]int          `json:"planner,omitempty"`
	Diagnostics          []model.DiagnosticEvent `json:"diagnostics,omitempty"`
	Health               ProjectionHealth        `json:"health"`
	Capabilities         map[string]any          `json:"capabilities"`
}

const (
	DiagnosticIngestionFailure      = "INGESTION_FAILURE"
	DiagnosticExtractionFailure     = "EXTRACTION_FAILURE"
	DiagnosticUnsupportedAnalysis   = "UNSUPPORTED_ANALYSIS"
	DiagnosticProjectionFailure     = "PROJECTION_FAILURE"
	DiagnosticProjectionMismatch    = "PROJECTION_MISMATCH"
	DiagnosticDanglingProvenance    = "DANGLING_PROVENANCE"
	DiagnosticCacheMismatch         = "CACHE_MISMATCH"
	DiagnosticPlannerFallback       = "PLANNER_FALLBACK"
	DiagnosticPlannerBudgetExceeded = "PLANNER_BUDGET_EXHAUSTED"
)

type IngestionQueueStatus struct {
	RepositoryID, CurrentRevision, PendingRevision, ManifestFingerprint, State, FailureDiagnostic string
	Attempt                                                                                       int `json:"attempt"`
	SelectedAt, ActivatedAt, CompletedAt                                                          string
}
type IngestionEvent struct {
	EventID, RepositoryID, Type, SourceRevision, TargetRevision, ManifestFingerprint, Timestamp, Status, FailureDiagnostic string
	Attempt                                                                                                                int
}

const (
	EventMirrorRevisionDiscovered = "mirror_revision_discovered"
	EventRevisionSelected         = "revision_selected"
	EventSourceDeltaCalculated    = "source_delta_calculated"
	EventIRDeltaStaged            = "ir_delta_staged"
	EventIRGenerationActivated    = "ir_generation_activated"
	EventProjectionRequested      = "projection_rebuild_requested"
	EventProjectionCompleted      = "projection_rebuild_completed"
	EventProjectionFailed         = "projection_rebuild_failed"
)

type ProjectionStatus struct {
	Kind              string         `json:"kind"`
	SchemaVersion     string         `json:"schema_version"`
	CatalogRevision   string         `json:"catalog_revision"`
	SourceGeneration  string         `json:"source_generation"`
	Builder           string         `json:"builder"`
	BuilderVersion    string         `json:"builder_version"`
	InputFingerprint  string         `json:"input_fingerprint"`
	Fingerprint       string         `json:"fingerprint"`
	State             string         `json:"state"`
	CreatedAt         string         `json:"created_at"`
	ActivatedAt       string         `json:"activated_at,omitempty"`
	RebuildReason     string         `json:"rebuild_reason"`
	RecordCounts      map[string]any `json:"record_counts"`
	ValidationSummary map[string]any `json:"validation_summary"`
}

type ProjectionHealth struct {
	Healthy      bool                    `json:"healthy"`
	Catalog      string                  `json:"catalog_revision,omitempty"`
	Diagnostics  []model.DiagnosticEvent `json:"diagnostics,omitempty"`
	SourceFresh  bool                    `json:"source_fresh"`
	CoverageRuns int                     `json:"coverage_runs"`
}

var requiredProjectionKinds = []string{"lookup", "lexical", "graph", "path", "ui", "cache", "landmarks"}

// ReplaceApprovedOwnership persists only validated catalog assertions. It is
// deliberately separate from extracted IR: absence remains an explicit unknown.
func (s *Store) ReplaceApprovedOwnership(ctx context.Context, repositories []model.Repository) error {
	if s.readOnly {
		return fmt.Errorf("store is read-only")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, `DELETE FROM approved_ownership`); err != nil {
		return err
	}
	for _, repository := range repositories {
		for _, ownership := range repository.Ownership {
			if _, err = tx.ExecContext(ctx, `INSERT INTO approved_ownership VALUES(?,?,?)`, repository.ID, ownership.Coordinate, ownership.Owner); err != nil {
				return err
			}
		}
	}
	return tx.Commit()
}

// ActiveFiles returns the prior manifest and source content from the
// authoritative derived store. It never reads a repository input.
func (s *Store) ActiveFiles(ctx context.Context, repo string) ([]model.File, error) {
	g, err := s.ActiveGeneration(ctx, repo)
	if err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx, `SELECT repo_id,path,sha256,size,language,classification,content FROM source_files WHERE generation_id=? ORDER BY path`, g.ID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []model.File
	for rows.Next() {
		var f model.File
		if err := rows.Scan(&f.RepoID, &f.Path, &f.SHA256, &f.Size, &f.Language, &f.Classification, &f.Content); err != nil {
			return nil, err
		}
		out = append(out, f)
	}
	return out, rows.Err()
}

// PreviousInputs reconstructs extraction inputs for unchanged files. Published
// generations are immutable, so the next generation materializes fresh IDs
// while retaining source-valid facts without reparsing those files.
func (s *Store) PreviousInputs(ctx context.Context, repo string) ([]model.Symbol, []model.Edge, error) {
	g, err := s.ActiveGeneration(ctx, repo)
	if err != nil {
		return nil, nil, err
	}
	syms, err := s.db.QueryContext(ctx, `SELECT e.repo_id,e.path,e.label,substr(e.kind,8),e.identity,v.start_byte,v.end_byte,v.start_line,v.start_column,v.end_line,v.end_column,e.extractor,1 FROM entities e JOIN evidence v ON v.evidence_id=e.evidence_id WHERE e.generation_id=? AND e.kind LIKE 'symbol:%' ORDER BY e.path,e.entity_id`, g.ID)
	if err != nil {
		return nil, nil, err
	}
	defer syms.Close()
	var symbols []model.Symbol
	for syms.Next() {
		var x model.Symbol
		if err := syms.Scan(&x.RepoID, &x.Path, &x.Name, &x.Kind, &x.Identity, &x.Span.StartByte, &x.Span.EndByte, &x.Span.StartLine, &x.Span.StartColumn, &x.Span.EndLine, &x.Span.EndColumn, &x.Extractor, &x.Confidence); err != nil {
			return nil, nil, err
		}
		symbols = append(symbols, x)
	}
	if err := syms.Err(); err != nil {
		return nil, nil, err
	}
	edges, err := s.db.QueryContext(ctx, `SELECT e.repo_id,v.path,s.label,o.label,s.identity,o.identity,c.predicate,v.start_byte,v.end_byte,v.start_line,v.start_column,v.end_line,v.end_column,c.extractor,c.derivation,c.confidence FROM claims c JOIN entities s ON s.entity_id=c.subject_id JOIN entities o ON o.entity_id=c.object_id JOIN evidence v ON v.evidence_id=c.evidence_id JOIN generations e ON e.generation_id=c.generation_id WHERE c.generation_id=? ORDER BY v.path,c.claim_id`, g.ID)
	if err != nil {
		return nil, nil, err
	}
	defer edges.Close()
	var out []model.Edge
	for edges.Next() {
		var x model.Edge
		if err := edges.Scan(&x.RepoID, &x.Path, &x.Source, &x.Target, &x.SourceIdentity, &x.TargetIdentity, &x.Kind, &x.Span.StartByte, &x.Span.EndByte, &x.Span.StartLine, &x.Span.StartColumn, &x.Span.EndLine, &x.Span.EndColumn, &x.Resolver, &x.Derivation, &x.Confidence); err != nil {
			return nil, nil, err
		}
		out = append(out, x)
	}
	return symbols, out, edges.Err()
}

func OpenWriter(dataDir string) (*Store, error) {
	abs, err := canonicalWriterPath(dataDir)
	if err != nil {
		return nil, err
	}
	guard, err := acquireDataAuthority(abs)
	if err != nil {
		return nil, err
	}
	if err = checkTransactionFence(abs); err != nil {
		guard.close()
		return nil, err
	}
	if err = os.MkdirAll(abs, 0700); err != nil {
		guard.close()
		return nil, err
	}
	if err = validateDataDirectory(abs); err != nil {
		guard.close()
		return nil, err
	}
	lock, err := acquireWriterLock(filepath.Join(abs, writerLockName))
	if err != nil {
		guard.close()
		return nil, err
	}
	lock.guard = guard
	path := filepath.Join(abs, DatabaseName)
	exists, err := databaseExists(path)
	if err != nil {
		lock.close()
		return nil, err
	}
	if exists {
		if err = prepareDatabaseFile(path); err == nil {
			err = enforceDatabasePermissions(path)
		}
		if err != nil {
			lock.close()
			return nil, err
		}
		db, openErr := sql.Open("sqlite", sqliteDSN(path, false))
		if openErr == nil {
			db.SetMaxOpenConns(1)
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			openErr = upgradeMaintainedFormat(ctx, db, path)
			cancel()
		}
		if openErr != nil {
			if db != nil {
				_ = db.Close()
			}
			lock.close()
			if errors.Is(openErr, errUnsupportedCanonicalFormat) {
				return nil, fmt.Errorf("existing derived database requires a clean IR reindex: rerun index with --reset-derived-data")
			}
			return nil, fmt.Errorf("unable to open or safely upgrade canonical knowledge: %w", openErr)
		}
		db.SetMaxOpenConns(1)
		if _, openErr = db.Exec(`PRAGMA foreign_keys=ON`); openErr != nil {
			_ = db.Close()
			lock.close()
			return nil, openErr
		}
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		openErr = scrubOperationalLedger(ctx, db)
		cancel()
		if openErr != nil {
			_ = db.Close()
			lock.close()
			return nil, fmt.Errorf("operational diagnostic recovery: %w", openErr)
		}
		return &Store{db: db, path: path, lock: lock}, nil
	}
	if err = prepareDatabaseFile(path); err != nil {
		lock.close()
		return nil, err
	}
	db, err := sql.Open("sqlite", sqliteDSN(path, false))
	if err != nil {
		lock.close()
		return nil, err
	}
	db.SetMaxOpenConns(1)
	if _, err = db.Exec(`PRAGMA foreign_keys=ON`); err == nil {
		_, err = db.Exec(schema)
	}
	if err == nil {
		err = verifyFormat(context.Background(), db)
	}
	if err == nil {
		err = enforceDatabasePermissions(path)
	}
	if err != nil {
		db.Close()
		lock.close()
		return nil, fmt.Errorf("initialize canonical V1 database: %w", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	err = scrubOperationalLedger(ctx, db)
	cancel()
	if err != nil {
		_ = db.Close()
		lock.close()
		return nil, fmt.Errorf("operational diagnostic initialization: %w", err)
	}
	return &Store{db: db, path: path, lock: lock}, nil
}

// ResetDerivedData removes only this agent's validated SQLite database and
// SQLite sidecars. It intentionally never follows symlinks or removes a
// repository input. The persistent writer lock is retained so locking cannot
// be bypassed while a reset is in progress.
func ResetDerivedData(dataDir string) error {
	abs, err := canonicalWriterPath(dataDir)
	if err != nil {
		return err
	}
	guard, err := acquireDataAuthority(abs)
	if err != nil {
		return err
	}
	defer guard.close()
	if err = checkTransactionFence(abs); err != nil {
		return err
	}
	if err = validateDataDirectory(abs); err != nil {
		return err
	}
	lock, err := acquireWriterLock(filepath.Join(abs, writerLockName))
	if err != nil {
		return err
	}
	defer lock.close()
	for _, name := range []string{DatabaseName, DatabaseName + "-journal", DatabaseName + "-shm", DatabaseName + "-wal"} {
		path := filepath.Join(abs, name)
		info, statErr := os.Lstat(path)
		if errors.Is(statErr, os.ErrNotExist) {
			continue
		}
		if statErr != nil {
			return fmt.Errorf("inspect derived data %s: %w", name, statErr)
		}
		if err = validateOwnedRegularFile(path, info); err != nil {
			return fmt.Errorf("derived data must be an owned regular file: %s", name)
		}
		if err = os.Remove(path); err != nil {
			return fmt.Errorf("remove derived data %s: %w", name, err)
		}
	}
	return nil
}
func OpenReadOnly(dataDir string) (*Store, error) {
	path, err := validateReadOnlyDatabase(dataDir)
	if err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite", sqliteDSN(path, true))
	if err != nil {
		return nil, err
	}
	if err = verifyFormat(context.Background(), db); err != nil {
		db.Close()
		return nil, fmt.Errorf("repository knowledge database requires a clean V1 reindex: %w", err)
	}
	return &Store{db: db, path: path, readOnly: true}, nil
}
func databaseExists(path string) (bool, error) {
	_, err := os.Lstat(path)
	if os.IsNotExist(err) {
		return false, nil
	}
	return err == nil, err
}
func verifyFormat(ctx context.Context, db *sql.DB) error {
	var got string
	if err := db.QueryRowContext(ctx, `SELECT value FROM schema_metadata WHERE key='format'`).Scan(&got); err != nil {
		return err
	}
	if got != format {
		return fmt.Errorf("unsupported schema format %q", got)
	}
	return nil
}
func (s *Store) Close() error {
	var e error
	if s.db != nil {
		e = s.db.Close()
	}
	if l := s.lock.close(); e == nil {
		e = l
	}
	return e
}
func (s *Store) DB() *sql.DB { return s.db }

// QueryCanonical is the read boundary for repository-knowledge consumers.
// Only V1 canonical tables are present in this database.
func (s *Store) QueryCanonical(ctx context.Context, query string, args ...any) (*sql.Rows, error) {
	return s.db.QueryContext(ctx, query, args...)
}

func (s *Store) QueryRowCanonical(ctx context.Context, query string, args ...any) *sql.Row {
	return s.db.QueryRowContext(ctx, query, args...)
}
func (s *Store) Path() string { return s.path }

func (s *Store) ReplaceRepository(ctx context.Context, snap model.Snapshot, files []model.File, symbols []model.Symbol, edges []model.Edge) error {
	if s.readOnly {
		return fmt.Errorf("store is read-only")
	}
	g, err := s.StageGeneration(ctx, snap, files, symbols, edges)
	if err != nil {
		return err
	}
	return s.ActivateGeneration(ctx, g.ID)
}

func (s *Store) ReplaceRepositoryWithCoverage(ctx context.Context, snap model.Snapshot, files []model.File, symbols []model.Symbol, edges []model.Edge, report model.CoverageReport) error {
	if s.readOnly {
		return fmt.Errorf("store is read-only")
	}
	g, err := s.StageGenerationWithCoverage(ctx, snap, files, symbols, edges, report)
	if err != nil {
		return err
	}
	return s.ActivateGeneration(ctx, g.ID)
}
func (s *Store) StageGeneration(ctx context.Context, snap model.Snapshot, files []model.File, symbols []model.Symbol, edges []model.Edge) (Generation, error) {
	report := model.CoverageReport{}
	for _, f := range files {
		capability := "lexical"
		if f.Language == "java" || f.Language == "typescript" || f.Language == "tsx" || f.Language == "javascript" || f.Language == "jsx" || f.Language == "kotlin" {
			capability = "lexical,structural"
		}
		report.Entries = append(report.Entries, model.CoverageEntry{Path: f.Path, Language: f.Language, Classification: f.Classification, Outcome: "included", Capability: capability})
	}
	return s.StageGenerationWithCoverage(ctx, snap, files, symbols, edges, report)
}

// DiscardStagedRepository removes only unpublished work for one repository.
// Active and historical canonical generations are never affected.
func (s *Store) DiscardStagedRepository(ctx context.Context, repositoryID string) error {
	if s.readOnly {
		return fmt.Errorf("store is read-only")
	}
	if repositoryID == "" {
		return fmt.Errorf("repository id is required")
	}
	_, err := s.db.ExecContext(ctx, `DELETE FROM generation_staging WHERE repo_id=?`, repositoryID)
	return err
}

func (s *Store) StageGenerationWithCoverage(ctx context.Context, snap model.Snapshot, files []model.File, symbols []model.Symbol, edges []model.Edge, report model.CoverageReport) (Generation, error) {
	if s.readOnly {
		return Generation{}, fmt.Errorf("store is read-only")
	}
	if snap.RepoID == "" {
		return Generation{}, fmt.Errorf("repository id is required")
	}
	id, err := newGenerationID()
	if err != nil {
		return Generation{}, err
	}
	now := time.Now().UTC()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Generation{}, err
	}
	defer tx.Rollback()
	if snap.Source.Kind == "" {
		snap.Source = model.SourceIdentity{ID: snap.RepoID, Kind: model.SourceKindRepository, AdapterVersion: model.RepositoryAdapterVersion}
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO generation_staging VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`, id, snap.RepoID, snap.Root, snap.Git.Commit, snap.Git.Branch, snap.Git.Dirty, snap.Git.UntrackedCount, snap.ContentHash, snap.FileCount, snap.TotalBytes, snap.IndexedAt.UTC().Format(time.RFC3339Nano), snap.ExtractorVersions, now.Format(time.RFC3339Nano), snap.Source.Kind, snap.Source.AdapterVersion)
	if err != nil {
		return Generation{}, err
	}
	for _, f := range files {
		if f.RepoID != snap.RepoID {
			return Generation{}, fmt.Errorf("file belongs to another repository")
		}
		sid := sourceID(id, f.Path, f.SHA256)
		_, err = tx.ExecContext(ctx, `INSERT INTO staged_source_files VALUES(?,?,?,?,?,?,?,?,?)`, id, sid, f.RepoID, f.Path, f.SHA256, f.Size, f.Language, f.Classification, f.Content)
		if err != nil {
			return Generation{}, err
		}
	}
	for _, d := range snap.CompilerDiagnostics {
		if _, err = tx.ExecContext(ctx, `INSERT INTO staged_compiler_diagnostics VALUES(?,?,?,?,?)`, id, d.Language, d.Code, d.Message, d.Path); err != nil {
			return Generation{}, err
		}
	}
	coverageID := coverageID(id, snap.ContentHash, snap.ExtractorVersions)
	status, diagnostic := "complete", ""
	for _, entry := range report.Entries {
		if entry.Outcome != "included" {
			status = "incomplete"
		}
		if diagnostic == "" && entry.Diagnostic != "" {
			diagnostic = entry.Diagnostic
		}
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO staged_coverage_runs VALUES(?,?,?,?,?,?,?,?,?)`, coverageID, id, snap.RepoID, snap.Git.Commit, snap.ContentHash, snap.ExtractorVersions, status, now.Format(time.RFC3339Nano), diagnostic); err != nil {
		return Generation{}, err
	}
	for _, entry := range report.Entries {
		if entry.Outcome == "" {
			return Generation{}, fmt.Errorf("coverage outcome is required")
		}
		if _, err = tx.ExecContext(ctx, `INSERT INTO staged_coverage_entries VALUES(?,?,?,?,?,?,?,?)`, coverageID, entry.Path, entry.Language, entry.Classification, entry.Outcome, entry.Reason, entry.Capability, entry.Diagnostic); err != nil {
			return Generation{}, err
		}
	}
	for _, x := range symbols {
		if x.RepoID != snap.RepoID {
			return Generation{}, fmt.Errorf("symbol belongs to another repository")
		}
		_, err = tx.ExecContext(ctx, `INSERT INTO staged_symbols VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?)`, id, x.RepoID, x.Path, x.Name, x.Kind, x.Identity, x.Span.StartByte, x.Span.EndByte, x.Span.StartLine, x.Span.StartColumn, x.Span.EndLine, x.Span.EndColumn, x.Extractor, x.Confidence)
		if err != nil {
			return Generation{}, err
		}
	}
	for _, x := range edges {
		if x.RepoID != snap.RepoID {
			return Generation{}, fmt.Errorf("edge belongs to another repository")
		}
		derivation := x.Derivation
		if derivation == "" {
			derivation = x.Resolver
		}
		if derivation == "" {
			derivation = "syntax_derived"
		}
		_, err = tx.ExecContext(ctx, `INSERT INTO staged_edges VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`, id, x.RepoID, x.Path, x.Source, x.Target, x.SourceIdentity, x.TargetIdentity, x.Kind, x.Span.StartByte, x.Span.EndByte, x.Span.StartLine, x.Span.StartColumn, x.Span.EndLine, x.Span.EndColumn, x.Resolver, derivation, x.Confidence)
		if err != nil {
			return Generation{}, err
		}
	}
	if s.stageFault != nil {
		return Generation{}, s.stageFault()
	}
	if err = tx.Commit(); err != nil {
		return Generation{}, err
	}
	return Generation{ID: id, RepoID: snap.RepoID, ContentHash: snap.ContentHash, CreatedAt: now}, nil
}
func (s *Store) ActivateGeneration(ctx context.Context, id string) error {
	return s.ActivateCatalog(ctx, []string{id})
}

// ActivateCatalog makes a complete staged repository set visible in one
// transaction. Callers must have staged every repository they intend to query.
func (s *Store) ActivateCatalog(ctx context.Context, ids []string) error {
	return s.ActivateCatalogWithSelections(ctx, ids, nil)
}

// ActivateCatalogWithSelections commits canonical knowledge, projections and
// selected ingestion identities in one transaction, including crash recovery.
func (s *Store) ActivateCatalogWithSelections(ctx context.Context, ids []string, selections []ActivationSelection) error {
	if s.readOnly {
		return fmt.Errorf("store is read-only")
	}
	if len(ids) == 0 {
		return fmt.Errorf("catalog activation requires generations")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	now := time.Now().UTC().Format(time.RFC3339Nano)
	cr, err := newCatalogRevisionID()
	if err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO catalog_revisions VALUES(?,?,?)`, cr, now, now); err != nil {
		return err
	}
	seen := map[string]bool{}
	for _, id := range ids {
		var repo string
		isStaged := true
		if err = tx.QueryRowContext(ctx, `SELECT repo_id FROM generation_staging WHERE generation_id=?`, id).Scan(&repo); err == sql.ErrNoRows {
			isStaged = false
			err = tx.QueryRowContext(ctx, `SELECT g.repo_id FROM generations g JOIN active_generations a ON a.generation_id=g.generation_id WHERE g.generation_id=?`, id).Scan(&repo)
		}
		if err != nil {
			return fmt.Errorf("staged or active generation not found: %w", err)
		}
		if seen[repo] {
			return fmt.Errorf("catalog activation has duplicate repository %q", repo)
		}
		seen[repo] = true
		if isStaged {
			if _, err = tx.ExecContext(ctx, `INSERT INTO generations SELECT generation_id,repo_id,root,git_commit,branch,dirty,untracked_count,content_hash,file_count,total_bytes,indexed_at,extractor_versions,created_at,?,source_kind,source_adapter_version FROM generation_staging WHERE generation_id=?`, now, id); err != nil {
				return err
			}
			if _, err = tx.ExecContext(ctx, `INSERT OR IGNORE INTO ir_repositories VALUES(?,?)`, repo, now); err != nil {
				return err
			}
			if _, err = tx.ExecContext(ctx, `INSERT INTO ir_source_revisions(revision_id,repository_id,source_kind,source_adapter_version,git_commit,content_hash,extractor_versions,indexed_at) SELECT generation_id,repo_id,source_kind,source_adapter_version,git_commit,content_hash,extractor_versions,indexed_at FROM generations WHERE generation_id=?`, id); err != nil {
				return err
			}
			if _, err = tx.ExecContext(ctx, `INSERT INTO source_files SELECT source_id,generation_id,repo_id,path,sha256,size,language,classification,content FROM staged_source_files WHERE generation_id=?`, id); err != nil {
				return err
			}
			if _, err = tx.ExecContext(ctx, `INSERT INTO compiler_diagnostics SELECT generation_id,language,code,message,path FROM staged_compiler_diagnostics WHERE generation_id=?`, id); err != nil {
				return err
			}
			diagnosticRows, queryErr := tx.QueryContext(ctx, `SELECT language,code,path FROM staged_compiler_diagnostics WHERE generation_id=? ORDER BY language,code,path`, id)
			if queryErr != nil {
				return queryErr
			}
			for diagnosticRows.Next() {
				var language, code, path string
				if queryErr = diagnosticRows.Scan(&language, &code, &path); queryErr != nil {
					diagnosticRows.Close()
					return queryErr
				}
				if queryErr = recordDiagnosticTx(ctx, tx, model.DiagnosticEvent{Code: DiagnosticExtractionFailure, Severity: model.DiagnosticWarning, Scope: model.DiagnosticScope{Repository: repo, Generation: id, Path: path}, Remediation: "repair the extractor configuration or use supported analysis", Metadata: map[string]string{"language": language, "extractor_code": code}}); queryErr != nil {
					diagnosticRows.Close()
					return queryErr
				}
			}
			if queryErr = diagnosticRows.Err(); queryErr != nil {
				diagnosticRows.Close()
				return queryErr
			}
			diagnosticRows.Close()
			if _, err = tx.ExecContext(ctx, `INSERT INTO coverage_runs SELECT coverage_id,generation_id,repository_id,source_revision,source_fingerprint,extractor_versions,status,completed_at,diagnostic FROM staged_coverage_runs WHERE generation_id=?`, id); err != nil {
				return err
			}
			if _, err = tx.ExecContext(ctx, `INSERT INTO coverage_entries SELECT e.coverage_id,e.path,e.language,e.classification,e.outcome,e.reason,e.capability,e.diagnostic FROM staged_coverage_entries e JOIN staged_coverage_runs r ON r.coverage_id=e.coverage_id WHERE r.generation_id=?`, id); err != nil {
				return err
			}
			if _, err = tx.ExecContext(ctx, `INSERT INTO invalidations SELECT generation_id,kind,path,old_path,sha256 FROM staged_invalidations WHERE generation_id=?`, id); err != nil {
				return err
			}
			if err = materialize(ctx, tx, id, repo); err != nil {
				return err
			}
			if _, err = tx.ExecContext(ctx, `INSERT INTO source_fts SELECT source_id,generation_id,repo_id,path,content FROM source_files WHERE generation_id=?`, id); err != nil {
				return err
			}
			if err = materializeSearch(ctx, tx, id); err != nil {
				return err
			}
		}
		if _, err = tx.ExecContext(ctx, `INSERT INTO catalog_revision_members VALUES(?,?,?)`, cr, repo, id); err != nil {
			return err
		}
		if isStaged {
			if _, err = tx.ExecContext(ctx, `INSERT INTO active_generations VALUES(?,?) ON CONFLICT(repo_id) DO UPDATE SET generation_id=excluded.generation_id`, repo, id); err != nil {
				return err
			}
			if _, err = tx.ExecContext(ctx, `DELETE FROM generation_staging WHERE generation_id=?`, id); err != nil {
				return err
			}
		}
	}
	if err = linkCatalog(ctx, tx, cr); err != nil {
		return err
	}
	if err = buildProjections(ctx, tx, cr, "catalog_activation"); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO active_catalog_revision VALUES(1,?) ON CONFLICT(singleton) DO UPDATE SET catalog_revision_id=excluded.catalog_revision_id`, cr); err != nil {
		return err
	}
	if err = invalidateNegativeEvidenceTx(ctx, tx); err != nil {
		return err
	}
	if err = completeSelectionsTx(ctx, tx, selections); err != nil {
		return err
	}
	if s.activationFault != nil {
		if err := s.activationFault(); err != nil {
			return err
		}
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) StageInvalidations(ctx context.Context, generation string, changes []model.FileChange) error {
	if s.readOnly {
		return fmt.Errorf("store is read-only")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, c := range changes {
		if _, err = tx.ExecContext(ctx, `INSERT INTO staged_invalidations VALUES(?,?,?,?,?)`, generation, c.Kind, c.Path, c.OldPath, c.SHA256); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// Retain removes only orphaned, published derived generations. Repository
// paths are not arguments to this operation and are never removed.
func (s *Store) Retain(ctx context.Context, historical int) error {
	_, err := s.RetainPrunedRoots(ctx, historical)
	return err
}

// RetainPrunedRoots returns only source roots of published generations removed
// by this committed retention transaction. Callers may reclaim strictly owned
// snapshot directories after rechecking references; external roots are data.
func (s *Store) RetainPrunedRoots(ctx context.Context, historical int) ([]string, error) {
	if s.readOnly {
		return nil, fmt.Errorf("store is read-only")
	}
	if historical < 0 {
		return nil, fmt.Errorf("historical generations must be non-negative")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS snapshot_gc_candidates (root TEXT PRIMARY KEY)`); err != nil {
		return nil, err
	}
	rows, err := tx.QueryContext(ctx, `SELECT catalog_revision_id FROM catalog_revisions WHERE catalog_revision_id NOT IN (SELECT catalog_revision_id FROM active_catalog_revision) ORDER BY activated_at DESC`)
	if err != nil {
		return nil, err
	}
	var old []string
	for rows.Next() {
		var id string
		if err = rows.Scan(&id); err != nil {
			rows.Close()
			return nil, err
		}
		old = append(old, id)
	}
	rows.Close()
	if len(old) > historical {
		for _, id := range old[historical:] {
			if _, err = tx.ExecContext(ctx, `DELETE FROM catalog_revisions WHERE catalog_revision_id=?`, id); err != nil {
				return nil, err
			}
		}
	}
	rootRows, err := tx.QueryContext(ctx, `SELECT DISTINCT root FROM generations WHERE generation_id NOT IN (SELECT generation_id FROM active_generations) AND generation_id NOT IN (SELECT generation_id FROM catalog_revision_members) ORDER BY root`)
	if err != nil {
		return nil, err
	}
	var pruned []string
	for rootRows.Next() {
		var root string
		if err = rootRows.Scan(&root); err != nil {
			rootRows.Close()
			return nil, err
		}
		pruned = append(pruned, root)
	}
	if err = rootRows.Err(); err != nil {
		rootRows.Close()
		return nil, err
	}
	rootRows.Close()
	for _, root := range pruned {
		if _, err = tx.ExecContext(ctx, `INSERT OR IGNORE INTO snapshot_gc_candidates(root) VALUES(?)`, root); err != nil {
			return nil, err
		}
	}
	if _, err = tx.ExecContext(ctx, `DELETE FROM generations WHERE generation_id NOT IN (SELECT generation_id FROM active_generations) AND generation_id NOT IN (SELECT generation_id FROM catalog_revision_members)`); err != nil {
		return nil, err
	}
	rows, err = tx.QueryContext(ctx, `SELECT root FROM snapshot_gc_candidates ORDER BY root`)
	if err != nil {
		return nil, err
	}
	pruned = nil
	for rows.Next() {
		var root string
		if err = rows.Scan(&root); err != nil {
			rows.Close()
			return nil, err
		}
		pruned = append(pruned, root)
	}
	if err = rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	rows.Close()
	if err = tx.Commit(); err != nil {
		return nil, err
	}
	return pruned, nil
}

// CompleteSnapshotGC acknowledges an owned root only after filesystem removal.
func (s *Store) CompleteSnapshotGC(ctx context.Context, root string) error {
	if s.readOnly {
		return fmt.Errorf("store is read-only")
	}
	_, err := s.db.ExecContext(ctx, `DELETE FROM snapshot_gc_candidates WHERE root=?`, root)
	return err
}

func (s *Store) SnapshotRootReferenced(ctx context.Context, root string) (bool, error) {
	var n int
	err := s.db.QueryRowContext(ctx, `SELECT (SELECT count(*) FROM generations WHERE root=?) + (SELECT count(*) FROM generation_staging WHERE root=?)`, root, root).Scan(&n)
	return n > 0, err
}

func (s *Store) CleanupStaging(ctx context.Context) error {
	if s.readOnly {
		return fmt.Errorf("store is read-only")
	}
	_, err := s.db.ExecContext(ctx, `DELETE FROM generation_staging`)
	return err
}

// materializeSearch creates a generation-bound lexical projection.  The
// canonical source/entity/evidence tables remain authoritative; this table is
// rebuilt on activation and is never queried without joining those records.
func materializeSearch(ctx context.Context, tx *sql.Tx, gid string) error {
	rows, err := tx.QueryContext(ctx, `SELECT f.source_id,e.entity_id,e.evidence_id,f.generation_id,f.repo_id,f.language,f.classification,f.content,f.path,e.label,e.kind,v.excerpt
		FROM source_files f JOIN evidence v ON v.source_id=f.source_id JOIN entities e ON e.evidence_id=v.evidence_id
		WHERE f.generation_id=? ORDER BY f.path,e.entity_id`, gid)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var sourceID, entityID, evidenceID, generationID, repoID, language, classification, content, path, label, kind, evidenceExcerpt string
		if err := rows.Scan(&sourceID, &entityID, &evidenceID, &generationID, &repoID, &language, &classification, &content, &path, &label, &kind, &evidenceExcerpt); err != nil {
			return err
		}
		indexedSource := evidenceExcerpt
		if kind == "file" {
			indexedSource = content
		}
		documentation, configuration, logs := "", "", ""
		switch classification {
		case "documentation":
			documentation = indexedSource
		case "configuration":
			configuration = indexedSource
		case "log", "logs", "error":
			logs = indexedSource
		}
		if strings.Contains(strings.ToLower(indexedSource), "error") {
			logs = indexedSource
		}
		if kind == "configuration_key" {
			configuration += " " + label
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO search_fts VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?)`, sourceID, entityID, evidenceID, generationID, repoID, language, classification, indexedSource, documentation, path, label, indexedSource, logs, configuration); err != nil {
			return err
		}
	}
	return rows.Err()
}

func materialize(ctx context.Context, tx *sql.Tx, gid, repo string) error {
	rows, err := tx.QueryContext(ctx, `SELECT source_id,path,sha256,language,content FROM source_files WHERE generation_id=? ORDER BY path`, gid)
	if err != nil {
		return err
	}
	defer rows.Close()
	files := map[string]Source{}
	for rows.Next() {
		var x Source
		if err = rows.Scan(&x.ID, &x.Path, &x.SHA256, &x.Language, &x.Content); err != nil {
			return err
		}
		x.GenerationID = gid
		x.RepoID = repo
		files[x.Path] = x
		location := locationID(gid, x.ID, 0, len(x.Content))
		if _, err = tx.ExecContext(ctx, `INSERT INTO ir_locations VALUES(?,?,?,?,?,?,?,?,?,?,?)`, location, gid, x.ID, x.Path, x.SHA256, 0, len(x.Content), 1, 1, lineCount(x.Content), 1+lastLineWidth(x.Content)); err != nil {
			return err
		}
		ev := evidenceID(gid, x.ID, 0, len(x.Content))
		if _, err = tx.ExecContext(ctx, `INSERT INTO evidence VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?)`, ev, gid, x.ID, repo, x.Path, x.SHA256, 0, len(x.Content), 1, 1, lineCount(x.Content), 1+lastLineWidth(x.Content), excerpt(x.Content, 0, len(x.Content))); err != nil {
			return err
		}
		canonical := canonicalEntityID(repo, "file", "file:"+x.Path)
		if err = ensureIREntity(ctx, tx, canonical, repo, "file", "file:"+x.Path, x.Path, "path-exact", location); err != nil {
			return err
		}
		if _, err = tx.ExecContext(ctx, `INSERT INTO entities VALUES(?,?,?,?,?,?,?,?,?,?,?)`, entityID(gid, "file", x.Path, x.Path), gid, repo, "file", x.Path, "file:"+x.Path, x.Path, x.Language, "source", ev, canonical); err != nil {
			return err
		}
	}
	if err = rows.Err(); err != nil {
		return err
	}
	syms, err := tx.QueryContext(ctx, `SELECT path,name,kind,identity,start_byte,end_byte,start_line,start_column,end_line,end_column,extractor,confidence FROM staged_symbols WHERE generation_id=? ORDER BY path,kind,name,identity,start_byte,end_byte,extractor`, gid)
	if err != nil {
		return err
	}
	defer syms.Close()
	for syms.Next() {
		var path, name, kind, identity, der string
		var sp model.Span
		var conf float64
		if err = syms.Scan(&path, &name, &kind, &identity, &sp.StartByte, &sp.EndByte, &sp.StartLine, &sp.StartColumn, &sp.EndLine, &sp.EndColumn, &der, &conf); err != nil {
			return err
		}
		f, ok := files[path]
		if !ok {
			return fmt.Errorf("symbol source missing")
		}
		ev, err := insertEvidence(ctx, tx, gid, f, sp)
		if err != nil {
			return err
		}
		if identity == "" {
			identity = "syntax:" + f.Language + ":" + path + ":" + kind + ":" + name + ":" + fmt.Sprint(sp.StartByte)
		}
		location := locationID(gid, f.ID, sp.StartByte, sp.EndByte)
		canonical := canonicalEntityID(repo, "symbol:"+kind, identity)
		if err = ensureIREntity(ctx, tx, canonical, repo, "symbol:"+kind, identity, name, "exact", location); err != nil {
			return err
		}
		eid := entityID(gid, "symbol:"+kind, identity, path)
		// Syntax and compiler frontends may report the same declaration. The
		// canonical identity is the deduplication boundary; retain the first
		// deterministic source-ordered record rather than failing activation.
		if _, err = tx.ExecContext(ctx, `INSERT OR IGNORE INTO entities VALUES(?,?,?,?,?,?,?,?,?,?,?)`, eid, gid, repo, "symbol:"+kind, name, identity, path, f.Language, der, ev, canonical); err != nil {
			return err
		}
	}
	if err = syms.Err(); err != nil {
		return err
	}
	edges, err := tx.QueryContext(ctx, `SELECT path,source,target,source_identity,target_identity,kind,start_byte,end_byte,start_line,start_column,end_line,end_column,resolver,derivation,confidence FROM staged_edges WHERE generation_id=? ORDER BY path,kind,source,target,source_identity,target_identity,start_byte,end_byte,resolver`, gid)
	if err != nil {
		return err
	}
	defer edges.Close()
	for edges.Next() {
		var path, sub, obj, subIdentity, objIdentity, pred, extractor, derivation string
		var sp model.Span
		var conf float64
		if err = edges.Scan(&path, &sub, &obj, &subIdentity, &objIdentity, &pred, &sp.StartByte, &sp.EndByte, &sp.StartLine, &sp.StartColumn, &sp.EndLine, &sp.EndColumn, &extractor, &derivation, &conf); err != nil {
			return err
		}
		f := files[path]
		ev, err := insertEvidence(ctx, tx, gid, f, sp)
		if err != nil {
			return err
		}
		sk := endpointKind(pred, sub)
		ok := endpointKind(pred, obj)
		if subIdentity == "" {
			subIdentity = "syntax-endpoint:" + sk + ":" + sub
		}
		if objIdentity == "" {
			objIdentity = "syntax-endpoint:" + ok + ":" + obj
		}
		sid := entityID(gid, sk, subIdentity, "")
		oid := entityID(gid, ok, objIdentity, "")
		location := locationID(gid, f.ID, sp.StartByte, sp.EndByte)
		canonicals := map[string]string{}
		for _, e := range []struct{ id, k, l, i string }{{sid, sk, sub, subIdentity}, {oid, ok, obj, objIdentity}} {
			canonical := canonicalEntityID(repo, e.k, e.i)
			canonicals[e.id] = canonical
			if err = ensureIREntity(ctx, tx, canonical, repo, e.k, e.i, e.l, "exact", location); err != nil {
				return err
			}
			_, err = tx.ExecContext(ctx, `INSERT OR IGNORE INTO entities VALUES(?,?,?,?,?,?,?,?,?,?,?)`, e.id, gid, repo, e.k, e.l, e.i, "", "", extractor, ev, canonical)
			if err != nil {
				return err
			}
		}
		cid := claimID(gid, sid, pred, oid, ev)
		// Multiple extractors can produce the same evidence-backed fact. Claims
		// are canonical by their generation/endpoints/predicate/evidence tuple.
		if _, err = tx.ExecContext(ctx, `INSERT OR IGNORE INTO claims VALUES(?,?,?,?,?,?,?,?,?)`, cid, gid, sid, pred, oid, ev, extractor, derivation, conf); err != nil {
			return err
		}
		fact := factID(canonicals[sid], pred, canonicals[oid], derivation)
		if _, err = tx.ExecContext(ctx, `INSERT OR IGNORE INTO ir_facts VALUES(?,?,?,?,?)`, fact, canonicals[sid], pred, canonicals[oid], derivation); err != nil {
			return err
		}
		if _, err = tx.ExecContext(ctx, `INSERT OR IGNORE INTO ir_fact_observations VALUES(?,?,?,?,?,?,?,?,?)`, "fo-"+digest(fact, gid, ev), fact, gid, ev, location, extractor, extractorVersion(extractor), conf, time.Now().UTC().Format(time.RFC3339Nano)); err != nil {
			return err
		}
	}
	return edges.Err()
}
func canonicalEntityID(repo, kind, identity string) string {
	return "ce-" + digest(repo, kind, identity)
}
func locationID(revision, source string, start, end int) string {
	return "l-" + digest(revision, source, fmt.Sprint(start), fmt.Sprint(end))
}
func factID(subject, predicate, object, derivation string) string {
	return "f-" + digest(subject, predicate, object, derivation)
}
func extractorVersion(extractor string) string { return "v1" }
func ensureIREntity(ctx context.Context, tx *sql.Tx, id, repo, kind, identity, alias, normalization, location string) error {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO ir_entities VALUES(?,?,?,?,?)`, id, repo, kind, identity, now); err != nil {
		return err
	}
	_, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO ir_entity_aliases VALUES(?,?,?,?)`, id, alias, normalization, location)
	return err
}
func catalogFingerprint(ctx context.Context, tx *sql.Tx, catalogRevision string) (string, error) {
	var fingerprint string
	err := tx.QueryRowContext(ctx, `SELECT COALESCE(group_concat(generation_id, ','),'') FROM (SELECT generation_id FROM catalog_revision_members WHERE catalog_revision_id=? ORDER BY generation_id)`, catalogRevision).Scan(&fingerprint)
	return fingerprint, err
}

// buildProjections is intentionally fed only from persisted IR tables. It is
// called inside the same transaction as activation, so a failed validation
// cannot publish a partial catalog or replace a prior active build.
func buildProjections(ctx context.Context, tx *sql.Tx, catalogRevision, reason string) error {
	kinds := append(append([]string{}, requiredProjectionKinds...), "vector")
	return buildProjectionSet(ctx, tx, catalogRevision, reason, kinds)
}

func buildProjectionSet(ctx context.Context, tx *sql.Tx, catalogRevision, reason string, kinds []string) error {
	fingerprint, err := catalogFingerprint(ctx, tx, catalogRevision)
	if err != nil {
		return err
	}
	for _, kind := range kinds {
		now := time.Now().UTC().Format(time.RFC3339Nano)
		builderVersion := projectionBuilderVersion(kind)
		buildID := "pb-" + digest(kind, builderVersion, catalogRevision, fingerprint, now)
		// Persist the private build first: projection rows carry a real foreign
		// key, but it is not eligible for reads until the pointer update below.
		if _, err = tx.ExecContext(ctx, `INSERT INTO projection_builds(projection_build_id,projection_kind,projection_schema_version,catalog_revision_id,source_ir_fingerprint,builder_name,builder_version,input_fingerprint,projection_fingerprint,created_at,activated_at,state,record_counts,validation_summary,rebuild_reason) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`, buildID, kind, "v1", catalogRevision, fingerprint, "store-ir-projection-builder", builderVersion, fingerprint, "", now, nil, "staged", `{}`, `{"valid":false,"state":"staged"}`, reason); err != nil {
			return err
		}
		state, counts, err := buildProjectionRows(ctx, tx, kind, buildID, catalogRevision, reason)
		if err != nil {
			return err
		}
		if err = validateProjectionRows(ctx, tx, kind, buildID, catalogRevision); err != nil {
			return fmt.Errorf("validate %s projection: %w", kind, err)
		}
		projectionFingerprint := digest(kind, builderVersion, fingerprint, counts)
		validation := `{"valid":true}`
		if state == "disabled" {
			validation = `{"valid":true,"disabled":true}`
		}
		if _, err = tx.ExecContext(ctx, `UPDATE projection_builds SET projection_fingerprint=?,activated_at=?,state=?,record_counts=?,validation_summary=? WHERE projection_build_id=?`, projectionFingerprint, nullableActivated(state, now), state, counts, validation, buildID); err != nil {
			return err
		}
		if state == "ready" {
			if _, err = tx.ExecContext(ctx, `INSERT INTO active_projection_builds(projection_kind,projection_build_id) VALUES(?,?) ON CONFLICT(projection_kind) DO UPDATE SET projection_build_id=excluded.projection_build_id`, kind, buildID); err != nil {
				return err
			}
		}
	}
	return nil
}

// RebuildProjections rebuilds agent-owned derived data from the active IR.
// It intentionally accepts no repository paths and never opens a checkout.
func (s *Store) RebuildProjections(ctx context.Context, kinds []string) error {
	if s.readOnly {
		return fmt.Errorf("store is read-only")
	}
	if len(kinds) == 0 {
		kinds = append([]string{}, requiredProjectionKinds...)
	}
	valid := map[string]bool{"lookup": true, "lexical": true, "graph": true, "path": true, "ui": true, "cache": true, "landmarks": true}
	for _, kind := range kinds {
		if !valid[kind] {
			return fmt.Errorf("unsupported projection kind %q", kind)
		}
	}
	r, err := s.ActiveCatalogRevision(ctx)
	if err != nil {
		return fmt.Errorf("cannot rebuild projections: no active IR catalog")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err = buildProjectionSet(ctx, tx, r.ID, "operator_rebuild", kinds); err != nil {
		_, _ = s.RecordDiagnostic(ctx, model.DiagnosticEvent{Code: DiagnosticProjectionFailure, Severity: model.DiagnosticError, Scope: model.DiagnosticScope{Projection: strings.Join(kinds, ",")}, Remediation: "inspect projection health, repair canonical evidence, then rebuild", Metadata: map[string]string{"operation": "operator_rebuild"}})
		return err
	}
	return tx.Commit()
}

func nullableActivated(state, now string) any {
	if state == "ready" {
		return now
	}
	return nil
}

func buildProjectionRows(ctx context.Context, tx *sql.Tx, kind, buildID, revision, reason string) (string, string, error) {
	member := `SELECT generation_id FROM catalog_revision_members WHERE catalog_revision_id=?`
	insert := func(table, columns, selectSQL string) (string, error) {
		res, err := tx.ExecContext(ctx, `INSERT INTO `+table+` `+columns+` `+selectSQL, buildID, revision)
		if err != nil {
			return "", err
		}
		n, _ := res.RowsAffected()
		return fmt.Sprintf(`{"records":%d}`, n), nil
	}
	switch kind {
	case "lookup":
		v, e := insert("projection_lookup_records", "(projection_build_id,generation_id,entity_id,evidence_id)", `SELECT ?,e.generation_id,e.entity_id,e.evidence_id FROM entities e WHERE e.generation_id IN (`+member+`)`)
		return "ready", v, e
	case "graph":
		v, e := insert("projection_graph_records", "(projection_build_id,generation_id,claim_id,subject_id,object_id,evidence_id)", `SELECT ?,c.generation_id,c.claim_id,c.subject_id,c.object_id,c.evidence_id FROM claims c WHERE c.generation_id IN (`+member+`)`)
		if e != nil {
			return "", "", e
		}
		if _, e = tx.ExecContext(ctx, `INSERT INTO projection_cross_graph_records(projection_build_id,catalog_revision_id,cross_claim_id) SELECT ?,catalog_revision_id,cross_claim_id FROM cross_claims WHERE catalog_revision_id=?`, buildID, revision); e != nil {
			return "", "", e
		}
		return "ready", v, nil
	case "path":
		v, e := insert("projection_path_records", "(projection_build_id,generation_id,claim_id,subject_id,object_id)", `SELECT ?,c.generation_id,c.claim_id,c.subject_id,c.object_id FROM claims c WHERE c.generation_id IN (`+member+`)`)
		return "ready", v, e
	case "ui":
		v, e := insert("projection_ui_nodes", "(projection_build_id,generation_id,entity_id,evidence_id)", `SELECT ?,e.generation_id,e.entity_id,e.evidence_id FROM entities e WHERE e.generation_id IN (`+member+`)`)
		if e != nil {
			return "", "", e
		}
		_, e = insert("projection_ui_edges", "(projection_build_id,generation_id,claim_id)", `SELECT ?,c.generation_id,c.claim_id FROM claims c WHERE c.generation_id IN (`+member+`)`)
		return "ready", v, e
	case "lexical":
		if _, e := tx.ExecContext(ctx, `DELETE FROM source_fts WHERE generation_id IN (`+member+`)`, revision); e != nil {
			return "", "", e
		}
		if _, e := tx.ExecContext(ctx, `DELETE FROM search_fts WHERE generation_id IN (`+member+`)`, revision); e != nil {
			return "", "", e
		}
		if _, e := tx.ExecContext(ctx, `INSERT INTO source_fts SELECT source_id,generation_id,repo_id,path,content FROM source_files WHERE generation_id IN (`+member+`)`, revision); e != nil {
			return "", "", e
		}
		rows, e := tx.QueryContext(ctx, `SELECT generation_id FROM catalog_revision_members WHERE catalog_revision_id=? ORDER BY generation_id`, revision)
		if e != nil {
			return "", "", e
		}
		defer rows.Close()
		for rows.Next() {
			var g string
			if e = rows.Scan(&g); e != nil {
				return "", "", e
			}
			if e = materializeSearch(ctx, tx, g); e != nil {
				return "", "", e
			}
		}
		if e = rows.Err(); e != nil {
			return "", "", e
		}
		var n int
		e = tx.QueryRowContext(ctx, `SELECT count(*) FROM search_fts WHERE generation_id IN (`+member+`)`, revision).Scan(&n)
		return "ready", fmt.Sprintf(`{"records":%d}`, n), e
	case "cache":
		return "ready", `{"records":0}`, nil
	case "landmarks":
		counts, err := buildLandmarks(ctx, tx, buildID, revision, reason)
		return "ready", counts, err
	case "vector":
		return "disabled", `{"records":0}`, nil
	default:
		return "", "", fmt.Errorf("unsupported projection kind %q", kind)
	}
}

func validateProjectionRows(ctx context.Context, tx *sql.Tx, kind, buildID, revision string) error {
	if kind == "lookup" {
		return validateLookupCompleteness(ctx, tx, buildID, revision)
	}
	if kind == "vector" || kind == "cache" || kind == "lexical" {
		return nil
	}
	var q string
	switch kind {
	case "landmarks":
		q = `SELECT count(*) FROM precomputed_records p
 LEFT JOIN projection_builds b ON b.projection_build_id=p.projection_build_id
 WHERE p.projection_build_id=? AND (b.projection_build_id IS NULL OR p.catalog_revision_id<>? OR p.evidence_ids='[]')`
	case "lookup", "ui":
		q = `SELECT count(*) FROM ` + map[string]string{"lookup": "projection_lookup_records", "ui": "projection_ui_nodes"}[kind] + ` p LEFT JOIN entities e ON e.entity_id=p.entity_id AND e.generation_id=p.generation_id LEFT JOIN evidence v ON v.evidence_id=p.evidence_id AND v.generation_id=p.generation_id WHERE p.projection_build_id=? AND (e.entity_id IS NULL OR v.evidence_id IS NULL OR p.generation_id NOT IN (SELECT generation_id FROM catalog_revision_members WHERE catalog_revision_id=?))`
	case "graph":
		q = `SELECT count(*) FROM projection_graph_records p LEFT JOIN claims c ON c.claim_id=p.claim_id AND c.generation_id=p.generation_id LEFT JOIN entities s ON s.entity_id=p.subject_id AND s.generation_id=p.generation_id LEFT JOIN entities o ON o.entity_id=p.object_id AND o.generation_id=p.generation_id LEFT JOIN evidence v ON v.evidence_id=p.evidence_id AND v.generation_id=p.generation_id WHERE p.projection_build_id=? AND (c.claim_id IS NULL OR s.entity_id IS NULL OR o.entity_id IS NULL OR v.evidence_id IS NULL OR p.generation_id NOT IN (SELECT generation_id FROM catalog_revision_members WHERE catalog_revision_id=?))`
	case "path":
		q = `SELECT count(*) FROM projection_path_records p LEFT JOIN claims c ON c.claim_id=p.claim_id AND c.generation_id=p.generation_id LEFT JOIN entities s ON s.entity_id=p.subject_id AND s.generation_id=p.generation_id LEFT JOIN entities o ON o.entity_id=p.object_id AND o.generation_id=p.generation_id WHERE p.projection_build_id=? AND (c.claim_id IS NULL OR s.entity_id IS NULL OR o.entity_id IS NULL OR p.generation_id NOT IN (SELECT generation_id FROM catalog_revision_members WHERE catalog_revision_id=?))`
	}
	var invalid int
	if err := tx.QueryRowContext(ctx, q, buildID, revision).Scan(&invalid); err != nil {
		return err
	}
	if invalid != 0 {
		return fmt.Errorf("contains %d foreign, missing, or inactive IR references", invalid)
	}
	return nil
}
func endpointKind(predicate, label string) string {
	if predicate == "DEFINES_CONFIGURATION" || predicate == "REFERENCES_CONFIGURATION" {
		return "configuration_key"
	}
	if predicate == "HAS_LOCAL_GUARD" {
		return "local_syntax_guard"
	}
	if predicate == "COVERAGE_GAP" {
		return "coverage_diagnostic"
	}
	if strings.Contains(predicate, "ROUTE") || strings.Contains(predicate, "ENDPOINT") {
		return "route"
	}
	if strings.Contains(predicate, "EVENT") || strings.Contains(predicate, "TOPIC") || strings.Contains(predicate, "QUEUE") {
		return "message_channel"
	}
	if predicate == "EMITS_LOG" || predicate == "EMITS_ERROR" {
		return "emission_site"
	}
	return "reference"
}
func insertEvidence(ctx context.Context, tx *sql.Tx, gid string, f Source, sp model.Span) (string, error) {
	if sp.EndByte > len(f.Content) || sp.StartByte < 0 {
		return "", fmt.Errorf("invalid source span")
	}
	id := evidenceID(gid, f.ID, sp.StartByte, sp.EndByte)
	if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO ir_locations VALUES(?,?,?,?,?,?,?,?,?,?,?)`, locationID(gid, f.ID, sp.StartByte, sp.EndByte), gid, f.ID, f.Path, f.SHA256, sp.StartByte, sp.EndByte, sp.StartLine, sp.StartColumn, sp.EndLine, sp.EndColumn); err != nil {
		return "", err
	}
	_, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO evidence VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?)`, id, gid, f.ID, f.RepoID, f.Path, f.SHA256, sp.StartByte, sp.EndByte, sp.StartLine, sp.StartColumn, sp.EndLine, sp.EndColumn, excerpt(f.Content, sp.StartByte, sp.EndByte))
	return id, err
}
func sourceID(g, p, h string) string { return "s-" + digest(g, p, h) }
func coverageID(g, fingerprint, extractor string) string {
	return "cov-" + digest(g, fingerprint, extractor)
}
func entityID(g, k, l, p string) string { return "e-" + digest(g, k, l, p) }
func evidenceID(g, s string, a, b int) string {
	return "v-" + digest(g, s, fmt.Sprint(a), fmt.Sprint(b))
}
func claimID(g, s, p, o, e string) string { return "c-" + digest(g, s, p, o, e) }
func digest(parts ...string) string {
	h := sha256.New()
	for _, p := range parts {
		h.Write([]byte(p))
		h.Write([]byte{0})
	}
	return hex.EncodeToString(h.Sum(nil))
}
func newGenerationID() (string, error) {
	var x [16]byte
	if _, e := rand.Read(x[:]); e != nil {
		return "", e
	}
	return "g1-" + hex.EncodeToString(x[:]), nil
}
func newCatalogRevisionID() (string, error) {
	var x [16]byte
	if _, e := rand.Read(x[:]); e != nil {
		return "", e
	}
	return "cg1-" + hex.EncodeToString(x[:]), nil
}
func (s *Store) ActiveCatalogRevision(ctx context.Context) (CatalogRevision, error) {
	var r CatalogRevision
	var a, b string
	err := s.db.QueryRowContext(ctx, `SELECT r.catalog_revision_id,r.created_at,r.activated_at FROM active_catalog_revision a JOIN catalog_revisions r ON r.catalog_revision_id=a.catalog_revision_id`).Scan(&r.ID, &a, &b)
	if err == nil {
		r.CreatedAt, _ = time.Parse(time.RFC3339Nano, a)
		r.ActivatedAt, _ = time.Parse(time.RFC3339Nano, b)
	}
	return r, err
}
func excerpt(s string, a, b int) string {
	if b-a > 600 {
		b = a + 600
	}
	return s[a:b]
}
func lineCount(s string) int     { return strings.Count(s, "\n") + 1 }
func lastLineWidth(s string) int { n := strings.LastIndex(s, "\n"); return len(s) - n - 1 }
func (s *Store) ActiveGeneration(ctx context.Context, repo string) (Generation, error) {
	var g Generation
	var a, b string
	err := s.db.QueryRowContext(ctx, `SELECT g.generation_id,g.repo_id,g.content_hash,g.created_at,g.activated_at FROM active_generations a JOIN generations g ON g.generation_id=a.generation_id WHERE a.repo_id=?`, repo).Scan(&g.ID, &g.RepoID, &g.ContentHash, &a, &b)
	if err != nil {
		return g, err
	}
	g.CreatedAt, _ = time.Parse(time.RFC3339Nano, a)
	g.ActivatedAt, _ = time.Parse(time.RFC3339Nano, b)
	return g, nil
}
func (s *Store) Status(ctx context.Context, repo string) ([]model.Snapshot, error) {
	q := `SELECT repo_id,root,git_commit,branch,dirty,untracked_count,content_hash,file_count,total_bytes,indexed_at,extractor_versions FROM generations WHERE generation_id IN (SELECT generation_id FROM active_generations)`
	var args []any
	if repo != "" {
		q += " AND repo_id=?"
		args = []any{repo}
	}
	q += " ORDER BY repo_id"
	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []model.Snapshot
	for rows.Next() {
		var x model.Snapshot
		var t string
		if err = rows.Scan(&x.RepoID, &x.Root, &x.Git.Commit, &x.Git.Branch, &x.Git.Dirty, &x.Git.UntrackedCount, &x.ContentHash, &x.FileCount, &x.TotalBytes, &t, &x.ExtractorVersions); err != nil {
			return nil, err
		}
		x.IndexedAt, _ = time.Parse(time.RFC3339Nano, t)
		out = append(out, x)
	}
	return out, rows.Err()
}
func (s *Store) Diagnostics(ctx context.Context, repo string) (Status, error) {
	snapshots, err := s.Status(ctx, repo)
	if err != nil {
		return Status{}, err
	}
	vectorAvailable, vectorReason := semantic.RuntimeAvailability()
	x := Status{
		Snapshots: snapshots, SupportedAdapters: map[string]string{model.SourceKindRepository: model.RepositoryAdapterVersion},
		DisabledCapabilities: map[string]string{"connected_mcp_sources": "outside_v1", "skills_plugins_capabilities": "outside_v1", "capability_recommendations": "outside_v1", "llm": "outside_v1", "planning": "outside_v1", "execution": "outside_v1", "subagents": "outside_v1", "autonomous_reasoning": "outside_v1"},
		Capabilities: map[string]any{
			"retrieval_routes":  []string{"exact", "lexical", "graph", "structural", "vector_optional"},
			"language_coverage": map[string]string{"java": "structural", "typescript": "structural", "tsx": "structural", "javascript": "structural", "jsx": "structural", "kotlin": "structural", "python": "lexical_only"},
			"vector":            map[string]any{"runtime_available": vectorAvailable, "reason": vectorReason},
		},
	}
	if len(snapshots) > 0 {
		x.ActiveSourceKinds = []string{model.SourceKindRepository}
	}
	if r, e := s.ActiveCatalogRevision(ctx); e == nil {
		x.ActiveCatalog = r.ID
	}
	_ = s.db.QueryRowContext(ctx, `SELECT count(*) FROM generations`).Scan(&x.PublishedGenerations)
	_ = s.db.QueryRowContext(ctx, `SELECT count(*) FROM generation_staging`).Scan(&x.StagedGenerations)
	var pages, pageSize int64
	if s.db.QueryRowContext(ctx, `PRAGMA page_count`).Scan(&pages) == nil && s.db.QueryRowContext(ctx, `PRAGMA page_size`).Scan(&pageSize) == nil {
		x.IndexBytes = pages * pageSize
	}
	x.Invalidations = map[string]int{}
	if x.Ingestion, err = s.IngestionStatus(ctx, repo); err != nil {
		return Status{}, err
	}
	if x.IngestionEvents, err = s.IngestionEvents(ctx, repo); err != nil {
		return Status{}, err
	}
	if x.Diagnostics, err = s.DiagnosticEvents(ctx, repo, false, 50); err != nil {
		return Status{}, err
	}
	rows, e := s.db.QueryContext(ctx, `SELECT i.kind,count(*) FROM invalidations i JOIN active_generations a ON a.generation_id=i.generation_id GROUP BY i.kind`)
	if e == nil {
		defer rows.Close()
		for rows.Next() {
			var kind string
			var n int
			if rows.Scan(&kind, &n) == nil {
				x.Invalidations[kind] = n
			}
		}
	}
	if x.ActiveCatalog != "" {
		rows, e := s.db.QueryContext(ctx, `SELECT p.projection_kind,p.projection_schema_version,p.catalog_revision_id,p.source_ir_fingerprint,p.builder_name,p.builder_version,p.input_fingerprint,p.projection_fingerprint,p.state,p.created_at,COALESCE(p.activated_at,''),p.record_counts,p.validation_summary,p.rebuild_reason FROM projection_builds p WHERE p.catalog_revision_id=? AND (p.state='disabled' OR p.projection_build_id IN (SELECT projection_build_id FROM active_projection_builds)) ORDER BY p.projection_kind`, x.ActiveCatalog)
		if e != nil {
			return Status{}, e
		}
		defer rows.Close()
		for rows.Next() {
			var p ProjectionStatus
			var counts, validation string
			if e = rows.Scan(&p.Kind, &p.SchemaVersion, &p.CatalogRevision, &p.SourceGeneration, &p.Builder, &p.BuilderVersion, &p.InputFingerprint, &p.Fingerprint, &p.State, &p.CreatedAt, &p.ActivatedAt, &counts, &validation, &p.RebuildReason); e != nil {
				return Status{}, e
			}
			_ = json.Unmarshal([]byte(counts), &p.RecordCounts)
			_ = json.Unmarshal([]byte(validation), &p.ValidationSummary)
			x.Projections = append(x.Projections, p)
		}
		if e = rows.Err(); e != nil {
			return Status{}, e
		}
	}
	x.Health = ProjectionHealth{Healthy: true, Catalog: x.ActiveCatalog, SourceFresh: len(x.Snapshots) > 0}
	for _, snapshot := range x.Snapshots {
		if snapshot.Git.Dirty {
			x.Health.SourceFresh = false
		}
	}
	_ = s.db.QueryRowContext(ctx, `SELECT count(*) FROM coverage_runs c JOIN active_generations a ON a.generation_id=c.generation_id`).Scan(&x.Health.CoverageRuns)
	derived, validationErr := s.ValidateActiveProjectionProvenance(ctx)
	if validationErr != nil {
		return Status{}, validationErr
	}
	for _, event := range derived {
		x.Diagnostics = append(x.Diagnostics, event)
		x.Health.Diagnostics = append(x.Health.Diagnostics, event)
		x.Health.Healthy = false
	}
	for _, projection := range x.Projections {
		if projection.State != "ready" && projection.State != "disabled" {
			event := model.DiagnosticEvent{Code: DiagnosticProjectionMismatch, Severity: model.DiagnosticWarning, Scope: model.DiagnosticScope{Projection: projection.Kind}, Remediation: "rebuild the projection from the active canonical IR", Metadata: map[string]string{"state": projection.State}}
			x.Diagnostics = append(x.Diagnostics, event)
			x.Health.Diagnostics = append(x.Health.Diagnostics, event)
			x.Health.Healthy = false
		}
	}
	sort.SliceStable(x.Diagnostics, func(i, j int) bool {
		return x.Diagnostics[i].Code+x.Diagnostics[i].ID < x.Diagnostics[j].Code+x.Diagnostics[j].ID
	})
	sort.SliceStable(x.Health.Diagnostics, func(i, j int) bool {
		return x.Health.Diagnostics[i].Code+x.Health.Diagnostics[i].ID < x.Health.Diagnostics[j].Code+x.Health.Diagnostics[j].ID
	})
	return x, nil
}
func ContentSnapshot(files []model.File) string {
	sort.Slice(files, func(i, j int) bool { return files[i].Path < files[j].Path })
	h := sha256.New()
	for _, f := range files {
		h.Write([]byte(f.Path))
		h.Write([]byte{0})
		h.Write([]byte(f.SHA256))
		h.Write([]byte{0})
	}
	return hex.EncodeToString(h.Sum(nil))
}
