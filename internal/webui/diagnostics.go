package webui

import (
	"net/http"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"strings"

	"github.com/AdamNi-7080/AIOS/internal/observability"
)

type diagnosticResponse struct {
	http.ResponseWriter
	status int
}

func (w *diagnosticResponse) WriteHeader(code int) {
	w.status = code
	w.ResponseWriter.WriteHeader(code)
}

// DiagnosticDetails contains aggregate operational facts only. In particular,
// repository IDs, source remotes, error strings, query text and excerpts are
// never copied from existing API responses into this surface.
func (s *Server) diagnosticDetails(r *http.Request) map[string]any {
	version := "devel"
	if info, ok := debug.ReadBuildInfo(); ok && info.Main.Version != "" {
		version = info.Main.Version
	}
	if version == "(devel)" {
		version = "devel"
	}
	details := map[string]any{
		"schema_version": 1, "product_version": version, "go_version": runtime.Version(),
		"knowledge_ir_format": "knowledge-ir-v10", "telemetry_policy": "local_redacted_v1",
		"stages": map[string]int{}, "coverage": map[string]int{}, "timings_ms": map[string]int{},
	}
	if _, records, err := s.obs.Snapshot(); err == nil {
		timings := map[string]int64{}
		counts := map[string]int64{}
		for _, row := range records {
			if row.Kind != "span" {
				continue
			}
			timings[row.Operation] += row.Duration
			counts[row.Operation]++
		}
		averages := map[string]int64{}
		for operation, total := range timings {
			averages[operation] = total / counts[operation]
		}
		details["timings_ms"] = averages
	}
	_, rootRecords, rootErr := observability.ReadExisting(filepath.Dir(s.dataDir))
	if rootErr == nil {
		upgrade := map[string]int{}
		for _, row := range rootRecords {
			if row.Operation == "upgrade" || row.Operation == "upgrade_recovery" {
				upgrade[row.Operation+"_"+row.Outcome]++
			}
		}
		details["upgrade_activity"] = upgrade
	}
	if status, err := s.readService().Status(r.Context()); err == nil {
		active := 0
		for _, repo := range status.Repositories {
			if repo.Active {
				active++
			}
		}
		details["coverage"] = map[string]int{"configured_repositories": len(status.Repositories), "active_repositories": active}
		projection := status.Projection
		if projection != "ready" && projection != "no_active_generation" && projection != "unavailable" {
			projection = "unavailable"
		}
		details["projection_state"] = projection
	}
	s.maintenanceMu.Lock()
	if s.maintainer != nil {
		counts := map[string]int{}
		for _, job := range s.maintainer.Status().Jobs {
			state := job.State
			if !safeDiagnosticStage(state) {
				state = "other"
			}
			counts[state]++
		}
		details["stages"] = counts
	}
	s.maintenanceMu.Unlock()
	return details
}
func safeDiagnosticStage(value string) bool {
	switch value {
	case "idle", "pending", "running", "retry_wait", "exhausted", "failed", "completed", "interrupted":
		return true
	}
	return false
}

func (s *Server) diagnosticsAPI(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	path := r.URL.Path
	if path == "/api/v1/diagnostics/export" {
		if r.Method != http.MethodPost || !s.authorised(r, true) {
			fail(w, 403, "valid origin, session, and CSRF token required")
			return
		}
		var in struct {
			Expanded    bool `json:"expanded"`
			Acknowledge bool `json:"acknowledge_warning"`
		}
		if decode(r, &in) != nil {
			fail(w, 400, "invalid diagnostics request")
			return
		}
		if in.Expanded && !in.Acknowledge {
			fail(w, 400, "expanded diagnostics require explicit warning acknowledgement")
			return
		}
		if s.obs == nil {
			fail(w, 503, "local diagnostics unavailable")
			return
		}
		ctx, span := s.obs.Start(r.Context(), "diagnostic_export", map[string]string{"result": "requested"})
		_ = ctx
		_, rootRecords, _ := observability.ReadExisting(filepath.Dir(s.dataDir))
		archive, err := s.obs.Archive(in.Expanded, s.diagnosticDetails(r), rootRecords...)
		if err != nil {
			observability.End(span, true)
			fail(w, 503, "diagnostic export unavailable")
			return
		}
		w.Header().Set("Content-Type", "application/zip")
		w.Header().Set("Content-Disposition", `attachment; filename="aios-diagnostics.zip"`)
		w.Header().Set("X-Content-Type-Options", "nosniff")
		n, writeErr := w.Write(archive)
		observability.End(span, writeErr != nil || n != len(archive))
		return
	}
	if r.Method != http.MethodGet || !s.authorised(r, false) {
		fail(w, 403, "authorised same-origin session required")
		return
	}
	status, _, err := s.obs.Snapshot()
	if err != nil {
		status.Available = false
	}
	if strings.HasSuffix(path, "/status") {
		jsonBody(w, status)
		return
	}
	if strings.HasSuffix(path, "/details") {
		details := s.diagnosticDetails(r)
		details["otel"] = status
		jsonBody(w, details)
		return
	}
	fail(w, 404, "diagnostics endpoint unavailable")
}
