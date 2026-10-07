package maintenance

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/AdamNi-7080/AIOS/internal/adapter"
	"github.com/AdamNi-7080/AIOS/internal/app"
	"github.com/AdamNi-7080/AIOS/internal/catalog"
	"github.com/AdamNi-7080/AIOS/internal/model"
	"github.com/AdamNi-7080/AIOS/internal/store"
)

func watchedFixture(t *testing.T) (string, string) {
	t.Helper()
	source, data := ownedTemp(t), ownedTemp(t)
	git := func(args ...string) {
		t.Helper()
		b, e := exec.Command("git", append([]string{"-C", source}, args...)...).CombinedOutput()
		if e != nil {
			t.Fatalf("git fixture: %v %s", e, b)
		}
	}
	git("init", "-q", "--initial-branch=main")
	if e := os.WriteFile(filepath.Join(source, "facts.txt"), []byte("original knowledge"), 0640); e != nil {
		t.Fatal(e)
	}
	git("add", ".")
	git("-c", "user.name=t", "-c", "user.email=t@e", "commit", "-qm", "one")
	t.Cleanup(func() {
		_ = filepath.Walk(data, func(path string, info os.FileInfo, e error) error {
			if e == nil && info.IsDir() {
				return os.Chmod(path, 0700)
			}
			return e
		})
	})
	return source, data
}

func actualRunner(source, data string, started chan<- time.Time) Runner {
	cfg := catalog.Config{Version: 1, Limits: catalog.Defaults(), Sources: []catalog.Source{{Kind: model.SourceKindRepository, ID: "repo"}}}
	return func(ctx context.Context, id string) (Outcome, error) {
		select {
		case started <- time.Now():
		default:
		}
		d, e := adapter.CaptureLocal(ctx, adapter.LocalRepository{ID: id, Path: source}, data, cfg.Limits)
		if e != nil {
			return Outcome{}, e
		}
		indexed, e := app.IngestMaintained(ctx, cfg, d, "watch-fixture", data)
		if e != nil {
			return Outcome{Revision: d.Revision}, e
		}
		changed := 0
		for _, n := range indexed.Changes {
			changed += n
		}
		return Outcome{Revision: d.Revision, Generation: indexed.ActiveGenerations[id], ChangedFiles: changed}, nil
	}
}

func TestActualOSWatchDebouncesAndActivatesOnlyChangedInput(t *testing.T) {
	source, data := watchedFixture(t)
	started := make(chan time.Time, 64)
	o := optionsForTest()
	o.QuietPeriod = 120 * time.Millisecond
	o.MaximumDebounce = 300 * time.Millisecond
	o.ReconcileInterval = time.Hour
	e, err := New(data, []Source{{ID: "repo", Mode: "direct", Path: source}}, actualRunner(source, data, started), o)
	if err != nil {
		t.Fatal(err)
	}
	if err = e.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer e.Close()
	first := waitJob(t, e, "repo", func(j Job) bool { return j.State == "idle" && j.ActiveGeneration != "" })
	if first.WatchError != "" {
		t.Fatalf("test did not establish actual watches: %+v", first)
	}
	indexBefore, err := os.ReadFile(filepath.Join(source, ".git/index"))
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(source, "facts.txt"), []byte("updated knowledge"), 0640); err != nil {
		t.Fatal(err)
	}
	time.Sleep(40 * time.Millisecond)
	if e.Status().Jobs[0].Runs != first.Runs {
		t.Fatal("edit ran before the quiet period")
	}
	updated := waitJob(t, e, "repo", func(j Job) bool { return j.State == "idle" && j.ActiveGeneration != first.ActiveGeneration })
	if updated.ChangedFiles != 1 || updated.Reason != "workspace_edit" {
		t.Fatalf("watch did not perform actual delta work: %+v", updated)
	}
	indexAfter, err := os.ReadFile(filepath.Join(source, ".git/index"))
	if err != nil || string(indexBefore) != string(indexAfter) {
		t.Fatal("watched update modified source index")
	}
	if body, err := os.ReadFile(filepath.Join(source, "facts.txt")); err != nil || string(body) != "updated knowledge" {
		t.Fatal("watched update modified source bytes")
	}
}

func TestLostOSWatchStreamReconcilesActualChangedGeneration(t *testing.T) {
	source, data := watchedFixture(t)
	started := make(chan time.Time, 64)
	o := optionsForTest()
	o.ReconcileInterval = 80 * time.Millisecond
	e, err := New(data, []Source{{ID: "repo", Mode: "direct", Path: source}}, actualRunner(source, data, started), o)
	if err != nil {
		t.Fatal(err)
	}
	if err = e.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer e.Close()
	first := waitJob(t, e, "repo", func(j Job) bool { return j.State == "idle" && j.ActiveGeneration != "" })
	// Close the actual OS event stream, then change source without any watcher
	// callback. Periodic reconciliation must capture/promote the real bytes.
	e.mu.Lock()
	watch := e.watch
	e.mu.Unlock()
	if watch == nil {
		t.Fatal("no actual OS watcher")
	}
	if err = watch.watcher.Close(); err != nil {
		t.Fatal(err)
	}
	<-watch.finished
	if err = os.WriteFile(filepath.Join(source, "facts.txt"), []byte("changed without watch events"), 0640); err != nil {
		t.Fatal(err)
	}
	updated := waitJob(t, e, "repo", func(j Job) bool { return j.State == "idle" && j.ActiveGeneration != first.ActiveGeneration })
	if updated.WatchError == "" || updated.ChangedFiles != 1 {
		t.Fatalf("lost event not disclosed/reconciled: %+v", updated)
	}
}

func TestContinuousActualEditsHaveMaximumDebounceDelay(t *testing.T) {
	source, data := watchedFixture(t)
	started := make(chan time.Time, 64)
	o := optionsForTest()
	o.QuietPeriod = 120 * time.Millisecond
	o.MaximumDebounce = 300 * time.Millisecond
	o.ReconcileInterval = time.Hour
	e, err := New(data, []Source{{ID: "repo", Mode: "direct", Path: source}}, actualRunner(source, data, started), o)
	if err != nil {
		t.Fatal(err)
	}
	if err = e.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer e.Close()
	first := waitJob(t, e, "repo", func(j Job) bool { return j.State == "idle" && j.ActiveGeneration != "" })
	for len(started) > 0 {
		<-started
	}
	begin := time.Now()
	ticker := time.NewTicker(35 * time.Millisecond)
	defer ticker.Stop()
	deadline := time.NewTimer(650 * time.Millisecond)
	defer deadline.Stop()
	observed := false
loop:
	for {
		select {
		case <-ticker.C:
			if err = os.WriteFile(filepath.Join(source, "facts.txt"), []byte(time.Now().Format(time.RFC3339Nano)), 0640); err != nil {
				t.Fatal(err)
			}
		case at := <-started:
			if !observed && at.Sub(begin) > 500*time.Millisecond {
				t.Fatal("continuous edits defeated maximum debounce")
			}
			observed = true
		case <-deadline.C:
			break loop
		}
	}
	if !observed {
		t.Fatal("no actual capture began under continuous edits")
	}
	if err = os.WriteFile(filepath.Join(source, "facts.txt"), []byte("final stable knowledge"), 0640); err != nil {
		t.Fatal(err)
	}
	db, err := store.OpenReadOnly(data)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	updated := waitJob(t, e, "repo", func(j Job) bool {
		if j.State != "idle" || j.ActiveGeneration == first.ActiveGeneration {
			return false
		}
		files, err := db.ActiveFiles(context.Background(), "repo")
		return err == nil && len(files) == 1 && files[0].Content == "final stable knowledge"
	})
	if updated.ChangedFiles > 1 {
		t.Fatal("continuous edits did not produce actual changed generation")
	}
}

func TestPortableClockGapReconcilesActualMissedInput(t *testing.T) {
	source, data := watchedFixture(t)
	var offset atomic.Int64
	o := optionsForTest()
	o.ReconcileInterval = time.Hour
	o.Now = func() time.Time { return time.Now().Add(time.Duration(offset.Load())) }
	e, err := New(data, []Source{{ID: "repo", Mode: "direct", Path: source}}, actualRunner(source, data, make(chan time.Time, 8)), o)
	if err != nil {
		t.Fatal(err)
	}
	if err = e.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer e.Close()
	first := waitJob(t, e, "repo", func(j Job) bool { return j.State == "idle" && j.ActiveGeneration != "" })
	e.mu.Lock()
	watch := e.watch
	e.mu.Unlock()
	if watch == nil {
		t.Fatal("no OS watch established")
	}
	watch.Close()
	if err = os.WriteFile(filepath.Join(source, "facts.txt"), []byte("edit missed across clock gap"), 0640); err != nil {
		t.Fatal(err)
	}
	offset.Store(int64(5 * time.Minute))
	updated := waitJob(t, e, "repo", func(j Job) bool { return j.State == "idle" && j.ActiveGeneration != first.ActiveGeneration })
	if updated.Reason != "wake_reconcile" || updated.ChangedFiles != 1 {
		t.Fatalf("clock gap did not reconcile actual input: %+v", updated)
	}
}

func TestCancellationAfterActualActivationKeepsDurableJobAndQueueIdentity(t *testing.T) {
	source, data := watchedFixture(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	base := actualRunner(source, data, make(chan time.Time, 8))
	run := func(ctx context.Context, id string) (Outcome, error) {
		ctx = app.WithProgress(ctx, func(p app.Progress) {
			if p.Stage == "activated" {
				cancel()
			}
		})
		return base(ctx, id)
	}
	e, err := New(data, []Source{{ID: "repo", Mode: "direct", Path: source}}, run, optionsForTest())
	if err != nil {
		t.Fatal(err)
	}
	if err = e.Start(ctx); err != nil {
		t.Fatal(err)
	}
	select {
	case <-e.done:
	case <-time.After(5 * time.Second):
		t.Fatal("post-activation cancellation did not finish")
	}
	e.Close()
	job := e.Status().Jobs[0]
	db, err := store.OpenReadOnly(data)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	active, err := db.ActiveGeneration(context.Background(), "repo")
	if err != nil {
		t.Fatal(err)
	}
	queues, err := db.IngestionStatus(context.Background(), "repo")
	if err != nil {
		t.Fatal(err)
	}
	if job.State != "idle" || job.ActiveGeneration != active.ID || job.ActiveRevision != queues[0].CurrentRevision || queues[0].State != "completed" || queues[0].PendingRevision != "" {
		t.Fatalf("post-activation durable identity disagrees: %+v %+v %+v", job, active, queues)
	}
	var target, activation string
	if err = db.DB().QueryRow(`SELECT target_generation_id,activated_at FROM ir_delta_records ORDER BY rowid DESC LIMIT 1`).Scan(&target, &activation); err != nil || target != active.ID || activation == "" {
		t.Fatal("activated queue/delta were not committed with actual knowledge")
	}
	restarted, err := New(data, []Source{{ID: "repo", Mode: "direct", Path: source}}, base, optionsForTest())
	if err != nil {
		t.Fatal(err)
	}
	if err = restarted.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer restarted.Close()
	updated := waitJob(t, restarted, "repo", func(j Job) bool { return j.State == "idle" && j.Runs > job.Runs })
	if updated.ActiveGeneration != active.ID || updated.ActiveRevision != job.ActiveRevision {
		t.Fatal("restart changed completed canonical identity")
	}
	var revision, sourceGeneration string
	if err = db.DB().QueryRow(`SELECT source_revision,source_generation_id FROM ir_delta_records ORDER BY rowid DESC LIMIT 1`).Scan(&revision, &sourceGeneration); err != nil || revision != job.ActiveRevision || sourceGeneration != active.ID {
		t.Fatal("restart delta source epoch disagrees with committed knowledge")
	}
}
