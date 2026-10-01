package maintenance

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/AdamNi-7080/AIOS/internal/store"
)

func TestRestoreCanonicalRepairsStaleJournalAfterActualActivation(t *testing.T) {
	source, data := watchedFixture(t)
	runner := actualRunner(source, data, make(chan time.Time, 8))
	initial, err := runner(context.Background(), "repo")
	if err != nil {
		t.Fatal(err)
	}
	e, err := New(data, []Source{{ID: "repo", Mode: "direct", Path: source}}, runner, Defaults())
	if err != nil {
		t.Fatal(err)
	}
	e.mu.Lock()
	job := e.state.Jobs["repo"]
	job.ActiveGeneration = initial.Generation
	job.ActiveRevision = initial.Revision
	job.State = "exhausted"
	job.Attempts = 5
	job.Error = "Repository update failed"
	job.LastSuccess = time.Now().Add(-time.Hour)
	e.state.Jobs["repo"] = job
	err = e.persistLocked()
	e.mu.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	e.Close()
	if err = os.WriteFile(filepath.Join(source, "facts.txt"), []byte("actual durable replacement"), 0640); err != nil {
		t.Fatal(err)
	}
	activated, err := runner(context.Background(), "repo")
	if err != nil {
		t.Fatal(err)
	}
	if activated.Generation == initial.Generation {
		t.Fatal("fixture did not activate replacement")
	}
	db, err := store.OpenReadOnly(data)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	generation, err := db.ActiveGeneration(context.Background(), "repo")
	if err != nil {
		t.Fatal(err)
	}
	reopened, err := New(data, []Source{{ID: "repo", Mode: "direct", Path: source}}, runner, Defaults())
	if err != nil {
		t.Fatal(err)
	}
	if reopened.Status().Jobs[0].ActiveGeneration != initial.Generation {
		t.Fatal("fixture journal was not stale")
	}
	state := CanonicalState{Revision: activated.Revision, Generation: generation.ID, ActivatedAt: generation.ActivatedAt}
	if err = reopened.RestoreCanonical(map[string]CanonicalState{"repo": state}); err != nil {
		t.Fatal(err)
	}
	got := reopened.Status().Jobs[0]
	if got.ActiveGeneration != generation.ID || got.ActiveRevision != activated.Revision || !got.LastSuccess.Equal(generation.ActivatedAt) || got.State != "idle" || got.Attempts != 0 || got.Error != "" {
		t.Fatalf("stale journal did not recover actual canonical activation: %+v", got)
	}
	reopened.Close()
	persisted, err := New(data, []Source{{ID: "repo", Mode: "direct", Path: source}}, runner, Defaults())
	if err != nil {
		t.Fatal(err)
	}
	defer persisted.Close()
	if persisted.Status().Jobs[0].ActiveGeneration != generation.ID || persisted.Status().Jobs[0].Error != "" {
		t.Fatal("canonical restoration did not persist")
	}
}
