package knowledge

import (
	"context"
	"fmt"
	"strings"

	"github.com/AdamNi-7080/AIOS/internal/store"
)

// Investigation is the portable, source-bearing payload shown and copied by
// Spotlight. Every finding is resolved from a canonical active-generation handle.
type Investigation struct {
	SchemaVersion     int                  `json:"schema_version"`
	Query             string               `json:"query"`
	SearchTerm        string               `json:"search_term"`
	Intent            string               `json:"intent"`
	Repository        string               `json:"repository,omitempty"`
	Status            string               `json:"status"`
	Generations       []string             `json:"generations"`
	Freshness         []RepositoryStatus   `json:"freshness"`
	Coverage          *store.CoverageBasis `json:"coverage,omitempty"`
	Findings          []Finding            `json:"findings"`
	CanonicalEvidence []Excerpt            `json:"canonical_evidence"`
	Relationships     []Claim              `json:"relationships"`
	Unknowns          []string             `json:"unknowns"`
	Budget            map[string]int       `json:"budget"`
	Truncated         bool                 `json:"truncated"`
}

type Finding struct {
	Entity   Entity `json:"entity"`
	Evidence string `json:"evidence"`
}

var investigationCommands = map[string]string{
	"question": "question", "symbol": "symbol", "path": "path", "log": "log",
	"event": "event", "route": "route", "config": "config",
}

// ParseInvestigation accepts one optional family command and one optional
// repository filter. Unknown commands are explicit unsupported input.
func ParseInvestigation(text string) (intent, repository, query string, ok bool) {
	fields := strings.Fields(strings.TrimSpace(text))
	intent = "question"
	ok = true
	for len(fields) > 0 {
		first := fields[0]
		if strings.HasPrefix(first, "/") && len(first) > 1 && !strings.Contains(first[1:], "/") {
			family, found := investigationCommands[strings.ToLower(first[1:])]
			if !found {
				return "unsupported", "", "", false
			}
			intent = family
			fields = fields[1:]
			continue
		}
		if strings.HasPrefix(first, "@") {
			if len(first) < 2 || repository != "" {
				return "unsupported", "", "", false
			}
			repository = first[1:]
			fields = fields[1:]
			continue
		}
		break
	}
	query = strings.Join(fields, " ")
	return
}

// SearchTerm extracts an explicit code-like subject from a natural question.
// It makes no attempt to infer causes or synthesize an answer.
func SearchTerm(intent, query string) string {
	if intent != "question" {
		return query
	}
	for _, raw := range strings.Fields(query) {
		term := strings.Trim(raw, "?.,:;()[]{}\"'`")
		if strings.ContainsAny(term, "_./") || (len(term) > 1 && strings.IndexFunc(term[1:], func(r rune) bool { return r >= 'A' && r <= 'Z' }) >= 0) {
			return term
		}
	}
	return query
}

func (s *Service) Investigation(ctx context.Context, text string) (Investigation, error) {
	intent, repo, query, ok := ParseInvestigation(text)
	out := Investigation{SchemaVersion: 1, Query: strings.TrimSpace(text), Intent: intent, Repository: repo,
		Generations: []string{}, Freshness: []RepositoryStatus{}, Findings: []Finding{}, CanonicalEvidence: []Excerpt{},
		Relationships: []Claim{}, Unknowns: []string{}, Budget: map[string]int{"results": 12, "excerpt_lines": 12, "relationships": 24}}
	if len(text) > 256 || !ok {
		out.Status = "unknown"
		out.Unknowns = append(out.Unknowns, "unsupported command or input bounds; use /question, /symbol, /path, /log, /event, /route, /config and optional @repository")
		return out, nil
	}
	out.SearchTerm = SearchTerm(intent, query)
	result, err := s.Query(ctx, Query{Text: out.SearchTerm, Repository: repo, Limit: 12})
	if err != nil {
		return out, err
	}
	out.Status, out.Coverage, out.Truncated = result.Status, result.Coverage, result.Truncated
	if result.Coverage != nil {
		out.Generations = append(out.Generations, result.Coverage.Generations...)
	}
	status, statusErr := s.Status(ctx)
	if statusErr != nil {
		return Investigation{}, statusErr
	}
	for _, active := range status.Repositories {
		if active.Active && (repo == "" || repo == active.ID) {
			out.Freshness = append(out.Freshness, active)
		}
	}
	for _, trace := range result.Trace {
		switch trace.Kind {
		case "projection_unavailable", "no_active_generation", "budget_exhausted", "unsupported_query", "empty_query", "coverage_incomplete":
			out.Unknowns = append(out.Unknowns, trace.Detail)
		}
	}
	if result.Coverage != nil {
		out.Unknowns = append(out.Unknowns, result.Coverage.Uncertainty...)
	}
	if intent == "question" && (strings.HasPrefix(strings.ToLower(query), "why ") || strings.HasPrefix(strings.ToLower(query), "how ")) {
		out.Unknowns = append(out.Unknowns, "causal explanation requires inspection of the cited source; matching evidence alone cannot prove a cause")
		if out.Status == "not_found" {
			out.Status = "unknown"
		}
	}
	for _, entity := range result.Entities {
		excerpt, e := s.Excerpt(ctx, entity.Evidence, 0, 0, 12)
		if e != nil {
			return Investigation{}, fmt.Errorf("investigation evidence changed: %w", e)
		}
		out.Findings = append(out.Findings, Finding{Entity: entity, Evidence: excerpt.Evidence})
		for i, line := range excerpt.Lines {
			runes := []rune(line)
			if len(runes) > 2048 {
				excerpt.Lines[i] = string(runes[:2048])
				excerpt.Truncated = true
			}
		}
		out.CanonicalEvidence = append(out.CanonicalEvidence, excerpt)
		if excerpt.Truncated {
			out.Truncated = true
		}
		if len(out.Relationships) < out.Budget["relationships"] {
			claims, truncated, e := s.Neighbors(ctx, entity.Handle, nil, 2)
			if e != nil {
				out.Unknowns = append(out.Unknowns, "relationship projection unavailable or stale")
				continue
			}
			out.Relationships = append(out.Relationships, claims...)
			if truncated {
				out.Truncated = true
			}
		}
	}
	if out.Truncated {
		out.Unknowns = append(out.Unknowns, "bounded results or excerpts; more evidence may exist")
	}
	if out.Status == "not_found" && out.Coverage != nil && !out.Coverage.Complete {
		out.Status = "unknown"
	}
	return out, nil
}
