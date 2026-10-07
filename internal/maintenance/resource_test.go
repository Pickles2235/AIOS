package maintenance

import (
	"context"
	"errors"
	"os"
	"strings"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"github.com/AdamNi-7080/AIOS/internal/resourcepolicy"
)

func fakeSignals(power string, idle float64) resourcepolicy.Signals {
	return resourcepolicy.Signals{Provider: "test", Power: power, Load: 0.1, IdleSeconds: idle, ObservedAt: time.Now().UTC(), Available: true}
}
func TestResourceDeferralResumeAndMaximumWait(t *testing.T) {
	for _, resume := range []bool{true, false} {
		t.Run(map[bool]string{true: "idle_resume", false: "maximum_wait"}[resume], func(t *testing.T) {
			root := ownedTemp(t)
			var calls atomic.Int32
			power := "battery"
			o := optionsForTest()
			o.MaximumDeferred = 120 * time.Millisecond
			o.Probe = func(context.Context) resourcepolicy.Signals { return fakeSignals(power, 0) }
			e, err := New(root, []Source{{ID: "repo", Mode: "mirror"}}, func(context.Context, string) (Outcome, error) {
				calls.Add(1)
				return Outcome{Revision: "rev", Generation: "gen", ChangedFiles: 0}, nil
			}, o)
			if err != nil {
				t.Fatal(err)
			}
			defer e.Close()
			if err = e.Start(context.Background()); err != nil {
				t.Fatal(err)
			}
			j := waitJob(t, e, "repo", func(j Job) bool { return j.State == "pending" && j.DeferredReason == "battery" })
			if j.PendingSince.IsZero() || calls.Load() != 0 {
				t.Fatalf("not deferred: %+v", j)
			}
			if resume {
				power = "ac"
				e.options.Probe = func(context.Context) resourcepolicy.Signals { return fakeSignals(power, 130) }
				p := e.RefreshPolicy(context.Background())
				if p.State != "idle_opportunity" {
					t.Fatal(p)
				}
			}
			j = waitJob(t, e, "repo", func(j Job) bool { return j.State == "idle" && j.ActiveGeneration == "gen" })
			if calls.Load() != 1 || j.DeferredReason != "" {
				t.Fatalf("starved or stale deferral: %+v", j)
			}
		})
	}
}
func TestResourceFairnessAndPendingCancel(t *testing.T) {
	root := ownedTemp(t)
	order := make(chan string, 2)
	started := make(chan struct{}, 1)
	release := make(chan struct{})
	o := optionsForTest()
	o.Probe = func(context.Context) resourcepolicy.Signals { return fakeSignals("ac", 0) }
	e, err := New(root, []Source{{ID: "a", Mode: "mirror"}, {ID: "b", Mode: "mirror"}}, func(ctx context.Context, id string) (Outcome, error) {
		order <- id
		if id == "a" { started <- struct{}{}; select { case <-release: case <-ctx.Done(): return Outcome{}, ctx.Err() } }
		return Outcome{Revision: "rev", Generation: "gen", ChangedFiles: 0}, nil
	}, o)
	if err != nil {
		t.Fatal(err)
	}
	defer e.Close()
	if err = e.Request("a", "check_now"); err != nil { t.Fatal(err) }
	if err = e.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	select { case <-started: case <-time.After(time.Second): t.Fatal("older work did not start") }
	waitJob(t, e, "b", func(j Job) bool { return j.State == "pending" })
	if err = e.Cancel("b"); err != nil {
		t.Fatal(err)
	}
	j := waitJob(t, e, "b", func(j Job) bool { return j.State == "idle" && strings.Contains(j.Error, "cancelled") })
	if j.ActiveGeneration != "" {
		t.Fatal("cancelled pending work activated")
	}
	close(release)
	waitJob(t, e, "a", func(j Job) bool { return j.State == "idle" && j.ActiveGeneration == "gen" })
	select {
	case id := <-order:
		if id != "a" {
			t.Fatal("cancelled work ran")
		}
	default:
		t.Fatal("fair work not run")
	}
}
func TestRunningCancelAndDiskFullKeepLastGood(t *testing.T) {
	root := ownedTemp(t)
	started := make(chan struct{}, 1)
	var calls atomic.Int32
	run := func(ctx context.Context, _ string) (Outcome, error) {
		switch calls.Add(1) {
		case 1:
			return Outcome{Revision: "first", Generation: "g1", ChangedFiles: 0}, nil
		case 2:
			started <- struct{}{}
			<-ctx.Done()
			return Outcome{}, ctx.Err()
		default:
			return Outcome{}, &os.PathError{Op: "write", Path: "/private/secret", Err: syscall.ENOSPC}
		}
	}
	o := optionsForTest()
	o.Probe = func(context.Context) resourcepolicy.Signals { return fakeSignals("ac", 0) }
	e, err := New(root, []Source{{ID: "repo", Mode: "mirror"}}, run, o)
	if err != nil {
		t.Fatal(err)
	}
	defer e.Close()
	if err = e.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	waitJob(t, e, "repo", func(j Job) bool { return j.State == "idle" && j.ActiveGeneration == "g1" })
	if err = e.Request("repo", "check_now"); err != nil {
		t.Fatal(err)
	}
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("second run did not start")
	}
	if err = e.Cancel("repo"); err != nil {
		t.Fatal(err)
	}
	j := waitJob(t, e, "repo", func(j Job) bool { return j.State == "idle" && strings.Contains(j.Error, "cancelled") })
	if j.ActiveGeneration != "g1" {
		t.Fatal("cancel discarded last good")
	}
	if err = e.Request("repo", "check_now"); err != nil {
		t.Fatal(err)
	}
	j = waitJob(t, e, "repo", func(j Job) bool { return j.State == "retry_wait" && strings.Contains(j.Error, "storage is full") })
	if j.ActiveGeneration != "g1" || strings.Contains(j.Error, "/private/") {
		t.Fatalf("disk failure leaked or removed last good: %+v", j)
	}
}

func TestJournalDiskFullRollsBackRequestAndKeepsActive(t *testing.T) {
	root := ownedTemp(t)
	o := optionsForTest()
	o.Probe = func(context.Context) resourcepolicy.Signals { return fakeSignals("ac", 0) }
	e, err := New(root, []Source{{ID: "repo", Mode: "mirror"}}, func(context.Context, string) (Outcome, error) {
		return Outcome{Revision: "rev", Generation: "g1", ChangedFiles: 0}, nil
	}, o)
	if err != nil {
		t.Fatal(err)
	}
	defer e.Close()
	if err = e.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	before := waitJob(t, e, "repo", func(j Job) bool { return j.State == "idle" && j.ActiveGeneration == "g1" })
	e.mu.Lock()
	e.persistFault = func() error { return &os.PathError{Op: "write", Path: "/private/secret", Err: syscall.ENOSPC} }
	e.mu.Unlock()
	if err = e.Request("repo", "check_now"); !errors.Is(err, syscall.ENOSPC) {
		t.Fatalf("expected injected ENOSPC: %v", err)
	}
	after := e.Status().Jobs[0]
	if after.ActiveGeneration != before.ActiveGeneration || after.State != "idle" || after.Requests != before.Requests || strings.Contains(e.Status().PersistenceError, "/private/") {
		t.Fatalf("failed journal write changed durable active state: %+v", after)
	}
	e.mu.Lock()
	e.persistFault = nil
	e.mu.Unlock()
	e.Close()
	restored, err := New(root, []Source{{ID: "repo", Mode: "mirror"}}, func(context.Context, string) (Outcome, error) {
		return Outcome{Revision: "rev", Generation: "g1", ChangedFiles: 0}, nil
	}, o)
	if err != nil {
		t.Fatal(err)
	}
	defer restored.Close()
	if restored.Status().Jobs[0].ActiveGeneration != "g1" {
		t.Fatal("journal ENOSPC lost last good")
	}
}

func TestCancelledRunnerSuccessCannotPromoteLateResult(t *testing.T) {
	root := ownedTemp(t)
	started := make(chan struct{}, 1)
	release := make(chan struct{})
	o := optionsForTest()
	o.Probe = func(context.Context) resourcepolicy.Signals { return fakeSignals("ac", 0) }
	e, err := New(root, []Source{{ID: "repo", Mode: "mirror"}}, func(ctx context.Context, _ string) (Outcome, error) {
		started <- struct{}{}
		<-release
		return Outcome{Revision: "late", Generation: "late-generation", ChangedFiles: 0}, nil
	}, o)
	if err != nil {
		t.Fatal(err)
	}
	defer e.Close()
	if err = e.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("runner did not start")
	}
	if err = e.Cancel("repo"); err != nil {
		t.Fatal(err)
	}
	close(release)
	j := waitJob(t, e, "repo", func(j Job) bool { return j.State == "idle" && strings.Contains(j.Error, "cancelled") })
	if j.ActiveGeneration != "" || j.ActiveRevision != "" {
		t.Fatalf("cancelled late result promoted: %+v", j)
	}
}

func TestStorageBudgetDefersThenResumesWithoutAttempt(t *testing.T) {
	root := ownedTemp(t)
	var denied atomic.Bool
	denied.Store(true)
	var calls atomic.Int32
	o := optionsForTest()
	o.Probe = func(context.Context) resourcepolicy.Signals { return fakeSignals("ac", 0) }
	o.StorageCheck = func(string) (resourcepolicy.Storage, error) {
		if denied.Load() {
			return resourcepolicy.Storage{StorageState: "budget", MaxOwnedBytes: 256, OwnedBytes: 224}, resourcepolicy.ErrStorageBudget
		}
		return resourcepolicy.Storage{StorageState: "available", MaxOwnedBytes: 256, OwnedBytes: 64}, nil
	}
	e, err := New(root, []Source{{ID: "repo", Mode: "mirror"}}, func(context.Context, string) (Outcome, error) {
		calls.Add(1)
		return Outcome{Revision: "rev", Generation: "gen", ChangedFiles: 0}, nil
	}, o)
	if err != nil {
		t.Fatal(err)
	}
	defer e.Close()
	if err = e.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	j := waitJob(t, e, "repo", func(j Job) bool { return j.State == "pending" && j.DeferredReason == "budget" })
	if j.Runs != 0 || calls.Load() != 0 || e.ResourceStatus().StorageState != "budget" {
		t.Fatalf("storage budget did not defer: %+v", j)
	}
	denied.Store(false)
	e.RefreshPolicy(context.Background())
	j = waitJob(t, e, "repo", func(j Job) bool { return j.State == "idle" && j.ActiveGeneration == "gen" })
	if j.Runs != 1 || calls.Load() != 1 {
		t.Fatalf("budget recovery did not resume: %+v", j)
	}
}

func TestConstrainedRetryKeepsFiniteWait(t *testing.T) {
	root := ownedTemp(t)
	var calls atomic.Int32
	o := optionsForTest()
	o.MaximumDeferred = 70 * time.Millisecond
	o.Probe = func(context.Context) resourcepolicy.Signals { return fakeSignals("battery", 0) }
	e, err := New(root, []Source{{ID: "repo", Mode: "mirror"}}, func(context.Context, string) (Outcome, error) {
		if calls.Add(1) == 1 {
			return Outcome{}, errors.New("temporary")
		}
		return Outcome{Revision: "rev", Generation: "gen"}, nil
	}, o)
	if err != nil {
		t.Fatal(err)
	}
	defer e.Close()
	if err = e.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	j := waitJob(t, e, "repo", func(j Job) bool { return j.State == "idle" && j.ActiveGeneration == "gen" })
	if calls.Load() != 2 || j.Runs != 2 {
		t.Fatalf("constrained retry starved: %+v", j)
	}
}
