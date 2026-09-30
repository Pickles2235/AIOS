package store

import (
	"context"
	"strings"
	"testing"
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
