// Package cache stores bounded, generation-aware work products. It is never a
// factual authority: callers must re-resolve every returned handle from IR.
package cache

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	_ "modernc.org/sqlite"
)

const DatabaseName = "cache.db"
const BuilderVersion = "cache-v1"

type Kind string

const (
	QueryPlan        Kind = "query_plan"
	Candidate        Kind = "candidate_result"
	Traversal        Kind = "traversal"
	Path             Kind = "path"
	EvidencePackage  Kind = "evidence_package"
	NegativeEvidence Kind = "negative_evidence"
	PrecomputedSlice Kind = "precomputed_slice"
)

var limits = map[Kind]int{QueryPlan: 2 << 20, Candidate: 16 << 20, Traversal: 8 << 20, Path: 4 << 20, EvidencePackage: 16 << 20, NegativeEvidence: 4 << 20, PrecomputedSlice: 8 << 20}

const maxEntries = 2048
const maxBytes = 64 << 20

type Metadata struct {
	Kind                   Kind              `json:"kind"`
	RequestFingerprint     string            `json:"request_fingerprint"`
	SourceScope            []string          `json:"source_scope"`
	GenerationFingerprint  string            `json:"generation_fingerprint"`
	ProjectionFingerprints map[string]string `json:"projection_fingerprints"`
	CoverageFingerprint    string            `json:"coverage_fingerprint,omitempty"`
	BudgetFingerprint      string            `json:"budget_fingerprint"`
	OutputShape            string            `json:"output_shape"`
	Builder                string            `json:"builder"`
}

type Entry struct {
	Key                       string
	Metadata                  Metadata
	Payload                   []byte
	CreatedAt, AccessedAt     time.Time
	Hits                      int64
	Size                      int64
	State, InvalidationReason string
}
type Diagnostics struct {
	State             string `json:"state"`
	UnavailableReason string `json:"unavailable_reason,omitempty"`
	Entries           int    `json:"entries"`
	Bytes             int64  `json:"bytes"`
	Hits              int64  `json:"hits"`
	Misses            int64  `json:"misses"`
	Coalesced         int64  `json:"coalesced"`
	Evictions         int64  `json:"evictions"`
	Invalidations     int64  `json:"invalidations"`
	StaleRejects      int64  `json:"stale_rejects"`
}

type Store struct {
	db       *sql.DB
	mu       sync.Mutex
	pending  map[string]*call
	disabled string
}
type call struct {
	done  chan struct{}
	entry Entry
	hit   bool
	err   error
}

func Fingerprint(value any) string {
	b, _ := json.Marshal(value)
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}

func Key(m Metadata) string {
	m.SourceScope = sorted(m.SourceScope)
	m.ProjectionFingerprints = sortedMap(m.ProjectionFingerprints)
	m.Builder = BuilderVersion
	return Fingerprint(m)
}

func Open(dataDir string) (*Store, error) {
	if err := os.MkdirAll(dataDir, 0700); err != nil {
		return nil, err
	}
	if err := os.Chmod(dataDir, 0700); err != nil {
		return nil, err
	}
	p := filepath.Join(dataDir, DatabaseName)
	db, err := sql.Open("sqlite", p)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	_, err = db.Exec(`PRAGMA journal_mode=WAL; CREATE TABLE IF NOT EXISTS entries (cache_key TEXT PRIMARY KEY, kind TEXT NOT NULL, metadata_json TEXT NOT NULL, payload BLOB NOT NULL, created_at TEXT NOT NULL, accessed_at TEXT NOT NULL, hits INTEGER NOT NULL, size INTEGER NOT NULL, state TEXT NOT NULL, invalidation_reason TEXT NOT NULL DEFAULT ''); CREATE TABLE IF NOT EXISTS metrics (name TEXT PRIMARY KEY, value INTEGER NOT NULL);`)
	if err != nil {
		db.Close()
		return nil, err
	}
	if err = os.Chmod(p, 0600); err != nil {
		db.Close()
		return nil, err
	}
	return &Store{db: db, pending: map[string]*call{}}, nil
}

// OpenReadOnly reports cache health without creating or repairing cache state.
func OpenReadOnly(dataDir string) (*Store, error) {
	p := filepath.Join(dataDir, DatabaseName)
	info, err := os.Stat(p)
	if errors.Is(err, os.ErrNotExist) {
		return Disabled("cache database is absent"), nil
	}
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Mode().Perm() != 0600 {
		return nil, fmt.Errorf("cache database must be owner-only regular file")
	}
	db, err := sql.Open("sqlite", "file:"+url.PathEscape(p)+"?mode=ro")
	if err != nil {
		return nil, err
	}
	if _, err = db.Exec(`SELECT 1 FROM entries LIMIT 1`); err != nil {
		db.Close()
		return nil, err
	}
	return &Store{db: db, pending: map[string]*call{}}, nil
}

func Disabled(reason string) *Store { return &Store{disabled: reason, pending: map[string]*call{}} }
func (s *Store) Close() error {
	if s.db != nil {
		return s.db.Close()
	}
	return nil
}
func (s *Store) Available() bool { return s != nil && s.db != nil && s.disabled == "" }

func (s *Store) Get(ctx context.Context, m Metadata, valid func(Metadata) bool) (Entry, bool) {
	if !s.Available() {
		return Entry{}, false
	}
	k := Key(m)
	var e Entry
	var meta, created, accessed string
	err := s.db.QueryRowContext(ctx, `SELECT metadata_json,payload,created_at,accessed_at,hits,size,state,invalidation_reason FROM entries WHERE cache_key=? AND state='ready'`, k).Scan(&meta, &e.Payload, &created, &accessed, &e.Hits, &e.Size, &e.State, &e.InvalidationReason)
	if err != nil {
		s.metric("misses", 1)
		return Entry{}, false
	}
	if json.Unmarshal([]byte(meta), &e.Metadata) != nil || !valid(e.Metadata) {
		_, _ = s.db.ExecContext(ctx, `UPDATE entries SET state='invalidated',invalidation_reason='dependency_mismatch' WHERE cache_key=?`, k)
		s.metric("stale_rejects", 1)
		s.metric("invalidations", 1)
		s.metric("misses", 1)
		return Entry{}, false
	}
	now := time.Now().UTC()
	_, _ = s.db.ExecContext(ctx, `UPDATE entries SET accessed_at=?,hits=hits+1 WHERE cache_key=?`, now.Format(time.RFC3339Nano), k)
	e.Key = k
	e.AccessedAt = now
	e.CreatedAt, _ = time.Parse(time.RFC3339Nano, created)
	s.metric("hits", 1)
	return e, true
}

func (s *Store) Put(ctx context.Context, m Metadata, payload []byte) error {
	if !s.Available() {
		return errors.New("cache unavailable")
	}
	if err := validate(m, payload); err != nil {
		return err
	}
	k := Key(m)
	now := time.Now().UTC()
	meta, _ := json.Marshal(m)
	if _, err := s.db.ExecContext(ctx, `INSERT OR REPLACE INTO entries(cache_key,kind,metadata_json,payload,created_at,accessed_at,hits,size,state,invalidation_reason) VALUES(?,?,?,?,?,?,?,?,?,'')`, k, string(m.Kind), meta, payload, now.Format(time.RFC3339Nano), now.Format(time.RFC3339Nano), 0, len(payload)+len(meta), "ready"); err != nil {
		return err
	}
	return s.evict(ctx)
}

func (s *Store) Do(ctx context.Context, m Metadata, valid func(Metadata) bool, build func(context.Context) ([]byte, error)) (Entry, bool, error) {
	if e, ok := s.Get(ctx, m, valid); ok {
		return e, true, nil
	}
	if !s.Available() {
		b, e := build(ctx)
		return Entry{Metadata: m, Payload: b}, false, e
	}
	k := Key(m)
	s.mu.Lock()
	if p := s.pending[k]; p != nil {
		s.mu.Unlock()
		select {
		case <-p.done:
			s.metric("coalesced", 1)
			return p.entry, p.hit, p.err
		case <-ctx.Done():
			return Entry{}, false, ctx.Err()
		}
	}
	p := &call{done: make(chan struct{})}
	s.pending[k] = p
	s.mu.Unlock()
	if e, ok := s.Get(ctx, m, valid); ok {
		p.entry, p.hit = e, true
	} else {
		b, err := build(ctx)
		p.err = err
		p.entry = Entry{Metadata: m, Payload: b}
		if err == nil {
			// Cache persistence is optional derived work. The freshly built
			// canonical work product remains safe to return on a cache outage.
			_ = s.Put(ctx, m, b)
		}
	}
	s.mu.Lock()
	delete(s.pending, k)
	close(p.done)
	s.mu.Unlock()
	return p.entry, p.hit, p.err
}

func (s *Store) Invalidate(ctx context.Context, reason string, affectedRepos []string) error {
	if !s.Available() {
		return nil
	}
	rows, err := s.db.QueryContext(ctx, `SELECT cache_key,metadata_json FROM entries WHERE state='ready'`)
	if err != nil {
		return err
	}
	defer rows.Close()
	affected := map[string]bool{}
	for _, r := range affectedRepos {
		affected[r] = true
	}
	var keys []string
	for rows.Next() {
		var k, raw string
		if err = rows.Scan(&k, &raw); err != nil {
			return err
		}
		var m Metadata
		if json.Unmarshal([]byte(raw), &m) == nil {
			for _, r := range m.SourceScope {
				if affected[r] {
					keys = append(keys, k)
					break
				}
			}
		}
	}
	for _, k := range keys {
		if _, err = s.db.ExecContext(ctx, `UPDATE entries SET state='invalidated',invalidation_reason=? WHERE cache_key=?`, reason, k); err != nil {
			return err
		}
		s.metric("invalidations", 1)
	}
	return rows.Err()
}

func (s *Store) Diagnostics(ctx context.Context) Diagnostics {
	d := Diagnostics{State: "ready"}
	if !s.Available() {
		d.State, d.UnavailableReason = "unavailable", s.disabled
		return d
	}
	_ = s.db.QueryRowContext(ctx, `SELECT count(*),COALESCE(sum(size),0) FROM entries WHERE state='ready'`).Scan(&d.Entries, &d.Bytes)
	rows, err := s.db.QueryContext(ctx, `SELECT name,value FROM metrics`)
	if err != nil {
		return d
	}
	defer rows.Close()
	for rows.Next() {
		var name string
		var value int64
		if rows.Scan(&name, &value) != nil {
			continue
		}
		switch name {
		case "hits":
			d.Hits = value
		case "misses":
			d.Misses = value
		case "coalesced":
			d.Coalesced = value
		case "evictions":
			d.Evictions = value
		case "invalidations":
			d.Invalidations = value
		case "stale_rejects":
			d.StaleRejects = value
		}
	}
	return d
}
func (s *Store) metric(name string, n int64) {
	if s.Available() {
		_, _ = s.db.Exec(`INSERT INTO metrics(name,value) VALUES(?,?) ON CONFLICT(name) DO UPDATE SET value=value+excluded.value`, name, n)
	}
}
func (s *Store) evict(ctx context.Context) error {
	for {
		var count int
		var bytes int64
		if err := s.db.QueryRowContext(ctx, `SELECT count(*),COALESCE(sum(size),0) FROM entries WHERE state='ready'`).Scan(&count, &bytes); err != nil {
			return err
		}
		if count <= maxEntries && bytes <= maxBytes {
			return nil
		}
		var k string
		err := s.db.QueryRowContext(ctx, `SELECT cache_key FROM entries WHERE state='ready' ORDER BY accessed_at,cache_key LIMIT 1`).Scan(&k)
		if err != nil {
			return err
		}
		if _, err = s.db.ExecContext(ctx, `DELETE FROM entries WHERE cache_key=?`, k); err != nil {
			return err
		}
		s.metric("evictions", 1)
	}
}
func validate(m Metadata, p []byte) error {
	if _, ok := limits[m.Kind]; !ok {
		return fmt.Errorf("unsupported cache kind %q", m.Kind)
	}
	if m.RequestFingerprint == "" || m.GenerationFingerprint == "" || m.BudgetFingerprint == "" || m.OutputShape == "" {
		return errors.New("cache metadata is incomplete")
	}
	if int64(len(p)) > int64(limits[m.Kind]) {
		return errors.New("cache entry exceeds kind limit")
	}
	lower := strings.ToLower(string(p))
	for _, bad := range []string{"password", "authorization", "session", "secret", "source_excerpts", "\"content\""} {
		if strings.Contains(lower, bad) {
			return fmt.Errorf("cache payload contains forbidden field %q", bad)
		}
	}
	return nil
}
func sorted(in []string) []string { out := append([]string(nil), in...); sort.Strings(out); return out }
func sortedMap(in map[string]string) map[string]string {
	out := map[string]string{}
	keys := make([]string, 0, len(in))
	for k := range in {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		out[k] = in[k]
	}
	return out
}
