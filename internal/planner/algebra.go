// Package planner defines the bounded, inspectable query representation used
// by repository-knowledge retrieval.  It deliberately contains no SQL or
// untyped graph expression language.
package planner

import (
	"fmt"
	"sort"
	"strings"
)

type Operator string

const (
	Resolve   Operator = "resolve"
	Exact     Operator = "exact"
	Lexical   Operator = "lexical"
	Filter    Operator = "filter"
	Traverse  Operator = "traverse"
	Intersect Operator = "intersect"
	Union     Operator = "union"
	Rank      Operator = "rank"
	Fetch     Operator = "fetch"
	Explain   Operator = "explain"

	DefaultTimeMS = 250
	MaxTimeMS     = 1000
	DefaultItems  = 100
	MaxItems      = 500
	DefaultDepth  = 3
	MaxDepth      = 4
	DefaultResult = 20
	MaxResult     = 100
)

var allowedFields = map[string]bool{"source": true, "documentation": true, "path": true, "symbol": true, "strings": true, "logs_errors": true, "configuration": true}
var allowedDirections = map[string]bool{"in": true, "out": true, "both": true}

// Budget applies independently to every bounded execution dimension.
type Budget struct {
	TimeMS, Candidates, Entities, Edges, Depth, Results int
}

func (b Budget) Normalized() (Budget, error) {
	if b.TimeMS == 0 {
		b.TimeMS = DefaultTimeMS
	}
	if b.Candidates == 0 {
		b.Candidates = DefaultItems
	}
	if b.Entities == 0 {
		b.Entities = DefaultItems
	}
	if b.Edges == 0 {
		b.Edges = DefaultItems
	}
	if b.Depth == 0 {
		b.Depth = DefaultDepth
	}
	if b.Results == 0 {
		b.Results = DefaultResult
	}
	for _, x := range []struct {
		name       string
		value, max int
	}{
		{"time_ms", b.TimeMS, MaxTimeMS}, {"candidates", b.Candidates, MaxItems},
		{"entities", b.Entities, MaxItems}, {"edges", b.Edges, MaxItems},
		{"depth", b.Depth, MaxDepth}, {"results", b.Results, MaxResult},
	} {
		if x.value < 1 || x.value > x.max {
			return Budget{}, fmt.Errorf("%s must be between 1 and %d", x.name, x.max)
		}
	}
	return b, nil
}

// Node is a fixed operator with bounded, typed attributes. Inputs form an
// acyclic operator tree; callers never supply this representation directly.
type Node struct {
	Operator   Operator `json:"operator"`
	Text       string   `json:"text,omitempty"`
	Fields     []string `json:"fields,omitempty"`
	Predicates []string `json:"predicates,omitempty"`
	Direction  string   `json:"direction,omitempty"`
	Depth      int      `json:"depth,omitempty"`
	Inputs     []*Node  `json:"inputs,omitempty"`
}

func (n Node) Validate(b Budget) error {
	if _, err := b.Normalized(); err != nil {
		return err
	}
	return n.validate(0)
}

func (n Node) validate(level int) error {
	if level > MaxDepth*3 {
		return fmt.Errorf("query algebra exceeds maximum nesting")
	}
	switch n.Operator {
	case Resolve, Exact, Lexical, Filter, Traverse, Intersect, Union, Rank, Fetch, Explain:
	default:
		return fmt.Errorf("unsupported query operator %q", n.Operator)
	}
	if len(n.Text) > 256 {
		return fmt.Errorf("query text exceeds 256 bytes")
	}
	if n.Operator == Traverse {
		if n.Direction == "" {
			n.Direction = "both"
		}
		if !allowedDirections[n.Direction] {
			return fmt.Errorf("unsupported traversal direction %q", n.Direction)
		}
		if n.Depth < 1 || n.Depth > MaxDepth {
			return fmt.Errorf("traversal depth must be between 1 and %d", MaxDepth)
		}
	}
	for _, field := range n.Fields {
		if !allowedFields[field] {
			return fmt.Errorf("unsupported search field %q", field)
		}
	}
	for _, predicate := range n.Predicates {
		if predicate == "" || len(predicate) > 96 || strings.ContainsAny(predicate, " ;'\"()") {
			return fmt.Errorf("invalid predicate")
		}
	}
	if len(n.Inputs) > MaxItems {
		return fmt.Errorf("too many query inputs")
	}
	for _, child := range n.Inputs {
		if child == nil {
			return fmt.Errorf("nil query input")
		}
		if err := child.validate(level + 1); err != nil {
			return err
		}
	}
	return nil
}

func StableFields(values []string) []string {
	out := append([]string(nil), values...)
	sort.Strings(out)
	return out
}
