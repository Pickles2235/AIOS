// Package reconcile keeps catalogued source snapshots current. Polling is only
// a speed mechanism: each pass is a full deterministic reconciliation.
package reconcile

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/AdamNi-7080/AIOS/internal/catalog"
	"github.com/AdamNi-7080/AIOS/internal/discover"
	"github.com/AdamNi-7080/AIOS/internal/model"
)

type ChangeReason string

const (
	ReasonStartup   ChangeReason = "startup"
	ReasonCreate    ChangeReason = "create"
	ReasonModify    ChangeReason = "modify"
	ReasonRename    ChangeReason = "rename"
	ReasonDelete    ChangeReason = "delete"
	ReasonUncertain ChangeReason = "uncertain"
)

// SourceState is an immutable-at-the-boundary description of one allowed root.
// Files maps catalogue-relative paths to their secure-discovery SHA-256 values.
type SourceState struct {
	RepoID string
	Files  map[string]string
}

// SourceScanner never traverses outside the explicitly catalogued roots.
type SourceScanner struct {
	Repositories []model.Repository
	Limits       model.Limits
}

func (s SourceScanner) Scan(context.Context) ([]SourceState, error) {
	states := make([]SourceState, 0, len(s.Repositories))
	for _, repo := range s.Repositories {
		files, err := discover.Files(repo, s.Limits)
		if err != nil {
			return nil, fmt.Errorf("scan %s: %w", repo.ID, err)
		}
		state := SourceState{RepoID: repo.ID, Files: make(map[string]string, len(files))}
		for _, file := range files {
			state.Files[file.Path] = file.SHA256
		}
		states = append(states, state)
	}
	sort.Slice(states, func(i, j int) bool { return states[i].RepoID < states[j].RepoID })
	return states, nil
}

type Scanner interface {
	Scan(context.Context) ([]SourceState, error)
}

type Change struct {
	RepoID  string
	Reasons []ChangeReason
}

type Config struct {
	Scanner Scanner
	// Baseline is the authoritative active indexed path-to-digest state. It
	// suppresses restart-only refreshes without suppressing real source changes.
	Baseline []SourceState
	Interval time.Duration
	Debounce time.Duration
	OnChange func(context.Context, Change) error
}

// Reconciler owns one ticker and one coalescing signal channel. It does not use
// a filesystem watcher; callers may feed watcher overflow/uncertainty into
// NotifyUncertain, but the ensuing complete scan remains authoritative.
type Reconciler struct {
	scanner   Scanner
	interval  time.Duration
	debounce  time.Duration
	onChange  func(context.Context, Change) error
	uncertain chan struct{}
	mu        sync.Mutex
	previous  map[string]SourceState
}

func New(cfg Config) (*Reconciler, error) {
	if cfg.Scanner == nil {
		return nil, fmt.Errorf("source scanner is required")
	}
	if cfg.OnChange == nil {
		return nil, fmt.Errorf("change callback is required")
	}
	if err := validateCataloguedScanner(cfg.Scanner); err != nil {
		return nil, err
	}
	if cfg.Interval <= 0 {
		cfg.Interval = 30 * time.Second
	}
	if cfg.Debounce <= 0 {
		cfg.Debounce = 250 * time.Millisecond
	}
	previous := make(map[string]SourceState, len(cfg.Baseline))
	for _, state := range cfg.Baseline {
		if state.RepoID != "" {
			previous[state.RepoID] = cloneState(state)
		}
	}
	return &Reconciler{scanner: cfg.Scanner, interval: cfg.Interval, debounce: cfg.Debounce, onChange: cfg.OnChange,
		uncertain: make(chan struct{}, 1), previous: previous}, nil
}

// validateCataloguedScanner applies the same root validation used for the
// persisted catalog at the point a reconciler takes ownership of roots. This
// prevents a caller from constructing a SourceScanner around a relative or
// symlinked path after the catalog was loaded.
func validateCataloguedScanner(scanner Scanner) error {
	var source SourceScanner
	switch value := scanner.(type) {
	case SourceScanner:
		source = value
	case *SourceScanner:
		if value == nil {
			return fmt.Errorf("source scanner is required")
		}
		source = *value
	default:
		return nil
	}
	limits := source.Limits
	if limits == (model.Limits{}) {
		limits = catalog.Defaults()
	}
	if err := catalog.Validate(catalog.Config{Version: 1, Repositories: source.Repositories, Limits: limits}); err != nil {
		return fmt.Errorf("validate reconciler repositories: %w", err)
	}
	return nil
}

// NotifyUncertain is non-blocking and may be called by a watcher event path.
// Overflow or incomplete event data must use this method rather than guessing.
func (r *Reconciler) NotifyUncertain() {
	select {
	case r.uncertain <- struct{}{}:
	default:
	}
}

// Run performs a startup scan, then bounded interval reconciliation until ctx
// ends. Bursts are coalesced by one debounce window.
func (r *Reconciler) Run(ctx context.Context) error {
	if err := r.reconcile(ctx, ReasonStartup, false); err != nil {
		return err
	}
	ticker := time.NewTicker(r.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			if err := r.reconcile(ctx, "", false); err != nil {
				return err
			}
		case <-r.uncertain:
			if !waitDebounce(ctx, r.debounce, r.uncertain) {
				return ctx.Err()
			}
			if err := r.reconcile(ctx, ReasonUncertain, true); err != nil {
				return err
			}
		}
	}
}

func waitDebounce(ctx context.Context, delay time.Duration, events <-chan struct{}) bool {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return false
		case <-events:
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
			timer.Reset(delay)
		case <-timer.C:
			return true
		}
	}
}

func (r *Reconciler) reconcile(ctx context.Context, forced ChangeReason, all bool) error {
	states, err := r.scanner.Scan(ctx)
	if err != nil {
		return err
	}
	next := make(map[string]SourceState, len(states))
	for _, state := range states {
		next[state.RepoID] = cloneState(state)
	}
	r.mu.Lock()
	previous := r.previous
	r.previous = next
	r.mu.Unlock()
	for _, state := range states {
		var reasons []ChangeReason
		if forced == ReasonStartup && previous[state.RepoID].Files == nil {
			reasons = []ChangeReason{ReasonStartup}
		} else if all {
			reasons = []ChangeReason{forced}
		} else {
			reasons = changed(previous[state.RepoID], state)
		}
		if len(reasons) > 0 {
			if err := r.onChange(ctx, Change{RepoID: state.RepoID, Reasons: reasons}); err != nil {
				return err
			}
		}
	}
	return nil
}

func cloneState(state SourceState) SourceState {
	files := make(map[string]string, len(state.Files))
	for path, hash := range state.Files {
		files[path] = hash
	}
	return SourceState{RepoID: state.RepoID, Files: files}
}

func changed(before, after SourceState) []ChangeReason {
	if before.Files == nil {
		return []ChangeReason{ReasonCreate}
	}
	added, removed := map[string]string{}, map[string]string{}
	reasons := map[ChangeReason]bool{}
	for path, hash := range after.Files {
		if old, ok := before.Files[path]; !ok {
			added[path] = hash
		} else if old != hash {
			reasons[ReasonModify] = true
		}
	}
	for path, hash := range before.Files {
		if _, ok := after.Files[path]; !ok {
			removed[path] = hash
		}
	}
	for addPath, addHash := range added {
		for removePath, removeHash := range removed {
			if addHash == removeHash {
				reasons[ReasonRename] = true
				delete(added, addPath)
				delete(removed, removePath)
				break
			}
		}
	}
	if len(added) > 0 {
		reasons[ReasonCreate] = true
	}
	if len(removed) > 0 {
		reasons[ReasonDelete] = true
	}
	result := make([]ChangeReason, 0, len(reasons))
	for reason := range reasons {
		result = append(result, reason)
	}
	sort.Slice(result, func(i, j int) bool { return result[i] < result[j] })
	return result
}
