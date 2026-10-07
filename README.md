# AgentOS V1 repository knowledge appliance

The approved product is a generic, local repository knowledge appliance for
macOS Apple Silicon, with a headless per-user daemon and a browser UI. Its
implementation is in progress: the [completion contract](.codex/completion/PRODUCT.md)
and [baseline gap map](.codex/completion/BASELINE-MAP.md) distinguish requirements
from delivered behaviour. No private repository estate or LLM connection is required.

Per-user daemon lifecycle and ZIP installation are now under verification. Build
an engineering ZIP with `make candidate-package VERSION=1.0.0-dev OUTPUT_DIR=/owned/path`
on Apple Silicon; see the [installation guide](docs/USER-GUIDE.md). This is not
candidate-ready until the full completion gates and independent review pass.

The current engine compiles 1–100 approved Git repository mirrors or clean local
Git workspaces into canonical Knowledge IR. Linux and other platforms are development
smoke environments; they do not verify the supported Apple Silicon product.

It compiles an approved repository catalog into deterministic source, symbol,
relationship, claim, and evidence records. It exposes them through bounded
stdio MCP tools.

The browser UI remains a loopback-served frontend. Knowledge IR is the product
and authority; FTS, graph, structural, vector, cache, and UI data are
rebuildable projections. V1 includes deterministic, bounded retrieval query
planning. Live connectors, LLM reasoning, agent/task planning and execution,
agents, capability inventory, and recommendations remain outside its scope.

Vector retrieval is optional and local-only. Enable it with
`"vector":{"enabled":true}`; it is a retrieval aid, never evidence. Rebuild
it with `projections rebuild --kind vector --config catalog.json --data-dir DATA`,
or clear it with `projections reset --kind vector --data-dir DATA`.
The macOS arm64 binary embeds the pinned Nomic v1.5 model (~262 MiB) and its
local llama.cpp embedding helper, so it has no runtime download or service
dependency. This materially increases binary size.
Install Git LFS and run `git lfs pull` after cloning to retrieve the model.

## Commands

```sh
go run ./cmd/aios catalog validate --config catalog.json
go run ./cmd/aios mirrors sync --registry mirrors.json --data-dir /absolute/data
go run ./cmd/aios ingest --all --config catalog.json --registry mirrors.json --data-dir /absolute/data
go run ./cmd/aios ingest --config catalog.json --registry mirrors.json --data-dir /absolute/data --repo frontend
go run ./cmd/aios local ingest --all --config catalog.json --registry local.json --data-dir /absolute/data
go run ./cmd/aios status --data-dir /absolute/data
go run ./cmd/aios projections rebuild --data-dir /absolute/data
go run ./cmd/aios projections rebuild --kind vector --config catalog.json --data-dir /absolute/data
go run ./cmd/aios benchmark --fixture scripts/benchmark/fixtures/retrieval-v1.json --data-dir /absolute/benchmark-data
go run ./cmd/aios serve --config catalog.json --data-dir /absolute/data
go run ./cmd/aios ui serve --config catalog.json --data-dir /absolute/data
make acceptance-v1 ACCEPTANCE_OUTPUT=/private/acceptance-run
```

`ingest --all` is the atomic initial catalog bootstrap. Use `ingest --repo ID`
for later revision deltas. The acceptance target creates 25 local fixture
remotes and runs the deterministic core on every supported platform; macOS
arm64 additionally exercises the optional bundled vector runtime. Fixture
acceptance proves the deterministic workflow. The assembled candidate requires all
[completion gates](.codex/completion/ACCEPTANCE.md), including native installed-artifact
evidence; see [the acceptance handoff](docs/acceptance-handoff.md).

`serve` provides read-only `kb.*` MCP operations, including deterministic bounded
query planning through `kb.query` and bounded architecture evidence through
`kb.slice`. The query planner selects and executes fixed retrieval operators
over active canonical evidence under explicit budgets; it does not generate
task plans or run agents. See
[`docs/reference/repository-knowledge-v1.md`](docs/reference/repository-knowledge-v1.md).

MCP responses use the V1 provenance envelope. Payload fields are nested under
`result`; use `result_handle` with `kb.explain` to inspect its bounded
canonical evidence chain.

## Roadmap boundary

V1 is deterministic repository knowledge only. Connected capability inputs,
bundled LLM reasoning, agent/task planning, action execution, subagents, and
autonomous reasoning are outside this release. The `planning` and `execution`
entries in `disabled_capabilities`, reported by CLI `status` and `kb.status`,
describe these excluded agent capabilities; deterministic query planning
remains available.

## Development harness

The approved installable V1 completion mission is described in
[the whole-product harness](.codex/completion/README.md). It covers the generic
Apple Silicon daemon, onboarding, maintained repositories, polished volumetric UI,
Benchmark Lab, explicit atomic upgrades and local redacted diagnostics. These are
completion requirements, not claims that all features are currently implemented.

See [the repository-local Codex harness](.codex/README.md) for repeatable setup,
validation, a dependency-ordered product task queue, persistent plans, and an
optional one-task CLI launcher. Start with `make harness-test`.

### Benchmark Lab

Open **Benchmark Lab** in the live UI for isolated Demo comparisons or saved My
Knowledge known-answer regressions. The [method and limits](docs/how-to/benchmark-lab.md)
explain immutable snapshot parity, literal-baseline semantics, outcome scoring
and export contents.

### Repository management

Open **Manage repositories** in the local UI to approve another source in the configured Direct or Mirror mode, change include/exclude scope, retry source access, or force a fresh compilation. Repository health shows active and attempted revisions, last success and next check. A whole **Rebuild knowledge base** keeps identity and approved configuration, and serves last-good knowledge until every replacement generation validates and activates together. A failed build keeps the previous catalog.

**Remove** asks for confirmation and cancels active work before purging that repository's owned knowledge, staged/retired history, snapshots and managed mirror. Shared derived caches and source-bearing migration backups are discarded. The original Git workspace is preserved. To withdraw an ID, use Remove before omitting it from the batch setup. If cleanup cannot complete, a durable removal record pauses evidence reads and maintenance; correct the owned storage issue and use **Finish removal**, or restart to retry recovery. Removed repositories cannot be retried until explicitly approved again.
