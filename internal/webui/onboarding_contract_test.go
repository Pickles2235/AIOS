package webui

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/AdamNi-7080/AIOS/internal/adapter"
	"github.com/AdamNi-7080/AIOS/internal/app"
	"github.com/AdamNi-7080/AIOS/internal/catalog"
	"github.com/AdamNi-7080/AIOS/internal/store"
)

func TestNamedOriginReplacementMigratesCapabilityAndExplicitLossRetry(t *testing.T) {
	s := server(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	defer s.Close()
	go s.Serve(ctx)
	jar, _ := cookiejar.New(nil)
	// Dial the actual server's loopback socket. Native DNS itself is covered by
	// the Darwin resolver test; this portable test exercises real HTTP origins.
	transport := &http.Transport{DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, "tcp", s.listener.Addr().String())
	}}
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport, Jar: jar, Timeout: 10 * time.Second}
	call := func(origin, path string, body any, csrf string, expected int) map[string]any {
		t.Helper()
		b, _ := json.Marshal(body)
		r, _ := http.NewRequest("POST", origin+path, strings.NewReader(string(b)))
		r.Header.Set("Origin", origin)
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("X-CSRF-Token", csrf)
		response, e := client.Do(r)
		if e != nil {
			t.Fatal(e)
		}
		defer response.Body.Close()
		if response.StatusCode != expected {
			t.Fatalf("origin transition %s %d", path, response.StatusCode)
		}
		var result map[string]any
		if json.NewDecoder(response.Body).Decode(&result) != nil {
			t.Fatal("invalid transition response")
		}
		return result
	}
	login := func(link string) (string, string) {
		t.Helper()
		parsed, e := url.Parse(link)
		if e != nil {
			t.Fatal(e)
		}
		origin := parsed.Scheme + "://" + parsed.Host
		token := strings.TrimPrefix(parsed.Fragment, "token=")
		result := call(origin, "/api/v1/session", map[string]string{"token": token}, "", 200)
		return origin, result["csrf_token"].(string)
	}
	origin, csrf := login(s.URL())
	name := fmt.Sprintf("aios-transition-%d-%d", os.Getpid(), time.Now().UnixNano())
	chosen := call(origin, "/api/v1/namespace", map[string]string{"namespace": name}, csrf, 200)
	oldOrigin, oldCSRF := login(chosen["launch_url"].(string))
	changed := call(oldOrigin, "/api/v1/namespace", map[string]string{"namespace": name + "-next"}, oldCSRF, 200)
	newOrigin, newCSRF := login(changed["launch_url"].(string))
	call(oldOrigin, "/api/v1/namespace", map[string]string{"namespace": name}, oldCSRF, 403)
	if !strings.Contains(newOrigin, name+"-next.") {
		t.Fatal("replacement link did not carry new origin")
	}
	// Registration loss is simulated by explicitly releasing this owned lease.
	// The user's saved choice remains and can be selected again without rename.
	s.namespaceMu.Lock()
	s.namespaceLease.Close()
	s.namespaceMu.Unlock()
	_, port, _ := net.SplitHostPort(s.listener.Addr().String())
	recovery := "http://localhost:" + port
	// Named cookie is not valid for localhost: use a fresh recovery capability.
	recoveryLink, _ := url.Parse(s.FreshURL())
	recoveryLink.Host = "localhost:" + port
	recovery, newCSRF = login(recoveryLink.String())
	retried := call(recovery, "/api/v1/namespace", map[string]string{"namespace": name + "-next"}, newCSRF, 200)
	if retried["namespace"] != name+"-next" || retried["active"] != true {
		t.Fatal("same-name recovery collided with its own stale lease")
	}
}

func approvedRequest(t *testing.T, s *Server, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	b, _ := json.Marshal(body)
	method := "POST"
	if body == nil {
		method = "GET"
	}
	r := httptest.NewRequest(method, path, strings.NewReader(string(b)))
	r.Host = s.listener.Addr().String()
	r.Header.Set("Origin", s.origin)
	r.Header.Set("X-CSRF-Token", s.csrf)
	r.AddCookie(&http.Cookie{Name: "aios_kb_session", Value: s.session})
	w := httptest.NewRecorder()
	s.api(w, r)
	return w
}
func sourceFixture(t *testing.T) string {
	t.Helper()
	root, e := filepath.EvalSymlinks(t.TempDir())
	if e != nil {
		t.Fatal(e)
	}
	for name, body := range map[string]string{"Publish.java": "class Publish {}\n", "Ignore.java": "class Ignore {}\n", "package.json": "{}\n"} {
		if e = os.WriteFile(filepath.Join(root, name), []byte(body), 0600); e != nil {
			t.Fatal(e)
		}
	}
	for _, args := range [][]string{{"init", "-q", "--initial-branch=main"}, {"add", "."}, {"-c", "user.name=fixture", "-c", "user.email=fixture@example.test", "commit", "-qm", "fixture"}} {
		b, e := exec.Command("git", append([]string{"-C", root}, args...)...).CombinedOutput()
		if e != nil {
			t.Fatalf("fixture: %v %s", e, b)
		}
	}
	return root
}
func TestPreviewScopePartialBatchAndExclusiveApproval(t *testing.T) {
	s := server(t)
	t.Cleanup(func() {
		_ = filepath.WalkDir(s.dataDir, func(path string, d os.DirEntry, e error) error {
			if e == nil && d.IsDir() {
				return os.Chmod(path, 0700)
			}
			return e
		})
	})
	defer s.Close()
	s.session = "fixture"
	s.csrf = "csrf"
	source := sourceFixture(t)
	body := map[string]any{"mode": "local", "local_repositories": []adapter.LocalRepository{{ID: "fixture", Path: source}}, "rules": map[string]ScopeRules{"fixture": {Exclude: []string{"Ignore.java"}}}}
	w := approvedRequest(t, s, "/api/v1/onboarding/preview", body)
	if w.Code != 200 {
		t.Fatalf("preview %d: %s", w.Code, w.Body)
	}
	var p struct {
		Valid        bool                `json:"valid"`
		Repositories []PreviewRepository `json:"repositories"`
	}
	if json.Unmarshal(w.Body.Bytes(), &p) != nil || !p.Valid || len(p.Repositories) != 1 || !p.Repositories[0].Valid || p.Repositories[0].Exclusions["catalog_pattern"] != 1 || len(p.Repositories[0].Languages) == 0 {
		t.Fatalf("actual scope missing: %s", w.Body)
	}
	if s.setup.State != "unconfigured" {
		t.Fatal("preview saved approval")
	}
	body["local_repositories"] = []adapter.LocalRepository{{ID: "fixture", Path: source}, {ID: "missing", Path: filepath.Join(source, "absent")}}
	w = approvedRequest(t, s, "/api/v1/onboarding/preview", body)
	if w.Code != 200 {
		t.Fatal(w.Code)
	}
	if json.Unmarshal(w.Body.Bytes(), &p) != nil || p.Valid || !p.Repositories[0].Valid || p.Repositories[1].Valid {
		t.Fatal("partial batch dishonestly validated")
	}
	body["repositories"] = []map[string]string{{"id": "other", "url": source, "ref": "refs/heads/main"}}
	if approvedRequest(t, s, "/api/v1/onboarding/configure", body).Code != 400 || s.setup.State != "unconfigured" {
		t.Fatal("mixed modes saved")
	}
	delete(body, "repositories")
	body["local_repositories"] = []adapter.LocalRepository{{ID: "fixture", Path: source}}
	if approvedRequest(t, s, "/api/v1/onboarding/configure", body).Code != 200 {
		t.Fatal("valid rules rejected")
	}
	if s.setup.Rules["fixture"].Exclude[0] != "Ignore.java" {
		t.Fatal("approved rules not saved")
	}
	if approvedRequest(t, s, "/api/v1/onboarding/start", struct{}{}).Code != 202 {
		t.Fatal("build not accepted")
	}
	deadline := time.Now().Add(20 * time.Second)
	for {
		s.mu.Lock()
		state := s.setup.State
		s.mu.Unlock()
		if state == "ready" {
			break
		}
		if state == "failed" || time.Now().After(deadline) {
			t.Fatalf("build %s", state)
		}
		time.Sleep(10 * time.Millisecond)
	}
	activity := approvedRequest(t, s, "/api/v1/activity", nil)
	var events struct {
		Events         []ActivityEvent `json:"events"`
		Sequence       uint64          `json:"sequence"`
		OldestSequence uint64          `json:"oldest_sequence"`
		StreamID       string          `json:"stream_id"`
	}
	if json.Unmarshal(activity.Body.Bytes(), &events) != nil {
		t.Fatal("bad activity")
	}
	if events.StreamID == "" || events.Sequence != 3 || events.OldestSequence != 1 {
		t.Fatalf("activity stream cursor or retention boundary missing: %+v", events)
	}
	stages := []string{}
	for i, event := range events.Events {
		stages = append(stages, event.Stage)
		if event.Queryable != (event.Stage == "activated") || event.Sequence != uint64(i+1) {
			t.Fatal("staged evidence claimed queryable or event sequence lost")
		}
	}
	if strings.Join(stages, ",") != "discovered,staged,activated" {
		t.Fatalf("actual boundaries %v", stages)
	}
	for i := 0; i < 513; i++ {
		s.recordProgress(app.Progress{Stage: "discovered", Repository: "fixture"})
	}
	activity = approvedRequest(t, s, "/api/v1/activity", nil)
	if json.Unmarshal(activity.Body.Bytes(), &events) != nil || events.StreamID == "" || events.Sequence != 516 || events.OldestSequence != 5 || len(events.Events) != 512 {
		t.Fatal("activity retention boundary or stream identity incorrect")
	}
	q := approvedRequest(t, s, "/api/v1/query", map[string]any{"text": "Ignore", "repository": "fixture"})
	if strings.Contains(q.Body.String(), `"label":"Ignore"`) {
		t.Fatal("excluded source queried")
	}
	b, _ := exec.Command("git", "-C", source, "status", "--porcelain=v1").Output()
	if len(b) != 0 {
		t.Fatal("source modified")
	}
}
func TestNamespaceCollisionSelectedAlternativePersistenceAndHostGuard(t *testing.T) {
	s, other := server(t), server(t)
	defer s.Close()
	defer other.Close()
	s.session = "one"
	s.csrf = "csrf"
	other.session = "two"
	other.csrf = "csrf"
	name := fmt.Sprintf("aios-web-%d-%d", os.Getpid(), time.Now().UnixNano())
	if w := approvedRequest(t, s, "/api/v1/namespace", map[string]string{"namespace": name}); w.Code != 200 {
		t.Fatalf("select %d %s", w.Code, w.Body)
	}
	w := approvedRequest(t, other, "/api/v1/namespace", map[string]string{"namespace": name})
	var collision struct {
		Suggestions       []string `json:"suggestions"`
		RequiresSelection bool     `json:"requires_selection"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &collision)
	if w.Code != 409 || !collision.RequiresSelection || len(collision.Suggestions) == 0 || other.namespace != "" {
		t.Fatal("collision silently renamed/persisted")
	}
	choice := collision.Suggestions[0]
	if approvedRequest(t, other, "/api/v1/namespace", map[string]string{"namespace": choice}).Code != 200 || other.namespace != choice {
		t.Fatal("explicit alternative rejected")
	}
	r := httptest.NewRequest("GET", "/api/v1/status", nil)
	r.Host = "foreign.example:" + strings.Split(s.listener.Addr().String(), ":")[1]
	r.AddCookie(&http.Cookie{Name: "aios_kb_session", Value: s.session})
	if s.authorised(r, false) {
		t.Fatal("DNS rebind Host accepted")
	}
	r.Host = s.namespaceLease.Host + ":" + strings.Split(s.listener.Addr().String(), ":")[1]
	r.Header.Set("Origin", s.origin)
	if s.authorised(r, false) {
		t.Fatal("cross-origin alias accepted")
	}
	r.Header.Set("Origin", "http://"+r.Host)
	if !s.authorised(r, false) {
		t.Fatal("selected same-origin name rejected")
	}
	s.Close()
	restarted, e := New(catalog.Config{}, s.db, nil)
	if e != nil {
		t.Fatal(e)
	}
	defer restarted.Close()
	if restarted.namespace != name || restarted.namespaceOriginLocked() == "" {
		t.Fatal("chosen namespace not restored")
	}
}
func TestLocalLogoGenerationPreservesIdentity(t *testing.T) {
	s := server(t)
	defer s.Close()
	s.session = "fixture"
	s.csrf = "csrf"
	first, e := store.LoadInstance(s.dataDir)
	if e != nil {
		t.Fatal(e)
	}
	w := approvedRequest(t, s, "/api/v1/instance/logo/generate", map[string]string{"seed": "reviewed"})
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body)
	}
	a, e := store.LoadInstance(s.dataDir)
	if e != nil || a.ID != first.ID || !strings.HasPrefix(a.Logo, "data:image/png;base64,") {
		t.Fatal("generated logo/identity not persisted")
	}
	if _, e = store.NormalizeInstance(a.InstanceSettings); e != nil {
		t.Fatal(e)
	}
	approvedRequest(t, s, "/api/v1/instance/logo/generate", map[string]string{"seed": "reviewed"})
	b, _ := store.LoadInstance(s.dataDir)
	if a.Logo != b.Logo {
		t.Fatal("local generation not deterministic")
	}
}
