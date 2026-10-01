//go:build !windows

package lifecycle

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestStopWaitsForLifetimeCleanupAfterControlCloses(t *testing.T) {
	root := canonicalTemp(t)
	o := Options{Root: root, DataDir: filepath.Join(root, "data")}
	lock, err := Lock(o.DataDir)
	if err != nil {
		t.Fatal(err)
	}
	defer lock.Close()
	ctx, cancel := context.WithCancel(context.Background())
	if err = ServeControl(ctx, o, func() State { return State{Running: true, PID: os.Getpid()} }, func() string { return "" }); err != nil {
		t.Fatal(err)
	}
	cancel()
	deadline := time.Now().Add(3 * time.Second)
	for {
		if _, err = Control(o, "status"); err != nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("control did not close")
		}
		time.Sleep(10 * time.Millisecond)
	}
	// The process has stopped accepting control, but final DNS/state cleanup
	// deliberately still owns the lifetime lock. A restart must not pass yet.
	short, cancelShort := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancelShort()
	if err = waitStopped(short, o); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("early completion: %v", err)
	}
	if err = lock.Close(); err != nil {
		t.Fatal(err)
	}
	if err = waitStopped(context.Background(), o); err != nil {
		t.Fatal(err)
	}
	// The barrier releases its probe rather than preventing the next daemon.
	next, err := Lock(o.DataDir)
	if err != nil {
		t.Fatal(err)
	}
	next.Close()
}

func TestStopBarrierRejectsUnsafeLockWithoutRetry(t *testing.T) {
	root := canonicalTemp(t)
	o := Options{Root: root, DataDir: filepath.Join(root, "data")}
	if err := PrepareDir(o.DataDir); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(root, "outside")
	if err := os.WriteFile(outside, []byte("preserved"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(o.DataDir, ".daemon.lock")); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := waitStopped(ctx, o); err == nil || errors.Is(err, context.Canceled) {
		t.Fatalf("unsafe metadata retried: %v", err)
	}
	bytes, err := os.ReadFile(outside)
	if err != nil || string(bytes) != "preserved" {
		t.Fatal("outside file changed")
	}
}
