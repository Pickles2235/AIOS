# AIOS Repository Knowledge Engine

This is a local-first, read-only repository knowledge engine. V1 compiles
exactly 25 approved Git repository mirrors into canonical Knowledge IR.

It compiles an approved repository catalog into deterministic source, symbol,
relationship, claim, and evidence records. It exposes them through bounded
stdio MCP tools.

The browser UI remains a loopback-served frontend. Knowledge IR is the product
and authority; FTS, graph, structural, vector, cache, and UI data are
rebuildable projections. V1 has no live connectors, LLMs, planning, execution,
agents, capability inventory, or recommendation surface.

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
acceptance proves the deterministic workflow, not readiness of the real
25-repository estate; see [the acceptance handoff](docs/acceptance-handoff.md).

`serve` provides read-only `kb.*` MCP operations, including bounded hybrid
planning through `kb.query` and bounded architecture evidence through
`kb.slice`. See
[`docs/reference/repository-knowledge-v1.md`](docs/reference/repository-knowledge-v1.md).

MCP responses use the V1 provenance envelope. Payload fields are nested under
`result`; use `result_handle` with `kb.explain` to inspect its bounded
canonical evidence chain.

## Roadmap boundary

V1 is deterministic repository knowledge only. Connected capability inputs,
bundled LLM reasoning, planning, execution, subagents, and autonomous reasoning
are outside this release.

## Development harness

See [the repository-local Codex harness](.codex/README.md) for repeatable setup,
validation, a dependency-ordered product task queue, persistent plans, and an
optional one-task CLI launcher. Start with `make harness-test`.
