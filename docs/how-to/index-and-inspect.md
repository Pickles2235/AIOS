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
