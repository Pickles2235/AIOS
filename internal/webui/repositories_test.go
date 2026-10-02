package webui

import (
	"context"
	"encoding/json"
	"net/http"
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
	"github.com/AdamNi-7080/AIOS/internal/knowledge"
	"github.com/AdamNi-7080/AIOS/internal/maintenance"
	"github.com/AdamNi-7080/AIOS/internal/mirror"
	"github.com/AdamNi-7080/AIOS/internal/store"
)

func managementServer(t *testing.T, mode string) (*Server, string, string) {
	t.Helper()
	s := server(t)
	s.session = "session"
	s.csrf = "csrf"
	t.Cleanup(func() {
		s.Close()
		_ = filepath.WalkDir(s.dataDir, func(path string, d os.DirEntry, e error) error {
			if e == nil && d.IsDir() {
				return os.Chmod(path, 0700)
			}
			return e
		})
	})
	a, b := sourceFixture(t), sourceFixture(t)
	body := map[string]any{"mode": mode}
	if mode == "local" {
		body["local_repositories"] = []adapter.LocalRepository{{ID: "one", Path: a}, {ID: "two", Path: b}}
	} else {
		body["repositories"] = []mirror.Repository{{ID: "one", URL: a, Ref: "refs/heads/main"}, {ID: "two", URL: b, Ref: "refs/heads/main"}}
	}
	if w := approvedRequest(t, s, "/api/v1/onboarding/configure", body); w.Code != 200 {
		t.Fatalf("configure %d %s", w.Code, w.Body)
	}
	if w := approvedRequest(t, s, "/api/v1/onboarding/start", struct{}{}); w.Code != 202 {
		t.Fatalf("start %d %s", w.Code, w.Body)
	}
	waitManagedBuild(t, s)
	return s, a, b
}
func waitManagedBuild(t *testing.T, s *Server) {
	t.Helper()
	s.mu.Lock()
	done := s.jobDone
	s.mu.Unlock()
	if done != nil {
		select {
		case <-done:
		case <-time.After(30 * time.Second):
			t.Fatal("build did not terminate")
		}
	}
	s.mu.Lock()
	state := s.setup.State
	diagnostic := s.setup.Error
	s.mu.Unlock()
	if state != "ready" {
		t.Fatalf("build %s: %s", state, diagnostic)
	}
}
func activeID(t *testing.T, s *Server, id string) string {
	t.Helper()
	g, err := s.db.ActiveGeneration(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	return g.ID
}
func sourceStatus(t *testing.T, path string) string {
	t.Helper()
	b, e := exec.Command("git", "-C", path, "status", "--porcelain=v1").Output()
	if e != nil {
		t.Fatal(e)
	}
	return string(b)
}

func TestRepositoryForceRebuildPurgeAndRestart(t *testing.T) {
	for _, mode := range []string{"local", "mirror"} {
		t.Run(mode, func(t *testing.T) {
			s, a, b := managementServer(t, mode)
			before := sourceStatus(t, a)
			identity, err := store.LoadInstance(s.dataDir)
			if err != nil {
				t.Fatal(err)
			}
			old, other := activeID(t, s, "one"), activeID(t, s, "two")
			w := approvedRequest(t, s, "/api/v1/repositories/rebuild", map[string]string{"repository": "one"})
			if w.Code != 202 {
				t.Fatalf("force %d %s", w.Code, w.Body)
			}
			waitManagedBuild(t, s)
			if activeID(t, s, "one") == old || activeID(t, s, "two") != other {
				t.Fatal("force rebuild did not recompile unchanged input or changed untouched member")
			}
			query, err := s.readService().Query(context.Background(), knowledge.Query{Text: "Publish", Repository: "one"})
			if err != nil || query.Status != "found" {
				t.Fatalf("fresh analysis missing: %v %s", err, query.Status)
			}
			oldHandle := query.Entities[0].Handle
			w = approvedRequest(t, s, "/api/v1/repositories/rebuild", struct{}{})
			if w.Code != 202 {
				t.Fatalf("full rebuild %d %s", w.Code, w.Body)
			}
			waitManagedBuild(t, s)
			if activeID(t, s, "two") == other {
				t.Fatal("whole KB did not recompile every member")
			}
			w = approvedRequest(t, s, "/api/v1/onboarding/configure", map[string]any{"mode": mode, "local_repositories": []adapter.LocalRepository{{ID: "one", Path: a}}})
			if w.Code == 200 {
				t.Fatal("configuration bypassed explicit purge")
			}
			w = approvedRequest(t, s, "/api/v1/repositories/remove", map[string]string{"repository": "one"})
			if w.Code != 200 {
				t.Fatalf("remove %d %s", w.Code, w.Body)
			}
			if _, err = s.readService().Entity(context.Background(), oldHandle); err == nil {
				t.Fatal("removed historical handle still resolves")
			}
			n, err := s.db.RepositoryOwnedRecords(context.Background(), "one")
			if err != nil || n != 0 {
				t.Fatalf("owned records remain %d %v", n, err)
			}
			for _, rel := range []string{"snapshots/one", "mirrors/one.git"} {
				if _, err = os.Lstat(filepath.Join(s.dataDir, rel)); !os.IsNotExist(err) {
					t.Fatal("owned source assets remain", rel, err)
				}
			}
			if w = approvedRequest(t, s, "/api/v1/repositories/retry", map[string]string{"repository": "one"}); w.Code != 404 {
				t.Fatal("removed repository could retry")
			}
			restart, err := New(catalog.Config{}, s.db, nil)
			if err != nil {
				t.Fatal(err)
			}
			defer restart.Close()
			if slicesContains(approvedIDs(restart.setup), "one") {
				t.Fatal("restart resurrected repository")
			}
			status, err := restart.readService().Status(context.Background())
			if err != nil || len(status.Repositories) != 1 || status.Repositories[0].ID != "two" {
				t.Fatal("survivor not restored", err)
			}
			after, err := store.LoadInstance(s.dataDir)
			if err != nil || after.ID != identity.ID {
				t.Fatal("management changed identity")
			}
			if sourceStatus(t, a) != before || sourceStatus(t, b) != "" {
				t.Fatal("management wrote source Git state")
			}
		})
	}
}
func slicesContains(ids []string, id string) bool {
	for _, x := range ids {
		if x == id {
			return true
		}
	}
	return false
}

func TestRemovalCancelsInflightBuildBeforePurge(t *testing.T) {
	s, a, _ := managementServer(t, "local")
	s.mu.Lock()
	cfg, _, err := managementConfig(s.setup)
	setup := cloneSetup(s.setup)
	s.mu.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	entered := make(chan struct{})
	ctx, cancel := context.WithCancel(context.Background())
	ctx = app.WithProgress(app.WithFullRebuild(ctx), func(p app.Progress) {
		if p.Stage == "discovered" {
			select {
			case <-entered:
			default:
				close(entered)
			}
			<-ctx.Done()
		}
	})
	s.mu.Lock()
	s.jobCancel = cancel
	s.jobDone = make(chan struct{})
	s.setup.State = "ingesting"
	s.mu.Unlock()
	go s.runSetup(ctx, cfg, mirror.Registry{}, setup.LocalRepositories)
	select {
	case <-entered:
	case <-time.After(10 * time.Second):
		t.Fatal("build did not reach actual immutable input")
	}
	response := make(chan *httptest.ResponseRecorder, 1)
	go func() {
		response <- approvedRequest(t, s, "/api/v1/repositories/remove", map[string]string{"repository": "one"})
	}()
	select {
	case w := <-response:
		if w.Code != 200 {
			t.Fatalf("remove %d %s", w.Code, w.Body)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("remove deadlocked against build completion")
	}
	if n, err := s.db.RepositoryOwnedRecords(context.Background(), "one"); err != nil || n != 0 {
		t.Fatal("inflight writer resurrected removed repository", n, err)
	}
	if sourceStatus(t, a) != "" {
		t.Fatal("cancellation modified source")
	}
}

func TestRemovalIntentRecoversFailedPhysicalPurgeAndNeverServesRemovedEvidence(t *testing.T) {
	s, a, _ := managementServer(t, "local")
	link := filepath.Join(s.dataDir, "snapshots", "one", "unsafe-link")
	if err := os.Symlink(a, link); err != nil {
		t.Fatal(err)
	}
	w := approvedRequest(t, s, "/api/v1/repositories/remove", map[string]string{"repository": "one"})
	if w.Code != 503 {
		t.Fatalf("linked purge did not require recovery %d %s", w.Code, w.Body)
	}
	if w = approvedRequest(t, s, "/api/v1/query", map[string]string{"text": "Publish"}); w.Code != 503 {
		t.Fatal("pending removal served evidence")
	}
	restart, err := New(catalog.Config{}, s.db, nil)
	if err != nil {
		t.Fatal(err)
	}
	restart.Close()
	if restart.removing != "one" || slicesContains(approvedIDs(restart.setup), "one") {
		t.Fatal("restart lost withdrawal intent")
	}
	if err = os.Remove(link); err != nil {
		t.Fatal(err)
	}
	recovered, err := New(catalog.Config{}, s.db, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer recovered.Close()
	if recovered.removing != "" {
		t.Fatal("restart did not finish actual cleanup")
	}
	if _, err = os.Stat(filepath.Join(s.dataDir, removalJournal)); !os.IsNotExist(err) {
		t.Fatal("completed removal intent remains")
	}
	if sourceStatus(t, a) != "" {
		t.Fatal("unsafe asset redirected deletion to workspace")
	}
}

func TestRepositoryControlsRejectMissingCSRFUnknownIDsAndInvalidShapes(t *testing.T) {
	s, _, _ := managementServer(t, "local")
	for _, path := range []string{"add", "remove", "rules", "retry", "rebuild"} {
		r := httptest.NewRequest(http.MethodPost, "/api/v1/repositories/"+path, strings.NewReader(`{}`))
		r.Host = s.listener.Addr().String()
		r.Header.Set("Origin", s.origin)
		r.AddCookie(&http.Cookie{Name: "aios_kb_session", Value: s.session})
		w := httptest.NewRecorder()
		s.api(w, r)
		if w.Code != 403 {
			t.Fatal("management accepted missing CSRF", path, w.Code)
		}
	}
	for _, body := range []any{map[string]string{"repository": "../one"}, map[string]string{"repository": "one", "untrusted": "path"}} {
		w := approvedRequest(t, s, "/api/v1/repositories/remove", body)
		if w.Code != 400 {
			t.Fatal("unsafe removal shape accepted", w.Code)
		}
	}
	w := approvedRequest(t, s, "/api/v1/repositories/remove", map[string]string{"repository": "missing"})
	if w.Code != 404 {
		t.Fatal("unknown ID did not fail")
	}
	w = approvedRequest(t, s, "/api/v1/repositories/purge-status?repository=missing", nil)
	var status map[string]any
	if json.Unmarshal(w.Body.Bytes(), &status) != nil || status["owned_records"] != float64(0) {
		t.Fatal("purge status invented owned data")
	}
}

func TestRepositoryAddAndScopeUseActualMaintainedBackend(t *testing.T) {
	s, _, _ := managementServer(t, "local")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	s.maintenanceMu.Lock()
	s.maintenanceContext = ctx
	s.maintenanceMu.Unlock()
	s.startMaintenance()
	third := sourceFixture(t)
	w := approvedRequest(t, s, "/api/v1/repositories/add", map[string]any{"local_repository": adapter.LocalRepository{ID: "three", Path: third}, "rules": ScopeRules{Exclude: []string{"Ignore.java"}}})
	if w.Code != 202 {
		t.Fatalf("add %d %s", w.Code, w.Body)
	}
	deadline := time.Now().Add(30 * time.Second)
	for {
		g, err := s.db.ActiveGeneration(ctx, "three")
		if err == nil && g.ID != "" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("approved member never activated")
		}
		time.Sleep(10 * time.Millisecond)
	}
	q, err := s.readService().Query(ctx, knowledge.Query{Repository: "three", Text: "Publish"})
	if err != nil || q.Status != "found" {
		t.Fatal("added source not searchable", q.Status, err)
	}
	q, err = s.readService().Query(ctx, knowledge.Query{Repository: "three", Text: "Ignore"})
	if err != nil || q.Status == "found" {
		t.Fatal("add ignored approved scope", q.Status, err)
	}
	old := activeID(t, s, "three")
	w = approvedRequest(t, s, "/api/v1/repositories/rules", map[string]any{"repository": "three", "rules": ScopeRules{Exclude: []string{"Publish.java"}}})
	if w.Code != 202 {
		t.Fatalf("scope %d %s", w.Code, w.Body)
	}
	deadline = time.Now().Add(30 * time.Second)
	for {
		generation, readErr := s.db.ActiveGeneration(ctx, "three")
		if readErr == nil && generation.ID != old {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("scope update did not replace canonical knowledge", readErr)
		}
		time.Sleep(10 * time.Millisecond)
	}
	q, err = s.readService().Query(ctx, knowledge.Query{Repository: "three", Text: "Ignore"})
	if err != nil || q.Status != "found" {
		t.Fatal("newly included source never compiled", q.Status, err)
	}
	q, err = s.readService().Query(ctx, knowledge.Query{Repository: "three", Text: "Publish"})
	if err != nil || q.Status == "found" {
		t.Fatal("scope still exposes excluded symbol", q.Status, err)
	}
	w = approvedRequest(t, s, "/api/v1/repositories/add", map[string]any{"local_repository": adapter.LocalRepository{ID: "three", Path: third}, "rules": ScopeRules{}})
	if w.Code != 400 {
		t.Fatal("duplicate approval accepted")
	}
	if w = approvedRequest(t, s, "/api/v1/repositories/retry", map[string]string{"repository": "three"}); w.Code != 202 {
		t.Fatal("retry not queued")
	}
	s.stopMaintenance()
	b, _ := os.ReadFile(filepath.Join(s.dataDir, "maintenance.json"))
	var jobs struct {
		Jobs map[string]maintenance.Job `json:"jobs"`
	}
	if json.Unmarshal(b, &jobs) != nil || jobs.Jobs["three"].Repository != "three" {
		t.Fatal("new member job not durable")
	}
}

func TestFullRebuildProjectionFailurePreservesCoherentLastGood(t *testing.T) {
	s, _, _ := managementServer(t, "local")
	old, err := s.db.ActiveCatalogRevision(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	writer, err := store.OpenWriter(s.dataDir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = writer.DB().Exec(`CREATE TRIGGER rebuild_fault BEFORE INSERT ON projection_lookup_records BEGIN SELECT RAISE(ABORT,'injected projection failure'); END`); err != nil {
		t.Fatal(err)
	}
	writer.Close()
	w := approvedRequest(t, s, "/api/v1/repositories/rebuild", struct{}{})
	if w.Code != 202 {
		t.Fatal("rebuild did not start")
	}
	s.mu.Lock()
	done := s.jobDone
	s.mu.Unlock()
	if done != nil {
		select {
		case <-done:
		case <-time.After(30 * time.Second):
			t.Fatal("failed rebuild did not end")
		}
	}
	current, err := s.db.ActiveCatalogRevision(context.Background())
	if err != nil || current.ID != old.ID {
		t.Fatal("failed rebuild published partial replacement", err)
	}
	for _, id := range []string{"one", "two"} {
		q, err := s.readService().Query(context.Background(), knowledge.Query{Repository: id, Text: "Publish"})
		if err != nil || q.Status != "found" {
			t.Fatal("last-good query unavailable", id, q.Status, err)
		}
	}
}

func TestMirrorManagementRejectsOwnedSourcesBeforeApprovalOrPurge(t *testing.T) {
	s, source, _ := managementServer(t, "mirror")
	inside := mirror.MirrorPath(s.dataDir, "one")
	before, err := os.ReadFile(filepath.Join(inside, "HEAD"))
	if err != nil {
		t.Fatal(err)
	}
	alias := filepath.Join(t.TempDir(), "owned-alias")
	if err = os.Symlink(inside, alias); err != nil {
		t.Fatal(err)
	}
	for _, remote := range []string{inside, alias, (&url.URL{Scheme: "file", Path: inside}).String(), s.dataDir} {
		r := mirror.Repository{ID: "unsafe", URL: remote, Ref: "refs/heads/main"}
		if w := approvedRequest(t, s, "/api/v1/repositories/add", map[string]any{"mirror_repository": r, "rules": ScopeRules{}}); w.Code != 400 {
			t.Fatalf("add accepted owned source %d %s", w.Code, w.Body)
		}
		if w := approvedRequest(t, s, "/api/v1/onboarding/configure", map[string]any{"mode": "mirror", "repositories": []mirror.Repository{{ID: "one", URL: remote, Ref: "refs/heads/main"}, {ID: "two", URL: remote, Ref: "refs/heads/main"}}}); w.Code != 400 {
			t.Fatal("batch configure accepted owned source", w.Code)
		}
		if w := approvedRequest(t, s, "/api/v1/onboarding/preview", map[string]any{"mode": "mirror", "repositories": []mirror.Repository{r}}); w.Code != 400 {
			t.Fatal("preview accepted owned source", w.Code)
		}
	}
	// A legacy unsafe approval must fail closed before a durable removal or
	// deletion of its source, even though old product versions allowed it.
	s.mu.Lock()
	s.setup.Repositories = append(s.setup.Repositories, mirror.Repository{ID: "legacy", URL: inside, Ref: "refs/heads/main"})
	s.mu.Unlock()
	if w := approvedRequest(t, s, "/api/v1/repositories/remove", map[string]string{"repository": "one"}); w.Code != 409 {
		t.Fatal("purge deleted a legacy approved source", w.Code)
	}
	after, err := os.ReadFile(filepath.Join(inside, "HEAD"))
	if err != nil || string(after) != string(before) {
		t.Fatal("overlap rejection mutated approved source", err)
	}
	if _, err = os.Stat(filepath.Join(s.dataDir, removalJournal)); !os.IsNotExist(err) {
		t.Fatal("rejected purge recorded intent")
	}
	if w := approvedRequest(t, s, "/api/v1/repositories/remove", map[string]string{"repository": "legacy"}); w.Code != 200 {
		t.Fatalf("cannot retire legacy dependent before provider %d %s", w.Code, w.Body)
	}
	if _, err = os.Stat(filepath.Join(inside, "HEAD")); err != nil {
		t.Fatal("retiring dependent deleted provider source", err)
	}
	// A saved legacy intent or changed alias after a crash also has an explicit
	// recovery path: keep ID/mode approvals, correct its URL, then finish purge.
	s.mu.Lock()
	s.setup.Repositories = append(s.setup.Repositories, mirror.Repository{ID: "legacy", URL: inside, Ref: "refs/heads/main"})
	err = s.persistSetup()
	s.mu.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	if err = writeRemoval(s.dataDir, "one"); err != nil {
		t.Fatal(err)
	}
	restarted, err := New(catalog.Config{}, s.db, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer restarted.Close()
	restarted.session = "session"
	restarted.csrf = "csrf"
	if restarted.removing != "one" {
		t.Fatal("unsafe pending intent lost")
	}
	restarted.mu.Lock()
	fixed := cloneSetup(restarted.setup)
	restarted.mu.Unlock()
	for i := range fixed.Repositories {
		if fixed.Repositories[i].ID == "legacy" {
			fixed.Repositories[i].URL = source
		}
	}
	w := approvedRequest(t, restarted, "/api/v1/onboarding/configure", map[string]any{"mode": "mirror", "repositories": fixed.Repositories})
	if w.Code != 200 {
		t.Fatalf("cannot correct URL during pending recovery %d %s", w.Code, w.Body)
	}
	if w = approvedRequest(t, restarted, "/api/v1/onboarding/start", struct{}{}); w.Code != 409 {
		t.Fatal("pending removal permitted ingestion")
	}
	if w = approvedRequest(t, restarted, "/api/v1/repositories/remove", map[string]string{"repository": "one"}); w.Code != 200 {
		t.Fatalf("corrected intent cannot recover %d %s", w.Code, w.Body)
	}
}
