package reconcile

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/AdamNi-7080/AIOS/internal/model"
)

func TestSourceScannerUsesSecureCatalogueDiscovery(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "safe.txt", "safe")
	writeFile(t, root, ".env", "must-not-index")
	scanner := SourceScanner{Repositories: []model.Repository{{ID: "only", Root: root}}, Limits: testLimits()}
	states, err := scanner.Scan(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(states) != 1 || states[0].RepoID != "only" || len(states[0].Files) != 1 || states[0].Files["safe.txt"] == "" {
		t.Fatalf("secure scanner states = %#v", states)
	}
}

func TestChangedClassifiesCreateModifyRenameAndDelete(t *testing.T) {
	cases := []struct {
		name          string
		before, after map[string]string
		want          []ChangeReason
	}{
		{"create", map[string]string{}, map[string]string{"new": "a"}, []ChangeReason{ReasonCreate}},
		{"modify", map[string]string{"same": "a"}, map[string]string{"same": "b"}, []ChangeReason{ReasonModify}},
		{"rename", map[string]string{"old": "a"}, map[string]string{"new": "a"}, []ChangeReason{ReasonRename}},
		{"delete", map[string]string{"gone": "a"}, map[string]string{}, []ChangeReason{ReasonDelete}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := changed(SourceState{RepoID: "repo", Files: tc.before}, SourceState{RepoID: "repo", Files: tc.after})
			if !sameReasons(got, tc.want) {
				t.Fatalf("reasons = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestReconcilerStartupAndOfflineModification(t *testing.T) {
	scanner := &memoryScanner{states: []SourceState{{RepoID: "repo", Files: map[string]string{"a": "one"}}}}
	changes := make(chan Change, 4)
	r, err := New(Config{Scanner: scanner, Interval: 10 * time.Millisecond, Debounce: time.Millisecond,
		OnChange: func(_ context.Context, change Change) error { changes <- change; return nil }})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- r.Run(ctx) }()
	assertChange(t, <-changes, "repo", []ChangeReason{ReasonStartup})
	scanner.set([]SourceState{{RepoID: "repo", Files: map[string]string{"a": "two"}}})
	assertChange(t, <-changes, "repo", []ChangeReason{ReasonModify})
	cancel()
	if err := <-done; err != context.Canceled {
		t.Fatalf("run error = %v", err)
	}
}

func TestReconcilerBaselineSuppressesUnchangedRestart(t *testing.T) {
	state := SourceState{RepoID: "repo", Files: map[string]string{"a": "one"}}
	scanner := &memoryScanner{states: []SourceState{state}}
	changes := make(chan Change, 1)
	r, err := New(Config{Scanner: scanner, Baseline: []SourceState{state}, Interval: time.Hour, Debounce: time.Millisecond, OnChange: func(_ context.Context, change Change) error { changes <- change; return nil }})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- r.Run(ctx) }()
	select {
	case change := <-changes:
		t.Fatalf("unchanged restart refreshed: %#v", change)
	case <-time.After(40 * time.Millisecond):
	}
	cancel()
	if err := <-done; err != context.Canceled {
		t.Fatalf("run error = %v", err)
	}
}

func TestReconcilerBaselineEmitsOneOfflineModificationAtStartup(t *testing.T) {
	scanner := &memoryScanner{states: []SourceState{{RepoID: "repo", Files: map[string]string{"a": "two"}}}}
	changes := make(chan Change, 2)
	r, err := New(Config{
		Scanner:  scanner,
		Baseline: []SourceState{{RepoID: "repo", Files: map[string]string{"a": "one"}}},
		Interval: time.Hour,
		Debounce: time.Millisecond,
		OnChange: func(_ context.Context, change Change) error { changes <- change; return nil },
	})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- r.Run(ctx) }()
	assertChange(t, <-changes, "repo", []ChangeReason{ReasonModify})
	select {
	case extra := <-changes:
		t.Fatalf("offline change emitted twice: %#v", extra)
	case <-time.After(20 * time.Millisecond):
	}
	cancel()
	if err := <-done; err != context.Canceled {
		t.Fatalf("run error = %v", err)
	}
}

func TestNewRejectsNonCanonicalOrSymlinkedSourceScannerRoots(t *testing.T) {
	real := t.TempDir()
	link := filepath.Join(t.TempDir(), "repo")
	if err := os.Symlink(real, link); err != nil {
		t.Fatal(err)
	}
	for _, root := range []string{"relative", link} {
		if _, err := New(Config{Scanner: SourceScanner{Repositories: []model.Repository{{ID: "repo", Root: root}}, Limits: model.Limits{MaxFileBytes: 1, MaxFilesPerRepo: 1, MaxTotalBytesPerRepo: 1, MaxResults: 1}}, OnChange: func(context.Context, Change) error { return nil }}); err == nil {
			t.Fatalf("New accepted unsafe root %q", root)
		}
	}
}

func TestReconcilerCoalescesUncertainBurstsIntoFullReconciliation(t *testing.T) {
	scanner := &memoryScanner{states: []SourceState{{RepoID: "repo", Files: map[string]string{"a": "one"}}}}
	changes := make(chan Change, 4)
	r, err := New(Config{Scanner: scanner, Interval: time.Hour, Debounce: 15 * time.Millisecond,
		OnChange: func(_ context.Context, change Change) error { changes <- change; return nil }})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- r.Run(ctx) }()
	assertChange(t, <-changes, "repo", []ChangeReason{ReasonStartup})
	for range 20 {
		r.NotifyUncertain()
	}
	assertChange(t, <-changes, "repo", []ChangeReason{ReasonUncertain})
	select {
	case extra := <-changes:
		t.Fatalf("burst was not coalesced: %#v", extra)
	case <-time.After(50 * time.Millisecond):
	}
}

func TestReconcilerNotifyUncertainIsRaceSafe(t *testing.T) {
	scanner := &memoryScanner{states: []SourceState{{RepoID: "repo", Files: map[string]string{}}}}
	r, err := New(Config{Scanner: scanner, Interval: time.Hour, Debounce: time.Millisecond, OnChange: func(context.Context, Change) error { return nil }})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- r.Run(ctx) }()
	for i := 0; i < 16; i++ {
		go r.NotifyUncertain()
	}
	time.Sleep(20 * time.Millisecond)
	cancel()
	if err := <-done; err != context.Canceled {
		t.Fatalf("run error = %v", err)
	}
}

type memoryScanner struct {
	mu     sync.Mutex
	states []SourceState
}

func (s *memoryScanner) Scan(context.Context) ([]SourceState, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	result := make([]SourceState, len(s.states))
	for i, state := range s.states {
		result[i] = cloneState(state)
	}
	return result, nil
}
func (s *memoryScanner) set(states []SourceState) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.states = states
}

func assertChange(t *testing.T, got Change, repo string, reasons []ChangeReason) {
	t.Helper()
	if got.RepoID != repo || !sameReasons(got.Reasons, reasons) {
		t.Fatalf("change = %#v, want repo %q reasons %v", got, repo, reasons)
	}
}
func sameReasons(got, want []ChangeReason) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}
func testLimits() model.Limits {
	return model.Limits{MaxFilesPerRepo: 20, MaxFileBytes: 1024, MaxTotalBytesPerRepo: 4096, MaxResults: 20}
}
func writeFile(t *testing.T, root, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(root, name), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}
