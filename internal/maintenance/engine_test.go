package maintenance

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/AdamNi-7080/AIOS/internal/resourcepolicy"
)

func ownedTemp(t *testing.T) string {
	t.Helper()
	root, e := filepath.EvalSymlinks(t.TempDir())
	if e != nil {
		t.Fatal(e)
	}
	if e = os.Chmod(root, 0700); e != nil {
		t.Fatal(e)
	}
	return root
}
func optionsForTest() Options {
	o := Defaults()
	o.Probe = func(context.Context) resourcepolicy.Signals { return fakeSignals("ac", 0) }
	o.TickInterval = 5 * time.Millisecond
	o.RetryBase = 40 * time.Millisecond
	o.RetryCap = 80 * time.Millisecond
	o.MaximumAttempts = 3
	return o
}
func waitJob(t *testing.T, e *Engine, id string, predicate func(Job) bool) Job {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		for _, job := range e.Status().Jobs {
			if job.Repository == id && predicate(job) {
				return job
			}
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("job %s did not reach outcome: %+v", id, e.Status())
	return Job{}
}

func TestCoalescedBurstRetainsOneFollowupAndRestartIdentity(t *testing.T) {
	root := ownedTemp(t)
	started := make(chan struct{}, 1)
	release := make(chan struct{})
	var calls atomic.Int32
	run := func(ctx context.Context, id string) (Outcome, error) {
		if calls.Add(1) == 1 {
			started <- struct{}{}
			select {
			case <-release:
			case <-ctx.Done():
				return Outcome{}, ctx.Err()
			}
		}
		return Outcome{Revision: "revision", Generation: "generation", ChangedFiles: 1}, nil
	}
	e, err := New(root, []Source{{ID: "repo", Mode: "mirror"}}, run, optionsForTest())
	if err != nil {
		t.Fatal(err)
	}
	if err = e.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(e.Close)
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("worker did not start")
	}
	for i := 0; i < 100; i++ {
		if err = e.Request("repo", "check_now"); err != nil {
			t.Fatal(err)
		}
	}
	if status := e.Status(); status.QueueCapacity != 100 || status.WorkerCapacity != 1 || status.Jobs[0].Coalesced < 100 {
		t.Fatalf("unbounded/uncoalesced burst: %+v", status)
	}
	close(release)
	job := waitJob(t, e, "repo", func(j Job) bool { return j.State == "idle" && j.Runs == 2 })
	if calls.Load() != 2 || job.ActiveGeneration != "generation" || job.LastSuccess.IsZero() {
		t.Fatal("burst execution or outcome dishonest")
	}
	e.Close()
	restored, err := New(root, []Source{{ID: "repo", Mode: "mirror"}}, run, optionsForTest())
	if err != nil {
		t.Fatal(err)
	}
	defer restored.Close()
	if !restored.Status().Restored || restored.Status().Jobs[0].ActiveGeneration != "generation" || restored.Status().Jobs[0].Runs != 2 {
		t.Fatal("durable state lost after restart")
	}
	info, err := os.Stat(filepath.Join(root, "maintenance.json"))
	if err != nil || info.Mode().Perm() != 0600 {
		t.Fatal("journal is not owner-only")
	}
}

func TestBoundedOfflineRetriesDoNotDisableHealthyRepository(t *testing.T) {
	root := ownedTemp(t)
	var failed atomic.Int32
	run := func(ctx context.Context, id string) (Outcome, error) {
		if id == "bad" {
			failed.Add(1)
			return Outcome{}, errors.New("private raw path and credentials must not persist")
		}
		return Outcome{Revision: "healthy-revision", Generation: "healthy-generation", ChangedFiles: 0}, nil
	}
	e, err := New(root, []Source{{ID: "bad", Mode: "mirror"}, {ID: "good", Mode: "mirror"}}, run, optionsForTest())
	if err != nil {
		t.Fatal(err)
	}
	if err = e.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer e.Close()
	healthy := waitJob(t, e, "good", func(j Job) bool { return j.State == "idle" && !j.LastSuccess.IsZero() })
	if healthy.ActiveGeneration != "healthy-generation" {
		t.Fatal("healthy repository disabled")
	}
	job := waitJob(t, e, "bad", func(j Job) bool { return j.State == "exhausted" })
	if job.Attempts != 3 || failed.Load() != 3 || !job.NextAttempt.IsZero() || !job.Stale {
		t.Fatalf("retry budget not bounded: %+v", job)
	}
	e.Close()
	restored, err := New(root, []Source{{ID: "bad", Mode: "mirror"}, {ID: "good", Mode: "mirror"}}, run, optionsForTest())
	if err != nil {
		t.Fatal(err)
	}
	if err = restored.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer restored.Close()
	waitJob(t, restored, "good", func(j Job) bool { return j.State == "idle" && j.Runs >= 2 })
	if failed.Load() != 3 {
		t.Fatal("restart reset exhausted retry budget")
	}
	bytes, err := os.ReadFile(filepath.Join(root, "maintenance.json"))
	if err != nil {
		t.Fatal(err)
	}
	if string(bytes) == "" || strings.Contains(string(bytes), "private raw path") {
		t.Fatal("freeform error persisted")
	}
}

func TestDurableQueueSupportsOneHundredAndRejectsOverflowBeforeMutation(t *testing.T) {
	root := ownedTemp(t)
	sources := make([]Source, 100)
	for i := range sources {
		sources[i] = Source{ID: fmt.Sprintf("repo-%03d", i), Mode: "mirror"}
	}
	run := func(context.Context, string) (Outcome, error) {
		return Outcome{Revision: "revision", Generation: "generation", ChangedFiles: 0}, nil
	}
	e, err := New(root, sources, run, Defaults())
	if err != nil {
		t.Fatal(err)
	}
	defer e.Close()
	if len(e.Status().Jobs) != 100 {
		t.Fatal("100 repository journal incomplete")
	}
	before, err := os.ReadFile(filepath.Join(root, "maintenance.json"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = New(root, append(sources, Source{ID: "extra", Mode: "mirror"}), run, Defaults()); err == nil {
		t.Fatal("overflow accepted")
	}
	after, err := os.ReadFile(filepath.Join(root, "maintenance.json"))
	if err != nil || string(before) != string(after) {
		t.Fatal("overflow mutated durable state")
	}
}

func TestRestartPreservesFutureRetryDeadline(t *testing.T) {
	root := ownedTemp(t)
	var calls atomic.Int32
	run := func(context.Context, string) (Outcome, error) { calls.Add(1); return Outcome{}, errors.New("offline") }
	o := optionsForTest()
	o.RetryBase = 300 * time.Millisecond
	o.RetryCap = time.Second
	e, err := New(root, []Source{{ID: "repo", Mode: "mirror"}}, run, o)
	if err != nil {
		t.Fatal(err)
	}
	if err = e.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	first := waitJob(t, e, "repo", func(j Job) bool { return j.State == "retry_wait" })
	e.Close()
	restarted, err := New(root, []Source{{ID: "repo", Mode: "mirror"}}, run, o)
	if err != nil {
		t.Fatal(err)
	}
	if err = restarted.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer restarted.Close()
	if deadline := restarted.Status().Jobs[0].NextAttempt; !deadline.Equal(first.NextAttempt) {
		t.Fatal("restart bypassed retry deadline")
	}
	time.Sleep(80 * time.Millisecond)
	if calls.Load() != 1 {
		t.Fatal("restart ran before persisted backoff")
	}
	second := waitJob(t, restarted, "repo", func(j Job) bool { return j.Attempts == 2 && j.State == "retry_wait" })
	if second.NextAttempt.Sub(second.LastAttempt) < 600*time.Millisecond {
		t.Fatal("restart reset exponential retry count")
	}
}
