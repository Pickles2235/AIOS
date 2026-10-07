// Package maintenance owns bounded durable repository work. Source approval,
// immutable capture and canonical activation remain outside this scheduler.
package maintenance

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"maps"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"sync"
	"syscall"
	"time"

	"errors"
	"github.com/AdamNi-7080/AIOS/internal/lifecycle"
	"github.com/AdamNi-7080/AIOS/internal/mirror"
	"github.com/AdamNi-7080/AIOS/internal/resourcepolicy"
)

const MaxRepositories = 100
const maxJournalBytes = 512 << 10

var repositoryID = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{0,62}$`)

type Source struct{ ID, Mode, Path string }
type Outcome struct {
	Revision, Generation string
	ChangedFiles         int
	RetentionWarning     string
}

type CanonicalState struct {
	Revision, Generation string
	ActivatedAt          time.Time
}

// RestoreCanonical reconciles journal identity with the actual canonical store
// before workers start, including a completed Build or post-commit restart.
func (e *Engine) RestoreCanonical(active map[string]CanonicalState) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.started || e.closed {
		return fmt.Errorf("canonical restore requires stopped maintenance")
	}
	old := maps.Clone(e.state.Jobs)
	for id, current := range active {
		job, ok := e.state.Jobs[id]
		if !ok || current.Generation == "" || current.Revision == "" || current.ActivatedAt.IsZero() {
			e.state.Jobs = old
			return fmt.Errorf("invalid canonical maintenance identity")
		}
		if job.ActiveGeneration != current.Generation {
			job.LastSuccess = current.ActivatedAt.UTC()
			job.Error = ""
			if job.State == "retry_wait" || job.State == "exhausted" {
				job.State = "idle"
				job.Attempts = 0
				job.NextAttempt = time.Time{}
			}
		}
		job.ActiveRevision = current.Revision
		job.ActiveGeneration = current.Generation
		e.state.Jobs[id] = job
	}
	if err := e.persistLocked(); err != nil {
		e.state.Jobs = old
		return err
	}
	return nil
}

type Runner func(context.Context, string) (Outcome, error)
type Options struct {
	MirrorInterval, ReconcileInterval, RetryBase, RetryCap, QuietPeriod, MaximumDebounce, TickInterval, JobTimeout time.Duration
	MaximumDeferred                                                                                                time.Duration
	MaximumAttempts                                                                                                int
	Now                                                                                                            func() time.Time
	Probe                                                                                                          resourcepolicy.Probe
	StorageCheck                                                                                                   func(string) (resourcepolicy.Storage, error)
	Preflight                                                                                                      func(context.Context, string) error
}

func Defaults() Options {
	return Options{MirrorInterval: 15 * time.Minute, ReconcileInterval: 30 * time.Second, RetryBase: 30 * time.Second, RetryCap: 15 * time.Minute, QuietPeriod: time.Second, MaximumDebounce: 5 * time.Second, TickInterval: 100 * time.Millisecond, JobTimeout: 15 * time.Minute, MaximumDeferred: resourcepolicy.MaxWait, MaximumAttempts: 5, Now: time.Now, Probe: resourcepolicy.Native, StorageCheck: func(root string) (resourcepolicy.Storage, error) {
		return resourcepolicy.CheckBudget(root, resourcepolicy.MaxOwnedBytes, resourcepolicy.MinimumFreeBytes, resourcepolicy.CaptureReserveBytes)
	}}
}

type Job struct {
	Repository        string    `json:"repository"`
	Mode              string    `json:"mode"`
	State             string    `json:"state"`
	Reason            string    `json:"reason"`
	Attempts          int       `json:"attempts"`
	Runs              uint64    `json:"runs"`
	Requests          uint64    `json:"requests"`
	Coalesced         uint64    `json:"coalesced"`
	Sequence          uint64    `json:"sequence"`
	Followup          bool      `json:"followup"`
	LastAttempt       time.Time `json:"last_attempt"`
	LastSuccess       time.Time `json:"last_success"`
	NextAttempt       time.Time `json:"next_attempt"`
	PendingSince      time.Time `json:"pending_since,omitempty"`
	DeferredReason    string    `json:"deferred_reason,omitempty"`
	NextPoll          time.Time `json:"next_poll"`
	FirstEdit         time.Time `json:"first_edit"`
	LastEdit          time.Time `json:"last_edit"`
	AttemptedRevision string    `json:"attempted_revision"`
	ActiveRevision    string    `json:"active_revision"`
	ActiveGeneration  string    `json:"active_generation"`
	ChangedFiles      int       `json:"changed_files"`
	Error             string    `json:"error,omitempty"`
	WatchError        string    `json:"watch_error,omitempty"`
	RetentionWarning  string    `json:"retention_warning,omitempty"`
	Stale             bool      `json:"stale"`
}

type journal struct {
	Schema                int            `json:"schema_version"`
	MirrorIntervalSeconds int            `json:"mirror_interval_seconds"`
	Sequence              uint64         `json:"sequence"`
	Jobs                  map[string]Job `json:"jobs"`
}
type Status struct {
	Jobs                   []Job                 `json:"jobs"`
	MirrorIntervalSeconds  int                   `json:"mirror_interval_seconds"`
	RetryMaximumAttempts   int                   `json:"retry_max_attempts"`
	Durable                bool                  `json:"durable"`
	Restored               bool                  `json:"restored"`
	Coalescing             bool                  `json:"coalescing"`
	QueueCapacity          int                   `json:"queue_capacity"`
	WorkerCapacity         int                   `json:"worker_capacity"`
	QuietSeconds           float64               `json:"quiet_seconds"`
	MaximumDebounceSeconds float64               `json:"maximum_debounce_seconds"`
	PersistenceError       string                `json:"persistence_error,omitempty"`
	Resource               resourcepolicy.Status `json:"resource"`
}

type Engine struct {
	mu                        sync.Mutex
	path                      string
	options                   Options
	state                     journal
	sources                   map[string]Source
	run                       Runner
	wake                      chan struct{}
	ctx                       context.Context
	cancel                    context.CancelFunc
	done                      chan struct{}
	started, restored, closed bool
	persistenceError          string
	watch                     *watchSet
	resource                  resourcepolicy.Status
	currentID                 string
	currentCancel             context.CancelFunc
	cancelledID               string
	persistFault              func() error // package-local fault seam; production is nil
}

func New(dataDir string, sources []Source, run Runner, options Options) (*Engine, error) {
	if len(sources) > MaxRepositories || run == nil {
		return nil, fmt.Errorf("invalid maintained repository set")
	}
	if options.Now == nil {
		options = Defaults()
	}
	if options.Probe == nil {
		options.Probe = resourcepolicy.Native
	}
	if options.StorageCheck == nil {
		options.StorageCheck = Defaults().StorageCheck
	}
	if options.MaximumDeferred <= 0 {
		options.MaximumDeferred = resourcepolicy.MaxWait
	}
	if options.MirrorInterval <= 0 || options.ReconcileInterval <= 0 || options.RetryBase <= 0 || options.RetryCap < options.RetryBase || options.QuietPeriod <= 0 || options.MaximumDebounce < options.QuietPeriod || options.TickInterval <= 0 || options.JobTimeout <= 0 || options.MaximumAttempts < 1 || options.MaximumAttempts > 10 {
		return nil, fmt.Errorf("invalid bounded maintenance policy")
	}
	if err := lifecycle.PrepareDir(dataDir); err != nil {
		return nil, err
	}
	e := &Engine{path: filepath.Join(dataDir, "maintenance.json"), options: options, sources: map[string]Source{}, run: run, wake: make(chan struct{}, 1), done: make(chan struct{}), state: journal{Schema: 1, MirrorIntervalSeconds: int(options.MirrorInterval.Seconds()), Jobs: map[string]Job{}}}
	e.resource = resourcepolicy.Evaluate(resourcepolicy.Signals{Provider: "unobserved", Power: "unknown", ObservedAt: e.now()}, options.MaximumDeferred)
	if _, err := os.Lstat(e.path); err == nil {
		f, err := lifecycle.OpenBoundedMetadata(e.path, maxJournalBytes)
		if err != nil {
			return nil, err
		}
		b, err := io.ReadAll(io.LimitReader(f, maxJournalBytes+1))
		f.Close()
		if err != nil || len(b) > maxJournalBytes {
			return nil, fmt.Errorf("maintenance journal exceeds bound")
		}
		d := json.NewDecoder(bytes.NewReader(b))
		d.DisallowUnknownFields()
		if d.Decode(&e.state) != nil || d.Decode(&struct{}{}) != io.EOF || e.state.Schema != 1 || len(e.state.Jobs) > MaxRepositories || e.state.Jobs == nil {
			return nil, fmt.Errorf("invalid maintenance journal")
		}
		if e.state.MirrorIntervalSeconds >= 10 && e.state.MirrorIntervalSeconds <= 86400 {
			e.options.MirrorInterval = time.Duration(e.state.MirrorIntervalSeconds) * time.Second
		}
		e.restored = true
		if e.state.Sequence > 1<<60 {
			return nil, fmt.Errorf("invalid maintenance sequence")
		}
		for id, job := range e.state.Jobs {
			if !repositoryID.MatchString(id) || job.Repository != id || job.Attempts < 0 || job.Attempts > 10 || len(job.Error) > 512 || len(job.ActiveRevision) > 1024 || len(job.AttemptedRevision) > 1024 || len(job.ActiveGeneration) > 256 {
				return nil, fmt.Errorf("invalid durable repository job")
			}
			switch job.State {
			case "idle", "pending", "running", "retry_wait", "exhausted":
			default:
				return nil, fmt.Errorf("invalid durable job state")
			}
		}
	} else if !os.IsNotExist(err) {
		return nil, err
	}
	mode := ""
	for _, source := range sources {
		if !repositoryID.MatchString(source.ID) || (source.Mode != "direct" && source.Mode != "mirror") || e.sources[source.ID].ID != "" {
			return nil, fmt.Errorf("invalid maintained source")
		}
		if mode != "" && mode != source.Mode {
			return nil, fmt.Errorf("maintained sources must use one mode")
		}
		mode = source.Mode
		e.sources[source.ID] = source
		job := e.state.Jobs[source.ID]
		job.Repository = source.ID
		job.Mode = source.Mode
		if job.State == "" {
			job.State = "idle"
		}
		if job.State == "running" {
			job.Attempts = max(0, job.Attempts-1) // resume the interrupted attempt
			job.State = "pending"
			job.Reason = "restart_repair"
			job.NextAttempt = e.now()
			job.PendingSince = e.now()
			job.Followup = false
		}
		if (job.State == "pending" || job.State == "retry_wait") && job.PendingSince.IsZero() {
			job.PendingSince = e.now()
		}
		e.state.Jobs[source.ID] = job
	}
	for id := range e.state.Jobs {
		if e.sources[id].ID == "" {
			delete(e.state.Jobs, id)
		}
	}
	if err := e.persistLocked(); err != nil {
		return nil, err
	}
	return e, nil
}

func (e *Engine) now() time.Time { return e.options.Now().UTC() }

func (e *Engine) persistLocked() error {
	if _, err := os.Lstat(e.path); err == nil {
		f, err := lifecycle.OpenBoundedMetadata(e.path, maxJournalBytes)
		if err != nil {
			return err
		}
		f.Close()
	} else if !os.IsNotExist(err) {
		return err
	}
	b, err := json.Marshal(e.state)
	if err != nil || len(b) > maxJournalBytes {
		return fmt.Errorf("maintenance journal exceeds bound")
	}
	if e.persistFault != nil {
		return e.persistFault()
	}
	f, err := os.CreateTemp(filepath.Dir(e.path), ".maintenance-*")
	if err != nil {
		return err
	}
	name := f.Name()
	defer os.Remove(name)
	if _, err = f.Write(append(b, '\n')); err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	if err = os.Rename(name, e.path); err != nil {
		return err
	}
	dir, err := os.Open(filepath.Dir(e.path))
	if err != nil {
		return err
	}
	defer dir.Close()
	return dir.Sync()
}

func (e *Engine) signal() {
	select {
	case e.wake <- struct{}{}:
	default:
	}
}

func (e *Engine) Request(id, reason string) error { return e.request(id, reason, false) }
func (e *Engine) MarkEdit(id string) error        { return e.request(id, "workspace_edit", true) }

func (e *Engine) request(id, reason string, edit bool) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.closed {
		return fmt.Errorf("maintenance is stopped")
	}
	job, ok := e.state.Jobs[id]
	if !ok {
		return fmt.Errorf("repository is not maintained")
	}
	if reason != "check_now" && reason != "workspace_edit" && reason != "restart_reconcile" && reason != "scheduled_poll" && reason != "lost_event_reconcile" && reason != "wake_reconcile" {
		return fmt.Errorf("invalid maintenance reason")
	}
	old, sequence := job, e.state.Sequence
	now := e.now()
	job.Requests++
	continuation := reason == "restart_reconcile" && (job.State == "retry_wait" || job.State == "pending")
	keepExhausted := reason == "restart_reconcile" && job.State == "exhausted" && job.NextPoll.After(now)
	if job.State == "running" || job.State == "pending" {
		job.Coalesced++
	}
	if job.State == "running" {
		job.Followup = true
	} else if job.State != "pending" && !keepExhausted {
		e.state.Sequence++
		job.Sequence = e.state.Sequence
		job.State = "pending"
		job.PendingSince = now
		if !continuation {
			job.Attempts = 0
		}
	}
	job.Reason = reason
	if job.State == "pending" && job.PendingSince.IsZero() {
		job.PendingSince = now
	}
	if keepExhausted {
		job.NextAttempt = time.Time{}
	} else if edit {
		if job.FirstEdit.IsZero() {
			job.FirstEdit = now
		}
		job.LastEdit = now
		job.NextAttempt = now.Add(e.options.QuietPeriod)
		maximum := job.FirstEdit.Add(e.options.MaximumDebounce)
		if maximum.Before(job.NextAttempt) {
			job.NextAttempt = maximum
		}
	} else {
		if !continuation || !job.NextAttempt.After(now) {
			job.NextAttempt = now
		}
		job.FirstEdit = time.Time{}
		job.LastEdit = time.Time{}
	}
	e.state.Jobs[id] = job
	if err := e.persistLocked(); err != nil {
		e.state.Jobs[id] = old
		e.state.Sequence = sequence
		e.persistenceError = "Unable to persist maintenance work; last-good knowledge remains available."
		return err
	}
	e.persistenceError = ""
	e.signal()
	return nil
}

func (e *Engine) SetMirrorInterval(seconds int) error {
	if seconds < 10 || seconds > 86400 {
		return fmt.Errorf("poll interval must be10–86400seconds")
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	old := e.options.MirrorInterval
	oldJobs := maps.Clone(e.state.Jobs)
	e.options.MirrorInterval = time.Duration(seconds) * time.Second
	e.state.MirrorIntervalSeconds = seconds
	for id, job := range e.state.Jobs {
		if job.Mode == "mirror" {
			job.NextPoll = job.LastAttempt.Add(e.options.MirrorInterval)
			e.state.Jobs[id] = job
		}
	}
	if err := e.persistLocked(); err != nil {
		e.options.MirrorInterval = old
		e.state.MirrorIntervalSeconds = int(old.Seconds())
		e.state.Jobs = oldJobs
		return err
	}
	e.signal()
	return nil
}

func (e *Engine) Status() Status {
	e.mu.Lock()
	defer e.mu.Unlock()
	now := e.now()
	jobs := make([]Job, 0, len(e.state.Jobs))
	for _, job := range e.state.Jobs {
		interval := e.options.MirrorInterval
		if job.Mode == "direct" {
			interval = e.options.ReconcileInterval
		}
		job.Stale = job.ActiveGeneration == "" || job.Error != "" || (!job.LastSuccess.IsZero() && now.Sub(job.LastSuccess) > 2*interval)
		jobs = append(jobs, job)
	}
	sort.Slice(jobs, func(i, j int) bool { return jobs[i].Repository < jobs[j].Repository })
	return Status{jobs, int(e.options.MirrorInterval.Seconds()), e.options.MaximumAttempts, e.persistenceError == "", e.restored, true, MaxRepositories, 1, e.options.QuietPeriod.Seconds(), e.options.MaximumDebounce.Seconds(), e.persistenceError, e.resource}
}

func (e *Engine) ResourceStatus() resourcepolicy.Status {
	e.mu.Lock()
	defer e.mu.Unlock()
	p := e.resource
	now := e.now()
	for _, job := range e.state.Jobs {
		if job.State == "running" {
			p.Running++
		}
		if job.State == "pending" || job.State == "retry_wait" {
			p.QueueDepth++
			if !job.PendingSince.IsZero() && now.After(job.PendingSince) {
				age := now.Sub(job.PendingSince).Seconds()
				if age > p.OldestJobAgeSeconds {
					p.OldestJobAgeSeconds = age
				}
			}
		}
	}
	return p
}

// RefreshPolicy samples outside the journal lock. Tests can inject a probe;
// production always uses the native local provider.
func (e *Engine) RefreshPolicy(ctx context.Context) resourcepolicy.Status {
	signals := e.options.Probe(ctx)
	policy := resourcepolicy.Evaluate(signals, e.options.MaximumDeferred)
	e.mu.Lock()
	previous := e.resource.Storage
	e.mu.Unlock()
	if previous.StorageObservedAt.IsZero() || time.Since(previous.StorageObservedAt) >= 30*time.Second {
		policy.Storage, _ = e.options.StorageCheck(filepath.Dir(e.path))
	} else {
		policy.Storage = previous
	}
	e.mu.Lock()
	e.resource = policy
	e.mu.Unlock()
	e.signal()
	return policy
}

// Cancel abandons pending work or cancels the active attempt. A cancelled
// runner is never allowed to promote its returned outcome in this journal.
func (e *Engine) Cancel(id string) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	job, ok := e.state.Jobs[id]
	if !ok {
		return fmt.Errorf("repository is not maintained")
	}
	if job.State != "pending" && job.State != "retry_wait" && job.State != "running" {
		return fmt.Errorf("repository has no cancellable work")
	}
	if job.State == "running" {
		e.cancelledID = id
		if e.currentID == id && e.currentCancel != nil {
			e.currentCancel()
		}
		return nil
	}
	old := job
	job.State = "idle"
	job.Followup = false
	job.NextAttempt = time.Time{}
	job.PendingSince = time.Time{}
	job.DeferredReason = ""
	job.Error = "Repository update cancelled; last-good knowledge remains available."
	e.state.Jobs[id] = job
	if err := e.persistLocked(); err != nil {
		e.state.Jobs[id] = old
		e.persistenceError = "Unable to persist cancellation; work remains queued."
		return err
	}
	e.signal()
	return nil
}

func (e *Engine) Start(ctx context.Context) error {
	e.mu.Lock()
	if e.started || e.closed {
		e.mu.Unlock()
		return fmt.Errorf("maintenance already started or closed")
	}
	e.started = true
	e.ctx, e.cancel = context.WithCancel(ctx)
	e.mu.Unlock()
	e.RefreshPolicy(e.ctx)
	go e.loop()
	watch, err := newWatchSet(e, e.sources)
	if err != nil {
		e.setWatchError("OS watches unavailable; periodic reconciliation remains active.")
	} else {
		e.mu.Lock()
		closed := e.closed
		if !closed {
			e.watch = watch
		}
		e.mu.Unlock()
		if closed && watch != nil {
			watch.Close()
		}
	}
	for id := range e.sources {
		if err := e.Request(id, "restart_reconcile"); err != nil {
			e.Close()
			return err
		}
	}
	return nil
}

func (e *Engine) setWatchError(message string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	for id, job := range e.state.Jobs {
		if job.Mode == "direct" {
			job.WatchError = message
			e.state.Jobs[id] = job
		}
	}
}

func (e *Engine) loop() {
	defer close(e.done)
	var pump sync.WaitGroup
	pump.Add(1)
	go func() { defer pump.Done(); e.poll() }()
	defer pump.Wait()
	for {
		select {
		case <-e.ctx.Done():
			return
		case <-e.wake:
		}
		for {
			e.mu.Lock()
			id := ""
			var sequence uint64
			now := e.now()
			for candidate, job := range e.state.Jobs {
				if (job.State == "pending" || job.State == "retry_wait") && !job.NextAttempt.After(now) && (id == "" || job.Sequence < sequence) {
					id = candidate
					sequence = job.Sequence
				}
			}
			if id == "" {
				e.mu.Unlock()
				break
			}
			job := e.state.Jobs[id]
			e.mu.Unlock()
			if e.options.Preflight != nil {
				preflightCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
				_ = e.options.Preflight(preflightCtx, filepath.Dir(e.path))
				cancel()
			}
			storage, storageErr := e.options.StorageCheck(filepath.Dir(e.path))
			e.mu.Lock()
			e.resource.Storage = storage
			job = e.state.Jobs[id]
			if (job.State != "pending" && job.State != "retry_wait") || job.NextAttempt.After(e.now()) {
				e.mu.Unlock()
				continue
			}
			if storageErr != nil {
				old := job
				job.DeferredReason = storage.StorageState
				e.state.Jobs[id] = job
				if job.DeferredReason != old.DeferredReason {
					if err := e.persistLocked(); err != nil {
						e.state.Jobs[id] = old
						e.persistenceError = "Unable to persist storage deferral; work paused."
					}
				}
				e.mu.Unlock()
				break
			}
			if e.resource.State == "constrained" && (job.PendingSince.IsZero() || now.Sub(job.PendingSince) < e.options.MaximumDeferred) {
				if job.DeferredReason != e.resource.DeferredReason {
					old := job
					job.DeferredReason = e.resource.DeferredReason
					e.state.Jobs[id] = job
					if err := e.persistLocked(); err != nil {
						e.state.Jobs[id] = old
						e.persistenceError = "Unable to persist resource deferral; work paused."
					}
				}
				e.mu.Unlock()
				break
			}
			old := job
			job.State = "running"
			job.PendingSince = time.Time{}
			job.DeferredReason = ""
			job.Attempts++
			job.Runs++
			job.LastAttempt = now
			job.FirstEdit = time.Time{}
			job.LastEdit = time.Time{}
			job.Followup = false
			e.state.Jobs[id] = job
			if err := e.persistLocked(); err != nil {
				e.state.Jobs[id] = old
				e.persistenceError = "Unable to persist job start; work paused."
				e.mu.Unlock()
				break
			}
			e.mu.Unlock()
			ctx, cancel := context.WithTimeout(e.ctx, e.options.JobTimeout)
			e.mu.Lock()
			e.currentID = id
			e.currentCancel = cancel
			if e.cancelledID == id {
				cancel()
			}
			e.mu.Unlock()
			outcome, err := e.run(ctx, id)
			cancel()
			if err == nil && (outcome.Revision == "" || outcome.Generation == "") {
				err = fmt.Errorf("maintained outcome lacks canonical identity")
			}
			e.mu.Lock()
			e.currentID = ""
			e.currentCancel = nil
			cancelled := e.cancelledID == id
			if cancelled {
				e.cancelledID = ""
			}
			job = e.state.Jobs[id]
			followupDue := job.NextAttempt
			now = e.now()
			job.AttemptedRevision = outcome.Revision
			job.ChangedFiles = outcome.ChangedFiles
			job.RetentionWarning = outcome.RetentionWarning
			if cancelled {
				job.State = "idle"
				job.Attempts = 0
				job.NextAttempt = time.Time{}
				job.Error = "Repository update cancelled; last-good knowledge remains available."
			} else if e.ctx.Err() != nil && err != nil {
				job.Attempts = max(0, job.Attempts-1)
				job.State = "pending"
				job.Reason = "restart_repair"
				job.NextAttempt = now
				job.Error = "Build interrupted; last-good knowledge remains available."
			} else if err == nil {
				job.State = "idle"
				job.Attempts = 0
				job.LastSuccess = now
				job.ActiveRevision = outcome.Revision
				job.ActiveGeneration = outcome.Generation
				job.Error = ""
				job.NextAttempt = time.Time{}
			} else {
				job.Error = "Repository update failed; last-good knowledge remains available. Check source access and retry."
				if errors.Is(err, syscall.ENOSPC) {
					job.Error = "Owned storage is full; last-good knowledge remains available. Free space, then Check now."
				}
				var gitError *mirror.GitError
				if errors.As(err, &gitError) {
					job.Error = gitError.Remediation()
				}
				if job.Attempts >= e.options.MaximumAttempts {
					job.State = "exhausted"
					job.NextAttempt = time.Time{}
					job.PendingSince = time.Time{}
				} else {
					job.State = "retry_wait"
					job.PendingSince = now
					delay := e.options.RetryBase * time.Duration(1<<uint(job.Attempts-1))
					if delay > e.options.RetryCap {
						delay = e.options.RetryCap
					}
					job.NextAttempt = now.Add(delay)
				}
			}
			interval := e.options.MirrorInterval
			if job.Mode == "direct" {
				interval = e.options.ReconcileInterval
			}
			job.NextPoll = now.Add(interval)
			if job.Followup && !cancelled && e.ctx.Err() == nil {
				job.State = "pending"
				job.Attempts = 0
				e.state.Sequence++
				job.Sequence = e.state.Sequence
				job.NextAttempt = followupDue
				job.PendingSince = now
				if job.NextAttempt.IsZero() || job.NextAttempt.Before(now) {
					job.NextAttempt = now
				}
			}
			job.Followup = false
			e.state.Jobs[id] = job
			if err := e.persistLocked(); err != nil {
				e.persistenceError = "Unable to persist job result; restart will reconcile safely."
			} else {
				e.persistenceError = ""
			}
			e.mu.Unlock()
			if e.ctx.Err() != nil {
				return
			}
		}
	}
}

func (e *Engine) poll() {
	ticker := time.NewTicker(e.options.TickInterval)
	defer ticker.Stop()
	previous := e.now()
	nextWatchRepair := previous.Add(30 * time.Second)
	nextPolicy := previous.Add(5 * time.Second)
	for {
		select {
		case <-e.ctx.Done():
			return
		case <-ticker.C:
		}
		now := e.now()
		if !now.Before(nextPolicy) {
			e.RefreshPolicy(e.ctx)
			nextPolicy = now.Add(5 * time.Second)
		}
		wake := now.Sub(previous) > max(2*time.Second, 5*e.options.TickInterval)
		previous = now
		e.mu.Lock()
		due := []Source{}
		for id, source := range e.sources {
			job := e.state.Jobs[id]
			if wake || ((job.State == "idle" || job.State == "exhausted") && !job.NextPoll.After(now)) {
				due = append(due, source)
			}
		}
		e.mu.Unlock()
		for _, source := range due {
			reason := "scheduled_poll"
			if source.Mode == "direct" {
				reason = "lost_event_reconcile"
			}
			if wake {
				reason = "wake_reconcile"
			}
			_ = e.Request(source.ID, reason)
		}
		if !now.Before(nextWatchRepair) {
			e.repairWatch()
			nextWatchRepair = now.Add(30 * time.Second)
		}
		e.signal()
	}
}

func (e *Engine) repairWatch() {
	e.mu.Lock()
	watch := e.watch
	e.mu.Unlock()
	if watch != nil {
		select {
		case <-watch.finished:
		default:
			return
		}
	}
	if e.ctx.Err() != nil {
		return
	}
	if watch != nil {
		watch.Close()
	}
	next, err := newWatchSet(e, e.sources)
	if err != nil {
		e.setWatchError("OS watch restart failed; periodic reconciliation remains active.")
		return
	}
	e.mu.Lock()
	closed := e.closed || e.ctx.Err() != nil
	if !closed {
		e.watch = next
	}
	e.mu.Unlock()
	if closed && next != nil {
		next.Close()
	}
}

func (e *Engine) Close() {
	e.mu.Lock()
	if !e.closed {
		e.closed = true
		if e.cancel != nil {
			e.cancel()
		}
	}
	started := e.started
	watch := e.watch
	e.mu.Unlock()
	if watch != nil {
		watch.Close()
	}
	if started {
		<-e.done
	}
}
