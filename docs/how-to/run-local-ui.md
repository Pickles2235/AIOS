# Run the local repository knowledge UI

Index a catalog first, then serve only its derived data:

```sh
aios ui serve --config /absolute/catalog.json --data-dir /absolute/agent-data
```

The server binds a random loopback address and prints a one-use launch URL.
Its capability stays in the URL fragment, is exchanged for an HttpOnly local
session, and is never sent as a query parameter or referrer.

The UI exposes only bounded, active-generation knowledge:

- repository and generation status;
- a bounded entity/claim projection;
- canonical entity, typed-neighbor, and evidence inspection; and
- deterministic source/structural query results and their trace.

The projection is an overview, not a claim that every indexed fact is drawn.
An unavailable or missing projection does not mean the indexed store was lost.
Source excerpts are returned only for active evidence handles and retain their
generation, revision, path, and source-line range.

No endpoint writes data, accesses repository paths, runs commands, manages
tasks, downloads models, generates wiki content, or connects to external tools.

The authenticated **Edit instance** form saves a name (1–80 printable characters),
a `#RRGGBB` seed colour, and an optional PNG/JPEG logo (256 KiB, up to 1024×1024).
Remote URLs and SVG are rejected. An accessible initial represents a missing logo.
Owner-only `instance.json` stores presentation metadata and a random stable ID,
independent of canonical IR and projections. Existing data receives Homefold defaults
on first UI launch; renames, restarts, ingestion, resets and rebuilds retain the ID.
