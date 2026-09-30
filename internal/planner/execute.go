package planner

import (
	"encoding/base64"
	"encoding/json"
	"sort"
	"time"
)

type Source string

const (
	SourceExact      Source = "exact"
	SourceLexical    Source = "lexical"
	SourceStructural Source = "structural"
	SourceLandmark   Source = "landmark"
	SourceGraph      Source = "graph"
	SourceVector     Source = "vector"
)

// Candidate contains only inspectable ranking inputs. Key must be a canonical
// entity/evidence identity supplied by the canonical store.
type Candidate struct {
	Key, Repository, Path, Evidence, Match string
	SpanStart, SourceRank, GraphDistance   int
	Confidence                             float64
	Sources                                []Source
	Ranks                                  map[Source]int
}

type Stop struct {
	Reason string `json:"reason"`
	Value  int    `json:"value"`
	Limit  int    `json:"limit"`
}
type Execution struct {
	Candidates []Candidate
	Stops      []Stop
	Started    time.Time
}

func (e *Execution) Add(source Source, candidates []Candidate, b Budget) {
	for _, candidate := range candidates {
		if len(e.Candidates) >= b.Candidates {
			e.stop("candidate_budget", len(e.Candidates), b.Candidates)
			return
		}
		if time.Since(e.Started) > time.Duration(b.TimeMS)*time.Millisecond {
			e.stop("time_budget", int(time.Since(e.Started).Milliseconds()), b.TimeMS)
			return
		}
		candidate.Sources = append(candidate.Sources, source)
		if candidate.Ranks == nil {
			candidate.Ranks = map[Source]int{}
		}
		if _, ok := candidate.Ranks[source]; !ok {
			candidate.Ranks[source] = candidate.SourceRank
		}
		e.Candidates = append(e.Candidates, candidate)
	}
}
func (e *Execution) stop(reason string, value, limit int) {
	for _, x := range e.Stops {
		if x.Reason == reason {
			return
		}
	}
	e.Stops = append(e.Stops, Stop{reason, value, limit})
}

func (e *Execution) Stop(reason string, value, limit int) { e.stop(reason, value, limit) }

func Fuse(candidates []Candidate) []Candidate {
	merged := map[string]Candidate{}
	for _, c := range candidates {
		old, exists := merged[c.Key]
		if !exists {
			merged[c.Key] = c
			continue
		}
		old.Sources = uniqueSources(append(old.Sources, c.Sources...))
		if old.Ranks == nil {
			old.Ranks = map[Source]int{}
		}
		for source, rank := range c.Ranks {
			if current, ok := old.Ranks[source]; !ok || rank < current {
				old.Ranks[source] = rank
			}
		}
		if evidenceTier(c) < evidenceTier(old) || (evidenceTier(c) == evidenceTier(old) && c.Confidence > old.Confidence) {
			old.Repository, old.Path, old.Evidence, old.Match, old.SpanStart, old.SourceRank, old.GraphDistance, old.Confidence = c.Repository, c.Path, c.Evidence, c.Match, c.SpanStart, c.SourceRank, c.GraphDistance, c.Confidence
		}
		merged[c.Key] = old
	}
	out := make([]Candidate, 0, len(merged))
	for _, c := range merged {
		c.Sources = uniqueSources(c.Sources)
		out = append(out, c)
	}
	sort.Slice(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if evidenceTier(a) != evidenceTier(b) {
			return evidenceTier(a) < evidenceTier(b)
		}
		if a.GraphDistance != b.GraphDistance {
			return a.GraphDistance < b.GraphDistance
		}
		if a.Confidence != b.Confidence {
			return a.Confidence > b.Confidence
		}
		if a.Repository != b.Repository {
			return a.Repository < b.Repository
		}
		if a.Path != b.Path {
			return a.Path < b.Path
		}
		if a.SpanStart != b.SpanStart {
			return a.SpanStart < b.SpanStart
		}
		return a.Key < b.Key
	})
	return out
}
func evidenceTier(c Candidate) int {
	for _, s := range c.Sources {
		if s == SourceExact {
			return 0
		}
	}
	for _, s := range c.Sources {
		if s == SourceStructural {
			return 1
		}
	}
	for _, s := range c.Sources {
		if s == SourceLandmark {
			return 2
		}
	}
	for _, s := range c.Sources {
		if s == SourceGraph {
			return 3
		}
	}
	return 3
}
func uniqueSources(in []Source) []Source {
	seen := map[Source]bool{}
	out := []Source{}
	for _, x := range in {
		if !seen[x] {
			seen[x] = true
			out = append(out, x)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

type Continuation struct {
	Generation, Family string
	Offset             int
}

func EncodeContinuation(c Continuation) string {
	b, _ := json.Marshal(c)
	return base64.RawURLEncoding.EncodeToString(b)
}
func DecodeContinuation(value, generation, family string) (Continuation, bool) {
	var c Continuation
	b, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil || json.Unmarshal(b, &c) != nil || c.Generation != generation || c.Family != family || c.Offset < 0 {
		return Continuation{}, false
	}
	return c, true
}
