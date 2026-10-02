package webui

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/AdamNi-7080/AIOS/internal/adapter"
	"github.com/AdamNi-7080/AIOS/internal/app"
	"github.com/AdamNi-7080/AIOS/internal/catalog"
	"github.com/AdamNi-7080/AIOS/internal/discover"
	"github.com/AdamNi-7080/AIOS/internal/lifecycle"
	"github.com/AdamNi-7080/AIOS/internal/mirror"
)

type ScopeRules struct {
	Include []string `json:"include,omitempty"`
	Exclude []string `json:"exclude,omitempty"`
}

func applyScope(cfg *catalog.Config, rules map[string]ScopeRules) error {
	known := map[string]bool{}
	for i := range cfg.Sources {
		x := &cfg.Sources[i]
		known[x.ID] = true
		x.Include = append([]string(nil), rules[x.ID].Include...)
		x.Exclude = append([]string(nil), rules[x.ID].Exclude...)
	}
	for id := range rules {
		if !known[id] {
			return fmt.Errorf("scope rules reference an unconfigured repository")
		}
	}
	return catalog.Validate(*cfg)
}

type ActivityEvent struct {
	app.Progress
	Sequence  uint64    `json:"sequence"`
	At        time.Time `json:"at"`
	Queryable bool      `json:"queryable"`
}

func (s *Server) recordProgress(p app.Progress) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.activitySequence++
	s.activity = append(s.activity, ActivityEvent{Progress: p, Sequence: s.activitySequence, At: time.Now().UTC(), Queryable: p.Stage == "activated"})
	if len(s.activity) > 512 {
		s.activity = append([]ActivityEvent(nil), s.activity[len(s.activity)-512:]...)
	}
}
func (s *Server) activityAPI(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet || !s.authorised(r, false) {
		fail(w, 403, "valid session required")
		return
	}
	s.mu.Lock()
	events := append([]ActivityEvent{}, s.activity...)
	seq := s.activitySequence
	s.mu.Unlock()
	jsonBody(w, map[string]any{"events": events, "sequence": seq, "retention_events": 512, "staged_evidence_queryable": false})
}

type PreviewRepository struct {
	ID         string         `json:"id"`
	Valid      bool           `json:"valid"`
	Error      string         `json:"error,omitempty"`
	Revision   string         `json:"revision,omitempty"`
	Files      int            `json:"files"`
	Languages  []string       `json:"languages"`
	Frameworks []string       `json:"frameworks"`
	Exclusions map[string]int `json:"exclusions"`
	Scope      ScopeRules     `json:"scope"`
}

func removePreview(path string) {
	_ = filepath.WalkDir(path, func(p string, d os.DirEntry, e error) error {
		if e == nil && d.IsDir() {
			return os.Chmod(p, 0700)
		}
		return e
	})
	_ = os.RemoveAll(path)
}
func (s *Server) previewAPI(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost || !s.authorised(r, true) {
		fail(w, 403, "valid origin/session/CSRF required")
		return
	}
	var in struct {
		Mode    string                    `json:"mode"`
		Paths   []string                  `json:"paths,omitempty"`
		URLs    []string                  `json:"urls,omitempty"`
		Local   []adapter.LocalRepository `json:"local_repositories,omitempty"`
		Mirrors []mirror.Repository       `json:"repositories,omitempty"`
		Rules   map[string]ScopeRules     `json:"rules,omitempty"`
	}
	if decode(r, &in) != nil {
		fail(w, 400, "invalid preview request")
		return
	}
	if (in.Mode != "local" && in.Mode != "mirror") || (in.Mode == "local" && (len(in.URLs)+len(in.Mirrors) > 0)) || (in.Mode == "mirror" && (len(in.Paths)+len(in.Local) > 0)) || (len(in.Paths) > 0 && len(in.Local) > 0) || (len(in.URLs) > 0 && len(in.Mirrors) > 0) {
		fail(w, 400, "Choose Direct OR Mirror with one source array")
		return
	}
	for i, p := range in.Paths {
		in.Local = append(in.Local, adapter.LocalRepository{ID: fmt.Sprintf("repo-%d", i+1), Path: p})
	}
	for i, p := range in.URLs {
		in.Mirrors = append(in.Mirrors, mirror.Repository{ID: fmt.Sprintf("repo-%d", i+1), URL: p, Ref: "refs/heads/main"})
	}
	var cfg catalog.Config
	var err error
	if in.Mode == "local" {
		reg := adapter.LocalRegistry{Version: 1, Repositories: in.Local}
		err = adapter.ValidateLocalRegistry(reg)
		if err == nil {
			err = lifecycle.ValidateInstallationSourceBoundaries(approvedAssetSources(Setup{LocalRepositories: in.Local}), s.dataDir)
		}
		cfg = catalog.Config{Version: 1, Limits: catalog.Defaults()}
		for _, x := range in.Local {
			cfg.Sources = append(cfg.Sources, catalog.Source{Kind: "repository", ID: x.ID})
		}
	} else {
		cfg, _, err = setupConfig(in.Mirrors)
		if err == nil {
			err = lifecycle.ValidateInstallationSourceBoundaries(mirror.Registry{Version: 1, Repositories: in.Mirrors}, s.dataDir)
		}
	}
	if err == nil {
		err = applyScope(&cfg, in.Rules)
	}
	if err != nil {
		fail(w, 400, "Invalid batch sources or scope. Use 1–100 unique repository IDs, canonical paths/approved Git URLs and safe patterns.")
		return
	}
	temp, err := os.MkdirTemp(s.dataDir, ".preview-")
	if err != nil {
		fail(w, 500, "unable to stage scope preview")
		return
	}
	defer removePreview(temp)
	ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
	defer cancel()
	out := []PreviewRepository{}
	exclusions := map[string]int{}
	allValid := true
	for i, source := range cfg.SourceRepositories() {
		p := PreviewRepository{ID: source.ID, Languages: []string{}, Frameworks: []string{}, Exclusions: map[string]int{}, Scope: in.Rules[source.ID]}
		var snapshot adapter.Discovery
		if in.Mode == "local" {
			_, err = adapter.InspectLocal(ctx, in.Local[i], s.dataDir)
			if err == nil {
				snapshot, err = adapter.CaptureLocalScoped(ctx, in.Local[i], temp, cfg.Limits, source)
			}
		} else {
			reg := mirror.Registry{Version: 1, Repositories: []mirror.Repository{in.Mirrors[i]}}
			_, err = mirror.Sync(ctx, reg, temp)
			if err == nil {
				snapshot, err = (adapter.RepositoryGit{Registry: reg, DataDir: temp}).Discover(ctx, source.ID)
			}
		}
		if err == nil {
			source.Root = snapshot.Root
			files, coverage, e := discover.FilesWithCoverage(source, cfg.Limits)
			err = e
			if err == nil {
				coverage.Entries = append(coverage.Entries, snapshot.Coverage.Entries...)
				languages := map[string]bool{}
				for _, f := range files {
					if f.Language != "" {
						languages[f.Language] = true
					}
				}
				for _, e := range coverage.Entries {
					if e.Outcome != "included" {
						p.Exclusions[e.Reason]++
						exclusions[e.Reason]++
					}
				}
				for l := range languages {
					p.Languages = append(p.Languages, l)
				}
				sort.Strings(p.Languages)
				for _, marker := range []struct{ path, label string }{{"pom.xml", "Maven manifest"}, {"build.gradle", "Gradle manifest"}, {"build.gradle.kts", "Gradle Kotlin manifest"}, {"package.json", "Node package manifest"}, {"tsconfig.json", "TypeScript configuration"}} {
					if info, e := os.Lstat(filepath.Join(snapshot.Root, marker.path)); e == nil && info.Mode().IsRegular() {
						p.Frameworks = append(p.Frameworks, marker.label)
					}
				}
				p.Valid = true
				p.Revision = snapshot.Revision
				p.Files = len(files)
			}
		}
		if err != nil {
			p.Error = "Unable to validate this source. Check Git access/path and retry; Direct mode is available without changing your selection."
			allValid = false
		}
		out = append(out, p)
	}
	jsonBody(w, map[string]any{"valid": allValid, "mode": in.Mode, "repositories": out, "exclusions": exclusions, "immutable_preview": true, "framework_detection": "manifest presence; does not certify dependency installation or compiler success"})
}
