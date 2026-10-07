package webui

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/AdamNi-7080/AIOS/internal/knowledge"
)

type sourceAction struct {
	LocalPath string `json:"-"`
	Line      int    `json:"-"`
	Kind      string `json:"kind"`
	Label     string `json:"label"`
	URL       string `json:"url,omitempty"`
	Available bool   `json:"available"`
	Reason    string `json:"reason,omitempty"`
}

type sourceActions struct {
	Actions    []sourceAction `json:"actions"`
	Generation string         `json:"generation"`
	Revision   string         `json:"revision"`
}

var revisionPattern = regexp.MustCompile(`^[0-9a-fA-F]{40,64}$`)
var remotePartPattern = regexp.MustCompile(`^[A-Za-z0-9_.-]+$`)

func (s *Server) cloudAPI(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == "/api/v1/cloud" {
		if r.Method != http.MethodGet || !s.authorised(r, false) {
			fail(w, 403, "authorised same-origin session required")
			return
		}
		s.mu.Lock()
		pending := s.removing != ""
		s.mu.Unlock()
		if pending {
			fail(w, 503, "Repository removal is recovering")
			return
		}
		limit := 100
		if raw := r.URL.Query().Get("limit"); raw != "" {
			n, e := strconv.Atoi(raw)
			if e != nil {
				fail(w, 400, "invalid limit")
				return
			}
			limit = n
		}
		page, err := s.readService().Cloud(r.Context(), r.URL.Query().Get("scope"), r.URL.Query().Get("snapshot"), r.URL.Query().Get("cursor"), limit, r.URL.Query().Get("entity"))
		if err != nil {
			if errors.Is(err, knowledge.ErrCloudStale) {
				fail(w, 409, err.Error())
			} else {
				fail(w, 400, err.Error())
			}
			return
		}
		w.Header().Set("Cache-Control", "no-store")
		jsonBody(w, page)
		return
	}
	if r.Method != http.MethodPost || !s.authorised(r, true) {
		fail(w, 403, "valid origin, session, and CSRF token required")
		return
	}
	var in struct {
		Handle string `json:"handle"`
		Kind   string `json:"kind,omitempty"`
	}
	if decode(r, &in) != nil {
		fail(w, 400, "invalid source action request")
		return
	}
	result, err := s.sourceActions(r, in.Handle)
	if err != nil {
		fail(w, 409, err.Error())
		return
	}
	if r.URL.Path == "/api/v1/source-actions/open" {
		if in.Kind != "editor" && in.Kind != "finder" {
			fail(w, 400, "unsupported source action")
			return
		}
		if runtime.GOOS != "darwin" {
			fail(w, 409, "local source opening requires macOS")
			return
		}
		if _, err = s.readService().Entity(r.Context(), in.Handle); err != nil {
			fail(w, 409, "entity generation changed")
			return
		}
		var action sourceAction
		for _, a := range result.Actions {
			if a.Kind == in.Kind {
				action = a
				break
			}
		}
		if !action.Available {
			fail(w, 409, action.Reason)
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()
		if in.Kind == "finder" {
			u, e := url.Parse(action.URL)
			if e != nil || u.Scheme != "file" {
				fail(w, 409, "local source is unavailable")
				return
			}
			err = exec.CommandContext(ctx, "open", "-R", u.Path).Run()
		} else {
			u, e := url.Parse(action.URL)
			if e != nil || u.Scheme != "vscode" {
				fail(w, 409, "editor destination is unavailable")
				return
			}
			err = exec.CommandContext(ctx, "code", editorArguments(action.LocalPath, action.Line)...).Run()
		}
		if err != nil {
			fail(w, 409, "source action could not be opened")
			return
		}
		jsonBody(w, map[string]bool{"opened": true})
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	jsonBody(w, result)
}

func (s *Server) sourceActions(r *http.Request, handle string) (sourceActions, error) {
	entity, err := s.readService().Entity(r.Context(), handle)
	if err != nil {
		return sourceActions{}, err
	}
	var revision, sha string
	var dirty bool
	err = s.db.QueryRowCanonical(r.Context(), `SELECT g.git_commit,g.dirty!=0 OR g.untracked_count>0,f.sha256 FROM entities e JOIN generations g ON g.generation_id=e.generation_id JOIN evidence v ON v.evidence_id=e.evidence_id JOIN source_files f ON f.generation_id=e.generation_id AND f.path=v.path WHERE e.entity_id=? AND e.generation_id=?`, func() string { h, _ := knowledge.Decode(handle); return h.ID }(), entity.Generation).Scan(&revision, &dirty, &sha)
	if err != nil {
		return sourceActions{}, fmt.Errorf("entity source is unavailable")
	}
	result := sourceActions{Generation: entity.Generation, Revision: revision, Actions: []sourceAction{
		{Kind: "editor", Label: "Open in editor", Reason: "current local source unavailable or differs from captured revision"},
		{Kind: "finder", Label: "Show in Finder", Reason: "current local source unavailable or differs from captured revision"},
		{Kind: "remote", Label: "View captured revision", Reason: "trusted commit-pinned remote unavailable"},
	}}
	if !safeRelativeSourcePath(entity.Path) {
		return result, nil
	}
	s.mu.Lock()
	setup := cloneSetup(s.setup)
	s.mu.Unlock()
	for _, local := range setup.LocalRepositories {
		if sourceMode(setup.ActiveMode) != "local" {
			break
		}
		if local.ID != entity.Repository {
			continue
		}
		if target, ok := matchingLocalFile(local.Path, entity.Path, sha); ok && runtime.GOOS == "darwin" {
			fileURL := (&url.URL{Scheme: "file", Path: target}).String()
			// Availability checks must not invoke an application. The explicit open
			// route uses this executable and reports invocation failures honestly.
			if _, e := exec.LookPath("code"); e == nil {
				result.Actions[0] = sourceAction{Kind: "editor", Label: "Open in editor", LocalPath: target, Line: entity.Span.StartLine, URL: "vscode://file" + (&url.URL{Path: target}).EscapedPath() + fmt.Sprintf(":%d", entity.Span.StartLine), Available: true}
			}

			result.Actions[1] = sourceAction{Kind: "finder", Label: "Show in Finder", URL: fileURL, Available: true}
		}
		break
	}
	if sourceMode(setup.ActiveMode) == "mirror" && !dirty && revisionPattern.MatchString(revision) {
		for _, mirror := range setup.Repositories {
			if mirror.ID != entity.Repository {
				continue
			}
			if remote, ok := trustedRemoteLink(mirror.URL, revision, entity.Path); ok {
				result.Actions[2] = sourceAction{Kind: "remote", Label: "View captured revision", URL: remote, Available: true}
			}
			break
		}
	}
	if _, err = s.readService().Entity(r.Context(), handle); err != nil {
		return sourceActions{}, fmt.Errorf("entity generation changed")
	}
	return result, nil
}

// VS Code's --goto parser treats colons inside legal macOS filenames as
// line/column separators. Preserve the exact file for those names, without a
// line hint, and use an argv terminator rather than a shell or URI dispatch.
func editorArguments(target string, line int) []string {
	if strings.Contains(target, ":") {
		return []string{"--", target}
	}
	return []string{"--goto", target + ":" + strconv.Itoa(line)}
}

func safeRelativeSourcePath(rel string) bool {
	return rel != "" && !path.IsAbs(rel) && path.Clean(rel) == rel && rel != "." && rel != ".." && !strings.HasPrefix(rel, "../") && !strings.Contains(rel, "\\") && !strings.ContainsRune(rel, '\x00')
}

func matchingLocalFile(root, rel, capturedSHA string) (string, bool) {
	if !safeRelativeSourcePath(rel) || !filepath.IsAbs(root) || filepath.Clean(root) != root {
		return "", false
	}
	realRoot, err := filepath.EvalSymlinks(root)
	if err != nil || realRoot != root {
		return "", false
	}
	target := filepath.Join(root, filepath.FromSlash(rel))
	contained, err := filepath.Rel(root, target)
	if err != nil || contained == ".." || strings.HasPrefix(contained, ".."+string(filepath.Separator)) {
		return "", false
	}
	realTarget, err := filepath.EvalSymlinks(target)
	if err != nil || realTarget != target {
		return "", false
	}
	confined, err := os.OpenRoot(root)
	if err != nil {
		return "", false
	}
	defer confined.Close()
	partial := ""
	for _, part := range strings.Split(rel, "/") {
		partial = path.Join(partial, part)
		info, e := confined.Lstat(partial)
		if e != nil || info.Mode()&os.ModeSymlink != 0 {
			return "", false
		}
	}
	info, err := confined.Lstat(rel)
	if err != nil || !info.Mode().IsRegular() {
		return "", false
	}
	if info.Size() > 32<<20 {
		return "", false
	}
	f, err := confined.Open(rel)
	if err != nil {
		return "", false
	}
	defer f.Close()
	digest := sha256.New()
	n, err := io.CopyN(digest, f, info.Size()+1)
	if err != nil && err != io.EOF {
		return "", false
	}
	if n != info.Size() {
		return "", false
	}
	if !strings.EqualFold(hex.EncodeToString(digest.Sum(nil)), capturedSHA) {
		return "", false
	}
	return target, true
}

func trustedRemoteLink(raw, revision, rel string) (string, bool) {
	if !safeRelativeSourcePath(rel) || !revisionPattern.MatchString(revision) {
		return "", false
	}
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.Port() != "" {
		return "", false
	}
	host := strings.ToLower(u.Hostname())
	if host != "github.com" && host != "gitlab.com" {
		return "", false
	}
	repo := strings.TrimSuffix(strings.Trim(u.Path, "/"), ".git")
	parts := strings.Split(repo, "/")
	if len(parts) < 2 || (host == "github.com" && len(parts) != 2) {
		return "", false
	}
	for _, part := range parts {
		if part == "" || part == "." || part == ".." || !remotePartPattern.MatchString(part) {
			return "", false
		}
	}
	segments := []string{}
	for _, part := range strings.Split(rel, "/") {
		segments = append(segments, url.PathEscape(part))
	}
	base := "https://" + host + "/" + strings.Join(parts, "/")
	if host == "gitlab.com" {
		return base + "/-/blob/" + revision + "/" + strings.Join(segments, "/"), true
	}
	return base + "/blob/" + revision + "/" + strings.Join(segments, "/"), true
}
