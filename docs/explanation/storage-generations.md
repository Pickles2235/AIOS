# Generations and storage

The writer verifies the exact `knowledge-ir-v10` schema format, SQLite integrity,
and foreign keys. Knowledge IR is authoritative; every index and visualisation
is a rebuildable projection. Indexing first stages source input and writes
immutable repository revisions, locations, canonical entities, aliases, facts,
observations, evidence, provenance, and diagnostics. Projection builders then
read IR only and stamp graph/search data with the exact catalog fingerprint.
Activation publishes the complete catalog revision atomically. Every required
projection (`lookup`, `lexical`, `graph`, `path`, `ui`, `cache`, and `landmarks`) is first
built privately from one immutable catalog generation, validated against its
entity, claim, and evidence handles, then activated with the catalog in the
same transaction. A failed or interrupted build rolls back and leaves the
previous catalog and active projection pointers intact.

`landmarks` is also required. It stores no source body: only deterministic
structured values and canonical IDs. During a repository delta it carries
unchanged-generation records forward, rebuilds records whose dependencies
changed, and rebuilds confirmed estate topology. Inactive, superseded, or
lower-confidence evidence cannot validate a new record.

Projection builds record their schema, IR/input/output fingerprints, builder
version, timestamps, health, record counts, validation summary, and rebuild
reason. Projection rows retain generation-bound IR handles only; builders read
SQLite IR records and never repository paths. Readers reject an unavailable or
misaligned projection rather than mixing generations. Direct canonical
entity/source/evidence reads remain available, so an unavailable map or FTS
does not mean knowledge has been lost.

Use `projections rebuild --data-dir DATA` to rebuild every enabled projection
without rediscovering or reading repositories; `--kind lexical,graph` limits a
repair to named kinds. The vector projection is local-only and disabled by
default. When enabled it stores generation-bound records that point only to
canonical entities and evidence; embeddings can never be returned as evidence.
Use `projections rebuild --kind vector --config CATALOG --data-dir DATA` to
rebuild it or `projections reset --kind vector --data-dir DATA` to clear it.
`status` and
`kb.status` report each build's health, source fingerprint, age, counts, and
rebuild state.

The release embeds the checksum-pinned Nomic v1.5 GGUF and a local
`llama-embedding` runtime in the macOS arm64 binary. At first use the runtime
is extracted owner-only below `DATA/vector-runtime`; it never downloads a model
or contacts a service. Other target platforms report vector unavailable until a
matching embedded native helper is packaged.

The separate cache projection has fixed V1 limits of 2,048 entries and 64 MiB,
with deterministic least-recently-accessed eviction and per-kind byte caps.
Its diagnostics report state, alignment, entries, bytes, hits, misses,
coalescing, evictions, invalidations, and stale rejections. Delete `cache.db`
to recover from cache corruption or loss; no reindex is required.

Mirror ingestion records an immutable delta before compilation, including source
and target revisions, registry/manifest fingerprint, Git-tree changes, counts,
and invalidation scope. Changed facts become active at the target generation;
facts derived only from removed files receive an explicit inactive lifecycle
record rather than disappearing. A normal source delta rebuilds affected files
and cross-identities; bootstrap, extractor-wide changes, and framework/contract
configuration are explicit full-repository fallbacks.

Readers open the database with SQLite read-only mode. A writer lock prevents concurrent ingestion writers. If ingestion fails, the queue retains its pending revision and failure diagnostic, while the active catalog remains unchanged; maintained ingestion retries or supersedes the failed selection safely. `status` and `kb.status` expose queue state, revisions, delta counts, and the bounded event audit trail.

The writer upgrades `knowledge-ir-v9` with a verified owner-only SQLite backup and an atomic transaction. The new format preserves distinct capture epochs when content reverts to earlier bytes; prior canonical observations remain intact. A failed migration retains the old format and last-good active generation. Backups remain in the owned data directory as `index.ir-9-backup-*.db`. Other historical derived formats require a clean IR reindex; repository inputs are never removed. This database migration alone does not certify a complete installed-package upgrade.

Successful activation keeps the active catalog and two prior catalog revisions by
default. Retention records obsolete owned snapshot roots durably, then reclaims
them after their capture/ingest lease ends. Cleanup validates owned paths and
canonical/staged references before deletion; a crash leaves the cleanup record
for the next successful ingest. Empty owned revision directories are removed.
External source workspaces and active or retained snapshots are never deleted.
Every captured owned root is recorded for cleanup before compilation. Failed
ingests discard unreferenced snapshots after the capture lease ends; staged
references retain their snapshot until staged work is discarded. Cleanup
candidates survive a restart. Background work reclaims up to four safe
unreferenced candidates before its storage admission check, so an orphaned
snapshot above the threshold does not permanently block future work. Status
and health reads remain read-only. Mirror archives stream into owned staging under
a separate 6 GiB extracted payload and 300,000-entry cap, with bounded tar
framing. Eligible file limits apply after exclusions; an overflow cannot
publish a partial snapshot.
