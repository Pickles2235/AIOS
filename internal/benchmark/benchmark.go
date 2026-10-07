package benchmark

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/AdamNi-7080/AIOS/internal/catalog"
	"github.com/AdamNi-7080/AIOS/internal/compiler"
	contextpkg "github.com/AdamNi-7080/AIOS/internal/context"
	"github.com/AdamNi-7080/AIOS/internal/discover"
	"github.com/AdamNi-7080/AIOS/internal/extract"
	"github.com/AdamNi-7080/AIOS/internal/knowledge"
	"github.com/AdamNi-7080/AIOS/internal/model"
	"github.com/AdamNi-7080/AIOS/internal/planner"
	"github.com/AdamNi-7080/AIOS/internal/store"
)

type Fixture struct {
	Repositories []Repository `json:"repositories"`
	Cases        []Case       `json:"cases"`
}
type Repository struct {
	ID       string   `json:"id"`
	Path     string   `json:"path"`
	Revision string   `json:"revision"`
	Include  []string `json:"include,omitempty"`
	Exclude  []string `json:"exclude,omitempty"`
}
type Case struct {
	ID                  string `json:"id"`
	Query               string `json:"query"`
	QueryClass          string `json:"query_class"`
	ExpectedRepository  string `json:"expected_repository"`
	ExpectedPath        string `json:"expected_path"`
	ExpectedLine        int    `json:"expected_line,omitempty"`
	ExpectedSnippet     string `json:"expected_snippet,omitempty"`
	ExpectedCoverageGap string `json:"expected_coverage_gap,omitempty"`
	ExpectedPredicate   string `json:"expected_predicate,omitempty"`
	ExpectedState       string `json:"expected_state,omitempty"`
}
type Result struct {
	ID                           string                 `json:"id"`
	Revision                     string                 `json:"revision"`
	QueryClass                   string                 `json:"query_class"`
	Expected                     map[string]string      `json:"expected"`
	Canonical                    Measurement            `json:"canonical"`
	Baseline                     Measurement            `json:"baseline"`
	Retrievers                   map[string]Measurement `json:"retrievers"`
	HybridVsSingle               map[string]bool        `json:"hybrid_vs_single"`
	Context                      PackageMeasurement     `json:"context_package"`
	UnboundedContext             PackageMeasurement     `json:"unbounded_context_package"`
	ExpectedState                string                 `json:"expected_state"`
	ResultState                  string                 `json:"result_state"`
	StateCorrect                 bool                   `json:"state_correct"`
	CoverageComplete             bool                   `json:"coverage_complete"`
	InvestigationState           string                 `json:"investigation_state"`
	InvestigationEvidenceCorrect bool                   `json:"investigation_evidence_correct"`
}
type PackageMeasurement struct {
	Correct           bool `json:"correct"`
	ProvenanceCorrect bool `json:"provenance_correct"`
	PackageBytes      int  `json:"package_bytes"`
	EstimatedTokens   int  `json:"estimated_tokens"`
	SourceLines       int  `json:"source_lines"`
	Truncated         bool `json:"truncated"`
}
type Measurement struct {
	Correct           bool             `json:"correct"`
	LatencyMS         float64          `json:"latency_ms"`
	ResultCount       int              `json:"result_count"`
	SourceReads       int              `json:"source_reads"`
	Trace             []string         `json:"trace"`
	Evidence          []map[string]any `json:"evidence"`
	ProvenanceCorrect bool             `json:"provenance_correct"`
	ExtractorCoverage map[string]bool  `json:"extractor_coverage,omitempty"`
	Rank              int              `json:"rank,omitempty"`
	PrecisionAt1      float64          `json:"precision_at_1"`
	RecallAtK         float64          `json:"recall_at_k"`
	ReciprocalRank    float64          `json:"reciprocal_rank"`
	NDCG              float64          `json:"ndcg"`
}
type Report struct {
	Fixture    string               `json:"fixture"`
	Results    []Result             `json:"results"`
	Indexing   IncrementalMetrics   `json:"incremental_indexing"`
	Aggregates map[string]Aggregate `json:"aggregates"`
	Quality    QualitySummary       `json:"quality"`
}

type Aggregate struct {
	Cases             int     `json:"cases"`
	PrecisionAt1      float64 `json:"precision_at_1"`
	RecallAtK         float64 `json:"recall_at_k"`
	K                 int     `json:"k"`
	MRR               float64 `json:"mrr"`
	NDCG              float64 `json:"ndcg"`
	AnswerCorrectness float64 `json:"answer_correctness"`
	Provenance        float64 `json:"provenance_correctness"`
	LatencyP50MS      float64 `json:"latency_p50_ms"`
	LatencyP95MS      float64 `json:"latency_p95_ms"`
}

type QualitySummary struct {
	ContextMedianBytes           float64 `json:"context_median_bytes"`
	ContextMedianEstimatedTokens float64 `json:"context_median_estimated_tokens"`
	LLMFreeAnswerRate            float64 `json:"llm_free_answer_rate"`
	HybridCorrectness            float64 `json:"hybrid_correctness"`
	BaselineCorrectness          float64 `json:"grep_baseline_correctness"`
	NegativeEvidenceAccuracy     float64 `json:"negative_evidence_accuracy"`
	CoverageCompleteness         float64 `json:"coverage_completeness"`
	UnknownRate                  float64 `json:"unknown_rate"`
}
type IncrementalMetrics struct {
	ColdIndexMS             float64        `json:"cold_index_ms"`
	WarmIndexMS             float64        `json:"warm_index_ms"`
	ChangedFileVisibilityMS float64        `json:"changed_file_visibility_ms"`
	ChangedFiles            map[string]int `json:"changed_files"`
	Invalidations           map[string]int `json:"invalidations"`
	IndexBytes              int64          `json:"index_bytes"`
	RetrievalCorrectBefore  bool           `json:"retrieval_correct_before"`
	RetrievalCorrectAfter   bool           `json:"retrieval_correct_after"`
}

func Run(ctx context.Context, fixturePath, dataDir string) (Report, error) {
	b, err := os.ReadFile(fixturePath)
	if err != nil {
		return Report{}, err
	}
	var fixture Fixture
	if err = json.Unmarshal(b, &fixture); err != nil {
		return Report{}, err
	}
	if len(fixture.Repositories) == 0 || len(fixture.Cases) == 0 {
		return Report{}, fmt.Errorf("benchmark fixture needs repositories and cases")
	}
	if err := validateFixture(fixture); err != nil {
		return Report{}, err
	}
	db, err := store.OpenWriter(dataDir)
	if err != nil {
		return Report{}, err
	}
	defer db.Close()
	base := filepath.Dir(fixturePath)
	coldStarted := time.Now()
	for _, repo := range fixture.Repositories {
		repoRoot, err := filepath.Abs(filepath.Join(base, repo.Path))
		if err != nil {
			return Report{}, err
		}
		if err = verifyFrozenRevision(repoRoot, repo.Revision); err != nil {
			return Report{}, fmt.Errorf("repository %s: %w", repo.ID, err)
		}
		files, coverage, err := readRepository(repo, repoRoot)
		if err != nil {
			return Report{}, err
		}
		snap := model.Snapshot{RepoID: repo.ID, Root: repoRoot, Git: model.GitState{Commit: repo.Revision, Branch: "fixture"}, ContentHash: store.ContentSnapshot(files), FileCount: len(files), IndexedAt: time.Unix(0, 0).UTC(), ExtractorVersions: "benchmark-fixture"}
		for _, f := range files {
			snap.TotalBytes += f.Size
		}
		var symbols []model.Symbol
		var edges []model.Edge
		for _, file := range files {
			s, e, parseErr := extract.Parse(file)
			if parseErr != nil {
				return Report{}, parseErr
			}
			symbols = append(symbols, s...)
			edges = append(edges, e...)
		}
		compiled, compileErr := compiler.Run(ctx, model.Repository{ID: repo.ID, Root: repoRoot}, files, benchmarkRuntime(fixturePath), dataDir)
		if compileErr != nil {
			return Report{}, compileErr
		}
		symbols = append(symbols, compiled.Symbols...)
		edges = append(edges, compiled.Edges...)
		for _, d := range compiled.Diagnostics {
			snap.CompilerDiagnostics = append(snap.CompilerDiagnostics, model.CompilerDiagnostic{Language: d.Language, Code: d.Code, Message: d.Message, Path: d.Path})
		}
		if err = db.ReplaceRepositoryWithCoverage(ctx, snap, files, symbols, edges, coverage); err != nil {
			return Report{}, err
		}
	}
	configured := make([]model.Repository, 0, len(fixture.Repositories))
	for _, repo := range fixture.Repositories {
		configured = append(configured, model.Repository{ID: repo.ID, Root: filepath.Join(base, repo.Path)})
	}
	investigation := knowledge.New(catalog.Config{Repositories: configured}, db)
	report := Report{Fixture: fixturePath}
	for _, c := range fixture.Cases {
		measurement := runCase(ctx, db, fixture, base, c)
		measurement.InvestigationState, measurement.InvestigationEvidenceCorrect = scoreInvestigation(ctx, investigation, c)
		report.Results = append(report.Results, measurement)
	}
	sort.Slice(report.Results, func(i, j int) bool { return report.Results[i].ID < report.Results[j].ID })
	report.Indexing.ColdIndexMS = float64(time.Since(coldStarted).Microseconds()) / 1000
	for _, r := range report.Results {
		report.Indexing.RetrievalCorrectBefore = report.Indexing.RetrievalCorrectBefore || r.Canonical.Correct
	}
	// A warm run is intentionally measured against the derived manifest rather
	// than mutating a fixture checkout. The engine's no-change path performs no
	// extraction/publication; this establishes the polling baseline.
	warmStarted := time.Now()
	for _, repo := range fixture.Repositories {
		if _, err := db.ActiveFiles(ctx, repo.ID); err != nil {
			return Report{}, err
		}
	}
	report.Indexing.WarmIndexMS = float64(time.Since(warmStarted).Microseconds()) / 1000
	// Measure visibility using an in-memory content revision staged entirely in
	// --data-dir. Source fixtures remain read-only.
	if len(fixture.Repositories) > 0 {
		repo := fixture.Repositories[0]
		root := filepath.Join(base, repo.Path)
		files, _, e := readRepository(repo, root)
		if e != nil {
			return Report{}, e
		}
		if len(files) > 0 {
			files[0].Content += "\n// benchmark incremental revision\n"
			sum := sha256.Sum256([]byte(files[0].Content))
			files[0].SHA256 = hex.EncodeToString(sum[:])
			files[0].Size = int64(len(files[0].Content))
			started := time.Now()
			var symbols []model.Symbol
			var edges []model.Edge
			for _, f := range files {
				s, e, parseErr := extract.Parse(f)
				if parseErr != nil {
					return Report{}, parseErr
				}
				symbols = append(symbols, s...)
				edges = append(edges, e...)
			}
			snap := model.Snapshot{RepoID: repo.ID, Root: root, Git: model.GitState{Commit: repo.Revision + "-incremental", Branch: "fixture"}, ContentHash: store.ContentSnapshot(files), FileCount: len(files), IndexedAt: time.Now().UTC(), ExtractorVersions: "benchmark-fixture"}
			for _, f := range files {
				snap.TotalBytes += f.Size
			}
			if e = db.ReplaceRepository(ctx, snap, files, symbols, edges); e != nil {
				return Report{}, e
			}
			report.Indexing.ChangedFileVisibilityMS = float64(time.Since(started).Microseconds()) / 1000
			report.Indexing.ChangedFiles = map[string]int{"modified": 1}
			report.Indexing.Invalidations = map[string]int{"file_derived": 1}
		}
	}
	if status, e := db.Diagnostics(ctx, ""); e == nil {
		report.Indexing.IndexBytes = status.IndexBytes
	}
	for _, c := range fixture.Cases {
		report.Indexing.RetrievalCorrectAfter = report.Indexing.RetrievalCorrectAfter || runCase(ctx, db, fixture, base, c).Canonical.Correct
	}
	report.Aggregates, report.Quality = aggregate(report.Results)
	return report, nil
}

func scoreInvestigation(ctx context.Context, service *knowledge.Service, c Case) (string, bool) {
	text := c.Query
	if c.ExpectedRepository != "" {
		text = "@" + c.ExpectedRepository + " " + text
	}
	switch c.QueryClass {
	case "event", "event_trace_unknown":
		text = "/event " + text
	case "route":
		text = "/route " + text
	case "path":
		text = "/path " + text
	case "config":
		text = "/config " + text
	case "log":
		text = "/log " + text
	case "symbol":
		text = "/symbol " + text
	}
	result, err := service.Investigation(ctx, text)
	if err != nil {
		return "error", false
	}
	byEvidence := make(map[string]knowledge.Excerpt, len(result.CanonicalEvidence))
	for _, excerpt := range result.CanonicalEvidence {
		byEvidence[excerpt.Evidence] = excerpt
	}
	for _, relation := range result.Relationships {
		if excerpt, ok := byEvidence[relation.Evidence]; !ok || excerpt.Generation != relation.Generation || excerpt.Path == "" || excerpt.StartLine < 1 || len(excerpt.Lines) == 0 {
			return result.Status, false
		}
	}
	if result.Status != c.ExpectedState {
		return result.Status, false
	}
	if c.ExpectedState == "not_found" {
		return result.Status, result.Coverage != nil && result.Coverage.Complete && len(result.Findings) == 0
	}
	if c.ExpectedState == "unknown" {
		if result.Coverage == nil || result.Coverage.Complete {
			return result.Status, false
		}
		return result.Status, c.ExpectedCoverageGap == "" || strings.Contains(strings.Join(append(result.Coverage.Exclusions, result.Coverage.Uncertainty...), "\n"), c.ExpectedCoverageGap)
	}
	for i, finding := range result.Findings {
		if i >= len(result.CanonicalEvidence) {
			break
		}
		evidence := result.CanonicalEvidence[i]
		if evidence.Repository != c.ExpectedRepository || evidence.Path != c.ExpectedPath || finding.Entity.Generation != evidence.Generation {
			continue
		}
		if c.ExpectedLine > 0 && (evidence.StartLine > c.ExpectedLine || evidence.EndLine < c.ExpectedLine) {
			continue
		}
		if c.ExpectedSnippet != "" && !strings.Contains(strings.Join(evidence.Lines, "\n"), c.ExpectedSnippet) {
			continue
		}
		if c.ExpectedPredicate != "" {
			matched := false
			for _, relation := range result.Relationships {
				claimSource := byEvidence[relation.Evidence]
				if relation.Predicate == c.ExpectedPredicate && claimSource.Repository == c.ExpectedRepository && claimSource.Path == c.ExpectedPath && (c.ExpectedLine == 0 || claimSource.StartLine <= c.ExpectedLine && claimSource.EndLine >= c.ExpectedLine) && (c.ExpectedSnippet == "" || strings.Contains(strings.Join(claimSource.Lines, "\n"), c.ExpectedSnippet)) {
					matched = true
					break
				}
			}
			if !matched {
				continue
			}
		}
		return result.Status, true
	}
	return result.Status, false
}

func benchmarkRuntime(fixturePath string) model.CompilerRuntime {
	r := model.CompilerRuntime{}
	if node, err := exec.LookPath("node"); err == nil {
		r.Node = node
	}
	root := filepath.Clean(filepath.Join(filepath.Dir(fixturePath), "..", "..", ".."))
	module := filepath.Join(root, "web", "node_modules", "typescript", "lib", "typescript.js")
	if absolute, err := filepath.Abs(module); err == nil {
		module = absolute
	}
	if _, err := os.Stat(module); err == nil {
		r.TypeScriptModule = module
	}
	return r
}

func runCase(ctx context.Context, db *store.Store, fixture Fixture, base string, c Case) Result {
	expected := map[string]string{"repository": c.ExpectedRepository, "path": c.ExpectedPath, "snippet": c.ExpectedSnippet, "line": fmt.Sprint(c.ExpectedLine)}
	searchText := knowledge.SearchTerm("question", c.Query)
	searchStarted := time.Now()
	hits, err := db.SearchCandidates(ctx, searchText, store.QueryFilter{})
	searchLatency := float64(time.Since(searchStarted).Microseconds()) / 1000
	structuralStarted := time.Now()
	structural, structuralErr := db.StructuralCandidates(ctx, searchText, store.QueryFilter{})
	structuralLatency := float64(time.Since(structuralStarted).Microseconds()) / 1000
	seeds := make([]string, 0, len(hits))
	for _, hit := range hits {
		seeds = append(seeds, hit.Entity.ID)
	}
	graphStarted := time.Now()
	graph, graphErr := db.GraphCandidates(ctx, seeds, store.QueryFilter{}, planner.DefaultDepth, planner.MaxItems)
	graphLatency := float64(time.Since(graphStarted).Microseconds()) / 1000
	retrievers := map[string]Measurement{
		"exact_only":      measure(c, filterCandidates(hits, func(h store.Candidate) bool { return h.MatchType != "lexical" }), "exact_only"),
		"lexical_only":    measure(c, filterCandidates(hits, func(h store.Candidate) bool { return h.MatchType == "lexical" }), "lexical_only"),
		"structural_only": measure(c, structural, "structural_only"),
		"graph_only":      measure(c, graph, "graph_only_canonical_claim_adjacency"),
	}
	for _, name := range []string{"exact_only", "lexical_only"} {
		m := retrievers[name]
		m.LatencyMS = searchLatency
		retrievers[name] = m
	}
	m := retrievers["structural_only"]
	m.LatencyMS = structuralLatency
	retrievers["structural_only"] = m
	m = retrievers["graph_only"]
	m.LatencyMS = graphLatency
	retrievers["graph_only"] = m
	if err != nil {
		m := retrievers["exact_only"]
		m.Trace = append(m.Trace, "error:"+err.Error())
		retrievers["exact_only"] = m
	}
	if structuralErr != nil {
		m := retrievers["structural_only"]
		m.Trace = append(m.Trace, "error:"+structuralErr.Error())
		retrievers["structural_only"] = m
	}
	if graphErr != nil {
		m := retrievers["graph_only"]
		m.Trace = append(m.Trace, "error:"+graphErr.Error())
		retrievers["graph_only"] = m
	}
	hybrid := hybridMeasure(c, hits, structural)
	hybrid.LatencyMS += searchLatency + structuralLatency + graphLatency
	retrievers["hybrid"] = hybrid
	canonical := hybrid
	if c.ExpectedPredicate != "" {
		var n int
		_ = db.DB().QueryRowContext(ctx, `SELECT count(*) FROM claims c JOIN evidence v ON v.evidence_id=c.evidence_id JOIN active_generations a ON a.generation_id=c.generation_id WHERE c.predicate=? AND c.generation_id=(SELECT generation_id FROM active_generations WHERE repo_id=?) AND v.path=? AND (?=0 OR v.start_line<=? AND v.end_line>=?)`, c.ExpectedPredicate, c.ExpectedRepository, c.ExpectedPath, c.ExpectedLine, c.ExpectedLine, c.ExpectedLine).Scan(&n)
		canonical.ProvenanceCorrect = canonical.Correct && n > 0
	} else {
		canonical.ProvenanceCorrect = canonical.Correct
	}
	canonical.ExtractorCoverage = map[string]bool{}
	rows, err := db.DB().QueryContext(ctx, `SELECT DISTINCT c.extractor FROM claims c JOIN active_generations a ON a.generation_id=c.generation_id WHERE a.repo_id=?`, c.ExpectedRepository)
	if err == nil {
		for rows.Next() {
			var x string
			if rows.Scan(&x) == nil {
				canonical.ExtractorCoverage[x] = true
			}
		}
		rows.Close()
	}
	started := time.Now()
	baseline := Measurement{Trace: []string{"literal_file_read_baseline"}}
	// The baseline deliberately reopens fixture files, mirroring a bounded
	// literal ripgrep/file-read search rather than reusing indexed content.
	for _, repo := range fixture.Repositories {
		files, _, readErr := readRepository(repo, filepath.Join(base, repo.Path))
		if readErr != nil {
			baseline.Trace = append(baseline.Trace, "read_error:"+readErr.Error())
			continue
		}
		for _, file := range files {
			baseline.SourceReads++
			if strings.Contains(strings.ToLower(file.Content), strings.ToLower(c.Query)) {
				baseline.ResultCount++
				baseline.Evidence = append(baseline.Evidence, map[string]any{"repository": repo.ID, "path": file.Path})
				if repo.ID == c.ExpectedRepository && file.Path == c.ExpectedPath {
					baseline.Correct = true
					if baseline.Rank == 0 {
						baseline.Rank = baseline.ResultCount
					}
				}
			}
		}
	}
	baseline.LatencyMS = float64(time.Since(started).Microseconds()) / 1000
	finalizeMeasurement(&baseline)
	for _, name := range []string{"exact_only", "lexical_only", "structural_only", "graph_only"} {
		m := retrievers[name]
		m.ExtractorCoverage = canonical.ExtractorCoverage
		retrievers[name] = m
	}
	retrievers["hybrid"] = canonical
	comparison := map[string]bool{}
	for _, name := range []string{"exact_only", "lexical_only", "structural_only", "graph_only"} {
		comparison[name] = hybrid.Correct && (!retrievers[name].Correct || hybrid.ResultCount <= retrievers[name].ResultCount)
	}
	contextCandidates := append(append([]store.Candidate{}, hits...), structural...)
	expectedState := c.ExpectedState
	if expectedState == "" {
		expectedState = "found"
	}
	capability := "lexical"
	if strings.Contains(c.QueryClass, "event") || strings.Contains(c.QueryClass, "trace") || strings.Contains(c.QueryClass, "caller") || strings.Contains(c.QueryClass, "path") {
		capability = "structural"
	}
	coverageRepo := ""
	if expectedState == "unknown" {
		coverageRepo = c.ExpectedRepository
	}
	basis, _ := db.Coverage(ctx, coverageRepo, capability, nil)
	resultState := "found"
	if len(hits)+len(structural) == 0 {
		if basis.Complete {
			resultState = "not_found"
		} else {
			resultState = "unknown"
		}
	}
	stateCorrect := resultState == expectedState
	if c.ExpectedCoverageGap != "" && !strings.Contains(strings.Join(append(basis.Exclusions, basis.Uncertainty...), "\n"), c.ExpectedCoverageGap) {
		stateCorrect = false
	}
	// A complete, evidence-backed absence is a correct canonical result even
	// though there is deliberately no positive entity to rank or cite.
	if expectedState == "not_found" && stateCorrect && basis.Complete {
		canonical.Correct = true
		canonical.ProvenanceCorrect = true
	}
	// Unknown is correct only when the searched capability has an actual
	// coverage gap. It must never be scored as a proven absence.
	if expectedState == "unknown" && stateCorrect && !basis.Complete {
		canonical.Correct = true
		canonical.ProvenanceCorrect = true
	}
	return Result{ID: c.ID, Revision: revisionFor(fixture, c.ExpectedRepository), QueryClass: c.QueryClass, Expected: expected, Canonical: canonical, Baseline: baseline, Retrievers: retrievers, HybridVsSingle: comparison, Context: measureContext(c, contextCandidates, contextpkg.Budget{}), UnboundedContext: measureContext(c, contextCandidates, contextpkg.Budget{MaxBytes: 256 * 1024, EstimatedTokens: 64 * 1024, Entities: 100, Edges: 100, Excerpts: 100, LinesPerExcerpt: 200}), ExpectedState: expectedState, ResultState: resultState, StateCorrect: stateCorrect, CoverageComplete: basis.Complete}
}

func validateFixture(f Fixture) error {
	seen := map[string]bool{}
	for _, c := range f.Cases {
		if c.ID == "" || seen[c.ID] {
			return fmt.Errorf("benchmark case id is required and unique")
		}
		seen[c.ID] = true
		if c.Query == "" {
			return fmt.Errorf("benchmark case %s query is required", c.ID)
		}
		if c.ExpectedState == "" {
			continue
		}
		if c.ExpectedState != "found" && c.ExpectedState != "not_found" && c.ExpectedState != "unknown" {
			return fmt.Errorf("benchmark case %s has invalid expected_state", c.ID)
		}
	}
	return nil
}

func measureContext(c Case, candidates []store.Candidate, budget contextpkg.Budget) PackageMeasurement {
	byKey := map[string]store.Candidate{}
	inputs := []planner.Candidate{}
	for _, hit := range candidates {
		key := hit.Entity.ID + "\x00" + hit.Evidence.ID
		byKey[key] = hit
		source := planner.SourceLexical
		if hit.MatchType != "lexical" {
			source = planner.SourceExact
		}
		if hit.MatchType == "structural" {
			source = planner.SourceStructural
		}
		inputs = append(inputs, planner.Candidate{Key: key, Repository: hit.Entity.RepoID, Path: hit.Entity.Path, SpanStart: hit.Evidence.Span.StartByte, Confidence: hit.Confidence, Sources: []planner.Source{source}})
	}
	fused := planner.Fuse(inputs)
	// The bounded package path models the compiler's small default candidate
	// window; the companion measurement retains every canonical candidate.
	if budget.MaxBytes == 0 && len(fused) > contextpkg.DefaultEntities {
		fused = fused[:contextpkg.DefaultEntities]
	}
	entities, excerpts, anchors := []contextpkg.Entity{}, []contextpkg.Excerpt{}, []string{}
	for _, candidate := range fused {
		hit := byKey[candidate.Key]
		anchors = append(anchors, hit.Entity.ID)
		entities = append(entities, contextpkg.Entity{Handle: hit.Entity.ID, Identity: hit.Entity.Identity, Repository: hit.Entity.RepoID, Path: hit.Entity.Path, Evidence: hit.Evidence.ID, Confidence: hit.Confidence, Role: "answer_anchor"})
		excerpts = append(excerpts, contextpkg.Excerpt{Handle: hit.Evidence.ID, Source: hit.Evidence.SourceID, Repository: hit.Evidence.RepoID, Path: hit.Evidence.Path, FileSHA256: hit.Evidence.SHA256, Generation: hit.Evidence.GenerationID, Text: hit.Evidence.Excerpt, Span: contextpkg.Span{StartLine: hit.Evidence.Span.StartLine, EndLine: hit.Evidence.Span.EndLine}, OriginalLines: hit.Evidence.Span.EndLine - hit.Evidence.Span.StartLine + 1})
	}
	p, err := contextpkg.Compile(contextpkg.Input{Package: contextpkg.Package{Status: "found", AnswerKind: c.QueryClass, AnswerAnchors: anchors, Derivation: map[string]string{"source": "canonical_benchmark"}}, Entities: entities, Excerpts: excerpts, Budget: budget})
	if err != nil {
		return PackageMeasurement{}
	}
	m := PackageMeasurement{PackageBytes: p.SerializedBytes, EstimatedTokens: p.EstimatedTokens, Truncated: p.Truncated}
	for _, x := range p.Entities {
		if x.Repository == c.ExpectedRepository && x.Path == c.ExpectedPath {
			m.Correct = true
		}
		if x.Evidence != "" {
			m.ProvenanceCorrect = true
		}
	}
	for _, x := range p.Excerpts {
		m.SourceLines += x.Span.EndLine - x.Span.StartLine + 1
	}
	return m
}

func filterCandidates(in []store.Candidate, keep func(store.Candidate) bool) []store.Candidate {
	out := []store.Candidate{}
	for _, x := range in {
		if keep(x) {
			out = append(out, x)
		}
	}
	return out
}
func measure(c Case, hits []store.Candidate, trace string) Measurement {
	started := time.Now()
	m := Measurement{Trace: []string{trace}}
	for index, h := range hits {
		m.ResultCount++
		m.Evidence = append(m.Evidence, map[string]any{"repository": h.Evidence.RepoID, "path": h.Evidence.Path, "start_line": h.Evidence.Span.StartLine, "end_line": h.Evidence.Span.EndLine})
		if expectedCandidate(c, h) {
			m.Correct = true
			if m.Rank == 0 {
				m.Rank = index + 1
			}
		}
	}
	m.ProvenanceCorrect = m.Correct
	m.LatencyMS = float64(time.Since(started).Microseconds()) / 1000
	finalizeMeasurement(&m)
	return m
}
func expectedCandidate(c Case, h store.Candidate) bool {
	return h.Evidence.RepoID == c.ExpectedRepository && h.Evidence.Path == c.ExpectedPath &&
		(c.ExpectedLine == 0 || (h.Evidence.Span.StartLine <= c.ExpectedLine && h.Evidence.Span.EndLine >= c.ExpectedLine)) &&
		(c.ExpectedSnippet == "" || strings.Contains(h.Evidence.Excerpt, c.ExpectedSnippet))
}
func hybridMeasure(c Case, lexical, structural []store.Candidate) Measurement {
	started := time.Now()
	byKey := map[string]store.Candidate{}
	inputs := []planner.Candidate{}
	add := func(source planner.Source, in []store.Candidate) {
		for _, h := range in {
			key := h.Entity.ID + "\x00" + h.Evidence.ID
			if old, ok := byKey[key]; !ok || benchmarkMatchRank(h.MatchType) < benchmarkMatchRank(old.MatchType) {
				byKey[key] = h
			}
			inputs = append(inputs, planner.Candidate{Key: key, Repository: h.Entity.RepoID, Path: h.Entity.Path, SpanStart: h.Evidence.Span.StartByte, Match: h.MatchType, Confidence: h.Confidence, Sources: []planner.Source{source}, Ranks: map[planner.Source]int{source: benchmarkMatchRank(h.MatchType)}})
		}
	}
	add(planner.SourceStructural, structural)
	for _, h := range lexical {
		source := planner.SourceLexical
		if h.MatchType != "lexical" {
			source = planner.SourceExact
		}
		add(source, []store.Candidate{h})
	}
	m := Measurement{Trace: []string{"hybrid_exact_lexical_structural_graph_fusion"}}
	for index, h := range planner.Fuse(inputs) {
		x := byKey[h.Key]
		m.ResultCount++
		m.Evidence = append(m.Evidence, map[string]any{"repository": x.Evidence.RepoID, "path": x.Evidence.Path, "start_line": x.Evidence.Span.StartLine, "end_line": x.Evidence.Span.EndLine})
		if expectedCandidate(c, x) {
			m.Correct = true
			if m.Rank == 0 {
				m.Rank = index + 1
			}
		}
	}
	m.ProvenanceCorrect = m.Correct
	m.LatencyMS = float64(time.Since(started).Microseconds()) / 1000
	finalizeMeasurement(&m)
	return m
}

func finalizeMeasurement(m *Measurement) {
	if m.Rank == 1 {
		m.PrecisionAt1 = 1
	}
	if m.Rank > 0 && m.Rank <= 5 {
		m.RecallAtK = 1
	}
	if m.Rank > 0 {
		m.ReciprocalRank = 1 / float64(m.Rank)
		m.NDCG = 1 / math.Log2(float64(m.Rank)+1)
	}
}

func aggregate(results []Result) (map[string]Aggregate, QualitySummary) {
	names := []string{"exact_only", "lexical_only", "structural_only", "graph_only", "hybrid", "grep_baseline"}
	out := map[string]Aggregate{}
	for _, name := range names {
		var measurements []Measurement
		for _, result := range results {
			if name == "grep_baseline" {
				measurements = append(measurements, result.Baseline)
			} else {
				measurements = append(measurements, result.Retrievers[name])
			}
		}
		latencies := make([]float64, 0, len(measurements))
		a := Aggregate{Cases: len(measurements), K: 5}
		for _, m := range measurements {
			a.PrecisionAt1 += m.PrecisionAt1
			a.RecallAtK += m.RecallAtK
			a.MRR += m.ReciprocalRank
			a.NDCG += m.NDCG
			if m.Correct {
				a.AnswerCorrectness++
			}
			if m.ProvenanceCorrect {
				a.Provenance++
			}
			latencies = append(latencies, m.LatencyMS)
		}
		if a.Cases > 0 {
			n := float64(a.Cases)
			a.PrecisionAt1 /= n
			a.RecallAtK /= n
			a.MRR /= n
			a.NDCG /= n
			a.AnswerCorrectness /= n
			a.Provenance /= n
			a.LatencyP50MS = percentile(latencies, .50)
			a.LatencyP95MS = percentile(latencies, .95)
		}
		out[name] = a
	}
	bytes, tokens := []float64{}, []float64{}
	correct, negativeCases, negativeCorrect, completeCoverage, unknown := 0, 0, 0, 0, 0
	for _, result := range results {
		bytes = append(bytes, float64(result.Context.PackageBytes))
		tokens = append(tokens, float64(result.Context.EstimatedTokens))
		if result.Canonical.Correct && result.Canonical.ProvenanceCorrect {
			correct++
		}
		if result.CoverageComplete {
			completeCoverage++
		}
		if result.ResultState == "unknown" {
			unknown++
		}
		if result.ExpectedState == "not_found" {
			negativeCases++
			if result.StateCorrect {
				negativeCorrect++
			}
		}
	}
	q := QualitySummary{ContextMedianBytes: percentile(bytes, .50), ContextMedianEstimatedTokens: percentile(tokens, .50)}
	if len(results) > 0 {
		q.LLMFreeAnswerRate = float64(correct) / float64(len(results))
		q.HybridCorrectness = out["hybrid"].AnswerCorrectness
		q.BaselineCorrectness = out["grep_baseline"].AnswerCorrectness
		q.CoverageCompleteness = float64(completeCoverage) / float64(len(results))
		q.UnknownRate = float64(unknown) / float64(len(results))
		if negativeCases > 0 {
			q.NegativeEvidenceAccuracy = float64(negativeCorrect) / float64(negativeCases)
		}
	}
	return out, q
}

func percentile(values []float64, quantile float64) float64 {
	if len(values) == 0 {
		return 0
	}
	sort.Float64s(values)
	index := int(math.Ceil(quantile*float64(len(values)))) - 1
	if index < 0 {
		index = 0
	}
	return values[index]
}
func benchmarkMatchRank(v string) int {
	if v == "lexical" {
		return 4
	}
	if v == "structural" {
		return 1
	}
	return 0
}
func revisionFor(f Fixture, id string) string {
	for _, r := range f.Repositories {
		if r.ID == id {
			return r.Revision
		}
	}
	return ""
}
func readRepository(repository Repository, root string) ([]model.File, model.CoverageReport, error) {
	return discover.FilesWithCoverage(model.Repository{ID: repository.ID, Root: root, Include: repository.Include, Exclude: repository.Exclude}, model.Limits{MaxFileBytes: 1 << 20, MaxFilesPerRepo: 20_000, MaxTotalBytesPerRepo: 256 << 20, MaxResults: 100})
}

func verifyFrozenRevision(root, revision string) error {
	if len(revision) != 40 {
		return nil
	}
	for _, character := range revision {
		if !strings.ContainsRune("0123456789abcdef", character) {
			return fmt.Errorf("frozen revision must be a lowercase 40-character Git commit")
		}
	}
	command := func(arguments ...string) (string, error) {
		cmd := exec.Command("git", append([]string{"--no-pager", "-c", "credential.helper=", "-C", root}, arguments...)...)
		cmd.Env = append([]string{"GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_NOSYSTEM=1", "GIT_TERMINAL_PROMPT=0", "LC_ALL=C"}, "PATH="+os.Getenv("PATH"))
		output, err := cmd.CombinedOutput()
		if err != nil {
			return "", fmt.Errorf("git %v: %w: %s", arguments, err, strings.TrimSpace(string(output)))
		}
		return strings.TrimSpace(string(output)), nil
	}
	head, err := command("rev-parse", "HEAD")
	if err != nil {
		return err
	}
	if head != revision {
		return fmt.Errorf("working tree HEAD %s does not match frozen revision %s", head, revision)
	}
	status, err := command("status", "--porcelain=v1", "--untracked-files=normal")
	if err != nil {
		return err
	}
	if status != "" {
		return fmt.Errorf("working tree is dirty; use a clean checkout of the frozen revision")
	}
	return nil
}
