# Acceptance handoff

Run the hermetic V1 gate from a clean output directory on any supported native
target:

```sh
make acceptance-v1 ACCEPTANCE_OUTPUT=/private/acceptance-run
```

The command creates 25 local bare Git remotes, validates and syncs the catalog,
atomically bootstraps IR and projections, queries real stdio MCP, advances four
repositories, ingests deltas, and checks stale-state and recovery behaviour.
It uses no network or credentials. macOS arm64 also gates the optional bundled
Nomic route; other targets require a truthful unavailable diagnostic and
continue through deterministic retrieval. The result is
`/private/acceptance-run/acceptance-report.json`, conforming to
[`reference/acceptance-report.schema.json`](reference/acceptance-report.schema.json).

A successful report has `disposition: accepted`, every case is `pass`, and two
runs have the same `semantic_fingerprint`. Timestamps, timings, generation IDs,
and evidence handles are intentionally excluded from that fingerprint. Logs for
failed commands are under the report directory's `logs` subdirectory.

Fixture acceptance does not prove completeness or correctness of the real
25-repository estate. Production readiness requires all of:

- exactly 25 approved catalog entries with reviewed ownership and boundaries;
- successful mirror sync at the intended revisions;
- supported-language indexing and explicit coverage diagnostics;
- healthy, generation-aligned required projections;
- representative exact, lexical, graph, structural/path, vector, negative, and
  architecture-slice checks with canonical evidence; and
- a reviewed list of exclusions and unsupported content.

Do not promote the production estate while any item is unverified. Missing
coverage, ambiguity, budget exhaustion, or projection gaps must remain
`unknown`; only complete supported coverage can establish `not_found`.

## Private real-estate gate

Keep the gold corpus outside the repository. It uses the benchmark fixture
shape with exactly 25 repositories pinned to clean 40-character Git revisions
and exactly 150 cases. Cases must be balanced at 30 each across exact,
structural/locate, trace/cause, cross-repository/impact, and negative/unknown;
every repository must be scoped by at least four cases.

```sh
make estate-acceptance-v1 \
  ESTATE_CORPUS=/private/aios-v1-gold.json \
  ACCEPTANCE_OUTPUT=/private/aios-v1-estate-run
```

The gate refuses dirty or revision-mismatched repositories, runs twice in
separate data directories, checks the published thresholds, and requires an
identical semantic fingerprint. Its report is privacy-safe: keep the private
corpus and detailed benchmark reports under the controlled acceptance output.
