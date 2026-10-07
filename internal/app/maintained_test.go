package app

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/AdamNi-7080/AIOS/internal/adapter"
	"github.com/AdamNi-7080/AIOS/internal/catalog"
	"github.com/AdamNi-7080/AIOS/internal/knowledge"
	"github.com/AdamNi-7080/AIOS/internal/model"
	"github.com/AdamNi-7080/AIOS/internal/store"
)

func TestMaintainedHealthyBootstrapCancellationAndNewRevisionRepair(t *testing.T) {
	source, data := canonicalTempDir(t), canonicalTempDir(t)
	t.Cleanup(func() {
		_ = filepath.Walk(data, func(path string, info os.FileInfo, e error) error {
			if e == nil && info.IsDir() {
				return os.Chmod(path, 0700)
			}
			return e
		})
	})
	git := func(args ...string) {
		t.Helper()
		b, e := exec.Command("git", append([]string{"-C", source}, args...)...).CombinedOutput()
		if e != nil {
			t.Fatalf("git fixture: %v %s", e, b)
		}
	}
	git("init", "-q", "--initial-branch=main")
	file := filepath.Join(source, "State.java")
	if e := os.WriteFile(file, []byte("class OriginalWorker {}"), 0640); e != nil {
		t.Fatal(e)
	}
	git("add", ".")
	git("-c", "user.name=t", "-c", "user.email=t@e", "commit", "-qm", "one")
	entry := adapter.LocalRepository{ID: "healthy", Path: source}
	cfg := catalog.Config{Version: 1, Limits: catalog.Defaults(), Sources: []catalog.Source{{Kind: model.SourceKindRepository, ID: "healthy"}, {Kind: model.SourceKindRepository, ID: "unavailable"}}}
	fingerprint := "approved-two-repositories"
	capture := func() adapter.Discovery {
		t.Helper()
		d, e := adapter.CaptureLocal(context.Background(), entry, data, cfg.Limits)
		if e != nil {
			t.Fatal(e)
		}
		return d
	}
	if _, e := IngestMaintained(context.Background(), cfg, capture(), fingerprint, data); e != nil {
		t.Fatal(e)
	}
	db, e := store.OpenReadOnly(data)
	if e != nil {
		t.Fatal(e)
	}
	defer db.Close()
	reader := knowledge.New(cfg, db)
	query := func(text string) knowledge.QueryResult {
		t.Helper()
		r, e := reader.Query(context.Background(), knowledge.Query{Text: text, Repository: "healthy"})
		if e != nil {
			t.Fatal(e)
		}
		return r
	}
	if query("OriginalWorker").Status != "found" {
		t.Fatal("healthy bootstrap blocked by unavailable member")
	}
	if _, e := db.ActiveGeneration(context.Background(), "unavailable"); e == nil {
		t.Fatal("invented unavailable generation")
	}
	before, e := db.ActiveCatalogRevision(context.Background())
	if e != nil {
		t.Fatal(e)
	}
	if e := os.WriteFile(file, []byte("class InterruptedWorker {}"), 0640); e != nil {
		t.Fatal(e)
	}
	failedDiscovery := capture()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ctx = WithProgress(ctx, func(p Progress) {
		if p.Stage == "staged" {
			if query("OriginalWorker").Status != "found" || query("InterruptedWorker").Status == "found" {
				t.Fatal("concurrent reader observed staged input")
			}
			cancel()
		}
	})
	if _, e = IngestMaintained(ctx, cfg, failedDiscovery, fingerprint, data); e == nil {
		t.Fatal("canceled staged input activated")
	}
	after, e := db.ActiveCatalogRevision(context.Background())
	if e != nil || after.ID != before.ID {
		t.Fatal("cancel replaced last-good catalog")
	}
	if query("OriginalWorker").Status != "found" || query("InterruptedWorker").Status == "found" {
		t.Fatal("staged input leaked into queries")
	}
	queues, e := db.IngestionStatus(context.Background(), "healthy")
	if e != nil || queues[0].State != "failed" || queues[0].PendingRevision != failedDiscovery.Revision {
		t.Fatalf("failure not durable: %+v %v", queues, e)
	}
	if e := os.WriteFile(file, []byte("class RepairedWorker {}"), 0640); e != nil {
		t.Fatal(e)
	}
	newDiscovery := capture()
	if newDiscovery.Revision == failedDiscovery.Revision {
		t.Fatal("working revision did not reflect bytes")
	}
	indexBefore, e := os.ReadFile(filepath.Join(source, ".git/index"))
	if e != nil {
		t.Fatal(e)
	}
	if _, e = IngestMaintained(context.Background(), cfg, newDiscovery, fingerprint, data); e != nil {
		t.Fatal(e)
	}
	if query("RepairedWorker").Status != "found" || query("InterruptedWorker").Status == "found" {
		t.Fatal("new revision repair lost canonical isolation")
	}
	events, e := db.IngestionEvents(context.Background(), "healthy")
	if e != nil {
		t.Fatal(e)
	}
	superseded := false
	for _, event := range events {
		if event.Type == "revision_superseded" && event.SourceRevision == failedDiscovery.Revision && event.TargetRevision == newDiscovery.Revision {
			superseded = true
		}
	}
	if !superseded {
		t.Fatal("interrupted revision supersession missing durable audit")
	}
	indexAfter, e := os.ReadFile(filepath.Join(source, ".git/index"))
	if e != nil || string(indexBefore) != string(indexAfter) {
		t.Fatal("maintenance changed source index")
	}
	body, e := os.ReadFile(file)
	if e != nil || string(body) != "class RepairedWorker {}" {
		t.Fatal("maintenance changed working bytes")
	}
}

func TestMaintainedRevisionRevisitsKeepPriorDeltaEpochsImmutable(t *testing.T) {
	source, data := canonicalTempDir(t), canonicalTempDir(t)
	t.Cleanup(func() {
		_ = filepath.Walk(data, func(path string, info os.FileInfo, e error) error {
			if e == nil && info.IsDir() {
				return os.Chmod(path, 0700)
			}
			return e
		})
	})
	git := func(args ...string) {
		t.Helper()
		if b, e := exec.Command("git", append([]string{"-C", source}, args...)...).CombinedOutput(); e != nil {
			t.Fatalf("fixture %v %s", e, b)
		}
	}
	git("init", "-q", "--initial-branch=main")
	path := filepath.Join(source, "facts.txt")
	if e := os.WriteFile(path, []byte("A"), 0640); e != nil {
		t.Fatal(e)
	}
	git("add", ".")
	git("-c", "user.name=t", "-c", "user.email=t@e", "commit", "-qm", "one")
	cfg := catalog.Config{Version: 1, Limits: catalog.Defaults(), Sources: []catalog.Source{{Kind: model.SourceKindRepository, ID: "repo"}}}
	writer, e := store.OpenWriter(data)
	if e != nil {
		t.Fatal(e)
	}
	writer.Close()
	db, e := store.OpenReadOnly(data)
	if e != nil {
		t.Fatal(e)
	}
	defer db.Close()
	prior := map[string]string{}
	for _, body := range []string{"A", "B", "A", "B"} {
		if e = os.WriteFile(path, []byte(body), 0640); e != nil {
			t.Fatal(e)
		}
		d, e := adapter.CaptureLocal(context.Background(), adapter.LocalRepository{ID: "repo", Path: source}, data, cfg.Limits)
		if e != nil {
			t.Fatal(e)
		}
		if _, e = IngestMaintained(context.Background(), cfg, d, "manifest", data); e != nil {
			t.Fatal(e)
		}
		rows, e := db.DB().Query(`SELECT delta_id,source_generation_id,target_generation_id,activated_at FROM ir_delta_records`)
		if e != nil {
			t.Fatal(e)
		}
		current := map[string]string{}
		for rows.Next() {
			var id, a, b, c string
			if e = rows.Scan(&id, &a, &b, &c); e != nil {
				t.Fatal(e)
			}
			encoded, _ := json.Marshal([]string{a, b, c})
			current[id] = string(encoded)
		}
		if e = rows.Err(); e != nil {
			t.Fatal(e)
		}
		rows.Close()
		for id, value := range prior {
			if current[id] != value {
				t.Fatal("revisit overwrote earlier immutable delta epoch")
			}
		}
		if len(current) != len(prior)+1 {
			t.Fatal("revisit did not record a distinct selected source epoch")
		}
		prior = current
	}
}

func TestMaintainedMetadataAndCoverageRefreshReuseUnchangedAnalysis(t *testing.T) {
	source, data := canonicalTempDir(t), canonicalTempDir(t)
	t.Cleanup(func() {
		_ = filepath.Walk(data, func(path string, info os.FileInfo, e error) error {
			if e == nil && info.IsDir() {
				return os.Chmod(path, 0700)
			}
			return e
		})
	})
	git := func(args ...string) {
		t.Helper()
		if b, e := exec.Command("git", append([]string{"-C", source}, args...)...).CombinedOutput(); e != nil {
			t.Fatalf("fixture %v %s", e, b)
		}
	}
	git("init", "-q", "--initial-branch=main")
	for _, path := range []string{"facts.txt", "excluded.txt"} {
		if e := os.WriteFile(filepath.Join(source, path), []byte("stable fixture knowledge"), 0640); e != nil {
			t.Fatal(e)
		}
	}
	git("add", ".")
	git("-c", "user.name=t", "-c", "user.email=t@e", "commit", "-qm", "one")
	cfg := catalog.Config{Version: 1, Limits: catalog.Defaults(), Sources: []catalog.Source{{Kind: model.SourceKindRepository, ID: "repo", Exclude: []string{"excluded.txt"}}}}
	entry := adapter.LocalRepository{ID: "repo", Path: source}
	run := func(manifest string) IndexResult {
		t.Helper()
		d, e := adapter.CaptureLocalScoped(context.Background(), entry, data, cfg.Limits, cfg.SourceRepositories()[0])
		if e != nil {
			t.Fatal(e)
		}
		out, e := IngestMaintained(context.Background(), cfg, d, manifest, data)
		if e != nil {
			t.Fatal(e)
		}
		return out
	}
	run("first")
	db, e := store.OpenReadOnly(data)
	if e != nil {
		t.Fatal(e)
	}
	defer db.Close()
	first, e := db.ActiveGeneration(context.Background(), "repo")
	if e != nil {
		t.Fatal(e)
	}
	git("-c", "user.name=t", "-c", "user.email=t@e", "commit", "--allow-empty", "-qm", "metadata")
	metadata := run("first")
	second, e := db.ActiveGeneration(context.Background(), "repo")
	if e != nil || second.ID == first.ID || len(metadata.Changes) != 0 {
		t.Fatal("metadata refresh did not bind fresh epoch with zero file deltas")
	}
	if metadata.Snapshots[0].Git.Commit == "" {
		t.Fatal("metadata provenance absent")
	}
	cfg.Sources[0].Exclude = nil
	cfg.Sources[0].Include = []string{"facts.txt"}
	coverage := run("scope-change")
	third, e := db.ActiveGeneration(context.Background(), "repo")
	if e != nil || third.ID == second.ID || len(coverage.Changes) != 0 {
		t.Fatal("coverage-only refresh reused stale epoch")
	}
	snap, report, e := db.ActiveInputMetadata(context.Background(), "repo")
	if e != nil || snap.Git != metadata.Snapshots[0].Git {
		t.Fatal("coverage-only refresh changed Git provenance")
	}
	found := false
	for _, entry := range report.Entries {
		if entry.Path == "excluded.txt" && entry.Outcome == "excluded" && entry.Reason == "not_included" {
			found = true
		}
	}
	if !found {
		t.Fatal("fresh canonical exclusion coverage missing")
	}
	files, e := db.ActiveFiles(context.Background(), "repo")
	if e != nil || len(files) != 1 || files[0].Content != "stable fixture knowledge" {
		t.Fatal("metadata/coverage refresh changed immutable included evidence")
	}
	git("-c", "user.name=t", "-c", "user.email=t@e", "commit", "--allow-empty", "-qm", "retention")
	if result := run("retention"); result.RetentionWarning != "" {
		t.Fatal(result.RetentionWarning)
	}
	var retained int
	if e = db.DB().QueryRow(`SELECT count(*) FROM generations WHERE repo_id='repo'`).Scan(&retained); e != nil || retained != 3 {
		t.Fatalf("configured active-plus-two-history retention failed: %d %v", retained, e)
	}
	latest, e := db.ActiveGeneration(context.Background(), "repo")
	if e != nil || latest.ID == third.ID {
		t.Fatal("retention displaced active generation")
	}
	if raw, e := os.ReadFile(filepath.Join(source, "facts.txt")); e != nil || string(raw) != "stable fixture knowledge" {
		t.Fatal("retention modified source")
	}
}
