# Index and inspect repositories

Index all catalogued repositories:

```sh
./bin/aios ingest --config /path/to/catalog.json --registry /path/to/mirrors.json --data-dir /path/to/data --repo frontend
```

V1 activation is catalog-atomic; partial `--repo` indexing is rejected. Only
`--data-dir` is written, and it must not be inside a catalogued root, including
through a symlink. The index is stored as `index.db` and SQLite sidecars. A
writer lock prevents concurrent indexers.

Inspect snapshots without reading source files:

```sh
./bin/aios status --data-dir /path/to/data
./bin/aios status --data-dir /path/to/data --repo frontend
```

Indexing captures Git state before and after discovery/extraction and refuses to publish if that state changes. Publication stages a complete generation and atomically activates it, so readers see the prior or new generation rather than a partial one.

## Mirror-backed ingestion

For a managed estate of 1–100 repositories, use a V1 source-policy catalog
(repository IDs and include/exclude rules, without checkout roots) and an
approved JSON mirror registry. Cron owns synchronization; ingestion does not
contact remotes:

```sh
./bin/aios mirrors sync --registry /private/mirrors.json --data-dir /private/data
./bin/aios ingest --config /private/catalog.json --registry /private/mirrors.json --data-dir /private/data --repo frontend
```

Run `mirrors sync` every 15 minutes. `status` reports selected/pending/current
revisions, attempts, failure diagnostics, invalidation scope, and the durable
event audit. The first mirror bootstrap remains catalog-atomic; subsequent
repository deltas retain unchanged active generations.

## Clean local Git workspaces

Use a matching catalog and local registry, then run `aios local ingest --all
--config catalog.json --registry local.json --data-dir /absolute/owned-data`.
Local registry entries contain `id` and a canonical absolute Git top-level `path`.
The browser source-type selector also supports this route. Workspaces must have a
committed HEAD and be clean, including staged state and nonignored untracked files.
Ignored files remain untouched and uncaptured. Dirty/untracked inputs fail without
source writes; commit or remove them yourself, then retry.

Capture reads pinned tree/blob objects with optional locks and lazy fetch disabled.
It never runs source hooks/builds or compiles live files. Symlinks, submodules, partial
clones, path collisions and overlapping source/data directories are rejected. The
file-count, per-file and total-byte catalog limits also bound all captured tracked
files before policy filtering. Repository export attributes do not alter the bytes.

Read-only owned snapshots are reused only after verifying them against the pinned
tree. A concurrent edit during capture fails before publication; edits afterward
cannot change snapshot evidence. Generation handles cite the captured Git commit
and file SHA256. Identical content may reuse its existing canonical generation
and original captured-commit provenance; revision discovery stays in the audit.
Stable identity is the explicitly approved repository ID, independent of snapshot
paths and projection builds. Failed capture/ingestion retains the active catalog;
retry after resolving the reported unsupported state.
