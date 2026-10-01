package store

import (
	"context"
	"github.com/AdamNi-7080/AIOS/internal/model"
	"strings"
	"testing"
	"time"
)

func TestRevisionSelectionIsDurableAndIdempotent(t *testing.T) {
	db, err := OpenWriter(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ctx := context.Background()
	if _, err = db.Discover(ctx, "repo", "one", "manifest"); err != nil {
		t.Fatal(err)
	}
	first, err := db.SelectRevision(ctx, "repo", "one", "manifest")
	if err != nil {
		t.Fatal(err)
	}
	second, err := db.SelectRevision(ctx, "repo", "one", "manifest")
	if err != nil {
		t.Fatal(err)
	}
	if first.Attempt != 1 || second.Attempt != 1 || second.PendingRevision != "one" {
		t.Fatalf("first=%#v second=%#v", first, second)
	}
	var n int
	if err = db.DB().QueryRow(`SELECT count(*) FROM ingestion_events WHERE event_type=?`, EventRevisionSelected).Scan(&n); err != nil || n != 1 {
		t.Fatalf("events=%d err=%v", n, err)
	}
	status, err := db.IngestionStatus(ctx, "repo")
	if err != nil || len(status) != 1 || status[0].State != "selected" {
		t.Fatalf("status=%#v err=%v", status, err)
	}
}
func TestRevisionSelectionRejectsOutOfOrder(t *testing.T) {
	db, err := OpenWriter(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err = db.SelectRevision(context.Background(), "repo", "one", "manifest"); err != nil {
		t.Fatal(err)
	}
	_, err = db.SelectRevision(context.Background(), "repo", "two", "manifest")
	if err == nil || !strings.Contains(err.Error(), "out-of-order") {
		t.Fatalf("err=%v", err)
	}
}

func TestSelectedQueueCompletionFailureRollsBackWholeCatalogActivation(t *testing.T) {
	db, err := OpenWriter(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	old := canonicalFixture(t, db, "old")
	ctx := context.Background()
	if _, err = db.SelectRepairRevision(ctx, "repo", "new", "manifest"); err != nil {
		t.Fatal(err)
	}
	delta, err := db.RecordSourceDelta(ctx, "repo", "old", "new", "manifest", "fixture", nil)
	if err != nil {
		t.Fatal(err)
	}
	file := model.File{RepoID: "repo", Path: "facts.txt", SHA256: "new", Size: 3, Content: "new", Language: "documentation"}
	g, err := db.StageGeneration(ctx, model.Snapshot{RepoID: "repo", Root: "/owned", Git: model.GitState{Commit: "new"}, ContentHash: "new", FileCount: 1, TotalBytes: 3, IndexedAt: time.Now(), ExtractorVersions: "fixture"}, []model.File{file}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.DB().Exec(`CREATE TRIGGER queue_completion_fault BEFORE UPDATE ON ingestion_queues WHEN NEW.state='completed' BEGIN SELECT RAISE(ABORT,'fixture queue completion fault'); END`); err != nil {
		t.Fatal(err)
	}
	if err = db.ActivateCatalogWithSelections(ctx, []string{g.ID}, []ActivationSelection{{Repository: "repo", Revision: "new", Fingerprint: "manifest", Delta: delta}}); err == nil {
		t.Fatal("queue completion failure reported activation success")
	}
	active, err := db.ActiveGeneration(ctx, "repo")
	if err != nil || active.ID != old.ID {
		t.Fatal("completion failure retracted last-good knowledge")
	}
	var target, activation string
	if err = db.DB().QueryRow(`SELECT target_generation_id,activated_at FROM ir_delta_records WHERE delta_id=?`, delta).Scan(&target, &activation); err != nil || target != "" || activation != "" {
		t.Fatal("completion failure partially activated delta")
	}
	queue, err := db.IngestionStatus(ctx, "repo")
	if err != nil || queue[0].PendingRevision != "new" || queue[0].CurrentRevision != "old" {
		t.Fatal("completion failure advanced selected queue")
	}
}
