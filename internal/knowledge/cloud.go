package knowledge

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"path"
	"sort"
	"strings"
)

var ErrCloudStale = errors.New("cloud snapshot, scope, or cursor is stale")

type CloudNode struct {
	Handle      string  `json:"handle"`
	Kind        string  `json:"kind"`
	Label       string  `json:"label"`
	Repository  string  `json:"repository"`
	Path        string  `json:"path"`
	Generation  string  `json:"generation"`
	Aggregate   bool    `json:"aggregate"`
	MemberCount int     `json:"member_count"`
	FileCount   int     `json:"file_count"`
	ChildScope  string  `json:"child_scope,omitempty"`
	Entity      *Entity `json:"entity,omitempty"`
}

type CloudCoverage struct {
	Complete    bool     `json:"complete"`
	Uncertainty []string `json:"uncertainty,omitempty"`
}

type CloudPage struct {
	Snapshot       string             `json:"snapshot"`
	Level          string             `json:"level"`
	Scope          string             `json:"scope"`
	Label          string             `json:"label"`
	Nodes          []CloudNode        `json:"nodes"`
	Edges          []Claim            `json:"edges"`
	TotalNodes     int                `json:"total_nodes"`
	TotalEntities  int                `json:"total_entities"`
	TotalFiles     int                `json:"total_files"`
	TotalClaims    int                `json:"total_claims"`
	EdgeCount      int                `json:"edge_count"`
	EdgesTruncated bool               `json:"edges_truncated"`
	NextCursor     string             `json:"next_cursor,omitempty"`
	Truncated      bool               `json:"truncated"`
	Generations    []RepositoryStatus `json:"generations"`
	Coverage       CloudCoverage      `json:"coverage"`
	CountScope     string             `json:"count_scope"`
	Focus          string             `json:"focus,omitempty"`
}

type cloudCursor struct {
	Snapshot string `json:"snapshot"`
	Scope    string `json:"scope"`
	Offset   int    `json:"offset"`
}

// A captured DECLARES_MODULE manifest owns its containing directory and all
// descendants until a more specific captured module. Other files fall into
// their first path component. This is a path membership rule, not inference
// that a directory is a compiler module.
const cloudGroupCTE = `WITH modules AS (
 SELECT DISTINCT CASE WHEN v.path='package.json' THEN '.' ELSE substr(v.path,1,length(v.path)-13) END AS dir
 FROM claims c JOIN evidence v ON v.evidence_id=c.evidence_id
 WHERE c.generation_id=? AND c.predicate='DECLARES_MODULE' AND (v.path='package.json' OR v.path LIKE '%/package.json')
), mapped AS (
 SELECT f.path,COALESCE((SELECT m.dir FROM modules m WHERE m.dir='.' OR substr(f.path,1,length(m.dir)+1)=m.dir||'/' ORDER BY length(m.dir) DESC LIMIT 1),
 CASE WHEN instr(f.path,'/')=0 THEN '.' ELSE substr(f.path,1,instr(f.path,'/')-1) END) AS grp
 FROM source_files f WHERE f.generation_id=?
)`

func cloudToken(gens []RepositoryStatus) string {
	b, _ := json.Marshal(gens)
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}

func cloudScope(kind, repo, value string) string {
	b, _ := json.Marshal([]string{kind, repo, value})
	return "cloud." + base64.RawURLEncoding.EncodeToString(b)
}

func parseCloudScope(raw string) (kind, repo, value string, err error) {
	if !strings.HasPrefix(raw, "cloud.") || len(raw) > 4096 {
		return "", "", "", ErrCloudStale
	}
	b, e := base64.RawURLEncoding.DecodeString(strings.TrimPrefix(raw, "cloud."))
	var parts []string
	if e != nil || json.Unmarshal(b, &parts) != nil || len(parts) != 3 {
		return "", "", "", ErrCloudStale
	}
	if parts[0] != "repo" && parts[0] != "group" && parts[0] != "file" {
		return "", "", "", ErrCloudStale
	}
	return parts[0], parts[1], parts[2], nil
}

func (s *Service) cloudGenerations(ctx context.Context) ([]RepositoryStatus, string, error) {
	rows, err := s.db.QueryCanonical(ctx, `SELECT a.repo_id,a.generation_id,g.git_commit,g.content_hash FROM active_generations a JOIN generations g ON g.generation_id=a.generation_id ORDER BY a.repo_id`)
	if err != nil {
		return nil, "", err
	}
	defer rows.Close()
	gens := []RepositoryStatus{}
	for rows.Next() {
		var g RepositoryStatus
		if err := rows.Scan(&g.ID, &g.Generation, &g.Revision, &g.ContentHash); err != nil {
			return nil, "", err
		}
		g.Active = true
		gens = append(gens, g)
	}
	if err := rows.Err(); err != nil {
		return nil, "", err
	}
	return gens, cloudToken(gens), nil
}

func cloudCursorEncode(c cloudCursor) string {
	b, _ := json.Marshal(c)
	return base64.RawURLEncoding.EncodeToString(b)
}

func cloudCursorDecode(raw string) (cloudCursor, error) {
	var c cloudCursor
	if len(raw) > 4096 {
		return c, ErrCloudStale
	}
	b, err := base64.RawURLEncoding.DecodeString(raw)
	if err != nil || json.Unmarshal(b, &c) != nil || c.Offset < 0 {
		return c, ErrCloudStale
	}
	return c, nil
}

// Cloud reads canonical active generations. Every aggregate is expandable to its
// exact descendant files; no projection, sample, or inferred edge is authority.
func (s *Service) Cloud(ctx context.Context, scope, snapshot, cursor string, limit int, focus ...string) (CloudPage, error) {
	if limit == 0 {
		limit = 100
	}
	if limit < 1 || limit > 256 {
		return CloudPage{}, fmt.Errorf("limit must be between 1 and 256")
	}
	gens, token, err := s.cloudGenerations(ctx)
	if err != nil {
		return CloudPage{}, err
	}
	if snapshot != "" && snapshot != token {
		return CloudPage{}, ErrCloudStale
	}
	focusHandle := ""
	focusOffset := 0
	if len(focus) > 0 && focus[0] != "" {
		if scope != "" || cursor != "" {
			return CloudPage{}, ErrCloudStale
		}
		entity, e := s.Entity(ctx, focus[0])
		if e != nil {
			return CloudPage{}, ErrCloudStale
		}
		focusHandle = entity.Handle
		scope = cloudScope("file", entity.Repository, entity.Path)
		h, _ := Decode(entity.Handle)
		e = s.db.QueryRowCanonical(ctx, `SELECT count(*) FROM entities e JOIN evidence v ON v.evidence_id=e.evidence_id WHERE e.generation_id=? AND v.path=? AND (e.kind<? OR (e.kind=? AND e.label<?) OR (e.kind=? AND e.label=? AND e.entity_id<?))`, entity.Generation, entity.Path, entity.Kind, entity.Kind, entity.Label, entity.Kind, entity.Label, h.ID).Scan(&focusOffset)
		if e != nil {
			return CloudPage{}, e
		}
	}
	kind, repo, value := "estate", "", ""
	if scope != "" {
		kind, repo, value, err = parseCloudScope(scope)
		if err != nil {
			return CloudPage{}, err
		}
	}
	gen := ""
	for _, g := range gens {
		if g.ID == repo {
			gen = g.Generation
		}
	}
	if kind != "estate" && gen == "" {
		return CloudPage{}, ErrCloudStale
	}
	off := 0
	if focusHandle != "" {
		off = (focusOffset / limit) * limit
	}
	if cursor != "" {
		c, e := cloudCursorDecode(cursor)
		if e != nil || c.Scope != scope || c.Snapshot != token {
			return CloudPage{}, ErrCloudStale
		}
		off = c.Offset
	}
	p := CloudPage{Snapshot: token, Scope: scope, Label: "Estate", Nodes: []CloudNode{}, Edges: []Claim{}, Generations: gens, Coverage: CloudCoverage{Complete: true}, CountScope: "all descendants; edges are claims between visible entity nodes"}
	p.Focus = focusHandle
	basis, err := s.db.Coverage(ctx, repo, "structural", nil)
	if err != nil {
		return CloudPage{}, err
	}
	p.Coverage.Complete = basis.Complete
	p.Coverage.Uncertainty = append(p.Coverage.Uncertainty, basis.Uncertainty...)
	p.Coverage.Uncertainty = append(p.Coverage.Uncertainty, basis.Exclusions...)
	for _, configured := range s.cfg.SourceRepositories() {
		found := false
		for _, g := range gens {
			if g.ID == configured.ID {
				found = true
				break
			}
		}
		if !found {
			p.Coverage.Complete = false
			p.Coverage.Uncertainty = append(p.Coverage.Uncertainty, "repository_not_indexed:"+configured.ID)
		}
	}
	if err := s.cloudRead(ctx, &p, kind, repo, value, gen, off, limit); err != nil {
		return CloudPage{}, err
	}
	if p.TotalNodes > off+len(p.Nodes) {
		p.NextCursor = cloudCursorEncode(cloudCursor{Snapshot: token, Scope: scope, Offset: off + len(p.Nodes)})
		p.Truncated = true
	}
	_, after, err := s.cloudGenerations(ctx)
	if err != nil {
		return CloudPage{}, err
	}
	if after != token {
		return CloudPage{}, ErrCloudStale
	}
	return p, nil
}

func (s *Service) cloudRead(ctx context.Context, p *CloudPage, kind, repo, value, gen string, off, limit int) error {
	var err error
	filter := ""
	args := []any{}
	if kind != "estate" {
		filter = " WHERE generation_id=?"
		args = append(args, gen)
	}
	if kind == "group" {
		if value == "." {
			filter += " AND instr(path,'/')=0"
		} else {
			filter += " AND substr(path,1,length(?)+1)=?"
			args = append(args, value, value+"/")
		}
	}
	if kind == "file" {
		if value == "" || path.IsAbs(value) || path.Clean(value) != value || strings.HasPrefix(value, "../") {
			return ErrCloudStale
		}
		filter += " AND path=?"
		args = append(args, value)
	}
	if kind == "estate" {
		p.Level = "estate"
		p.TotalNodes = len(p.Generations)
		for i := off; i < len(p.Generations) && len(p.Nodes) < limit; i++ {
			g := p.Generations[i]
			members, files, claims, e := s.cloudCounts(ctx, g.Generation, "", "")
			if e != nil {
				return e
			}
			p.Nodes = append(p.Nodes, CloudNode{Handle: cloudScope("repo", g.ID, ""), Kind: "repository", Label: g.ID, Repository: g.ID, Generation: g.Generation, Aggregate: true, MemberCount: members, FileCount: files, ChildScope: cloudScope("repo", g.ID, "")})
			p.TotalEntities += members
			p.TotalFiles += files
			p.TotalClaims += claims
		}
		// Estate totals include repositories beyond the current page.
		p.TotalEntities, p.TotalFiles, p.TotalClaims, err = s.cloudCounts(ctx, "", "", "")
		return err
	}
	if kind == "repo" {
		p.Level = "repository"
		p.Label = repo
		if e := s.db.QueryRowCanonical(ctx, cloudGroupCTE+` SELECT count(DISTINCT grp) FROM mapped`, gen, gen).Scan(&p.TotalNodes); e != nil {
			return e
		}
		rows, e := s.db.QueryCanonical(ctx, cloudGroupCTE+` SELECT grp,count(*) FROM mapped GROUP BY grp ORDER BY grp LIMIT ? OFFSET ?`, gen, gen, limit, off)
		if e != nil {
			return e
		}
		groups := []string{}
		fileCounts := []int{}
		for rows.Next() {
			var g string
			var n int
			if e = rows.Scan(&g, &n); e != nil {
				rows.Close()
				return e
			}
			groups = append(groups, g)
			fileCounts = append(fileCounts, n)
		}
		e = rows.Err()
		rows.Close()
		if e != nil {
			return e
		}
		for i := 0; i < len(groups) && len(p.Nodes) < limit; i++ {
			g := groups[i]
			members, _, _, e := s.cloudCounts(ctx, gen, "group", g)
			if e != nil {
				return e
			}
			label := g + " (path group)"
			groupKind := "path_group"
			if module := s.cloudModuleLabel(ctx, gen, g); module != "" {
				label = module + " (captured module at " + g + ")"
				groupKind = "module"
			}
			p.Nodes = append(p.Nodes, CloudNode{Handle: cloudScope("group", repo, g), Kind: groupKind, Label: label, Repository: repo, Path: g, Generation: gen, Aggregate: true, MemberCount: members, FileCount: fileCounts[i], ChildScope: cloudScope("group", repo, g)})
		}
	} else if kind == "group" {
		p.Level = "package"
		p.Label = value + " (path group)"
		if value == "" || path.IsAbs(value) || path.Clean(value) != value || strings.HasPrefix(value, "../") {
			return ErrCloudStale
		}
		rows, e := s.db.QueryCanonical(ctx, cloudGroupCTE+` SELECT path FROM mapped WHERE grp=? ORDER BY path LIMIT ? OFFSET ?`, gen, gen, value, limit+1, off)
		if e != nil {
			return e
		}
		paths := []string{}
		for rows.Next() {
			var x string
			if e = rows.Scan(&x); e != nil {
				rows.Close()
				return e
			}
			paths = append(paths, x)
		}
		e = rows.Err()
		rows.Close()
		if e != nil {
			return e
		}
		for _, file := range paths {
			if len(p.Nodes) == limit {
				break
			}
			members, _, _, e := s.cloudCounts(ctx, gen, "file", file)
			if e != nil {
				return e
			}
			p.Nodes = append(p.Nodes, CloudNode{Handle: cloudScope("file", repo, file), Kind: "file", Label: path.Base(file), Repository: repo, Path: file, Generation: gen, Aggregate: true, MemberCount: members, FileCount: 1, ChildScope: cloudScope("file", repo, file)})
		}
	} else if kind == "file" {
		p.Level = "file"
		p.Label = value
		var exists int
		if e := s.db.QueryRowCanonical(ctx, `SELECT count(*) FROM source_files WHERE generation_id=? AND path=?`, gen, value).Scan(&exists); e != nil {
			return e
		}
		if exists == 0 {
			return ErrCloudStale
		}
		rows, e := s.db.QueryCanonical(ctx, `SELECT e.entity_id FROM entities e JOIN evidence v ON v.evidence_id=e.evidence_id WHERE e.generation_id=? AND v.path=? ORDER BY e.kind,e.label,e.entity_id LIMIT ? OFFSET ?`, gen, value, limit+1, off)
		if e != nil {
			return e
		}
		ids := []string{}
		for rows.Next() {
			var id string
			if e = rows.Scan(&id); e != nil {
				rows.Close()
				return e
			}
			ids = append(ids, id)
		}
		e = rows.Err()
		rows.Close()
		if e != nil {
			return e
		}
		for _, id := range ids {
			if len(p.Nodes) == limit {
				break
			}
			entity, e := s.entity(ctx, gen, id)
			if e != nil {
				return e
			}
			p.Nodes = append(p.Nodes, CloudNode{Handle: entity.Handle, Kind: entity.Kind, Label: entity.Label, Repository: repo, Path: entity.Path, Generation: gen, Entity: &entity})
		}
		if err := s.cloudVisibleEdges(ctx, p, limit); err != nil {
			return err
		}
	}
	p.TotalEntities, p.TotalFiles, p.TotalClaims, err = s.cloudCounts(ctx, gen, kind, value)
	if err != nil {
		return err
	}
	if kind == "group" {
		p.TotalNodes = p.TotalFiles
		if p.TotalFiles == 0 {
			return ErrCloudStale
		}
	}
	if kind == "file" {
		p.TotalNodes = p.TotalEntities
	}
	return nil
}

func (s *Service) cloudCounts(ctx context.Context, gen, kind, value string) (entities, files, claims int, err error) {
	if kind == "group" {
		members := cloudGroupCTE + ` SELECT path FROM mapped WHERE grp=?`
		memberArgs := []any{gen, gen, value}
		if err = s.db.QueryRowCanonical(ctx, cloudGroupCTE+` SELECT count(*) FROM mapped WHERE grp=?`, memberArgs...).Scan(&files); err != nil {
			return
		}
		if err = s.db.QueryRowCanonical(ctx, `WITH group_paths AS (`+members+`) SELECT count(*) FROM entities e JOIN evidence v ON v.evidence_id=e.evidence_id WHERE e.generation_id=? AND v.path IN (SELECT path FROM group_paths)`, append(memberArgs, gen)...).Scan(&entities); err != nil {
			return
		}
		err = s.db.QueryRowCanonical(ctx, `WITH group_paths AS (`+members+`) SELECT count(*) FROM claims c JOIN entities s ON s.entity_id=c.subject_id JOIN evidence sv ON sv.evidence_id=s.evidence_id JOIN entities o ON o.entity_id=c.object_id JOIN evidence ov ON ov.evidence_id=o.evidence_id WHERE c.generation_id=? AND sv.path IN (SELECT path FROM group_paths) AND ov.path IN (SELECT path FROM group_paths)`, append(memberArgs, gen)...).Scan(&claims)
		return
	}
	fileWhere := ""
	eWhere := ""
	cWhere := ""
	args := []any{}
	if gen != "" {
		fileWhere = " WHERE generation_id=?"
		eWhere = " WHERE generation_id=?"
		cWhere = " WHERE c.generation_id=?"
		args = []any{gen}
	}
	if kind == "group" || kind == "file" {
		clause := " AND path=?"
		if kind == "group" {
			if value == "." {
				clause = " AND instr(path,'/')=0"
			} else {
				clause = " AND substr(path,1,length(?)+1)=?"
			}
		}
		appendFilter := func(base string) (string, []any) {
			a := append([]any(nil), args...)
			if kind == "group" && value != "." {
				a = append(a, value, value+"/")
			} else if kind == "file" {
				a = append(a, value)
			}
			return base + clause, a
		}
		q, a := appendFilter(fileWhere)
		if err = s.db.QueryRowCanonical(ctx, "SELECT count(*) FROM source_files"+q, a...).Scan(&files); err != nil {
			return
		}
		q, a = appendFilter(eWhere)
		eq := strings.Replace(q, "generation_id", "e.generation_id", 1)
		eq = strings.Replace(eq, "path", "v.path", 1)
		if err = s.db.QueryRowCanonical(ctx, "SELECT count(*) FROM entities e JOIN evidence v ON v.evidence_id=e.evidence_id"+eq, a...).Scan(&entities); err != nil {
			return
		}
		// Claims belong to this scope only when both canonical endpoints do.
		endpoint := func(alias string) (string, []any) {
			if kind == "file" {
				return " AND " + alias + ".path=?", []any{value}
			}
			if value == "." {
				return " AND instr(" + alias + ".path,'/')=0", nil
			}
			return " AND substr(" + alias + ".path,1,length(?)+1)=?", []any{value, value + "/"}
		}
		sq, sa := endpoint("s")
		oq, oa := endpoint("o")
		claimArgs := append(append(append([]any(nil), args...), sa...), oa...)
		err = s.db.QueryRowCanonical(ctx, `SELECT count(*) FROM claims c JOIN entities s ON s.entity_id=c.subject_id JOIN evidence sv ON sv.evidence_id=s.evidence_id JOIN entities o ON o.entity_id=c.object_id JOIN evidence ov ON ov.evidence_id=o.evidence_id`+cWhere+strings.Replace(sq, "s.path", "sv.path", 1)+strings.Replace(oq, "o.path", "ov.path", 1), claimArgs...).Scan(&claims)
		return
	}
	if gen == "" {
		if err = s.db.QueryRowCanonical(ctx, `SELECT count(*) FROM source_files f JOIN active_generations a ON a.generation_id=f.generation_id`).Scan(&files); err != nil {
			return
		}
		if err = s.db.QueryRowCanonical(ctx, `SELECT count(*) FROM entities e JOIN active_generations a ON a.generation_id=e.generation_id`).Scan(&entities); err != nil {
			return
		}
		err = s.db.QueryRowCanonical(ctx, `SELECT count(*) FROM claims c JOIN active_generations a ON a.generation_id=c.generation_id`).Scan(&claims)
		return
	}
	if err = s.db.QueryRowCanonical(ctx, "SELECT count(*) FROM source_files"+fileWhere, args...).Scan(&files); err != nil {
		return
	}
	if err = s.db.QueryRowCanonical(ctx, "SELECT count(*) FROM entities"+eWhere, args...).Scan(&entities); err != nil {
		return
	}
	err = s.db.QueryRowCanonical(ctx, "SELECT count(*) FROM claims c"+cWhere, args...).Scan(&claims)
	return
}

func (s *Service) cloudModuleLabel(ctx context.Context, gen, group string) string {
	manifest := "package.json"
	if group != "." {
		manifest = group + "/package.json"
	}
	var label string
	_ = s.db.QueryRowCanonical(ctx, `SELECT o.label FROM claims c JOIN evidence v ON v.evidence_id=c.evidence_id JOIN entities o ON o.entity_id=c.object_id WHERE c.generation_id=? AND c.predicate='DECLARES_MODULE' AND v.path=? ORDER BY c.claim_id LIMIT 1`, gen, manifest).Scan(&label)
	return label
}

func (s *Service) cloudVisibleEdges(ctx context.Context, p *CloudPage, limit int) error {
	if len(p.Nodes) == 0 {
		return nil
	}
	ids := make([]string, 0, len(p.Nodes))
	gen := p.Nodes[0].Generation
	for _, n := range p.Nodes {
		h, e := Decode(n.Handle)
		if e != nil {
			return e
		}
		ids = append(ids, h.ID)
	}
	sort.Strings(ids)
	marks := strings.TrimRight(strings.Repeat("?,", len(ids)), ",")
	args := []any{gen}
	for _, id := range ids {
		args = append(args, id)
	}
	for _, id := range ids {
		args = append(args, id)
	}
	q := `SELECT count(*) FROM claims WHERE generation_id=? AND subject_id IN (` + marks + `) AND object_id IN (` + marks + `)`
	if err := s.db.QueryRowCanonical(ctx, q, args...).Scan(&p.EdgeCount); err != nil {
		return err
	}
	rows, err := s.db.QueryCanonical(ctx, strings.Replace(q, "count(*)", "claim_id", 1)+` ORDER BY claim_id LIMIT ?`, append(args, limit)...)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return err
		}
		c, e := s.claim(ctx, gen, id)
		if e != nil {
			return e
		}
		p.Edges = append(p.Edges, c)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	p.EdgesTruncated = p.EdgeCount > len(p.Edges)
	return nil
}
