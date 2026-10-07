package webui

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/AdamNi-7080/AIOS/internal/benchmark"
	"github.com/AdamNi-7080/AIOS/internal/resourcepolicy"
	"github.com/AdamNi-7080/AIOS/internal/store"
)

const benchmarkReportLimit = 8 << 20

func (s *Server) benchmarkRead(name string, out any) error {
	p := filepath.Join(s.dataDir, "benchmark-"+name+".json")
	st, e := os.Lstat(p)
	if e != nil {
		return e
	}
	if !st.Mode().IsRegular() || st.Size() > benchmarkReportLimit {
		return fmt.Errorf("invalid saved benchmark state")
	}
	b, e := os.ReadFile(p)
	if e != nil {
		return e
	}
	return json.Unmarshal(b, out)
}
func (s *Server) benchmarkWrite(name string, v any) error {
	b, e := json.Marshal(v)
	if e != nil {
		return e
	}
	if len(b) > benchmarkReportLimit {
		return fmt.Errorf("benchmark report exceeds 8 MiB")
	}
	f, e := os.CreateTemp(s.dataDir, ".benchmark-save-")
	if e != nil {
		return e
	}
	defer os.Remove(f.Name())
	if e = f.Chmod(0600); e == nil {
		_, e = f.Write(b)
	}
	if e == nil {
		e = f.Sync()
	}
	if closeErr := f.Close(); e == nil {
		e = closeErr
	}
	if e != nil {
		return e
	}
	if e = os.Rename(f.Name(), filepath.Join(s.dataDir, "benchmark-"+name+".json")); e != nil {
		return e
	}
	d, e := os.Open(s.dataDir)
	if e != nil {
		return e
	}
	defer d.Close()
	return d.Sync()
}
func (s *Server) benchmarkAPI(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	if !s.authorised(r, r.Method != http.MethodGet) {
		fail(w, 403, "valid origin, session and CSRF token required")
		return
	}
	if r.URL.Path == "/api/v1/benchmarks/cancel" {
		if r.Method != http.MethodPost {
			fail(w, 405, "method not allowed")
			return
		}
		var in struct{}
		if decode(r, &in) != nil {
			fail(w, 400, "invalid request")
			return
		}
		s.mu.Lock()
		cancel := s.benchmarkCancel
		s.mu.Unlock()
		if cancel != nil {
			cancel()
		}
		jsonBody(w, map[string]bool{"cancel_requested": cancel != nil})
		return
	}
	if r.Method != http.MethodGet && !s.benchmarkMu.TryLock() {
		fail(w, 409, "benchmark already running; cancel or wait")
		return
	}
	if r.Method != http.MethodGet {
		defer s.benchmarkMu.Unlock()
	}
	switch r.URL.Path {
	case "/api/v1/benchmarks/clear":
		if r.Method != http.MethodPost {
			fail(w, 405, "method not allowed")
			return
		}
		var in struct{}
		if decode(r, &in) != nil {
			fail(w, 400, "invalid request")
			return
		}
		for _, name := range []string{"cases", "latest"} {
			if e := os.Remove(filepath.Join(s.dataDir, "benchmark-"+name+".json")); e != nil && !os.IsNotExist(e) {
				fail(w, 500, "cannot clear saved benchmarks")
				return
			}
		}
		jsonBody(w, map[string]bool{"cleared": true})
		return
	case "/api/v1/benchmarks/cases":
		if r.Method == http.MethodGet {
			cases := []benchmark.LabCase{}
			if e := s.benchmarkRead("cases", &cases); e != nil && !os.IsNotExist(e) {
				fail(w, 500, "saved cases unavailable")
				return
			}
			jsonBody(w, map[string]any{"cases": cases, "max_cases": benchmark.LabMaxCases})
			return
		}
		if r.Method != http.MethodPost {
			fail(w, 405, "method not allowed")
			return
		}
		var in struct {
			Cases []benchmark.LabCase `json:"cases"`
		}
		if decode(r, &in) != nil || benchmark.ValidateLabCases(in.Cases) != nil {
			fail(w, 400, "declare 1–40 unique bounded cases with valid expected states and found paths")
			return
		}
		if e := s.benchmarkWrite("cases", in.Cases); e != nil {
			fail(w, 500, "cannot save cases")
			return
		}
		jsonBody(w, in)
		return
	case "/api/v1/benchmarks/latest", "/api/v1/benchmarks/export":
		if r.Method != http.MethodGet {
			fail(w, 405, "method not allowed")
			return
		}
		var out benchmark.LabReport
		if e := s.benchmarkRead("latest", &out); e != nil {
			fail(w, 404, "no saved benchmark report")
			return
		}
		if r.URL.Path == "/api/v1/benchmarks/export" {
			w.Header().Set("Content-Disposition", `attachment; filename="aios-benchmark.json"`)
		}
		jsonBody(w, out)
		return
	case "/api/v1/benchmarks/run":
		if r.Method != http.MethodPost {
			fail(w, 405, "method not allowed")
			return
		}
		var in struct {
			Mode string `json:"mode"`
		}
		if decode(r, &in) != nil || (in.Mode != "demo" && in.Mode != "my_knowledge") {
			fail(w, 400, "mode must be demo or my_knowledge")
			return
		}
		cases := benchmark.DemoCases()
		if in.Mode == "my_knowledge" {
			if s.benchmarkRead("cases", &cases) != nil || benchmark.ValidateLabCases(cases) != nil {
				fail(w, 400, "save known-answer cases before replay")
				return
			}
		}
		s.mu.Lock()
		pending := s.removing != ""
		s.mu.Unlock()
		if pending {
			fail(w, 409, "repository removal in progress")
			return
		}
		if _, e := resourcepolicy.CheckBudget(s.dataDir, resourcepolicy.MaxOwnedBytes, resourcepolicy.MinimumFreeBytes, 1<<30); e != nil {
			fail(w, 503, "storage budget prevents benchmark capture")
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Minute)
		defer cancel()
		s.mu.Lock()
		s.benchmarkCancel = cancel
		s.mu.Unlock()
		defer func() { s.mu.Lock(); s.benchmarkCancel = nil; s.mu.Unlock() }()
		// Remove only interrupted workspaces with our explicit ownership marker.
		entries, readErr := os.ReadDir(s.dataDir)
		if readErr != nil {
			fail(w, 500, "cannot inspect benchmark workspace")
			return
		}
		for _, entry := range entries {
			if !strings.HasPrefix(entry.Name(), ".benchmark-work-") || !entry.IsDir() {
				continue
			}
			old := filepath.Join(s.dataDir, entry.Name())
			marker, readErr := os.ReadFile(filepath.Join(old, ".owner"))
			if readErr == nil && benchmarkOwnerDead(marker, func(pid int) error {
				process, e := os.FindProcess(pid)
				if e != nil {
					return e
				}
				return process.Signal(syscall.Signal(0))
			}) {
				if e := os.RemoveAll(old); e != nil {
					fail(w, 500, "cannot clean interrupted benchmark")
					return
				}
			}
		}
		work, e := os.MkdirTemp(s.dataDir, ".benchmark-work-")
		if e != nil {
			fail(w, 500, "cannot create benchmark workspace")
			return
		}
		defer os.RemoveAll(work)
		if e = os.WriteFile(filepath.Join(work, ".owner"), []byte(fmt.Sprintf("aios-benchmark-v1\n%d", os.Getpid())), 0600); e != nil {
			fail(w, 500, "cannot mark benchmark workspace")
			return
		}
		started := time.Now()
		var db *store.Store

		if in.Mode == "demo" {
			db, e = benchmark.BuildDemo(ctx, work)
		} else {
			// Hold a rollback-journal read lease across size admission and copy;
			// maintenance cannot commit a growing generation between the two.
			leaseDB, leaseErr := store.OpenReadOnly(s.dataDir)
			if leaseErr != nil {
				fail(w, 503, "snapshot lease unavailable")
				return
			}
			defer leaseDB.Close()
			tx, leaseErr := leaseDB.DB().BeginTx(ctx, nil)
			if leaseErr != nil {
				fail(w, 503, "snapshot lease unavailable")
				return
			}
			defer tx.Rollback()
			var journal string
			var pages, pageSize int64
			e = tx.QueryRowContext(ctx, "PRAGMA journal_mode").Scan(&journal)
			if e == nil && journal != "delete" {
				fail(w, 503, "benchmark size lease requires rollback journal")
				return
			}
			if e == nil {
				e = tx.QueryRowContext(ctx, "SELECT count(*) FROM active_generations").Scan(&pages)
			}
			if e == nil {
				e = tx.QueryRowContext(ctx, "PRAGMA page_count").Scan(&pages)
			}
			if e == nil {
				e = tx.QueryRowContext(ctx, "PRAGMA page_size").Scan(&pageSize)
			}
			if e == nil && (pages < 0 || pageSize <= 0 || pages > (256<<20)/pageSize) {
				fail(w, 413, "indexed database exceeds 256 MiB comparison copy bound")
				return
			}

			if e == nil {
				e = os.Mkdir(filepath.Join(work, "kb"), 0700)
			}
			p := filepath.Join(work, "kb", store.DatabaseName)
			if e == nil {
				_, e = s.db.DB().ExecContext(ctx, "VACUUM INTO ?", p)
				_ = tx.Rollback()
			}
			if e == nil {
				var st os.FileInfo
				st, e = os.Stat(p)
				if e == nil && st.Size() > 256<<20 {
					e = fmt.Errorf("copy exceeds 256 MiB")
				}
				if e == nil {
					e = os.Chmod(p, 0600)
				}
			}
			if e == nil {
				db, e = store.OpenReadOnly(filepath.Join(work, "kb"))
			}
		}
		captureMS := float64(time.Since(started).Microseconds()) / 1000
		if e != nil {
			fail(w, 500, "benchmark capture failed; original knowledge unchanged")
			return
		}
		defer db.Close()
		s.mu.Lock()
		configured := s.setup.Active.SourceRepositories()
		s.mu.Unlock()
		if in.Mode == "demo" {
			configured = nil
		}
		out, e := benchmark.RunLab(ctx, db, work, in.Mode, cases, configured...)
		if e != nil {
			if ctx.Err() != nil {
				fail(w, 408, "benchmark canceled or exceeded two-minute budget")
			} else {
				fail(w, 422, "benchmark corpus unavailable, invalid or exceeds bounds")
			}
			return
		}
		if in.Mode == "demo" {
			out.IndexingCost = map[string]any{"cold_index_ms": captureMS, "warm_index_ms": nil, "warm_index_note": "not run"}
		} else {
			out.IndexingCost = map[string]any{"snapshot_copy_ms": captureMS, "cold_index_ms": nil, "warm_index_ms": nil, "index_note": "existing indexed generation; original indexing cost unavailable"}
		}
		var prior benchmark.LabReport
		if s.benchmarkRead("latest", &prior) == nil && prior.Mode == in.Mode {
			for i := range out.Results {
				for _, old := range prior.Results {
					a, b := out.Results[i].Case, old.Case
					a.DeclaredSnapshot = ""
					b.DeclaredSnapshot = ""
					if a == b {
						v := old.KBCorrect
						out.Results[i].PreviousKBCorrect = &v
						out.Results[i].Regressed = v && !out.Results[i].KBCorrect
						break
					}
				}
			}
		}
		if e = s.benchmarkWrite("latest", out); e != nil {
			fail(w, 500, "benchmark report could not persist")
			return
		}
		if in.Mode == "my_knowledge" {
			for i := range cases {
				if cases[i].DeclaredSnapshot == "" {
					cases[i].DeclaredSnapshot = out.SourceSnapshotSHA256
				}
			}
			if e = s.benchmarkWrite("cases", cases); e != nil {
				fail(w, 500, "report saved but case binding failed")
				return
			}
		}
		jsonBody(w, out)
		return
	}
	fail(w, 404, "unknown benchmark route")
}

// Ambiguous ownership fails closed: only a valid product marker and an explicit
// no-such-process result authorize cleanup, including on non-Unix platforms.
func benchmarkOwnerDead(marker []byte, probe func(int) error) bool {
	parts := strings.Split(string(marker), "\n")
	if len(parts) != 2 || parts[0] != "aios-benchmark-v1" {
		return false
	}
	pid, e := strconv.Atoi(parts[1])
	if e != nil || pid <= 0 {
		return false
	}
	e = probe(pid)
	return errors.Is(e, os.ErrProcessDone) || errors.Is(e, syscall.ESRCH)
}
