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

Fixture acceptance proves only its generic 25-repository test corpus. Product
catalogs accept 1–100 repositories; 25 is a benchmark size, never an installation
prerequisite. The supported installable product is macOS Apple Silicon. Linux and
other native targets provide developer smoke coverage only.

Candidate readiness requires the full [completion acceptance contract](../.codex/completion/ACCEPTANCE.md):
all native installed-artifact, lifecycle, UI, privacy, recovery, upgrade and scale
gates at one tested revision, plus separate independent review. The hermetic engine
gate alone cannot establish that readiness. No private or employer corpus is required.
Missing coverage, ambiguity, budget exhaustion or projection gaps remain `unknown`;
only complete supported coverage can establish `not_found`.

## Optional reviewed-corpus validation

The historical `estate-acceptance-v1` target remains a generic opt-in regression
tool. Keep the reviewed corpus outside the repository; it may use generic fixtures
or voluntarily supplied repositories. It uses the benchmark fixture shape with
25 repositories pinned to clean 40-character Git revisions and 150 cases, balanced
at 30 each across exact, structural/locate, trace/cause, cross-repository/impact and
negative/unknown; every repository is scoped by at least four cases. These fixed
counts preserve this benchmark's useful thresholds; they do not constrain the product.

```sh
make estate-acceptance-v1 \
  ESTATE_CORPUS=/private/reviewed-gold.json \
  ACCEPTANCE_OUTPUT=/private/reviewed-corpus-run
```

The gate refuses dirty or revision-mismatched repositories, runs twice in separate
data directories, checks its existing thresholds and requires identical semantic
fingerprints. Detailed reports stay in the owned output directory. Later Benchmark
Lab work must report literal grep wins and wrong/unknown KB outcomes honestly;
this legacy comparison gate is not a substitute for that product gate.
