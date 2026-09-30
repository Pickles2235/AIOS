# V1 architecture

V1 catalogs contain exactly 25 `repository` sources and source-selection
policy. A separate strict JSON mirror registry contains the
approved 25 Git addresses and refs. External cron runs `mirrors sync`; AIOS
then compiles only a selected immutable Git archive materialized under its
owner-only data directory. User checkouts are never compilation inputs.

Every selection creates durable discovered, selected, source-delta, IR-staged,
IR-activated, and projection/cache lifecycle events. Git tree/hash evidence is
the authority for added, modified, renamed, and removed source; filesystem
watchers are not authoritative. Per-repository queues serialize revisions while
the catalog writer atomically activates a complete generation set.

Tree-sitter and optional JDK/TypeScript compiler helpers produce deterministic
Java and TypeScript/TSX source facts. SQLite stores immutable Knowledge IR
repository revisions, locations, repository-scoped semantic entities, aliases,
facts, observations, cited evidence, provenance, and diagnostics. **Knowledge
IR is authoritative; every index and visualisation is a rebuildable
projection.** FTS5/BM25 and graph projections carry the catalog fingerprint
and builder metadata used to reject stale data. Unchanged repositories reuse
their active generation during an incremental catalog update.

`cache.db` is a separate owner-only, disposable projection beside `index.db`.
It stores deterministic request hashes, generation/projection/coverage
fingerprints, bounded canonical handles, and compact selection metadata only.
It never stores query text, source bodies, credentials, sessions, or factual
responses. A hit is always rehydrated from active IR; mismatch rejects the
entry and cache failure falls back to canonical retrieval.

Consumers are read-only. MCP uses bounded newline-delimited JSON-RPC on stdio
and registers only `kb.*` operations. The browser API binds an ephemeral IPv4
loopback port, exchanges a one-use URL-fragment capability for an HttpOnly
same-site session, and requires same-origin CSRF tokens for POST queries.

`internal/planner` supplies deterministic retrieval query planning for
`kb.query`. Fixed intents or surfaced rule-based text classification select
an inspectable query algebra; execution is bounded by time, candidates,
entities, edges, traversal depth, and result count. Generic lookup resolves
exact identity first, with lexical fallback for absent or ambiguous matches;
explicit graph/path intents use validated canonical anchors. Results include
the plan, stage trace, coverage, and active-generation evidence. See the
[query planner reference](reference/repository-knowledge-v1.md#deterministic-query-planner).

The browser UI is an embedded static projection of canonical SQLite evidence.
There are no user-repository writes, arbitrary filesystem ingestion, live
connectors, LLM reasoning, agent/task planning or execution, or action-capable
operations. Executing a bounded retrieval plan stays within read-only knowledge
operations and supplies no agent runtime. The optional local vector projection
is rebuildable, disabled by default, and never evidence.
