# Repository Knowledge Engine V1

This reference describes implemented engine behaviour. The expanded Apple Silicon
[product contract](../../.codex/completion/PRODUCT.md) and
[gap map](../../.codex/completion/BASELINE-MAP.md) govern completion. MCP is an optional
read-only integration, never required onboarding.

V1 is a local, read-only repository knowledge engine. It indexes the approved
catalogue into deterministic source, symbol, relationship, evidence, and claim
records. Repositories remain inputs; all derived state is stored under the
agent-owned data directory.

Java and TypeScript/TSX compiler frontends are optional additive extractors.
They use JDK/TypeScript APIs through helpers cached below `--data-dir`; they do
not run Maven, Gradle, npm, pnpm, or yarn, and never emit into a checkout.
Optional `compiler_runtime` catalog paths select agent-owned `java_home`,
`node`, and `typescript_module` fallbacks. Repository-local TypeScript remains
preferred. Missing runtimes, invalid projects, and unresolved dynamic behavior
are retained as bounded coverage diagnostics while Tree-sitter evidence remains
available; callers must not treat those gaps as a negative structural result.

## Commands

Use `catalog validate`, `mirrors sync`, `ingest`, `status`, `projections rebuild`, `doctor`, and `serve`. The `serve`
command is a stdio MCP server and exposes only read-only `kb.*` tools:

- `kb.resolve`, `kb.search`, `kb.query`, `kb.entity`, and `kb.source`
- `kb.slice`
- `kb.landmarks`
- `kb.neighbors`, `kb.path`, `kb.references`, `kb.events`, and `kb.impact`
- `kb.explain`

Results are bounded and include canonical entity, claim, source, and evidence
handles. `kb.e1`, `kb.s1`, `kb.v1`, and `kb.c1` are opaque typed handles for an
entity, source revision, evidence span, and claim respectively. They are bound
to the active immutable repository generation: after a reindex activates a new
generation, old handles are rejected as stale.

Compiler entities expose a deterministic semantic identity plus their opaque
generation handle. Claims record extractor, derivation (`compiler_resolved`,
`syntax_derived`, `syntax_composed`, `local_syntax`, or `coverage_gap`), confidence, source span, source revision,
and generation. `kb.references` supports callers, callees, definitions,
references, imports, exports, inheritance, implementations, aliases, and
modules; traversal outputs remain bounded and generation-paginated.

`kb.resolve` and `kb.search` use the same deterministic candidate plan and are
resumable with generation-bound cursors. They accept repository, entity-kind,
language, classification, active-version (generation ID or Git commit), and
minimum-confidence filters. Search fields are `source`, `documentation`,
`path`, `symbol`, `strings`, `logs_errors`, and `configuration`.

Exact identifier, path, literal, and protocol/configuration-name candidates
rank above FTS5/BM25 lexical candidates. Within a match class, field relevance,
confidence, then repository/path/span/handle provide stable ordering. The
returned `plan_trace` names the deterministic plan and candidate classes; every
factual entity has cited source evidence plus its derivation/confidence where a
claim is returned.

`found` means direct or ranked canonical evidence was returned. `not_found`
means every requested active, capability-appropriate indexed scope and field
was searched with no supporting fact. `unknown` means coverage cannot support
the conclusion, including an unindexed repository, an excluded/unreadable/
unsupported file, an unavailable extractor or projection, an ambiguous target,
or an exhausted search budget. Both negative outcomes include bounded
`searched_scope`, `coverage` (repositories, fields, indexes, generations,
coverage records, and exclusions), and `uncertainty`; `unknown` includes retry
guidance. A result limit always supplies
`truncated`, `truncation_reason`, and a continuation cursor.

## Deterministic query planner

Query planning means selecting and executing fixed retrieval operators over
active canonical knowledge. `internal/planner` implements this deterministic
bounded query algebra. LLM reasoning, agent/task planning, action execution,
subagents, and autonomous reasoning remain outside V1. The existing
`disabled_capabilities` entries `planning` and `execution` in CLI `status` and
`kb.status` refer to those excluded agent capabilities, not retrieval planning.

`kb.query` accepts text, an optional fixed intent, and the same repository/entity/language/classification/
version/confidence filters as search, and bounded `time_ms`, `max_candidates`,
`max_entities`, `max_edges`, `depth`, and `limit` inputs. It does not accept
SQL, graph expressions, or caller-supplied plans. The server classifies fixed
query features and compiles an inspectable bounded algebra with only `resolve`,
`exact`, `lexical`, `filter`, `traverse`, `intersect`, `union`, `rank`, `fetch`,
and `explain` operators.

Intents include lookup, text search, relationship traversal, path finding,
impact, producers, consumers, architecture slice, and compatible fixed event,
route, configuration, reference, cause, and negative-verification operations.
Text-only compatibility inference is deterministic and surfaced; it never uses
a model or hidden semantic heuristic. Generic lookup runs unique exact identity
resolution first, then lexical FTS/BM25 only when exact is absent or ambiguous.
Explicit graph/path modes validate canonical anchors before their capability.
Vector retrieval remains local-only, optional, and disabled by default. When
enabled and available, it runs after other retrieval routes are insufficient
or for explicit `retrieval_mode:"vector"`; its scores never constitute evidence.

`plan_explanation` records normalized intent, selected strategy, stage reasons,
bounds, generation/projection versions, and coverage. Projection records are
never facts: returned results always hydrate canonical IR evidence.

Default budgets are 250 ms, 100 candidates/entities/edges, depth 3, and 20
results. Hard caps are 1 s, 500 candidates/entities/edges, depth 4, and 100
results. Results return the query family/features, executed plan,
candidate-generation and ranking trace, source ranks/confidence, evidence
anchors, applied limits, budget stops, and a generation-bound expansion handle.
Exact and source-backed structural evidence rank ahead of lexical evidence;
graph distance and confidence only refine a matching evidence tier. Final ties
use repository, path, source span, and handle.

`not_found` requires complete active coverage and completion of every planned
candidate source. `unknown` is returned when an active generation is absent,
coverage gaps make the conclusion unsafe, or a budget prevents completion.

Cross-repository links activate only with a complete catalog revision. They
retain repository-local spellings and evidence, and expose their normalization
rule, resolver, matching inputs, both anchors, and confidence. Event links do
not assert broker delivery; HTTP links require an exact extracted method and
literal normalized path. Dynamic and ambiguous candidates are `unknown`.

Other bounded operations
report `truncated`, `truncation_reason`, and `applied_limits`; callers should
refine the query, type, scope, or depth rather than assume an omitted fact does
not exist. Traversal accepts `in`, `out`, or `both` direction, typed predicates,
depth up to 4, plus independently bounded fanout, entities, edges, and paths.
Continuations and traces report filtered candidates, budget stops, and final
evidence anchors. Source output is capped at 200 lines.

## Domain extraction coverage

Java/Spring extraction recognizes mapping annotations (including composed
class/method paths), literal REST targets, Spring events, Kafka/JMS producers
and listeners, `@Value`/`@ConfigurationProperties`, direct SQL/JPA table facts,
and logger/throw sites. TypeScript/TSX recognizes React Router route elements,
their explicit rendered component, literal `fetch`/Axios targets, browser
events, `process.env`/`import.meta.env`, and console/throw sites. Directly
enclosing `if` or conditional syntax is returned as `HAS_LOCAL_GUARD`; it is a
local syntax fact, not end-to-end causality.

`kb.events`, `kb.neighbors`, `kb.references`, `kb.impact`, and `kb.explain`
include bounded canonical entities and evidence for returned claims. Their
`coverage.extractor_diagnostics` contains cited `COVERAGE_GAP` claims for
encountered computed targets and unsupported idioms. Dynamic routing, URLs,
topics, reflection, dependency injection, and runtime registration never
produce fabricated relationships; a conclusion that depends on such a gap is
unknown.

## Coverage and negative evidence

Every active repository generation has one immutable canonical coverage run.
It binds the repository/source revision, content fingerprint, extractor
versions, completion timestamp, run status, and per-path outcome. Entries are
included, excluded, unreadable, or unsupported and retain only path metadata,
classification, capability, and bounded reason/diagnostic—not source bodies.

Supported negative results create canonical negative-evidence records with the
normalized assertion, query type, deterministic plan fingerprint, searched
scope, timestamp, and dependencies on the active generation, coverage record,
and scoped indexes. A negative record is invalid when a dependent generation,
coverage record, extractor/index capability, or protocol identity changes.
An unrelated repository delta does not invalidate a repository-scoped absence.
MCP results expose the record ID as `negative_evidence`; consumers must not
reuse it after invalidation.

## Upgrade boundary

The current engine rejects databases with a different canonical schema. The
crash-safe supported-upgrade transaction is pending completion milestone 07.
The command refuses an old `index.db`; remove or recreate the agent-owned
`--data-dir`, then run mirror ingestion again. It does not migrate, alias, back up, or
preserve legacy derived records. Indexing remains read-only against source
checkouts.

## Projection lifecycle

The immutable Knowledge IR is the only factual authority. Lookup, lexical FTS,
typed graph, path, UI, and cache records are generation-bound projections.
Each carries a builder/version, input and deterministic output fingerprint,
timestamps, validation/count summary, and reason. A new catalog is activated
only when its required projections have built and validated atomically. Failed
or interrupted work preserves the prior active catalog and projection builds.

`projections rebuild --data-dir DATA` repairs derived state from the persisted
IR without opening a repository. Stale or unavailable projections are rejected
with a safe-unavailable diagnostic; callers may still resolve canonical source
and evidence handles. Vector projection is optional and disabled by default. On Apple Silicon it uses
the bundled Nomic embedding runtime; candidate scores never constitute evidence.

## Precomputed high-value knowledge

The required `landmarks` projection stores compact deterministic records for
repository summaries, service summaries, architecture landmarks, important
paths, dependencies, contracts, test anchors, boundaries, ownership facts, and
estate-topology facts. Every record has a stable ID, active catalog revision,
builder/version, confidence, coverage, recomputation reason, dependency IDs,
and canonical evidence/claim expansion handles.

Records are selected only from canonical entities, claims, fact lifecycle,
diagnostics, confirmed cross-repository links, and optional approved catalog
ownership metadata. Important paths are limited to manifests, configuration,
tests, routes/controllers, listeners/events, and other evidence-backed
structural anchors. Missing or conflicting ownership is explicit; no record
asserts runtime/deployment topology or an undocumented contract.

Activation retains unchanged repository-generation records and rebuilds changed
records and estate topology from the new IR. Failed staging retains the prior
active projection. A landmark can seed `kb.query`, but factual output expands
back to canonical evidence and never outranks newer or contradictory IR.

## Deliberately excluded

V1 does not register capabilities, run language models, generate wiki material,
execute tasks, create worktrees, or invoke external tools. Its optional local
vector projection uses the bundled embedding runtime only as a final retrieval
aid; vector similarity is never factual evidence.

## Context packages

`kb.context` deterministically selects minimum-sufficient canonical evidence
from the existing hybrid plan. Direct/compiler-resolved records outrank
structural, graph, and lexical candidates; ties use confidence, graph distance,
repository, path, span, and canonical handle. It deduplicates aliases,
relationships, and source spans before applying explicit budgets.

Every package is generation and revision bound. Facts resolve to canonical
entities, claims, or cited source excerpts with extractor, derivation, and
confidence. Omissions are explicit and expandable through active-generation
handles. Negative verification is `not_found` only when active coverage is
complete; otherwise it remains `unknown`.

## Architecture slices

`kb.slice` returns compact reusable architecture evidence for fixed overview,
boundary, contract/API, event/topic, HTTP, configuration, dependency,
test-to-feature, impact, and entity-neighbourhood questions. An unambiguous
declared-symbol anchor or explicit active entity handles seeds a fixed typed
profile. Direction/profile, depth, fanout, nodes, edges, and source lines are
bounded; every returned fact retains active canonical evidence, provenance,
derivation, and confidence.

Each slice has a generation-bound opaque handle, coverage/uncertainty, trace,
budgets, and expansion omissions. Dependency generation, coverage, or required
projection changes invalidate its cache record and stale its handle.
`kb.context` accepts the handle to compile that same evidence subgraph under its
own limits. Fixture size measurements do not establish general retrieval or
token-efficiency claims.

## Benchmark

Run the checked-in deterministic fixture with a fresh agent-owned directory:

```sh
go run ./cmd/aios benchmark \
  --fixture scripts/benchmark/fixtures/retrieval-v1.json \
  --data-dir /absolute/benchmark-data
```

The report records each question's fixture revision and query class, expected
and returned evidence, plus exact-only, lexical-only, structural-only,
graph-only, and hybrid measurements. Hybrid comparisons are deterministic and
provenance-backed; they do not claim general effectiveness beyond the checked-in
fixture. It also retains a literal file-read baseline comparable to ripgrep.
For each case it additionally reports the bounded context-package byte size,
estimated token count, source lines, answer and provenance correctness, and a
matching unbounded canonical-context reference for size comparison. Token
counts are deterministic estimates, not measured model usage.
It separately reports supported-negative accuracy, coverage completeness, and
unknown rate; those are not retrieval precision metrics. Unknown-rate drivers
should be read as work needed to improve coverage, not as evidence of absence.
Recreate the supplied data directory for a fresh run; it contains derived state
only.
