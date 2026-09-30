// Package contextpkg builds compact, deterministic evidence packages. It has
// no repository access: callers must supply canonical, generation-bound input.
package contextpkg

import (
	"encoding/json"
	"fmt"
	"sort"
)

const (
	DefaultMaxBytes        = 5960
	DefaultEstimatedTokens = 1490
	DefaultEntities        = 8
	DefaultEdges           = 16
	DefaultExcerpts        = 4
	DefaultLines           = 12
)

type Budget struct {
	MaxBytes, EstimatedTokens, Entities, Edges, Excerpts, LinesPerExcerpt int
}

func (b Budget) Normalized() (Budget, error) {
	if b.MaxBytes == 0 {
		b.MaxBytes = DefaultMaxBytes
	}
	if b.EstimatedTokens == 0 {
		b.EstimatedTokens = DefaultEstimatedTokens
	}
	if b.Entities == 0 {
		b.Entities = DefaultEntities
	}
	if b.Edges == 0 {
		b.Edges = DefaultEdges
	}
	if b.Excerpts == 0 {
		b.Excerpts = DefaultExcerpts
	}
	if b.LinesPerExcerpt == 0 {
		b.LinesPerExcerpt = DefaultLines
	}
	for _, v := range []struct {
		name       string
		value, max int
	}{
		{"max_bytes", b.MaxBytes, 256 * 1024}, {"estimated_tokens", b.EstimatedTokens, 64 * 1024},
		{"max_entities", b.Entities, 100}, {"max_edges", b.Edges, 100}, {"max_excerpts", b.Excerpts, 100}, {"max_lines_per_excerpt", b.LinesPerExcerpt, 200},
	} {
		if v.value < 1 || v.value > v.max {
			return Budget{}, fmt.Errorf("%s must be between 1 and %d", v.name, v.max)
		}
	}
	return b, nil
}

type Span struct {
	StartLine int `json:"start_line"`
	EndLine   int `json:"end_line"`
}
type Entity struct {
	Handle, Kind, Label, Identity, Repository, Path, Generation, Evidence string
	Confidence                                                            float64 `json:"confidence"`
	Role                                                                  string  `json:"role"`
	EvidenceTier, GraphDistance                                           int     `json:"-"`
}
type Relationship struct {
	Handle, Subject, Predicate, Object, Evidence, ObjectEvidence, Extractor, Derivation, Generation string
	Confidence                                                                                      float64 `json:"confidence"`
	Role                                                                                            string  `json:"role"`
	EvidenceTier, GraphDistance                                                                     int     `json:"-"`
}
type Excerpt struct {
	Handle, Source, Repository, Path, FileSHA256, Generation, Text string
	Span                                                           Span `json:"span"`
	OriginalLines                                                  int  `json:"original_lines"`
	Truncated                                                      bool `json:"truncated,omitempty"`
}
type RepositoryGeneration struct{ Repository, Generation, GitRevision, ContentHash, ExtractorVersions string }
type Expansion struct {
	Kind, Operation string
	Input           map[string]any `json:"input"`
}
type Omission struct {
	Kind, Key, Reason string
	Expansion         Expansion `json:"expansion"`
}
type Package struct {
	Status          string                 `json:"status"`
	AnswerKind      string                 `json:"answer_kind"`
	AnswerAnchors   []string               `json:"answer_anchors,omitempty"`
	Entities        []Entity               `json:"entities,omitempty"`
	Relationships   []Relationship         `json:"relationships,omitempty"`
	Excerpts        []Excerpt              `json:"source_excerpts,omitempty"`
	Repositories    []RepositoryGeneration `json:"repository_generations,omitempty"`
	Derivation      map[string]string      `json:"derivation"`
	ExecutedPlan    any                    `json:"executed_query_plan"`
	Trace           []string               `json:"executed_query_plan_trace"`
	Coverage        map[string]any         `json:"coverage"`
	Uncertainty     []string               `json:"uncertainty,omitempty"`
	Ambiguity       []string               `json:"ambiguity,omitempty"`
	Omissions       []Omission             `json:"omissions,omitempty"`
	Truncated       bool                   `json:"truncated,omitempty"`
	AppliedBudget   Budget                 `json:"applied_budget"`
	SerializedBytes int                    `json:"serialized_bytes"`
	EstimatedTokens int                    `json:"estimated_tokens"`
	TokenEstimator  string                 `json:"token_estimator"`
}
type Input struct {
	Package       Package
	Entities      []Entity
	Relationships []Relationship
	Excerpts      []Excerpt
	Budget        Budget
}

// EstimateTokens is deliberately an estimate, not runtime token telemetry.
func EstimateTokens(bytes int) int { return (bytes + 3) / 4 }

func Compile(in Input) (Package, error) {
	b, err := in.Budget.Normalized()
	if err != nil {
		return Package{}, err
	}
	for _, x := range in.Entities {
		if x.Handle != "" && x.Evidence == "" {
			return Package{}, fmt.Errorf("entity %q has no canonical evidence", x.Handle)
		}
	}
	for _, x := range in.Relationships {
		if x.Handle != "" && x.Evidence == "" {
			return Package{}, fmt.Errorf("relationship %q has no canonical evidence", x.Handle)
		}
	}
	for _, x := range in.Excerpts {
		if x.Handle == "" || x.Source == "" {
			return Package{}, fmt.Errorf("source excerpt has no canonical handle")
		}
	}
	p := in.Package
	p.AppliedBudget = b
	p.TokenEstimator = "utf8_bytes_div_4_ceiling_estimate"
	p.Entities = selectEntities(in.Entities, b.Entities, &p)
	p.AnswerAnchors = selectedAnchors(p.AnswerAnchors, p.Entities)
	p.Relationships = selectRelationships(in.Relationships, b.Edges, &p)
	p.Excerpts = selectExcerpts(in.Excerpts, b.Excerpts, b.LinesPerExcerpt, &p)
	for {
		p.SerializedBytes = 0
		p.EstimatedTokens = 0
		bytes, err := json.Marshal(p)
		if err != nil {
			return Package{}, err
		}
		p.SerializedBytes, p.EstimatedTokens = len(bytes), EstimateTokens(len(bytes))
		if p.SerializedBytes <= b.MaxBytes && p.EstimatedTokens <= b.EstimatedTokens {
			return p, nil
		}
		if len(p.Excerpts) > 0 {
			x := p.Excerpts[len(p.Excerpts)-1]
			p.Excerpts = p.Excerpts[:len(p.Excerpts)-1]
			omit(&p, "source_excerpt", x.Handle, "package_budget", sourceExpansion(x))
			continue
		}
		if len(p.Relationships) > 0 {
			x := p.Relationships[len(p.Relationships)-1]
			p.Relationships = p.Relationships[:len(p.Relationships)-1]
			omit(&p, "relationship", x.Handle, "package_budget", neighborsExpansion(x.Subject))
			continue
		}
		if len(p.Entities) > 1 {
			x := p.Entities[len(p.Entities)-1]
			p.Entities = p.Entities[:len(p.Entities)-1]
			p.AnswerAnchors = removeAnchor(p.AnswerAnchors, x.Handle)
			omit(&p, "entity", x.Handle, "package_budget", entityExpansion(x.Handle))
			continue
		}
		return Package{}, fmt.Errorf("context budget cannot fit decisive evidence")
	}
}
func selectedAnchors(anchors []string, entities []Entity) []string {
	seen := map[string]bool{}
	for _, x := range entities {
		seen[x.Handle] = true
	}
	out := []string{}
	for _, x := range anchors {
		if seen[x] {
			out = append(out, x)
		}
	}
	return out
}
func removeAnchor(anchors []string, handle string) []string {
	out := anchors[:0]
	for _, x := range anchors {
		if x != handle {
			out = append(out, x)
		}
	}
	return out
}
func selectEntities(values []Entity, limit int, p *Package) []Entity {
	seen, out := map[string]bool{}, []Entity{}
	sort.SliceStable(values, func(i, j int) bool { return entityLess(values[i], values[j]) })
	for _, x := range values {
		k := x.Identity
		if k == "" {
			k = x.Handle
		}
		if seen[k] {
			continue
		}
		seen[k] = true
		if len(out) >= limit {
			omit(p, "entity", x.Handle, "entity_budget", entityExpansion(x.Handle))
			continue
		}
		out = append(out, x)
	}
	return out
}
func selectRelationships(values []Relationship, limit int, p *Package) []Relationship {
	seen, out := map[string]bool{}, []Relationship{}
	sort.SliceStable(values, func(i, j int) bool { return relationshipLess(values[i], values[j]) })
	for _, x := range values {
		k := x.Subject + "\x00" + x.Predicate + "\x00" + x.Object
		if seen[k] {
			continue
		}
		seen[k] = true
		if len(out) >= limit {
			omit(p, "relationship", x.Handle, "edge_budget", neighborsExpansion(x.Subject))
			continue
		}
		out = append(out, x)
	}
	return out
}
func selectExcerpts(values []Excerpt, limit, lines int, p *Package) []Excerpt {
	seen, out := map[string]bool{}, []Excerpt{}
	sort.SliceStable(values, func(i, j int) bool {
		return values[i].Repository+"\x00"+values[i].Path+"\x00"+values[i].Handle < values[j].Repository+"\x00"+values[j].Path+"\x00"+values[j].Handle
	})
	for _, x := range values {
		k := x.Repository + "\x00" + x.Path + fmt.Sprintf("\x00%d\x00%d", x.Span.StartLine, x.Span.EndLine)
		if seen[k] {
			continue
		}
		seen[k] = true
		if x.OriginalLines > lines {
			x.Truncated = true
			omit(p, "source_range", x.Handle, "source_line_budget", sourceExpansion(x))
		}
		if len(out) >= limit {
			omit(p, "source_excerpt", x.Handle, "excerpt_budget", sourceExpansion(x))
			continue
		}
		out = append(out, x)
	}
	return out
}
func entityLess(a, b Entity) bool {
	if a.Role != b.Role {
		return a.Role == "answer_anchor"
	}
	if a.EvidenceTier != b.EvidenceTier {
		return a.EvidenceTier < b.EvidenceTier
	}
	if a.Confidence != b.Confidence {
		return a.Confidence > b.Confidence
	}
	if a.GraphDistance != b.GraphDistance {
		return a.GraphDistance < b.GraphDistance
	}
	return a.Repository+"\x00"+a.Path+"\x00"+a.Handle < b.Repository+"\x00"+b.Path+"\x00"+b.Handle
}
func relationshipLess(a, b Relationship) bool {
	if a.Role != b.Role {
		return a.Role == "decisive"
	}
	if a.EvidenceTier != b.EvidenceTier {
		return a.EvidenceTier < b.EvidenceTier
	}
	if a.Confidence != b.Confidence {
		return a.Confidence > b.Confidence
	}
	if a.GraphDistance != b.GraphDistance {
		return a.GraphDistance < b.GraphDistance
	}
	return a.Subject+"\x00"+a.Predicate+"\x00"+a.Object+"\x00"+a.Handle < b.Subject+"\x00"+b.Predicate+"\x00"+b.Object+"\x00"+b.Handle
}
func omit(p *Package, kind, key, reason string, expansion Expansion) {
	p.Truncated = true
	p.Omissions = append(p.Omissions, Omission{kind, key, reason, expansion})
}
func entityExpansion(handle string) Expansion {
	return Expansion{"entity", "kb.entity", map[string]any{"handle": handle}}
}
func neighborsExpansion(handle string) Expansion {
	return Expansion{"neighbors", "kb.neighbors", map[string]any{"handle": handle, "direction": "both"}}
}
func sourceExpansion(x Excerpt) Expansion {
	return Expansion{"source_range", "kb.source", map[string]any{"handle": x.Handle, "before": 0, "after": x.OriginalLines}}
}
