// Package slice defines bounded, generation-bound architecture views. Slices
// carry canonical handles only; they are never a factual authority.
package slice

import (
	"fmt"
	"sort"
	"strings"
)

type Kind string

const (
	RepositoryServiceOverview Kind = "repository_service_overview"
	ServiceBoundary           Kind = "service_boundary"
	ContractSurface           Kind = "contract_api_surface"
	EventFlow                 Kind = "event_topic_flow"
	HTTPFlow                  Kind = "http_route_client_flow"
	Configuration             Kind = "configuration_boundary"
	Dependency                Kind = "dependency_boundary"
	TestFeature               Kind = "test_to_feature"
	Impact                    Kind = "impact"
	Neighbourhood             Kind = "entity_neighbourhood"
)

var kinds = map[Kind]bool{
	RepositoryServiceOverview: true, ServiceBoundary: true,
	ContractSurface: true, EventFlow: true, HTTPFlow: true, Configuration: true,
	Dependency: true, TestFeature: true, Impact: true, Neighbourhood: true,
}

func (k Kind) Valid() bool { return kinds[k] }

type Selector struct {
	Kind          Kind     `json:"kind"`
	Anchor        string   `json:"anchor,omitempty"`
	EntityHandles []string `json:"entity_handles,omitempty"`
	Repository    string   `json:"repo_id,omitempty"`
}

func (s Selector) Normalized() (Selector, error) {
	if !s.Kind.Valid() {
		return Selector{}, fmt.Errorf("unsupported slice kind %q", s.Kind)
	}
	s.Anchor = strings.TrimSpace(s.Anchor)
	if s.Anchor == "" && len(s.EntityHandles) == 0 {
		return Selector{}, fmt.Errorf("slice anchor is required")
	}
	if s.Anchor != "" && len(s.EntityHandles) > 0 {
		return Selector{}, fmt.Errorf("use anchor or entity_handles, not both")
	}
	if len(s.EntityHandles) > 4 {
		return Selector{}, fmt.Errorf("at most 4 entity handles are allowed")
	}
	s.Repository = strings.TrimSpace(s.Repository)
	s.EntityHandles = append([]string(nil), s.EntityHandles...)
	sort.Strings(s.EntityHandles)
	for i, h := range s.EntityHandles {
		if h == "" || (i > 0 && h == s.EntityHandles[i-1]) {
			return Selector{}, fmt.Errorf("invalid or duplicate entity handle")
		}
	}
	return s, nil
}

type Budget struct{ Depth, Fanout, Entities, Edges, SourceLines int }

const (
	DefaultDepth       = 2
	DefaultFanout      = 20
	DefaultEntities    = 20
	DefaultEdges       = 30
	DefaultSourceLines = 40
	MaxDepth           = 4
	MaxFanout          = 100
	MaxEntities        = 100
	MaxEdges           = 100
	MaxSourceLines     = 200
)

func (b Budget) Normalized() (Budget, error) {
	if b.Depth == 0 {
		b.Depth = DefaultDepth
	}
	if b.Fanout == 0 {
		b.Fanout = DefaultFanout
	}
	if b.Entities == 0 {
		b.Entities = DefaultEntities
	}
	if b.Edges == 0 {
		b.Edges = DefaultEdges
	}
	if b.SourceLines == 0 {
		b.SourceLines = DefaultSourceLines
	}
	for _, x := range []struct {
		name       string
		value, max int
	}{{"depth", b.Depth, MaxDepth}, {"max_fanout", b.Fanout, MaxFanout}, {"max_entities", b.Entities, MaxEntities}, {"max_edges", b.Edges, MaxEdges}, {"max_source_lines", b.SourceLines, MaxSourceLines}} {
		if x.value < 1 || x.value > x.max {
			return Budget{}, fmt.Errorf("%s must be between 1 and %d", x.name, x.max)
		}
	}
	return b, nil
}

type Omission struct {
	Kind, Handle, Reason string
	Expansion            map[string]any `json:"expansion"`
}
type Provenance struct {
	PrecomputedHandle string `json:"precomputed_handle,omitempty"`
	CacheState        string `json:"cache_state,omitempty"`
}
type Slice struct {
	Handle                string         `json:"handle"`
	Kind                  Kind           `json:"kind"`
	Anchors               []string       `json:"anchors"`
	GenerationFingerprint string         `json:"generation_fingerprint"`
	Repositories          []string       `json:"repositories"`
	Coverage              map[string]any `json:"coverage"`
	Uncertainty           []string       `json:"uncertainty,omitempty"`
	Trace                 []string       `json:"selection_trace"`
	Budget                Budget         `json:"applied_budget"`
	Omissions             []Omission     `json:"omissions,omitempty"`
	Provenance            Provenance     `json:"provenance,omitempty"`
}

func Predicates(k Kind) ([]string, string) {
	switch k {
	case EventFlow:
		return []string{"PUBLISHES_EVENT", "CONSUMES_EVENT", "PRODUCES_TOPIC", "CONSUMES_TOPIC", "SENDS_QUEUE", "LISTENS_QUEUE"}, "both"
	case HTTPFlow, ContractSurface:
		return []string{"DECLARES_EFFECTIVE_ENDPOINT", "DECLARES_ENDPOINT", "DECLARES_UI_ROUTE", "INVOKES_API"}, "both"
	case Configuration:
		return []string{"DEFINES_CONFIGURATION", "REFERENCES_CONFIGURATION", "BINDS_CONFIGURATION"}, "both"
	case Dependency:
		return []string{"IMPORTS", "DEPENDS_ON", "REFERENCES", "CALLS"}, "both"
	case TestFeature:
		return []string{"TESTS", "CALLS", "REFERENCES"}, "both"
	case Impact:
		return nil, "in"
	case ServiceBoundary:
		return []string{"CALLS", "INVOKES_API", "PUBLISHES_EVENT", "CONSUMES_EVENT", "IMPORTS"}, "both"
	default:
		return nil, "both"
	}
}
