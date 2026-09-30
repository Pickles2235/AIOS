package planner

import (
	"fmt"
	"sort"
	"strings"
)

// Intent is a fixed capability request. It is deliberately not natural-language
// semantics: omitted intent is derived only by the stable Classify rules.
type Intent string

const (
	IntentLookup                Intent = "lookup"
	IntentTextSearch            Intent = "text_search"
	IntentRelationshipTraversal Intent = "relationship_traversal"
	IntentPathFinding           Intent = "path_finding"
	IntentImpact                Intent = "impact"
	IntentProducers             Intent = "producers"
	IntentConsumers             Intent = "consumers"
	IntentArchitectureSlice     Intent = "architecture_slice"
	IntentEventTrace            Intent = "event_trace"
	IntentRouteTrace            Intent = "route_trace"
	IntentConfiguration         Intent = "configuration_lookup"
	IntentReferences            Intent = "references"
	IntentExplainCause          Intent = "explain_cause"
	IntentNegativeVerification  Intent = "negative_verification"
)

type Strategy string

const (
	StrategyExact      Strategy = "exact"
	StrategyLexical    Strategy = "lexical"
	StrategyGraph      Strategy = "graph"
	StrategyStructural Strategy = "structural"
	StrategyPath       Strategy = "path"
	StrategyVector     Strategy = "vector"
	StrategySlice      Strategy = "architecture_slice"
)

// Request contains only caller-controlled constraints. Handles remain opaque
// and are validated by the MCP boundary before a graph/path operation runs.
type Request struct {
	Text, Repository, Version, Anchor, From, To string
	Intent                                      Intent
	RelationshipTypes                           []string
	Direction                                   string
	RequireEvidence                             bool
	Inferred                                    bool
}

func (r Request) Normalized() (Request, error) {
	r.Text = strings.TrimSpace(r.Text)
	r.Repository = strings.TrimSpace(r.Repository)
	r.Version = strings.TrimSpace(r.Version)
	r.Anchor, r.From, r.To = strings.TrimSpace(r.Anchor), strings.TrimSpace(r.From), strings.TrimSpace(r.To)
	r.Direction = strings.ToLower(strings.TrimSpace(r.Direction))
	if r.Direction != "" && r.Direction != "in" && r.Direction != "out" && r.Direction != "both" {
		return Request{}, fmt.Errorf("direction must be in, out, or both")
	}
	for i := range r.RelationshipTypes {
		r.RelationshipTypes[i] = strings.ToUpper(strings.TrimSpace(r.RelationshipTypes[i]))
		if r.RelationshipTypes[i] == "" {
			return Request{}, fmt.Errorf("relationship type is required")
		}
	}
	sort.Strings(r.RelationshipTypes)
	r.RelationshipTypes = uniqueStrings(r.RelationshipTypes)
	if r.Intent == "" {
		r.Intent, r.Inferred = inferIntent(r.Text), true
	}
	if !validIntent(r.Intent) {
		return Request{}, fmt.Errorf("unsupported query intent %q", r.Intent)
	}
	if r.Text == "" && r.Anchor == "" && r.From == "" {
		return Request{}, fmt.Errorf("query text or anchor is required")
	}
	if r.Intent == IntentPathFinding && (r.From == "" || r.To == "") {
		return Request{}, fmt.Errorf("path_finding requires from and to entity handles")
	}
	if r.Intent == IntentArchitectureSlice && r.Anchor == "" {
		return Request{}, fmt.Errorf("architecture_slice requires an anchor")
	}
	return r, nil
}

type Stage struct {
	Strategy Strategy `json:"strategy"`
	Reason   string   `json:"reason"`
}
type QueryPlan struct {
	Intent     string     `json:"intent"`
	Strategy   Strategy   `json:"strategy"`
	Inferred   bool       `json:"inferred"`
	Stages     []Stage    `json:"stages"`
	Precedence []Strategy `json:"precedence"`
}

// Plan declares the only permitted stage order. Execution records whether
// each declared stage was selected, skipped, insufficient, or unavailable.
func Plan(in Request, _ Budget) (QueryPlan, error) {
	r, err := in.Normalized()
	if err != nil {
		return QueryPlan{}, err
	}
	p := QueryPlan{Intent: string(r.Intent), Inferred: r.Inferred, Precedence: []Strategy{StrategyExact, StrategyLexical, StrategyGraph, StrategyStructural, StrategyPath, StrategyVector}}
	switch r.Intent {
	case IntentTextSearch:
		p.Strategy, p.Stages = StrategyLexical, []Stage{{StrategyLexical, "explicit_text_search"}}
	case IntentPathFinding:
		p.Strategy, p.Stages = StrategyPath, []Stage{{StrategyPath, "explicit_path_finding"}}
	case IntentArchitectureSlice:
		p.Strategy, p.Stages = StrategySlice, []Stage{{StrategyExact, "resolve_slice_anchor"}, {StrategySlice, "explicit_architecture_slice"}}
	case IntentRelationshipTraversal, IntentImpact, IntentProducers, IntentConsumers, IntentEventTrace, IntentRouteTrace, IntentConfiguration, IntentReferences, IntentExplainCause:
		p.Strategy, p.Stages = StrategyGraph, []Stage{{StrategyExact, "resolve_graph_anchor"}, {StrategyGraph, "explicit_relationship_capability"}}
	case IntentNegativeVerification:
		p.Strategy, p.Stages = StrategyExact, []Stage{{StrategyExact, "negative_verification_exact"}, {StrategyLexical, "exact_insufficient"}, {StrategyGraph, "relationship_scope_requires_graph"}}
	default:
		p.Strategy, p.Stages = StrategyExact, []Stage{{StrategyExact, "cheapest_unique_identifier_route"}, {StrategyLexical, "exact_absent_or_ambiguous"}}
	}
	return p, nil
}

func inferIntent(text string) Intent {
	f := Classify(text)
	switch {
	case f.Negative:
		return IntentNegativeVerification
	case f.Impact:
		return IntentImpact
	case f.Consumer:
		return IntentConsumers
	case f.Caller:
		return IntentProducers
	case f.Cause:
		return IntentExplainCause
	case f.Event:
		return IntentEventTrace
	case f.Route:
		return IntentRouteTrace
	case f.Configuration:
		return IntentConfiguration
	case f.Update:
		return IntentReferences
	default:
		return IntentLookup
	}
}
func validIntent(v Intent) bool {
	for _, x := range []Intent{IntentLookup, IntentTextSearch, IntentRelationshipTraversal, IntentPathFinding, IntentImpact, IntentProducers, IntentConsumers, IntentArchitectureSlice, IntentEventTrace, IntentRouteTrace, IntentConfiguration, IntentReferences, IntentExplainCause, IntentNegativeVerification} {
		if v == x {
			return true
		}
	}
	return false
}
func uniqueStrings(in []string) []string {
	out := in[:0]
	for _, x := range in {
		if len(out) == 0 || out[len(out)-1] != x {
			out = append(out, x)
		}
	}
	return out
}
