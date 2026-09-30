package store

import (
	"context"
	"database/sql"
	"strings"
	"testing"
	"time"

	"github.com/AdamNi-7080/AIOS/internal/model"
)

func canonicalFixture(t *testing.T, db *Store, hash string) Generation {
	t.Helper()
	file := model.File{RepoID: "repo", Path: "src/service.go", SHA256: hash, Size: 31, Language: "go", Classification: "source", Content: "func Publish() { emit(Event) }"}
	snapshot := model.Snapshot{RepoID: "repo", Root: "/repo", Git: model.GitState{Commit: hash, Branch: "main"}, ContentHash: hash, FileCount: 1, TotalBytes: file.Size, IndexedAt: time.Now().UTC(), ExtractorVersions: model.ExtractorVersion}
	symbol := model.Symbol{RepoID: "repo", Path: file.Path, Name: "Publish", Kind: "function", Span: model.Span{StartByte: 0, EndByte: 14, StartLine: 1, StartColumn: 1, EndLine: 1, EndColumn: 15}, Extractor: "fixture", Confidence: 1}
	edge := model.Edge{RepoID: "repo", Path: file.Path, Source: "Publish", Target: "Event", Kind: "EMITS_EVENT", Span: model.Span{StartByte: 17, EndByte: 28, StartLine: 1, StartColumn: 18, EndLine: 1, EndColumn: 29}, Resolver: "fixture", Confidence: .9}
	g, err := db.StageGeneration(context.Background(), snapshot, []model.File{file}, []model.Symbol{symbol}, []model.Edge{edge})
	if err != nil {
		t.Fatal(err)
	}
	if err = db.ActivateGeneration(context.Background(), g.ID); err != nil {
		t.Fatal(err)
	}
	return g
}

func TestCanonicalGenerationMaterializesProvenancedFacts(t *testing.T) {
	db, err := OpenWriter(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	g := canonicalFixture(t, db, "one")
	var entities, evidence, claims int
	for _, q := range []struct {
		q string
		p *int
	}{{`SELECT count(*) FROM entities WHERE generation_id=?`, &entities}, {`SELECT count(*) FROM evidence WHERE generation_id=?`, &evidence}, {`SELECT count(*) FROM claims WHERE generation_id=?`, &claims}} {
		if err := db.DB().QueryRow(q.q, g.ID).Scan(q.p); err != nil {
			t.Fatal(err)
		}
	}
	if entities < 4 || evidence < 3 || claims != 1 {
		t.Fatalf("entities=%d evidence=%d claims=%d", entities, evidence, claims)
	}
	var derivation, revision string
	var confidence float64
	err = db.DB().QueryRow(`SELECT c.derivation,c.confidence,e.file_sha256 FROM claims c JOIN evidence e ON e.evidence_id=c.evidence_id WHERE c.generation_id=?`, g.ID).Scan(&derivation, &confidence, &revision)
	if err != nil || derivation != "fixture" || confidence != .9 || revision != "one" {
		t.Fatalf("provenance=%q %.2f %q: %v", derivation, confidence, revision, err)
	}
	var legacy string
	if err := db.DB().QueryRow(`SELECT name FROM sqlite_master WHERE name='edges'`).Scan(&legacy); err != sql.ErrNoRows {
		t.Fatalf("legacy table retained: %q %v", legacy, err)
	}
}

func TestLandmarksAreDeterministicAndEvidenceBacked(t *testing.T) {
	ctx := context.Background()
	db, err := OpenWriter(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	f := model.File{RepoID: "repo", Path: "config/application.yml", SHA256: "one", Size: 20, Language: "yaml", Classification: "configuration", Content: "feature.enabled=true"}
	sp := model.Span{StartLine: 1, StartColumn: 1, EndLine: 1, EndColumn: 20, EndByte: 20}
	g, err := db.StageGeneration(ctx, model.Snapshot{RepoID: "repo", Root: "/repo", Git: model.GitState{Commit: "one"}, ContentHash: "one", FileCount: 1, TotalBytes: f.Size, IndexedAt: time.Now(), ExtractorVersions: "fixture"}, []model.File{f}, nil, []model.Edge{{RepoID: "repo", Path: f.Path, Source: "config", Target: "feature.enabled", Kind: "DEFINES_CONFIGURATION", Span: sp, Resolver: "fixture", Confidence: .95}})
	if err != nil {
		t.Fatal(err)
	}
	if err = db.ActivateGeneration(ctx, g.ID); err != nil {
		t.Fatal(err)
	}
	var n, missing int
	if err = db.DB().QueryRow(`SELECT count(*) FROM precomputed_records p JOIN active_projection_builds a ON a.projection_build_id=p.projection_build_id WHERE a.projection_kind='landmarks'`).Scan(&n); err != nil || n < 3 {
		t.Fatalf("records=%d err=%v", n, err)
	}
	if err = db.DB().QueryRow(`SELECT count(*) FROM precomputed_records WHERE evidence_ids='[]' OR source_generations='[]'`).Scan(&missing); err != nil || missing != 0 {
		t.Fatalf("missing provenance=%d err=%v", missing, err)
	}
	var first, second string
	if err = db.DB().QueryRow(`SELECT group_concat(record_id,',') FROM (SELECT record_id FROM precomputed_records ORDER BY record_id)`).Scan(&first); err != nil {
		t.Fatal(err)
	}
	if err = db.RebuildProjections(ctx, []string{"landmarks"}); err != nil {
		t.Fatal(err)
	}
	if err = db.DB().QueryRow(`SELECT group_concat(record_id,',') FROM (SELECT record_id FROM precomputed_records WHERE projection_build_id=(SELECT projection_build_id FROM active_projection_builds WHERE projection_kind='landmarks') ORDER BY record_id)`).Scan(&second); err != nil {
		t.Fatal(err)
	}
	if first != second {
		t.Fatalf("unstable records: %q != %q", first, second)
	}
}

func TestLandmarksUseOnlyApprovedOwnershipOrExplicitUnknown(t *testing.T) {
	ctx := context.Background()
	db, err := OpenWriter(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err = db.ReplaceApprovedOwnership(ctx, []model.Repository{{ID: "repo", Ownership: []model.Ownership{{Coordinate: "repository", Owner: "platform"}}}}); err != nil {
		t.Fatal(err)
	}
	f := model.File{RepoID: "repo", Path: "README.md", SHA256: "one", Size: 4, Language: "text", Classification: "documentation", Content: "repo"}
	g, err := db.StageGeneration(ctx, model.Snapshot{RepoID: "repo", Root: "/repo", Git: model.GitState{Commit: "one"}, ContentHash: "one", FileCount: 1, TotalBytes: f.Size, IndexedAt: time.Now(), ExtractorVersions: "fixture"}, []model.File{f}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err = db.ActivateGeneration(ctx, g.ID); err != nil {
		t.Fatal(err)
	}
	var summary string
	if err = db.DB().QueryRow(`SELECT summary_json FROM precomputed_records p JOIN active_projection_builds a ON a.projection_build_id=p.projection_build_id WHERE a.projection_kind='landmarks' AND p.record_kind='ownership_fact'`).Scan(&summary); err != nil || !strings.Contains(summary, `"owner":"platform"`) {
		t.Fatalf("ownership=%s err=%v", summary, err)
	}
}

func TestIRCanonicalEntityAndFactSurviveReindexAndProjectionMustMatchCatalog(t *testing.T) {
	ctx := context.Background()
	db, err := OpenWriter(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	first := canonicalFixture(t, db, "one")
	var entity, fact string
	if err = db.DB().QueryRow(`SELECT canonical_entity_id FROM entities WHERE generation_id=? AND label='Publish'`, first.ID).Scan(&entity); err != nil {
		t.Fatal(err)
	}
	if err = db.DB().QueryRow(`SELECT fact_id FROM ir_facts`).Scan(&fact); err != nil {
		t.Fatal(err)
	}
	second := canonicalFixture(t, db, "two")
	var again string
	var observations int
	if err = db.DB().QueryRow(`SELECT canonical_entity_id FROM entities WHERE generation_id=? AND label='Publish'`, second.ID).Scan(&again); err != nil || entity != again {
		t.Fatalf("canonical entity changed: %q %q %v", entity, again, err)
	}
	if err = db.DB().QueryRow(`SELECT count(*) FROM ir_fact_observations WHERE fact_id=?`, fact).Scan(&observations); err != nil || observations != 2 {
		t.Fatalf("fact observations=%d err=%v", observations, err)
	}
	r, err := db.ActiveCatalogRevision(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.DB().Exec(`UPDATE projection_builds SET state='failed' WHERE projection_kind='lexical' AND catalog_revision_id=?`, r.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = db.SearchCandidates(ctx, "Publish", QueryFilter{}); err == nil || !strings.Contains(err.Error(), "projection unavailable") {
		t.Fatalf("stale projection accepted: %v", err)
	}
}

func TestProjectionRebuildIsDeterministicAndKeepsPriorBuildOnFailure(t *testing.T) {
	ctx := context.Background()
	db, err := OpenWriter(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	canonicalFixture(t, db, "one")
	var beforeID, beforeFingerprint string
	if err = db.DB().QueryRow(`SELECT p.projection_build_id,p.projection_fingerprint FROM active_projection_builds a JOIN projection_builds p ON p.projection_build_id=a.projection_build_id WHERE a.projection_kind='lexical'`).Scan(&beforeID, &beforeFingerprint); err != nil {
		t.Fatal(err)
	}
	if err = db.RebuildProjections(ctx, []string{"lexical"}); err != nil {
		t.Fatal(err)
	}
	var afterID, afterFingerprint, reason string
	if err = db.DB().QueryRow(`SELECT p.projection_build_id,p.projection_fingerprint,p.rebuild_reason FROM active_projection_builds a JOIN projection_builds p ON p.projection_build_id=a.projection_build_id WHERE a.projection_kind='lexical'`).Scan(&afterID, &afterFingerprint, &reason); err != nil {
		t.Fatal(err)
	}
	if beforeID == afterID || beforeFingerprint != afterFingerprint || reason != "operator_rebuild" {
		t.Fatalf("before=%s/%s after=%s/%s reason=%s", beforeID, beforeFingerprint, afterID, afterFingerprint, reason)
	}
	if err = db.RebuildProjections(ctx, []string{"not-a-projection"}); err == nil {
		t.Fatal("invalid rebuild kind succeeded")
	}
	var retained string
	if err = db.DB().QueryRow(`SELECT projection_build_id FROM active_projection_builds WHERE projection_kind='lexical'`).Scan(&retained); err != nil || retained != afterID {
		t.Fatalf("active build changed after failed rebuild: %s %v", retained, err)
	}
	if results, queryErr := db.LexicalCandidates(ctx, "Publish", QueryFilter{Repository: "repo"}); queryErr != nil || len(results) == 0 {
		t.Fatalf("last known good projection not queryable: results=%d err=%v", len(results), queryErr)
	}
	status, err := db.Diagnostics(ctx, "")
	if err != nil {
		t.Fatal(err)
	}
	var vectorDisabled bool
	for _, p := range status.Projections {
		if p.Kind == "vector" && p.State == "disabled" {
			vectorDisabled = true
		}
	}
	if !vectorDisabled {
		t.Fatalf("vector status missing: %#v", status.Projections)
	}
}

func TestProjectionValidationRejectsForeignGenerationReferences(t *testing.T) {
	ctx := context.Background()
	db, err := OpenWriter(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	first := canonicalFixture(t, db, "one")
	second := canonicalFixture(t, db, "two")
	var entity, evidence string
	if err = db.DB().QueryRow(`SELECT entity_id,evidence_id FROM entities WHERE generation_id=? LIMIT 1`, first.ID).Scan(&entity, &evidence); err != nil {
		t.Fatal(err)
	}
	r, err := db.ActiveCatalogRevision(ctx)
	if err != nil {
		t.Fatal(err)
	}
	tx, err := db.DB().BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	build := "foreign-build"
	if _, err = tx.Exec(`INSERT INTO projection_builds VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`, build, "lookup", "v1", r.ID, second.ID, "test", "v1", second.ID, "test", "now", nil, "staged", `{}`, `{}`, "test"); err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(`INSERT INTO projection_lookup_records VALUES(?,?,?,?)`, build, first.ID, entity, evidence); err != nil {
		t.Fatal(err)
	}
	if err = validateProjectionRows(ctx, tx, "lookup", build, r.ID); err == nil || !strings.Contains(err.Error(), "foreign") {
		t.Fatalf("foreign reference accepted: %v", err)
	}
}

func TestDiagnosticsDeriveProjectionHealthAndPersistRebuildFailure(t *testing.T) {
	ctx := context.Background()
	db, err := OpenWriter(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	canonicalFixture(t, db, "one")
	if err = db.RebuildProjections(ctx, []string{"not-a-projection"}); err == nil {
		t.Fatal("invalid projection rebuild succeeded")
	}
	status, err := db.Diagnostics(ctx, "")
	if err != nil || status.Health.Catalog == "" || !status.Health.Healthy {
		t.Fatalf("status=%#v err=%v", status, err)
	}
	var found bool
	for _, event := range status.Diagnostics {
		if event.Code == DiagnosticProjectionFailure {
			found = true
		}
	}
	if found {
		t.Fatal("validation failure before rebuild should not emit a build failure event")
	}
	if _, err = db.DB().Exec(`UPDATE projection_builds SET state='failed' WHERE projection_kind='lookup' AND projection_build_id=(SELECT projection_build_id FROM active_projection_builds WHERE projection_kind='lookup')`); err != nil {
		t.Fatal(err)
	}
	status, err = db.Diagnostics(ctx, "")
	if err != nil || status.Health.Healthy {
		t.Fatalf("status=%#v err=%v", status, err)
	}
	found = false
	for _, event := range status.Diagnostics {
		if event.Code == DiagnosticProjectionMismatch && event.Scope.Projection == "lookup" {
			found = true
		}
	}
	if !found {
		t.Fatalf("missing projection mismatch: %#v", status.Diagnostics)
	}
}

func TestCanonicalGenerationDeduplicatesRepeatedSymbolIdentity(t *testing.T) {
	db, err := OpenWriter(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	file := model.File{RepoID: "repo", Path: "src/A.ts", SHA256: "one", Size: 10, Language: "typescript", Classification: "source", Content: "const a=1;"}
	snap := model.Snapshot{RepoID: "repo", Root: "/repo", Git: model.GitState{Commit: "one"}, ContentHash: "one", FileCount: 1, TotalBytes: file.Size, IndexedAt: time.Now(), ExtractorVersions: model.ExtractorVersion}
	span := model.Span{StartByte: 0, EndByte: 9, StartLine: 1, StartColumn: 1, EndLine: 1, EndColumn: 10}
	symbol := model.Symbol{RepoID: "repo", Path: file.Path, Name: "a", Kind: "variable", Identity: "typescript:a", Span: span, Extractor: "typescript-compiler-v1", Confidence: 1}
	g, err := db.StageGeneration(context.Background(), snap, []model.File{file}, []model.Symbol{symbol, symbol}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err = db.ActivateGeneration(context.Background(), g.ID); err != nil {
		t.Fatal(err)
	}
	var count int
	if err = db.DB().QueryRow(`SELECT count(*) FROM entities WHERE generation_id=? AND identity='typescript:a'`, g.ID).Scan(&count); err != nil || count != 1 {
		t.Fatalf("count=%d err=%v", count, err)
	}
}

func TestCanonicalGenerationDeduplicatesRepeatedClaim(t *testing.T) {
	db, err := OpenWriter(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	file := model.File{RepoID: "repo", Path: "src/A.java", SHA256: "one", Size: 26, Language: "java", Classification: "source", Content: `send("customer.changed");`}
	snap := model.Snapshot{RepoID: "repo", Root: "/repo", Git: model.GitState{Commit: "one"}, ContentHash: "one", FileCount: 1, TotalBytes: file.Size, IndexedAt: time.Now(), ExtractorVersions: model.ExtractorVersion}
	span := model.Span{StartByte: 0, EndByte: 25, StartLine: 1, StartColumn: 1, EndLine: 1, EndColumn: 26}
	edge := model.Edge{RepoID: "repo", Path: file.Path, Source: "send", Target: "customer.changed", Kind: "PRODUCES_TOPIC", Span: span, Resolver: "java-domain-v1", Derivation: "syntax_derived", Confidence: .95}
	g, err := db.StageGeneration(context.Background(), snap, []model.File{file}, nil, []model.Edge{edge, edge})
	if err != nil {
		t.Fatal(err)
	}
	if err = db.ActivateGeneration(context.Background(), g.ID); err != nil {
		t.Fatal(err)
	}
	var count int
	if err = db.DB().QueryRow(`SELECT count(*) FROM claims WHERE generation_id=? AND predicate='PRODUCES_TOPIC'`, g.ID).Scan(&count); err != nil || count != 1 {
		t.Fatalf("count=%d err=%v", count, err)
	}
}

func TestSearchProjectionIndexesWholeContentOnlyForFileEntity(t *testing.T) {
	db, err := OpenWriter(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	content := "const marker = 1;" + strings.Repeat(" padding", 200)
	file := model.File{RepoID: "repo", Path: "src/A.ts", SHA256: "one", Size: int64(len(content)), Language: "typescript", Classification: "source", Content: content}
	snap := model.Snapshot{RepoID: "repo", Root: "/repo", Git: model.GitState{Commit: "one"}, ContentHash: "one", FileCount: 1, TotalBytes: file.Size, IndexedAt: time.Now(), ExtractorVersions: model.ExtractorVersion}
	span := model.Span{StartByte: 0, EndByte: 16, StartLine: 1, StartColumn: 1, EndLine: 1, EndColumn: 17}
	symbol := model.Symbol{RepoID: "repo", Path: file.Path, Name: "marker", Kind: "variable", Identity: "typescript:marker", Span: span, Extractor: "fixture", Confidence: 1}
	g, err := db.StageGeneration(context.Background(), snap, []model.File{file}, []model.Symbol{symbol}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err = db.ActivateGeneration(context.Background(), g.ID); err != nil {
		t.Fatal(err)
	}
	rows, err := db.DB().Query(`SELECT e.kind,length(f.source) FROM search_fts f JOIN entities e ON e.entity_id=f.entity_id WHERE f.generation_id=? ORDER BY e.kind`, g.ID)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	lengths := map[string]int{}
	for rows.Next() {
		var kind string
		var length int
		if err = rows.Scan(&kind, &length); err != nil {
			t.Fatal(err)
		}
		lengths[kind] = length
	}
	if lengths["file"] != len(content) || lengths["symbol:variable"] >= len(content) {
		t.Fatalf("projected lengths=%v content=%d", lengths, len(content))
	}
}

func TestCanonicalDomainFactsMaterializeTypedGuardAndDiagnosticEntities(t *testing.T) {
	db, err := OpenWriter(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	file := model.File{RepoID: "repo", Path: "src/A.java", SHA256: "h", Size: 64, Language: "java", Classification: "source", Content: "void publish() { if (enabled) kafkaTemplate.send(\"topic\", body); }"}
	snap := model.Snapshot{RepoID: "repo", Root: "/repo", Git: model.GitState{Commit: "one"}, ContentHash: "h", FileCount: 1, TotalBytes: file.Size, IndexedAt: time.Now(), ExtractorVersions: model.ExtractorVersion}
	sp := model.Span{StartByte: 17, EndByte: 58, StartLine: 1, StartColumn: 18, EndLine: 1, EndColumn: 59}
	edges := []model.Edge{
		{RepoID: "repo", Path: file.Path, Source: "publish", Target: "topic", Kind: "PRODUCES_TOPIC", Span: sp, Resolver: "fixture", Derivation: "syntax_derived", Confidence: .95},
		{RepoID: "repo", Path: file.Path, Source: "publish", Target: "enabled", Kind: "HAS_LOCAL_GUARD", Span: sp, Resolver: "fixture", Derivation: "local_syntax", Confidence: 1},
		{RepoID: "repo", Path: file.Path, Source: "publish", Target: "computed_topic", Kind: "COVERAGE_GAP", Span: sp, Resolver: "fixture", Derivation: "coverage_gap", Confidence: 0},
	}
	g, err := db.StageGeneration(context.Background(), snap, []model.File{file}, nil, edges)
	if err != nil {
		t.Fatal(err)
	}
	if err = db.ActivateGeneration(context.Background(), g.ID); err != nil {
		t.Fatal(err)
	}
	for _, want := range []struct {
		predicate, kind, derivation string
		confidence                  float64
	}{
		{"PRODUCES_TOPIC", "message_channel", "syntax_derived", .95},
		{"HAS_LOCAL_GUARD", "local_syntax_guard", "local_syntax", 1},
		{"COVERAGE_GAP", "coverage_diagnostic", "coverage_gap", 0},
	} {
		var kind, derivation string
		var confidence float64
		var line int
		err = db.DB().QueryRow(`SELECT o.kind,c.derivation,c.confidence,e.start_line FROM claims c JOIN entities o ON o.entity_id=c.object_id JOIN evidence e ON e.evidence_id=c.evidence_id WHERE c.generation_id=? AND c.predicate=?`, g.ID, want.predicate).Scan(&kind, &derivation, &confidence, &line)
		if err != nil || kind != want.kind || derivation != want.derivation || confidence != want.confidence || line != 1 {
			t.Fatalf("%s: kind=%q derivation=%q confidence=%v line=%d err=%v", want.predicate, kind, derivation, confidence, line, err)
		}
	}
}

func TestCanonicalGenerationsKeepHistoryButOnlyNewestIsActive(t *testing.T) {
	db, err := OpenWriter(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	first := canonicalFixture(t, db, "one")
	second := canonicalFixture(t, db, "two")
	if first.ID == second.ID {
		t.Fatal("generation ids must be distinct")
	}
	var active, historical int
	if err := db.DB().QueryRow(`SELECT count(*) FROM claims c JOIN active_generations a ON a.generation_id=c.generation_id WHERE c.generation_id=?`, second.ID).Scan(&active); err != nil || active != 1 {
		t.Fatalf("active=%d %v", active, err)
	}
	if err := db.DB().QueryRow(`SELECT count(*) FROM claims c JOIN active_generations a ON a.generation_id=c.generation_id WHERE c.generation_id=?`, first.ID).Scan(&historical); err != nil || historical != 0 {
		t.Fatalf("historical=%d %v", historical, err)
	}
	if _, err := db.ActiveGeneration(context.Background(), "repo"); err != nil {
		t.Fatal(err)
	}
}

func TestCatalogActivationIsAtomicAndRecordsEveryMember(t *testing.T) {
	ctx := context.Background()
	db, err := OpenWriter(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	stage := func(repo string) Generation {
		f := model.File{RepoID: repo, Path: "src/A.java", SHA256: repo, Size: 10, Language: "java", Classification: "source", Content: "class A {}"}
		g, err := db.StageGeneration(ctx, model.Snapshot{RepoID: repo, Root: "/" + repo, Git: model.GitState{Commit: repo}, ContentHash: repo, FileCount: 1, TotalBytes: f.Size, IndexedAt: time.Now(), ExtractorVersions: "fixture"}, []model.File{f}, nil, nil)
		if err != nil {
			t.Fatal(err)
		}
		return g
	}
	a, b := stage("producer"), stage("consumer")
	if err := db.ActivateCatalog(ctx, []string{a.ID, b.ID}); err != nil {
		t.Fatal(err)
	}
	r, err := db.ActiveCatalogRevision(ctx)
	if err != nil || r.ID == "" {
		t.Fatalf("revision=%#v err=%v", r, err)
	}
	var members int
	if err := db.DB().QueryRow(`SELECT count(*) FROM catalog_revision_members WHERE catalog_revision_id=?`, r.ID).Scan(&members); err != nil || members != 2 {
		t.Fatalf("members=%d err=%v", members, err)
	}
	for _, repo := range []string{"producer", "consumer"} {
		if _, err := db.ActiveGeneration(ctx, repo); err != nil {
			t.Fatalf("%s inactive: %v", repo, err)
		}
	}
}

func TestCatalogLinksExactCrossRepositoryEventEvidence(t *testing.T) {
	ctx := context.Background()
	db, err := OpenWriter(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	stage := func(repo, predicate string) Generation {
		f := model.File{RepoID: repo, Path: "src/A.java", SHA256: repo, Size: 30, Language: "java", Classification: "source", Content: "void event() { customerChanged(); }"}
		sp := model.Span{StartByte: 0, EndByte: 10, StartLine: 1, StartColumn: 1, EndLine: 1, EndColumn: 11}
		g, err := db.StageGeneration(ctx, model.Snapshot{RepoID: repo, Root: "/" + repo, Git: model.GitState{Commit: repo}, ContentHash: repo, FileCount: 1, TotalBytes: f.Size, IndexedAt: time.Now(), ExtractorVersions: "fixture"}, []model.File{f}, nil, []model.Edge{{RepoID: repo, Path: f.Path, Source: "event", Target: "CustomerChanged", Kind: predicate, Span: sp, Resolver: "fixture", Confidence: .95}})
		if err != nil {
			t.Fatal(err)
		}
		return g
	}
	p, c := stage("producer", "PUBLISHES_EVENT"), stage("consumer", "CONSUMES_EVENT")
	if err := db.ActivateCatalog(ctx, []string{p.ID, c.ID}); err != nil {
		t.Fatal(err)
	}
	r, err := db.ActiveCatalogRevision(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var subject string
	if err := db.DB().QueryRow(`SELECT c.object_id FROM claims c JOIN generations g ON g.generation_id=c.generation_id WHERE g.repo_id='producer'`).Scan(&subject); err != nil {
		t.Fatal(err)
	}
	links, truncated, err := db.CrossClaims(ctx, r.ID, subject, "out", []string{"EVENT_PRODUCER_CONSUMER"}, 10)
	if err != nil || truncated || len(links) != 1 || links[0].Resolver != "direct-canonical-identity" || links[0].SubjectEvidenceID == "" || links[0].ObjectEvidenceID == "" {
		t.Fatalf("links=%#v truncated=%v err=%v", links, truncated, err)
	}
	var memberships int
	if err := db.DB().QueryRow(`SELECT count(*) FROM identity_memberships WHERE identity_id IN (SELECT identity_id FROM canonical_identities WHERE catalog_revision_id=?)`, r.ID).Scan(&memberships); err != nil || memberships != 2 {
		t.Fatalf("memberships=%d err=%v", memberships, err)
	}
}

func TestCanonicalSemanticIdentityIsDeterministicWithinGeneration(t *testing.T) {
	db, err := OpenWriter(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	file := model.File{RepoID: "repo", Path: "src/A.java", SHA256: "one", Size: 10, Language: "java", Classification: "source", Content: "class A {}"}
	snap := model.Snapshot{RepoID: "repo", Root: "/repo", Git: model.GitState{Commit: "one"}, ContentHash: "one", FileCount: 1, TotalBytes: file.Size, IndexedAt: time.Now(), ExtractorVersions: "fixture"}
	g, err := db.StageGeneration(context.Background(), snap, []model.File{file}, []model.Symbol{{RepoID: "repo", Path: file.Path, Name: "A", Kind: "class", Identity: "java:type:example.A", Span: model.Span{StartByte: 0, EndByte: 7, StartLine: 1, StartColumn: 1, EndLine: 1, EndColumn: 8}, Extractor: "javac-v1", Confidence: 1}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err = db.ActivateGeneration(context.Background(), g.ID); err != nil {
		t.Fatal(err)
	}
	var got string
	if err = db.DB().QueryRow(`SELECT identity FROM entities WHERE generation_id=? AND label='A'`, g.ID).Scan(&got); err != nil || got != "java:type:example.A" {
		t.Fatalf("identity=%q err=%v", got, err)
	}
}

func TestExistingLegacyDatabaseRequiresCleanReindex(t *testing.T) {
	dir := t.TempDir()
	db, err := sql.Open("sqlite", dir+"/index.db")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(`CREATE TABLE files(x TEXT)`); err != nil {
		t.Fatal(err)
	}
	db.Close()
	_, err = OpenWriter(dir)
	if err == nil || !strings.Contains(err.Error(), "--reset-derived-data") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestSearchCandidatesUsesFieldedFTSAndActiveVersion(t *testing.T) {
	ctx := context.Background()
	db, err := OpenWriter(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	file := model.File{RepoID: "repo", Path: "config/application.yml", SHA256: "h", Size: 31, Language: "yaml", Classification: "configuration", Content: "PING_ONE_MFA_ENV_ID: example"}
	snap := model.Snapshot{RepoID: "repo", Root: "/repo", Git: model.GitState{Commit: "commit"}, ContentHash: "h", FileCount: 1, TotalBytes: file.Size, IndexedAt: time.Now(), ExtractorVersions: model.ExtractorVersion}
	g, err := db.StageGeneration(ctx, snap, []model.File{file}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err = db.ActivateGeneration(ctx, g.ID); err != nil {
		t.Fatal(err)
	}
	hits, err := db.SearchCandidates(ctx, "PING_ONE_MFA_ENV_ID", QueryFilter{Repository: "repo", Fields: []string{"configuration"}, Version: "commit"})
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) == 0 || hits[0].Evidence.Path != file.Path {
		t.Fatalf("hits=%#v", hits)
	}
	if _, err = db.SearchCandidates(ctx, "example", QueryFilter{Version: "old"}); err == nil {
		t.Fatal("accepted inactive version")
	}
	if _, err = db.SearchCandidates(ctx, "example", QueryFilter{Fields: []string{"unsafe"}}); err == nil {
		t.Fatal("accepted unsafe field")
	}
}

func TestSearchCandidatesRanksExactIdentifierBeforeLexical(t *testing.T) {
	ctx := context.Background()
	db, err := OpenWriter(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	files := []model.File{
		{RepoID: "repo", Path: "a.md", SHA256: "a", Size: 20, Language: "documentation", Classification: "documentation", Content: "Worker is documented"},
		{RepoID: "repo", Path: "z.java", SHA256: "z", Size: 15, Language: "java", Classification: "source", Content: "class Worker {}"},
	}
	symbol := model.Symbol{RepoID: "repo", Path: "z.java", Name: "Worker", Kind: "class", Span: model.Span{StartByte: 6, EndByte: 12, StartLine: 1, StartColumn: 7, EndLine: 1, EndColumn: 13}, Extractor: "fixture", Confidence: 1}
	edge := model.Edge{RepoID: "repo", Path: "a.md", Source: "documentation", Target: "Worker", Kind: "REFERENCES", Span: model.Span{StartByte: 0, EndByte: 6, StartLine: 1, StartColumn: 1, EndLine: 1, EndColumn: 7}, Resolver: "fixture", Confidence: 1}
	g, err := db.StageGeneration(ctx, model.Snapshot{RepoID: "repo", Root: "/repo", Git: model.GitState{Commit: "one"}, ContentHash: "one", FileCount: 2, TotalBytes: 35, IndexedAt: time.Now(), ExtractorVersions: "fixture"}, files, []model.Symbol{symbol}, []model.Edge{edge})
	if err != nil {
		t.Fatal(err)
	}
	if err = db.ActivateGeneration(ctx, g.ID); err != nil {
		t.Fatal(err)
	}
	hits, err := db.SearchCandidates(ctx, "Worker", QueryFilter{})
	if err != nil || len(hits) == 0 || hits[0].MatchType != "exact_identifier" || hits[0].Entity.Path != "z.java" {
		t.Fatalf("hits=%#v err=%v", hits, err)
	}
}

func TestGraphCandidatesUsesBoundedCanonicalClaims(t *testing.T) {
	ctx := context.Background()
	db, err := OpenWriter(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	f := model.File{RepoID: "repo", Path: "A.java", SHA256: "h", Size: 30, Language: "java", Classification: "source", Content: "void publish(){ consume(Event); }"}
	sp := model.Span{StartByte: 0, EndByte: 10, StartLine: 1, StartColumn: 1, EndLine: 1, EndColumn: 11}
	g, err := db.StageGeneration(ctx, model.Snapshot{RepoID: "repo", Root: "/repo", Git: model.GitState{Commit: "one"}, ContentHash: "h", FileCount: 1, TotalBytes: f.Size, IndexedAt: time.Now(), ExtractorVersions: "fixture"}, []model.File{f}, []model.Symbol{{RepoID: "repo", Path: f.Path, Name: "publish", Kind: "method", Span: sp, Extractor: "fixture", Confidence: 1}}, []model.Edge{{RepoID: "repo", Path: f.Path, Source: "publish", Target: "Event", Kind: "PUBLISHES_EVENT", Span: sp, Resolver: "fixture", Confidence: .9}})
	if err != nil {
		t.Fatal(err)
	}
	if err = db.ActivateGeneration(ctx, g.ID); err != nil {
		t.Fatal(err)
	}
	var seed string
	if err = db.DB().QueryRow(`SELECT entity_id FROM entities WHERE generation_id=? AND label='publish'`, g.ID).Scan(&seed); err != nil {
		t.Fatal(err)
	}
	hits, err := db.GraphCandidates(ctx, []string{seed}, QueryFilter{Repository: "repo"}, 1, 10)
	if err != nil || len(hits) == 0 || hits[0].MatchType != "graph" {
		t.Fatalf("hits=%#v err=%v", hits, err)
	}
}

func TestCatalogActivationCanReuseAnUnchangedActiveGeneration(t *testing.T) {
	ctx := context.Background()
	db, err := OpenWriter(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	first := canonicalFixture(t, db, "one")
	file := model.File{RepoID: "other", Path: "src/B.java", SHA256: "two", Size: 10, Language: "java", Classification: "source", Content: "class B {}"}
	second, err := db.StageGeneration(ctx, model.Snapshot{RepoID: "other", Root: "/other", Git: model.GitState{Commit: "two"}, ContentHash: "two", FileCount: 1, TotalBytes: file.Size, IndexedAt: time.Now(), ExtractorVersions: "fixture"}, []model.File{file}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err = db.ActivateCatalog(ctx, []string{first.ID, second.ID}); err != nil {
		t.Fatal(err)
	}
	revision, err := db.ActiveCatalogRevision(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var members int
	if err = db.DB().QueryRow(`SELECT count(*) FROM catalog_revision_members WHERE catalog_revision_id=?`, revision.ID).Scan(&members); err != nil || members != 2 {
		t.Fatalf("members=%d err=%v", members, err)
	}
}
