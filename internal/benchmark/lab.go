package benchmark

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/AdamNi-7080/AIOS/internal/catalog"
	"github.com/AdamNi-7080/AIOS/internal/discover"
	"github.com/AdamNi-7080/AIOS/internal/extract"
	"github.com/AdamNi-7080/AIOS/internal/knowledge"
	"github.com/AdamNi-7080/AIOS/internal/model"
	"github.com/AdamNi-7080/AIOS/internal/store"
)

const LabMaxBytes int64 = 64 << 20
const LabMaxCases = 40

type LabCase struct {
	Case
	Repository       string `json:"repository,omitempty"`
	Capability       string `json:"capability,omitempty"`
	DeclaredSnapshot string `json:"declared_snapshot,omitempty"`
}
type ManifestFile struct {
	Repository string `json:"repository"`
	Path       string `json:"path"`
	SHA256     string `json:"sha256"`
	Bytes      int64  `json:"bytes"`
	Generation string `json:"generation"`
	Revision   string `json:"revision"`
}
type LabResult struct {
	PreviousKBCorrect       *bool                 `json:"previous_kb_correct,omitempty"`
	Regressed               bool                  `json:"regressed"`
	Case                    LabCase               `json:"case"`
	KBOutcome               string                `json:"kb_outcome"`
	KBState                 string                `json:"kb_state"`
	KBCorrect               bool                  `json:"kb_correct"`
	StateCorrect            bool                  `json:"state_correct"`
	GrepCorrect             bool                  `json:"grep_correct"`
	GrepWins                bool                  `json:"grep_wins"`
	KBWins                  bool                  `json:"kb_wins"`
	StaleExpectation        bool                  `json:"stale_expectation"`
	FirstQueryMS            float64               `json:"first_query_ms"`
	WarmQueryMS             float64               `json:"warm_query_ms"`
	BaselineMS              float64               `json:"baseline_ms"`
	SourceFilesRead         int                   `json:"source_files_read"`
	SourceBytesRead         int64                 `json:"source_bytes_read"`
	ContextBytes            int                   `json:"context_bytes"`
	EstimatedTokens         int                   `json:"estimated_tokens"`
	BaselineContextBytes    int                   `json:"baseline_context_bytes"`
	BaselineTruncated       bool                  `json:"baseline_truncated"`
	BaselineEstimatedTokens int                   `json:"baseline_estimated_tokens"`
	KBSourceBytesRead       int64                 `json:"kb_source_bytes_read"`
	KB                      Measurement           `json:"kb_metrics"`
	Baseline                Measurement           `json:"baseline_metrics"`
	Query                   knowledge.QueryResult `json:"query_result"`
	Evidence                []knowledge.Excerpt   `json:"evidence"`
	Error                   string                `json:"error,omitempty"`
}
type LabReport struct {
	SourcePreparationMS    float64              `json:"source_preparation_ms"`
	Mode                   string               `json:"mode"`
	Isolated               bool                 `json:"isolated"`
	CreatedAt              time.Time            `json:"created_at"`
	SourceSnapshotSHA256   string               `json:"source_snapshot_sha256"`
	BaselineSnapshotSHA256 string               `json:"baseline_snapshot_sha256"`
	Manifest               []ManifestFile       `json:"manifest"`
	BaselineInvocation     string               `json:"baseline_invocation"`
	Temperature            string               `json:"temperature"`
	IndexingCost           map[string]any       `json:"indexing_cost"`
	MeasurementNotes       []string             `json:"measurement_notes"`
	Results                []LabResult          `json:"results"`
	Aggregates             map[string]Aggregate `json:"aggregates"`
}

func ValidateLabCases(cases []LabCase) error {
	if len(cases) == 0 || len(cases) > LabMaxCases {
		return fmt.Errorf("declare 1 to %d cases", LabMaxCases)
	}
	seen := map[string]bool{}
	for _, c := range cases {
		if c.ID == "" || len(c.ID) > 80 || seen[c.ID] || strings.TrimSpace(c.Query) == "" || len(c.Query) > 256 || len(c.ExpectedSnippet) > 1024 || len(c.ExpectedPath) > 1024 || len(c.ExpectedRepository) > 63 || len(c.Repository) > 63 || c.ExpectedLine < 0 {
			return fmt.Errorf("invalid or duplicate bounded case")
		}
		seen[c.ID] = true
		if c.ExpectedState != "found" && c.ExpectedState != "not_found" && c.ExpectedState != "unknown" {
			return fmt.Errorf("declare expected_state: found, not_found or unknown")
		}
		if c.ExpectedState == "found" && (c.ExpectedPath == "" || c.ExpectedRepository == "") {
			return fmt.Errorf("found requires expected repository and path")
		}
		if c.Capability != "" && c.Capability != "lexical" && c.Capability != "structural" && c.Capability != "path" {
			return fmt.Errorf("unsupported capability")
		}
		if c.DeclaredSnapshot != "" && !validDigest(c.DeclaredSnapshot) {
			return fmt.Errorf("invalid declared snapshot")
		}
	}
	return nil
}
func validDigest(s string) bool {
	b, e := hex.DecodeString(s)
	return e == nil && len(b) == 32 && strings.ToLower(s) == s
}
func hashBytes(b []byte) string   { s := sha256.Sum256(b); return hex.EncodeToString(s[:]) }
func elapsed(t time.Time) float64 { return float64(time.Since(t).Microseconds()) / 1000 }

// Demo cases are declared independently of retrieval output. Duplicate names and
// an unsupported structural route deliberately expose wrong and unknown answers.
func DemoCases() []LabCase {
	return []LabCase{
		{Case: Case{ID: "symbol", Query: "routeCharge", ExpectedRepository: "demo", ExpectedPath: "src/ChargeRouter.ts", ExpectedLine: 1, ExpectedSnippet: "routeCharge", ExpectedState: "found"}},
		{Case: Case{ID: "literal-route", Query: "refund-ledger-marker", ExpectedRepository: "demo", ExpectedPath: "docs/route.proto", ExpectedLine: 1, ExpectedSnippet: "refund-ledger-marker", ExpectedState: "found"}, Capability: "structural"},
		{Case: Case{ID: "unsupported-route", Query: "missingDynamicRoute", ExpectedRepository: "demo", ExpectedState: "unknown"}, Capability: "structural"},
		{Case: Case{ID: "duplicate-name-ambiguous", Query: "sharedName", ExpectedRepository: "demo", ExpectedState: "unknown"}},
		{Case: Case{ID: "operator-exploratory", Query: "->>", ExpectedRepository: "demo", ExpectedPath: "docs/sequence.md", ExpectedLine: 1, ExpectedSnippet: "Client->>Ledger: refund", ExpectedState: "found"}},
	}
}
func BuildDemo(ctx context.Context, root string) (*store.Store, error) {
	source := filepath.Join(root, "source")
	for p, c := range map[string]string{"src/ChargeRouter.ts": "export function routeCharge() { return 'charged'; }\n", "src/A.ts": "export function sharedName() { return 'first'; }\n", "src/Z.ts": "export function sharedName() { return 'second'; }\n", "docs/route.proto": "// refund-ledger-marker\nservice DynamicRoute {}\n", "docs/sequence.md": "Client->>Ledger: refund\n"} {
		target := filepath.Join(source, p)
		if err := os.MkdirAll(filepath.Dir(target), 0700); err != nil {
			return nil, err
		}
		if err := os.WriteFile(target, []byte(c), 0600); err != nil {
			return nil, err
		}
	}
	files, coverage, err := discover.FilesWithCoverage(model.Repository{ID: "demo", Root: source}, model.Limits{MaxFileBytes: 1 << 20, MaxFilesPerRepo: 100, MaxTotalBytesPerRepo: LabMaxBytes, MaxResults: 100})
	if err != nil {
		return nil, err
	}
	db, err := store.OpenWriter(filepath.Join(root, "kb"))
	if err != nil {
		return nil, err
	}
	var symbols []model.Symbol
	var edges []model.Edge
	for _, f := range files {
		sy, ed, e := extract.Parse(f)
		if e != nil {
			db.Close()
			return nil, e
		}
		symbols = append(symbols, sy...)
		edges = append(edges, ed...)
	}
	snap := model.Snapshot{RepoID: "demo", Root: source, Git: model.GitState{Commit: "demo-v1"}, ContentHash: store.ContentSnapshot(files), FileCount: len(files), IndexedAt: time.Unix(0, 0).UTC(), ExtractorVersions: model.ExtractorVersion}
	for _, f := range files {
		snap.TotalBytes += f.Size
	}
	if err = db.ReplaceRepositoryWithCoverage(ctx, snap, files, symbols, edges, coverage); err != nil {
		db.Close()
		return nil, err
	}
	return db, nil
}

// RunLab reads only a private, frozen store. Literal inputs are materialized from
// the very canonical source bytes used by that store; no live workspace is read.
func RunLab(ctx context.Context, db *store.Store, root, mode string, cases []LabCase, configured ...model.Repository) (LabReport, error) {
	prepareStarted := time.Now()
	out := LabReport{Mode: mode, Isolated: true, CreatedAt: time.Now().UTC(), Temperature: "cold", BaselineInvocation: "aios.literal.v1: fixed-string, case-sensitive UTF-8 line scan; repository/path/line order; all captured files; no regex, shell or ripgrep", IndexingCost: map[string]any{}, MeasurementNotes: []string{
		"First and repeated query wall time include API retrieval and evidence reads. OS and SQLite caches are uncontrolled; cold means first query in this comparison process, not cold physical storage.",
		"Source bytes/files count successful logical file reads by the literal baseline. KB reads canonical SQLite data and zero source files; physical disk bytes and SQLite page reads are unavailable.",
		"Context bytes are UTF-8 JSON bytes of returned query plus excerpts; estimated tokens = ceil(context_bytes / 4), not a tokenizer or billed usage.",
		"Answer correctness scores the first answer against declared repository/path/line/snippet. Rank/recall expose later matches. Unknown is never answer-correct; state correctness is separate. Equal case weighting.",
		"My Knowledge is an immutable copy of the indexed generation, which can be older than the workspace. No workspace is inspected. Excluded/unreadable source bytes are not available to either method.",
		"Demo operator case is exploratory: selected to probe punctuation retrieval after initial Demo inspection, then source/expectation frozen before scoring. Duplicate-name case declares ambiguity (unknown); a returned found status is a state classification miss, not proof of a false factual claim.",
		"Export contains declared queries, snippets and returned source excerpts. Only the latest report and up to 40 declared cases are retained locally; temporary corpus copies are removed.",
	}}
	if e := ValidateLabCases(cases); e != nil {
		return out, e
	}
	var count, totalBytes int64
	if e := db.DB().QueryRowContext(ctx, `SELECT count(*),COALESCE(sum(length(CAST(content AS BLOB))),0) FROM source_files f JOIN active_generations a ON a.generation_id=f.generation_id`).Scan(&count, &totalBytes); e != nil {
		return out, e
	}
	if count > 10000 || totalBytes > LabMaxBytes {
		return out, fmt.Errorf("comparison corpus exceeds limits")
	}
	snapshots, err := db.Status(ctx, "")
	if err != nil {
		return out, err
	}
	cfg := catalog.Config{}
	var files []model.File
	var total int64
	for _, snap := range snapshots {
		cfg.Repositories = append(cfg.Repositories, model.Repository{ID: snap.RepoID})
		g, e := db.ActiveGeneration(ctx, snap.RepoID)
		if e != nil {
			return out, e
		}
		fs, e := db.ActiveFiles(ctx, snap.RepoID)
		if e != nil {
			return out, e
		}
		for _, f := range fs {
			if int64(len(f.Content)) != f.Size || hashBytes([]byte(f.Content)) != f.SHA256 {
				return out, fmt.Errorf("canonical source hash or size mismatch")
			}
			total += f.Size
			if total > LabMaxBytes || len(files) >= 10000 {
				return out, fmt.Errorf("comparison exceeds 64 MiB / 10000 file limit")
			}
			files = append(files, f)
			out.Manifest = append(out.Manifest, ManifestFile{f.RepoID, f.Path, f.SHA256, f.Size, g.ID, snap.Git.Commit})
		}
	}
	if len(files) == 0 {
		return out, fmt.Errorf("no indexed source files available")
	}
	activeIDs := map[string]bool{}
	for _, repo := range cfg.Repositories {
		activeIDs[repo.ID] = true
	}
	for _, repo := range configured {
		if !activeIDs[repo.ID] {
			cfg.Repositories = append(cfg.Repositories, model.Repository{ID: repo.ID})
		}
	}
	manifest, _ := json.Marshal(out.Manifest)
	out.SourceSnapshotSHA256 = hashBytes(manifest)
	baselineDir := filepath.Join(root, "literal")
	if err = os.Mkdir(baselineDir, 0700); err != nil {
		return out, err
	}
	for i, f := range files {
		if err = os.WriteFile(filepath.Join(baselineDir, fmt.Sprint(i)), []byte(f.Content), 0600); err != nil {
			return out, err
		}
	}
	// Independently verify the actual baseline bytes, not just copied metadata.
	baselineManifest := append([]ManifestFile(nil), out.Manifest...)
	for i := range files {
		b, e := os.ReadFile(filepath.Join(baselineDir, fmt.Sprint(i)))
		if e != nil {
			return out, e
		}
		baselineManifest[i].SHA256 = hashBytes(b)
		baselineManifest[i].Bytes = int64(len(b))
	}
	b, _ := json.Marshal(baselineManifest)
	out.BaselineSnapshotSHA256 = hashBytes(b)
	if out.SourceSnapshotSHA256 != out.BaselineSnapshotSHA256 {
		return out, fmt.Errorf("baseline snapshot mismatch")
	}
	out.SourcePreparationMS = elapsed(prepareStarted)
	service := knowledge.New(cfg, db)
	for _, c := range cases {
		if err := ctx.Err(); err != nil {
			return out, err
		}
		row := LabResult{Case: c, Evidence: []knowledge.Excerpt{}, StaleExpectation: c.DeclaredSnapshot != "" && c.DeclaredSnapshot != out.SourceSnapshotSHA256}
		query := func() (knowledge.QueryResult, []knowledge.Excerpt, error) {
			q, e := service.Query(ctx, knowledge.Query{Text: c.Query, Repository: c.Repository, Capability: c.Capability, Limit: 20})
			xs := []knowledge.Excerpt{}
			if e != nil {
				return q, xs, e
			}
			for _, ent := range q.Entities {
				x, e := service.Excerpt(ctx, ent.Evidence, 0, 0, 100)
				if e != nil {
					return q, xs, e
				}
				if len(strings.Join(x.Lines, "\n")) > 16384 {
					return q, xs, fmt.Errorf("evidence exceeds 16 KiB per item")
				}
				xs = append(xs, x)
			}
			return q, xs, nil
		}
		started := time.Now()
		row.Query, row.Evidence, err = query()
		row.FirstQueryMS = elapsed(started)
		if err != nil {
			row.Error = "knowledge query or evidence unavailable"
			row.KBState = "unknown"
		} else {
			row.KBState = row.Query.Status
		}
		started = time.Now()
		_, _, warmErr := query()
		row.WarmQueryMS = elapsed(started)
		if warmErr != nil {
			row.Error = "repeated query unavailable"
		}
		row.StateCorrect = row.Error == "" && row.KBState == c.ExpectedState
		for i, x := range row.Evidence {
			if matchesLab(c, x.Repository, x.Path, x.StartLine, x.EndLine, strings.Join(x.Lines, "\n")) && row.KB.Rank == 0 {
				row.KB.Rank = i + 1
			}
		}
		row.KBCorrect = row.KBState == "found" && row.KB.Rank == 1 && c.ExpectedState == "found"
		if row.KBState == "not_found" && c.ExpectedState == "not_found" && row.Query.Coverage != nil && row.Query.Coverage.Complete {
			row.KBCorrect = true
		}
		if row.Error != "" {
			row.KBCorrect = false
		}
		row.KBOutcome = "incorrect"
		if row.KBCorrect {
			row.KBOutcome = "correct"
		} else if row.Error != "" {
			row.KBOutcome = "error"
		} else if row.KBState == "unknown" {
			row.KBOutcome = "unknown"
		} else if row.KBState == "found" && c.ExpectedState == "found" {
			row.KBOutcome = "wrong_answer"
		} else {
			row.KBOutcome = "state_mismatch"
		}
		for _, tr := range row.Query.Trace {
			if tr.Kind == "unsupported_query" {
				row.KBOutcome = "unsupported"
			}
		}
		row.KB.Correct = row.KBCorrect
		row.KB.ProvenanceCorrect = row.KBCorrect
		row.KB.ResultCount = len(row.Evidence)
		row.KB.LatencyMS = row.FirstQueryMS
		finalizeMeasurement(&row.KB)
		contextJSON, _ := json.Marshal(struct {
			Query    knowledge.QueryResult `json:"query"`
			Evidence []knowledge.Excerpt   `json:"evidence"`
		}{row.Query, row.Evidence})
		row.ContextBytes = len(contextJSON)
		row.EstimatedTokens = (row.ContextBytes + 3) / 4
		started = time.Now()
		for i, f := range files {
			if c.Repository != "" && c.Repository != f.RepoID {
				continue
			}
			if err := ctx.Err(); err != nil {
				return out, err
			}
			content, e := os.ReadFile(filepath.Join(baselineDir, fmt.Sprint(i)))
			if e != nil {
				return out, e
			}
			row.SourceFilesRead++
			row.SourceBytesRead += int64(len(content))
			for line, text := range strings.Split(string(content), "\n") {
				if strings.Contains(text, c.Query) {
					row.Baseline.ResultCount++
					if len(row.Baseline.Evidence) < 100 && len(text) <= 16384 {
						row.Baseline.Evidence = append(row.Baseline.Evidence, map[string]any{"repository": f.RepoID, "path": f.Path, "line": line + 1, "text": text})
					}
					if row.Baseline.Rank == 0 && matchesLab(c, f.RepoID, f.Path, line+1, line+1, text) {
						row.Baseline.Rank = row.Baseline.ResultCount
					}
				}
			}
		}
		row.BaselineTruncated = row.Baseline.ResultCount > len(row.Baseline.Evidence)
		row.BaselineMS = elapsed(started)
		row.GrepCorrect = c.ExpectedState == "found" && row.Baseline.Rank == 1
		// Literal absence proves absence only in the captured corpus. Coverage gaps
		// disclosed by the KB prevent an answer-correct absence for either method.
		if c.ExpectedState == "not_found" && row.Baseline.ResultCount == 0 && row.Query.Coverage != nil && row.Query.Coverage.Complete {
			row.GrepCorrect = true
		}
		row.Baseline.Correct = row.GrepCorrect
		row.Baseline.ProvenanceCorrect = row.GrepCorrect
		row.Baseline.LatencyMS = row.BaselineMS
		row.Baseline.SourceReads = row.SourceFilesRead
		finalizeMeasurement(&row.Baseline)
		baselineJSON, _ := json.Marshal(row.Baseline.Evidence)
		row.BaselineContextBytes = len(baselineJSON)
		row.BaselineEstimatedTokens = (row.BaselineContextBytes + 3) / 4
		row.GrepWins = row.GrepCorrect && !row.KBCorrect
		row.KBWins = row.KBCorrect && !row.GrepCorrect
		out.Results = append(out.Results, row)
	}
	rs := []Result{}
	for _, r := range out.Results {
		rs = append(rs, Result{Baseline: r.Baseline, Retrievers: map[string]Measurement{"hybrid": r.KB}})
	}
	all, _ := aggregate(rs)
	out.Aggregates = map[string]Aggregate{"kb": all["hybrid"], "literal": all["grep_baseline"]}
	return out, nil
}
func matchesLab(c LabCase, repo, path string, start, end int, text string) bool {
	return repo == c.ExpectedRepository && path == c.ExpectedPath && (c.ExpectedLine == 0 || start <= c.ExpectedLine && end >= c.ExpectedLine) && (c.ExpectedSnippet == "" || strings.Contains(text, c.ExpectedSnippet))
}
