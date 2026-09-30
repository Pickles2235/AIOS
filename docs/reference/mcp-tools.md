# MCP tools

`serve` exposes only read-only, stdio `kb.*` tools. Factual results resolve to
active immutable Knowledge IR observations and their evidence; graph and FTS
only accelerate IR-backed lookup and are unavailable when their catalog
fingerprint is stale. No tool reads arbitrary paths, writes source state,
executes work, or invokes a model or service.

| Tool | Purpose |
| --- | --- |
| `kb.resolve`, `kb.search` | Deterministic canonical candidates and cited evidence. |
| `kb.query` | Bounded deterministic staged planner result with explanation. |
| `kb.slice` | Bounded typed architecture subgraph from active canonical IR. |
| `kb.landmarks` | Bounded deterministic high-value records derived from canonical IR. |
| `kb.context` | Compact deterministic canonical evidence package. |
| `kb.entity`, `kb.source` | Fetch a generation-bound canonical record or bounded source lines. |
| `kb.neighbors`, `kb.path`, `kb.references`, `kb.events`, `kb.impact`, `kb.explain` | Bounded typed canonical-claim traversal. |

See [Repository Knowledge Engine V1](repository-knowledge-v1.md) for fielded
search, ranking, filters, continuation, and negative outcomes.

`kb.status` also reports cache health. Cache trace entries (`cache_hit`,
`cache_miss`, `cache_coalesced`, or `cache_unavailable`) are diagnostics only;
they do not alter factual ranking or evidence authority.

## V1 provenance envelope

All `kb.*` responses use the V1 envelope:

```json
{
  "api_version": 1,
  "provenance": {
    "assertion_status": "direct",
    "canonical_ids": ["kb.e1...", "kb.c1..."],
    "evidence_handles": ["kb.v1..."],
    "ir_generations": ["g1-..."],
    "projection_state": "canonical_ir",
    "fact_state": "direct",
    "bounded": false
  },
  "result_handle": "kb.r1...",
  "result": { "status": "found" }
}
```

Clients read factual payload fields below `result`. `assertion_status` is
`direct`, `derived`, `inferred`, `stale`, `bounded`, or `unavailable`.
Scores, vector distances, graph rows, planner traces, cache entries, and
landmark summaries are retrieval/projection metadata; only canonical evidence
handles support source assertions. `result_handle` is generation-bound and
becomes `unknown` after supersession.

`kb.explain` accepts `{"handle":"..."}` for a result, entity, claim,
evidence, relationship claim, or landmark. It returns the same envelope with
the bounded canonical entities, claims, evidence chain, coverage, and planner
trace. Existing `subject`, `object`, `claim`, and `query` inputs remain
accepted during the response-shape migration.

## Diagnostics and operator workflow

`kb.status` and CLI `status` return source freshness, active catalog and
generation state, projection health, coverage-run count, and active safe
diagnostics. Events are append-only and may later carry a resolution timestamp.
Their scope contains only repository/revision/generation/path/range/projection
or a query fingerprint; diagnostic payloads never retain source excerpts,
request text, credentials, tokens, or source bodies.

| Code | Meaning | Operator action |
| --- | --- | --- |
| `INGESTION_FAILURE` | A repository generation did not complete. | Inspect the queue event and reindex the affected repository. |
| `EXTRACTION_FAILURE` | A configured extractor reported a failure. | Repair configuration or use supported analysis; do not assert absence. |
| `UNSUPPORTED_ANALYSIS` | Requested structural analysis is outside V1 support. | Treat the result as unknown. |
| `PROJECTION_FAILURE` | A rebuild failed before activation. | Keep the prior active build and rerun `projections rebuild`. |
| `PROJECTION_MISMATCH` | Active projection state/fingerprint is unsuitable. | Rebuild the named projection. |
| `DANGLING_PROVENANCE` | A projection reference no longer resolves to IR. | Rebuild; reindex if validation persists. |
| `CACHE_MISMATCH` | Cached metadata does not match active IR/projections. | Discard the cache entry and retry. |
| `PLANNER_FALLBACK` | Exact resolution required a deterministic fallback. | Narrow with handles, repository, or structural intent. |
| `PLANNER_BUDGET_EXHAUSTED` | A bounded planner stopped before full coverage. | Narrow scope or follow the continuation handle. |

For `not_found`, verify `result.coverage.complete` is true. Otherwise the
engine returns `unknown`: coverage, a required projection, language support,
or the planner budget was insufficient. For unhealthy status, preserve the
canonical database, rebuild projections first, then reindex only if canonical
evidence validation still fails.

## `kb.query` planner modes

`kb.query` accepts optional fixed intents including `lookup`, `text_search`,
`relationship_traversal`, `path_finding`, `impact`, `producers`, `consumers`,
and `architecture_slice`. Text-only requests remain compatible through
surfaced deterministic inference. Scope, active version, filters,
`relationship_types`, direction, evidence requirement, and budgets constrain
the request.

Generic lookup uses exact identity first and invokes lexical FTS/BM25 only when
exact resolution is absent or ambiguous. Explicit graph/path modes validate
canonical anchors before their required capability. `plan_explanation` records
the selected/skipped/insufficient stages, generations, projections, coverage,
and bounds. Projection gaps, ambiguity, or exhaustion return `unknown` or a
bounded result; `not_found` requires complete applicable coverage.

```json
{"text":"PublishCustomerChanged","intent":"consumers","repo_id":"customer","relationship_types":["CONSUMES_EVENT"],"depth":2,"require_evidence":true}
```

## `kb.landmarks`

`kb.landmarks` reads the active `landmarks` projection only. It accepts
`repo_id`, `service`, `kinds`, `minimum_confidence`, `limit`, and a
generation-bound cursor. Results include structured summaries, builder/version,
coverage, recomputation reason, source generations, and canonical claim/evidence
expansion handles. It returns no generated narrative or source body.

## `kb.context`

`kb.context` accepts the same query scope and filters as `kb.query`, plus
`max_bytes`, `estimated_tokens`, `max_entities`, `max_edges`, `max_excerpts`,
and `max_lines_per_excerpt`. It returns answer kind and canonical anchors,
compact entity summaries, decisive relationships, cited excerpts, active
generation/Git revision/content hash, extraction metadata, the executed plan,
coverage/uncertainty, omissions, and expansion actions.

Defaults are 8 KiB serialized JSON, 2,000 estimated tokens, 12 entities, 16
edges, 8 excerpts, and 12 source lines per excerpt. Token counts are estimates
using `utf8_bytes_div_4_ceiling_estimate`, never measured runtime usage.
Omissions identify the budget reason and use generation-bound `kb.entity`,
`kb.neighbors`, or `kb.source` inputs, which reject after supersession.

## `kb.slice`

`kb.slice` is a read-only derived view, not factual authority. It accepts one
fixed `kind`, a catalogued `repo_id`, and either an `anchor` resolving to one
declared symbol or up to four active `entity_handles`. It never accepts SQL,
graph expressions, row IDs, or arbitrary paths. Ambiguous, unsupported,
uncovered, and absent anchors return `unknown`; resolve first for a handle.

Kinds are `repository_service_overview`, `service_boundary`,
`contract_api_surface`, `event_topic_flow`, `http_route_client_flow`,
`configuration_boundary`, `dependency_boundary`, `test_to_feature`, `impact`,
and `entity_neighbourhood`. Each has a fixed source-supported predicate profile;
none asserts runtime topology, broker delivery, or causality beyond source.

Bounds are `depth` (2/4), `max_fanout` (20/100), `max_entities` (20/100),
`max_edges` (30/100), and `max_source_lines` (40/200). Results are stable,
deduplicated canonical nodes, claims, and evidence with derivation, confidence,
coverage, trace, explicit omissions, and active expansion inputs. The opaque
handle binds selector, budget, and generation; it rejects after supersession and
can be supplied to `kb.context` as `slice_handle` instead of `text`. Cache
provenance is diagnostic; cached handles are revalidated against active IR.
