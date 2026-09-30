# CLI reference

The V1 command surface is deliberately small:

| Command | Required options | Behaviour |
| --- | --- | --- |
| `catalog validate` | `--config` | Validate the explicit repository allowlist. |
| `mirrors sync` | `--registry`, `--data-dir` | Clone/fetch the approved managed Git mirrors under agent-owned storage. |
| `ingest --all` | `--config`, `--registry`, `--data-dir` | Stage all approved mirrors (1–100) and atomically activate the initial catalog. |
| `ingest --repo ID` | `--config`, `--registry`, `--data-dir` | Compile and activate one approved repository revision delta. |
| `status` | `--data-dir` | Read source, generation, projection, and cache diagnostics. |
| `projections rebuild` | `--data-dir`; optional `--kind` | Rebuild agent-owned projections from persisted IR only. |
| `doctor` | `--config`; optional `--data-dir` | Validate local prerequisites and paths. |
| `benchmark` | `--fixture`, `--data-dir`; optional `--output` | Run deterministic retrieval quality measurements. |
| `serve` | `--config`, `--data-dir` | Serve bounded read-only `kb.*` MCP over stdio. |
| `ui serve` | `--config`, `--data-dir` | Serve the read-only browser projection on loopback. |

There are no mutation, execution, generation, or external-service commands.
`--all` and `--repo` are mutually exclusive. Failed bootstrap staging or
projection leaves the prior active catalog queryable.

Run the complete hermetic gate with
`make acceptance-v1 ACCEPTANCE_OUTPUT=/private/acceptance-run`. The deterministic
core runs on every supported native target; macOS arm64 additionally exercises
the bundled Nomic runtime.
