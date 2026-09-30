# Troubleshooting and recovery

- **Catalog rejected:** use canonical absolute, non-symlink repository roots;
  review any repository-count change before regenerating the allowlist.
- **Permission rejected:** the data directory must be owner-only and outside
  every catalogued repository. Repair ownership/mode or create a new directory.
- **Old or corrupt database:** stop MCP/UI readers, move the derived directory
  aside, create a fresh `0700` directory, and rerun mirror ingestion.
- **Interrupted ingestion:** rerun `ingest --all` for bootstrap or `ingest
  --repo ID` for a delta. Incomplete staging is discarded; the prior active
  generation remains queryable.
- **Failed projection or invalid activation:** inspect the command diagnostic,
  repair the input, and retry. Activation validation prevents partial or
  foreign-generation projections from replacing the last known good state.
- **Missing vector projection:** canonical, exact, lexical, graph, and
  structural/path routes remain available. Run `projections rebuild --kind
  vector --config CATALOG --data-dir DATA`; vector-only requests are `unknown`
  until it is ready.
- **Stale cache:** repeat the query. Cache entries are generation and projection
  bound and cannot establish facts independently.
- **Coverage diagnostic:** install/configure an agent-owned JDK or Node and
  TypeScript runtime, or treat the affected compiler conclusion as unknown.
- **Stale handle:** repeat the lookup against the active generation. Handles do
  not cross generation activation.
- **Acceptance failure:** inspect `acceptance-report.json` and the named log in
  its `commands` entry. Rerun into a new private output directory after repair;
  compare `semantic_fingerprint`, not timing or generated identifiers.
- **MCP framing error:** send one JSON-RPC object per line below 1 MiB. The
  server reports malformed/oversized input and continues with later frames.
- **UI authorization failure:** reopen the freshly printed fragment URL; its
  one-use capability cannot be replayed.
