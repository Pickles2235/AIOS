package webui

import (
	"context"
	"crypto/sha256"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/AdamNi-7080/AIOS/internal/store"
)

func TestUpgradeHealthServesAuthenticatedReadsWithoutRestorationOrWrites(t *testing.T) {
	original, _, _ := managementServer(t, "local")
	data := original.dataDir
	if e := original.Close(); e != nil {
		t.Fatal(e)
	}
	// Normal startup would inspect these durable controls. Health must leave
	// them untouched, and must never start their removals or namespace services.
	for _, name := range []string{removalJournal, "namespace.json"} {
		if e := os.WriteFile(filepath.Join(data, name), []byte("invalid pending control"), 0600); e != nil {
			t.Fatal(e)
		}
	}
	snapshot := func() map[string][32]byte {
		out := map[string][32]byte{}
		e := filepath.WalkDir(data, func(path string, d os.DirEntry, e error) error {
			if e != nil {
				return e
			}
			if d.IsDir() {
				return nil
			}
			info, e := d.Info()
			if e != nil {
				return e
			}
			if !info.Mode().IsRegular() {
				return nil
			}
			b, e := os.ReadFile(path)
			if e == nil {
				out[path] = sha256.Sum256(b)
			}
			return e
		})
		if e != nil {
			t.Fatal(e)
		}
		return out
	}
	before := snapshot()
	ro, e := store.OpenReadOnly(data)
	if e != nil {
		t.Fatal(e)
	}
	defer ro.Close()
	health, e := NewUpgradeHealth(ro)
	if e != nil {
		t.Fatal(e)
	}
	defer health.Close()
	health.session = "session"
	health.csrf = "csrf"
	for _, path := range []string{"/api/v1/instance", "/api/v1/status", "/api/v1/projection?repo=one"} {
		w := approvedRequest(t, health, path, nil)
		if w.Code != http.StatusOK {
			t.Fatalf("health read %s: %d %s", path, w.Code, w.Body)
		}
	}
	for _, path := range []string{"/api/v1/instance", "/api/v1/onboarding/configure", "/api/v1/onboarding/start", "/api/v1/repositories/remove", "/api/v1/repositories/rebuild", "/api/v1/namespace", "/api/v1/daemon/stop"} {
		w := approvedRequest(t, health, path, map[string]string{"repo_id": "one"})
		if w.Code != http.StatusServiceUnavailable {
			t.Fatalf("health mutation %s: %d %s", path, w.Code, w.Body)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- health.Serve(ctx) }()
	client := &http.Client{Timeout: 5 * time.Second}
	r, e := http.NewRequest(http.MethodGet, health.origin+"/api/v1/status", nil)
	if e != nil {
		t.Fatal(e)
	}
	r.AddCookie(&http.Cookie{Name: "aios_kb_session", Value: "session"})
	resp, e := client.Do(r)
	if e != nil {
		cancel()
		t.Fatal(e)
	}
	resp.Body.Close()
	if resp.StatusCode != 200 {
		cancel()
		t.Fatal("live read rejected", resp.StatusCode)
	}
	cancel()
	if e = <-done; e != nil {
		t.Fatal(e)
	}
	if health.maintainer != nil || health.jobDone != nil || health.namespaceLease != nil {
		t.Fatal("health started background or namespace workers")
	}
	if after := snapshot(); !reflect.DeepEqual(before, after) {
		t.Fatal("read-only health changed owned files")
	}
}
