package planner

import (
	"regexp"
	"strings"
)

type Family string

const (
	ExactLookup     Family = "exact_lookup"
	LocateChange    Family = "locate_change"
	CallerReference Family = "caller_reference"
	EventTrace      Family = "event_trace"
	ExplainCause    Family = "explain_cause"
	RouteTrace      Family = "route_trace"
	Configuration   Family = "configuration_lookup"
	Impact          Family = "impact"
	Negative        Family = "negative_verification"
)

type Features struct {
	Quoted, Identifier, Path, Event, Route, Configuration bool
	Caller, Consumer, Impact, Cause, Update, Negative     bool
}

// SliceKind identifies the cheapest fixed architecture answer shape available
// to a caller that has supplied an explicit slice anchor. Query execution keeps
// hybrid retrieval when no explicit selector is present.
func SliceKind(text string) (string, bool) {
	lower := strings.ToLower(text)
	switch {
	case containsAny(lower, "repository overview", "service overview"):
		return "repository_service_overview", true
	case containsAny(lower, "service boundary"):
		return "service_boundary", true
	case containsAny(lower, "contract surface", "api surface"):
		return "contract_api_surface", true
	case containsAny(lower, "event flow", "topic flow"):
		return "event_topic_flow", true
	case containsAny(lower, "http flow", "route flow", "client flow"):
		return "http_route_client_flow", true
	case containsAny(lower, "configuration boundary"):
		return "configuration_boundary", true
	case containsAny(lower, "dependency boundary"):
		return "dependency_boundary", true
	case containsAny(lower, "test to feature", "test-to-feature"):
		return "test_to_feature", true
	case containsAny(lower, "entity neighbourhood", "entity neighborhood"):
		return "entity_neighbourhood", true
	}
	return "", false
}

var identifier = regexp.MustCompile(`\b[A-Za-z_][A-Za-z0-9_]*(?:\.[A-Za-z_][A-Za-z0-9_]*)*\b`)
var path = regexp.MustCompile(`(?:^|\s)(?:[A-Za-z0-9_.-]+/)+[A-Za-z0-9_.-]+|\B/[A-Za-z0-9_{}./-]+`)

func Classify(text string) Features {
	lower := strings.ToLower(text)
	f := Features{
		Quoted:     strings.Count(text, "\"") >= 2 || strings.Count(text, "'") >= 2,
		Path:       path.FindString(text) != "",
		Identifier: identifier.FindString(text) != "" && (strings.ContainsAny(text, "_.") || strings.IndexFunc(text, func(r rune) bool { return r >= 'A' && r <= 'Z' }) >= 0),
	}
	f.Event = containsAny(lower, "event", "topic", "queue", "kafka", "jms", "message")
	f.Route = containsAny(lower, "route", "endpoint", "http", "get ", "post ", "put ", "patch ", "delete ") || strings.Contains(text, "/")
	f.Configuration = containsAny(lower, "config", "configuration", "property", "properties", "environment", "env", "setting")
	f.Caller = containsAny(lower, "caller", "callers", "reference", "references", "called by")
	f.Consumer = containsAny(lower, "consumer", "consumes", "listener", "subscribes")
	f.Impact = containsAny(lower, "impact", "affected", "depend", "dependents")
	f.Cause = containsAny(lower, "cause", "why", "error", "exception", "failure", "log")
	f.Update = containsAny(lower, "where update", "where is update", "change", "update")
	f.Negative = containsAny(lower, "not found", "does not", "doesn't", "no caller", "no consumer", "any ") && containsAny(lower, "exist", "found", "caller", "consumer", "reference", "route")
	return f
}

func containsAny(s string, terms ...string) bool {
	for _, x := range terms {
		if strings.Contains(s, x) {
			return true
		}
	}
	return false
}

// Compile selects one stable family. More specific evidence-seeking intents
// intentionally win over generic identifier and lexical matches.
func Compile(text string, b Budget) (Family, Features, Node, error) {
	b, err := b.Normalized()
	if err != nil {
		return "", Features{}, Node{}, err
	}
	f := Classify(text)
	family := ExactLookup
	switch {
	case f.Negative:
		family = Negative
	case f.Impact:
		family = Impact
	case f.Caller || f.Consumer:
		family = CallerReference
	case f.Cause:
		family = ExplainCause
	case f.Event:
		family = EventTrace
	case f.Route:
		family = RouteTrace
	case f.Configuration:
		family = Configuration
	case f.Update:
		family = LocateChange
	}
	var root Node
	sources := []*Node{{Operator: Exact, Text: text}, {Operator: Lexical, Text: text, Fields: StableFields([]string{"symbol", "path", "source", "documentation", "strings", "logs_errors", "configuration"})}}
	switch family {
	case EventTrace:
		sources = append(sources, &Node{Operator: Traverse, Direction: "both", Depth: b.Depth, Predicates: []string{"PUBLISHES_EVENT", "CONSUMES_EVENT", "PRODUCES_TOPIC", "CONSUMES_TOPIC", "SENDS_QUEUE", "LISTENS_QUEUE"}})
	case RouteTrace:
		sources = append(sources, &Node{Operator: Traverse, Direction: "both", Depth: b.Depth, Predicates: []string{"DECLARES_EFFECTIVE_ENDPOINT", "DECLARES_UI_ROUTE", "INVOKES_API"}})
	case Configuration:
		sources = append(sources, &Node{Operator: Traverse, Direction: "both", Depth: b.Depth, Predicates: []string{"DEFINES_CONFIGURATION", "REFERENCES_CONFIGURATION", "BINDS_CONFIGURATION"}})
	case CallerReference:
		sources = append(sources, &Node{Operator: Traverse, Direction: "in", Depth: b.Depth, Predicates: []string{"CALLS", "REFERENCES"}})
	case Impact:
		sources = append(sources, &Node{Operator: Traverse, Direction: "in", Depth: b.Depth})
	case ExplainCause:
		sources = append(sources, &Node{Operator: Traverse, Direction: "both", Depth: 1, Predicates: []string{"HAS_LOCAL_GUARD", "EMITS_LOG", "EMITS_ERROR", "COVERAGE_GAP"}})
	}
	root = Node{Operator: Explain, Inputs: []*Node{{Operator: Fetch, Inputs: []*Node{{Operator: Rank, Inputs: []*Node{{Operator: Union, Inputs: sources}}}}}}}
	if err := root.Validate(b); err != nil {
		return "", Features{}, Node{}, err
	}
	return family, f, root, nil
}
