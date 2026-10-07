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

Knowledge-read endpoints query only the derived store. Explicit authenticated
setup and branding endpoints write owned state as described below. Setup runs
the approved Git snapshot pipeline; it never runs source hooks or build scripts.
The UI does not download models, generate wiki content, or connect to agent tools.

The authenticated **Edit instance** form saves a name (1–80 printable characters),
a `#RRGGBB` seed colour, and an optional PNG/JPEG logo (256 KiB, up to 1024×1024).
Remote URLs and SVG are rejected. An accessible initial represents a missing logo.
Owner-only `instance.json` stores presentation metadata and a random stable ID,
independent of canonical IR and projections. Existing data receives AgentOS defaults
on first UI launch; renames, restarts, ingestion, resets and rebuilds retain the ID.

For first run, launch `aios ui serve --data-dir /absolute/owned-data` without
`--config`. The secure browser setup accepts approved repository IDs, HTTPS/SSH
Git URLs (no embedded credentials), or absolute local remotes, plus full Git refs.
Setup validates a bounded estate, syncs mirrors, and atomically ingests snapshots.
Stage feedback comes from actual work; errors allow explicit retry, and cancellation
or restart preserves the last active generation. An unfinished job is marked
interrupted on restart. Configuration is owner-only `setup.json`; temporary compiler
inputs and snapshots stay below the data directory. All setup mutations require
the browser session, matching Origin and CSRF token. MCP remains read-only.

Open **Spotlight** with Command-K on macOS or Ctrl-K elsewhere. Submit a query,
use Up/Down to select a result, and press Enter to inspect its captured source.
Escape closes the dialog and returns focus to its opener. The ordinary query
form and entity list remain available. Refreshing the page resumes the existing
HttpOnly session; restarting the server requires its new secure launch link.

The Operations row shows contextual Indexing, Health, Storage and size,
Performance, Coverage, Scheduled jobs, and Active query cards when useful.
Open any card manually from that row. Cards start in the page dock; drag the
title or focus Move and use arrow keys to float one. Shift-arrows move faster;
Home, Reset position, or Reset all module positions returns cards to the dock.
Saved floating coordinates are clamped to the viewport on reload and resize.
The activity API includes a process stream ID, latest sequence, and oldest
retained sequence. A lost history interval or restarted stream is shown as a
warning alongside current job/setup state. Failed operational reads leave the
last observation marked stale until a successful reconciliation.

The knowledge cloud uses only the current bounded projection page. It labels
its repository, generation, node/claim counts and limits. Edges are drawn when
both endpoints are on that page; unseen endpoints are not invented. **Next
projection page** replaces the page without merging generations. A cursor is
bound to the repository and generation and is rejected after activation changes.
Select an active repository to change the query and cloud scope.

Search feedback distinguishes empty and unsupported inputs, unavailable or stale
projections, incomplete coverage (`unknown`), complete covered absence
(`not_found`), positive evidence (`found`), and bounded results. Queries support
up to 256 text bytes, 1–100 results, confidence 0–1, a 500-candidate output budget
and a one-second service time budget. Coverage exclusions and uncertainty remain
visible even for positive answers. Evidence selections are revalidated and show
the captured Git commit, file SHA256, generation and line range; stale selections
clear the previous excerpt and prompt a refresh. Presentation and ranking never
supply new evidence.
