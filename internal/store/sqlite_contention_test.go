package store

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"testing"
	"time"
)

func TestMaintenanceWriterWaitsForShortReadLease(t *testing.T) {
	root := t.TempDir()
	if err := os.Chmod(root, 0700); err != nil {
		t.Fatal(err)
	}
	w, err := OpenWriter(root)
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()
	r, err := OpenReadOnly(root)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	tx, err := r.db.BeginTx(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	var value string
	if err = tx.QueryRow("SELECT value FROM schema_metadata WHERE key='format'").Scan(&value); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { _, err := w.db.Exec("UPDATE schema_metadata SET value=value WHERE key='format'"); done <- err }()
	time.Sleep(100 * time.Millisecond)
	if err = tx.Rollback(); err != nil {
		t.Fatal(err)
	}
	if err = <-done; err != nil {
		t.Fatalf("short canonical reader prevented maintained write: %v", err)
	}
}

func TestMaintenanceContentionCancellationIsBounded(t *testing.T) {
	root := t.TempDir()
	if err := os.Chmod(root, 0700); err != nil {
		t.Fatal(err)
	}
	w, err := OpenWriter(root)
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()
	r, err := OpenReadOnly(root)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	tx, err := r.db.BeginTx(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	var value string
	if err = tx.QueryRow("SELECT value FROM schema_metadata WHERE key='format'").Scan(&value); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	start := time.Now()
	_, err = w.db.ExecContext(ctx, "UPDATE schema_metadata SET value=value WHERE key='format'")
	if !errors.Is(err, context.DeadlineExceeded) || time.Since(start) > 6*time.Second {
		t.Fatalf("contention cancellation was not bounded: %v %s", err, time.Since(start))
	}
	if err = tx.Rollback(); err != nil {
		t.Fatal(err)
	}
	if _, err = w.db.Exec("UPDATE schema_metadata SET value=value WHERE key='format'"); err != nil {
		t.Fatalf("cancelled contention poisoned next write: %v", err)
	}
}

func TestBusyTimeoutAppliesToEveryReadConnection(t *testing.T) {
	root := t.TempDir()
	if err := os.Chmod(root, 0700); err != nil {
		t.Fatal(err)
	}
	w, err := OpenWriter(root)
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()
	r, err := OpenReadOnly(root)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	first, err := r.db.Conn(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer first.Close()
	second, err := r.db.Conn(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer second.Close()
	for _, conn := range []*sql.Conn{first, second} {
		var got int
		if err = conn.QueryRowContext(context.Background(), "PRAGMA busy_timeout").Scan(&got); err != nil || got != 5000 {
			t.Fatalf("connection timeout: %d %v", got, err)
		}
	}
}
