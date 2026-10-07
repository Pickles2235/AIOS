// Package observability records bounded, local OpenTelemetry spans and metrics.
// All persisted fields are selected from a closed operational vocabulary. Raw
// source, query, path, remote, error and credential bytes never enter a sink.
package observability

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/metric"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
)

const maxFile = 256 << 10
const maxFiles = 3
const maxArchive = 1 << 20

var safeToken = regexp.MustCompile(`^[a-z][a-z0-9_]{0,39}$`)
var versionToken = regexp.MustCompile(`^v?[0-9]+(\.[0-9]+){1,3}([+-][a-zA-Z0-9.]+)?$`)
var safeValues = map[string]bool{
	"transaction": true, "recovery": true, "requested": true, "ok": true, "failed": true, "error": true, "success": true,
	"get": true, "post": true, "pending": true, "running": true, "retry_wait": true, "exhausted": true, "idle": true,
	"completed": true, "cancelled": true, "interrupted": true, "validation": true, "activation": true, "health": true,
	"commit": true, "rollback": true, "other": true,
}
var operations = map[string]bool{
	"http_request": true, "query": true, "ingest": true, "job": true,
	"upgrade": true, "upgrade_recovery": true, "diagnostic_export": true,
	"setup": true, "preview": true,
}

type Record struct {
	Timestamp string            `json:"timestamp"`
	Kind      string            `json:"kind"`
	Operation string            `json:"operation"`
	Trace     string            `json:"trace"`
	Span      string            `json:"span"`
	Parent    string            `json:"parent,omitempty"`
	Duration  int64             `json:"duration_ms"`
	Outcome   string            `json:"outcome"`
	Fields    map[string]string `json:"fields,omitempty"`
}

type Status struct {
	OTEL        bool              `json:"otel"`
	LocalOnly   bool              `json:"local_only"`
	Correlation bool              `json:"correlation"`
	Available   bool              `json:"available"`
	Retention   int               `json:"retention_bytes"`
	Operations  map[string]uint64 `json:"operations"`
	Dropped     uint64            `json:"dropped"`
	Version     string            `json:"version"`
}

type Collector struct {
	mu       sync.Mutex
	root     *secureRoot
	provider *sdktrace.TracerProvider
	meter    *sdkmetric.MeterProvider
	counter  metric.Int64Counter
	counts   map[string]uint64
	dropped  uint64
}

type contextKey struct{}

func WithCollector(ctx context.Context, c *Collector) context.Context {
	return context.WithValue(ctx, contextKey{}, c)
}
func From(ctx context.Context) *Collector { c, _ := ctx.Value(contextKey{}).(*Collector); return c }

// Open creates only an owned local diagnostic directory. Callers performing
// read-only health validation must not call Open.
func Open(dataDir string) (*Collector, error) {
	root, err := openRoot(dataDir)
	if err != nil {
		return nil, err
	}
	c := &Collector{root: root, counts: map[string]uint64{}}
	c.provider = sdktrace.NewTracerProvider(sdktrace.WithSyncer(c))
	c.meter = sdkmetric.NewMeterProvider(sdkmetric.WithReader(sdkmetric.NewManualReader()))
	c.counter, err = c.meter.Meter("aios.local").Int64Counter("aios.operations")
	if err != nil {
		root.close()
		return nil, err
	}
	return c, nil
}

func (c *Collector) Close() error {
	if c == nil {
		return nil
	}
	_ = c.provider.Shutdown(context.Background())
	_ = c.meter.Shutdown(context.Background())
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.root.close()
}

// Opaque hashes identifiers before they reach traces, metrics or exports.
func Opaque(value string) string {
	if value == "" {
		return ""
	}
	h := sha256.Sum256([]byte(value))
	return "h_" + hex.EncodeToString(h[:12])
}
func OpaqueIfNeeded(value string) string {
	if value == "" || (len(value) == 26 && value[:2] == "h_" && isHex(value[2:])) {
		return value
	}
	return Opaque(value)
}

// SafeFields deliberately has no freeform value path. Identifiers are opaque;
// everything else must be a short token or is replaced with "other".
func SafeFields(fields map[string]string) map[string]string {
	clean := map[string]string{}
	for key, value := range fields {
		switch key {
		case "repository", "revision", "generation", "source", "query", "job":
			if value != "" {
				clean[key] = Opaque(value)
			}
		case "stage", "state", "result", "code", "method":
			if safeToken.MatchString(value) && safeValues[value] {
				clean[key] = value
			} else {
				clean[key] = "other"
			}
		}
	}
	return clean
}

func (c *Collector) Start(ctx context.Context, operation string, fields map[string]string) (context.Context, trace.Span) {
	if c == nil {
		return ctx, trace.SpanFromContext(ctx)
	}
	if !operations[operation] {
		operation = "http_request"
	}
	attrs := []attribute.KeyValue{}
	for key, value := range SafeFields(fields) {
		attrs = append(attrs, attribute.String(key, value))
	}
	sort.Slice(attrs, func(i, j int) bool { return attrs[i].Key < attrs[j].Key })
	return c.provider.Tracer("aios.local").Start(ctx, operation, trace.WithAttributes(attrs...))
}

func End(span trace.Span, failed bool) {
	if failed {
		span.SetStatus(codes.Error, "operation_failed")
	} else {
		span.SetStatus(codes.Ok, "")
	}
	span.End()
}

// ExportSpans is a synchronous SDK exporter. Never persist SDK attributes
// without the final allowlist; an external parent ID is hashed as well.
func (c *Collector) ExportSpans(ctx context.Context, spans []sdktrace.ReadOnlySpan) error {
	for _, span := range spans {
		name := span.Name()
		if !operations[name] {
			name = "http_request"
		}
		fields := map[string]string{}
		for _, a := range span.Attributes() {
			fields[string(a.Key)] = a.Value.AsString()
		}
		outcome := "ok"
		if span.Status().Code == codes.Error {
			outcome = "error"
		}
		record := Record{Timestamp: span.EndTime().UTC().Format(time.RFC3339Nano), Kind: "span", Operation: name,
			Trace: Opaque(span.SpanContext().TraceID().String()), Span: Opaque(span.SpanContext().SpanID().String()),
			Duration: max(0, span.EndTime().Sub(span.StartTime()).Milliseconds()), Outcome: outcome, Fields: safeExportFields(fields)}
		if parent := span.Parent(); parent.IsValid() {
			record.Parent = Opaque(parent.SpanID().String())
		}
		if err := c.write(record); err != nil {
			return errors.New("local diagnostic sink unavailable")
		}
		log := record
		log.Kind = "log"
		log.Duration = 0
		if err := c.write(log); err != nil {
			return errors.New("local diagnostic sink unavailable")
		}
		c.counter.Add(context.Background(), 1) // no active span: metric exemplars cannot carry trace IDs
	}
	return nil
}

func safeExportFields(fields map[string]string) map[string]string {
	clean := map[string]string{}
	for key, value := range fields {
		switch key {
		case "repository", "revision", "generation", "source", "query", "job":
			// Start already made these opaque; reject any injected raw value.
			if len(value) == 26 && value[:2] == "h_" && isHex(value[2:]) {
				clean[key] = value
			} else {
				clean[key] = Opaque(value)
			}
		case "stage", "state", "result", "code", "method":
			if safeToken.MatchString(value) && safeValues[value] {
				clean[key] = value
			} else {
				clean[key] = "other"
			}
		}
	}
	return clean
}
func isHex(v string) bool { _, err := hex.DecodeString(v); return err == nil }

func (c *Collector) Shutdown(context.Context) error { return nil }

func (c *Collector) write(record Record) error {
	b, err := json.Marshal(record)
	if err != nil {
		return err
	}
	if len(b)+1 > 4096 {
		return fmt.Errorf("diagnostic record exceeds bound")
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if err = c.root.append(append(b, '\n'), maxFile, maxFiles); err != nil {
		c.dropped++
		return err
	}
	if record.Kind == "span" {
		c.counts[record.Operation]++
	}
	return nil
}

func (c *Collector) Snapshot() (Status, []Record, error) {
	if c == nil {
		return Status{Version: "1", LocalOnly: true, Retention: maxFile * maxFiles}, nil, errors.New("diagnostics unavailable")
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	status := Status{OTEL: true, LocalOnly: true, Correlation: true, Available: true, Retention: maxFile * maxFiles, Operations: map[string]uint64{}, Dropped: c.dropped, Version: "1"}
	for key, value := range c.counts {
		status.Operations[key] = value
	}
	data, err := c.root.read(maxFile, maxFiles)
	if err != nil {
		status.Available = false
		return status, nil, err
	}
	var records []Record
	for _, line := range bytes.Split(data, []byte{'\n'}) {
		if len(line) == 0 {
			continue
		}
		var row Record
		if json.Unmarshal(line, &row) == nil && operations[row.Operation] {
			if _, err := time.Parse(time.RFC3339Nano, row.Timestamp); err != nil {
				row.Timestamp = ""
			}
			if row.Kind != "span" && row.Kind != "log" {
				row.Kind = "log"
			}
			if row.Outcome != "ok" && row.Outcome != "error" {
				row.Outcome = "unknown"
			}
			row.Fields = safeExportFields(row.Fields)
			row.Trace, row.Span, row.Parent = opaqueIfNeeded(row.Trace), opaqueIfNeeded(row.Span), opaqueIfNeeded(row.Parent)
			records = append(records, row)
		}
	}
	return status, records, nil
}
func opaqueIfNeeded(v string) string {
	if v == "" || (len(v) == 26 && v[:2] == "h_" && isHex(v[2:])) {
		return v
	}
	return Opaque(v)
}

// Archive never copies arbitrary files. Expanded output includes bounded
// sanitized individual spans; default output only includes aggregate metrics.
func (c *Collector) Archive(expanded bool, details any, extra ...Record) ([]byte, error) {
	status, records, err := c.Snapshot()
	if err != nil {
		return nil, err
	}
	if len(extra) > 256 {
		extra = extra[len(extra)-256:]
	}
	records = append(records, extra...)
	var buffer bytes.Buffer
	w := zip.NewWriter(&buffer)
	add := func(name string, value any) error {
		file, e := w.Create(name)
		if e != nil {
			return e
		}
		return json.NewEncoder(file).Encode(value)
	}
	if err = add("status.json", status); err == nil {
		err = add("health.json", safeArchiveDetails(details))
	}
	if err == nil && expanded {
		err = add("spans.json", records)
	}
	if closeErr := w.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return nil, err
	}
	if buffer.Len() > maxArchive {
		return nil, fmt.Errorf("diagnostic archive exceeds bound")
	}
	return buffer.Bytes(), nil
}

func safeArchiveDetails(value any) map[string]any {
	in, ok := value.(map[string]any)
	if !ok {
		return map[string]any{"schema_version": 1}
	}
	out := map[string]any{"schema_version": 1, "knowledge_ir_format": "knowledge-ir-v10", "telemetry_policy": "local_redacted_v1"}
	if version, ok := in["product_version"].(string); ok && len(version) <= 40 && (version == "devel" || versionToken.MatchString(version)) {
		out["product_version"] = version
	}
	if version, ok := in["go_version"].(string); ok && len(version) <= 40 && strings.HasPrefix(version, "go1.") {
		out["go_version"] = version
	}
	if state, ok := in["projection_state"].(string); ok && (state == "ready" || state == "no_active_generation" || state == "unavailable") {
		out["projection_state"] = state
	}
	if coverage, ok := in["coverage"].(map[string]int); ok {
		out["coverage"] = map[string]int{"configured_repositories": max(0, coverage["configured_repositories"]), "active_repositories": max(0, coverage["active_repositories"])}
	}
	if stages, ok := in["stages"].(map[string]int); ok {
		clean := map[string]int{}
		for key, n := range stages {
			if safeValues[key] && n >= 0 {
				clean[key] = n
			}
		}
		out["stages"] = clean
	}
	if timings, ok := in["timings_ms"].(map[string]int64); ok {
		clean := map[string]int64{}
		for key, n := range timings {
			if operations[key] && n >= 0 {
				clean[key] = n
			}
		}
		out["timings_ms"] = clean
	}
	if activity, ok := in["upgrade_activity"].(map[string]int); ok {
		clean := map[string]int{}
		for key, n := range activity {
			if (key == "upgrade_ok" || key == "upgrade_error" || key == "upgrade_recovery_ok" || key == "upgrade_recovery_error") && n >= 0 {
				clean[key] = n
			}
		}
		out["upgrade_activity"] = clean
	}
	return out
}

// ReadExisting observes root-level upgrade records without creating metadata.
// It is safe to call from read-only upgrade health and rejected transactions.
func ReadExisting(base string) (Status, []Record, error) {
	root, err := openExistingRoot(base)
	if err != nil {
		return Status{}, nil, err
	}
	defer root.close()
	c := &Collector{root: root, counts: map[string]uint64{}}
	return c.Snapshot()
}

// Copying a collector snapshot to an output is intentionally unsupported.
var _ io.Writer = (*noRawWriter)(nil)

type noRawWriter struct{}

func (*noRawWriter) Write([]byte) (int, error) {
	return 0, errors.New("raw diagnostic output disabled")
}
