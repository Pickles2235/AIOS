# 01-query-planning-docs: Clarify deterministic query planning versus agent/task planning

Status: ready. See the queue for authoritative status and dependencies.

## Baseline and resumption

Cloned Pickles2235/AIOS at 63e34d7 and fetched origin/main. The queue selected
01-query-planning-docs; it has no predecessors. The requested launcher created
branch codex/01-query-planning-docs-115091ddc531 but exited 1 before edits because
the nested Codex CLI authentication was expired (HTTP 401). Resumed the exact
printed task prompt in the active session without changing authentication.
Launcher artifacts: .git/aios-codex-runs/01-query-planning-docs-115091ddc531/.

## Decisions

- Keep this task documentation-only. Preserve query behavior, protocol fields,
  disabled-capability keys, feature flags, module path, and the documentation layout.
- Describe internal/planner as deterministic bounded retrieval query algebra.
  Agent/task planning, LLM reasoning, action execution, and autonomous agents
  remain outside V1; executing retrieval operators does not provide a task runtime.
- Explain the existing planning/execution outside_v1 labels rather than rename
  them. Checked internal/store/store.go and the MCP status contract test.
- Verify statements against planner algebra, classifier, request/stage selection,
  MCP query execution and tests, and CLI wiring. Fix the reference's claim that
  vector is disabled in V1 to describe its existing opt-in behavior.

## Progress

- Read AGENTS.md, .codex/README.md, queue, plan, handoff template, scoped docs,
  planner implementation/tests, MCP implementation/tests, and CLI wiring.
- Verified eligibility with python3 scripts/codex_harness.py next and prompt.
- Marked in_progress before implementation. Updated README, architecture,
  retrieval explanation, and the MCP/product references to distinguish bounded
  query planning from excluded agent/task planning and execution.
- Clarified existing disabled-capability labels and local embedding retrieval.
  No source code, protocol, query behavior, flags, or generated assets changed.
- Inspected the complete scoped diff; all required checks passed. Marked ready.

## Actual validation

Linux amd64, Python 3.12.14, Go 1.26.1, JDK 21.0.12.1. Installed the real Go
language toolchain and JDK under /workspace/toolchains outside the repository.
All commands used PATH=/workspace/toolchains/go/bin:/workspace/toolchains/jdk/bin:$PATH.

- make harness-test: passed (queue valid, 11 offline harness regression tests).
- python3 -m unittest discover -s scripts -p 'test_*.py': passed (25 tests).
- go test ./internal/planner ./internal/mcp ./cmd/aios: passed (all three packages).
- git diff --check: passed.
- Reviewed claims against internal/planner/{algebra,classify,request}.go,
  their adjacent tests, internal/mcp/v1.go and v1_test.go, store Diagnostics,
  and cmd/aios/main.go. Existing tests cover deterministic stage precedence,
  inspectable query plans, bounded results, canonical evidence, and the unchanged
  disabled_capabilities.planning status contract.

The task is documentation-only. Full harness-validate, web, ingestion acceptance,
native macOS, and private-estate checks were not run; no runtime or UI changes
require those gates for this task. Validation output is in the active session;
only the failed launcher generated repository-local run artifacts.

## Blockers and recovery

The optional nested launcher could not authenticate; its exact prompt was
completed in the active session. No implementation or validation blocker remains.
Publication must be verified before recording completed/completion_commit.

## Handoff

Task ID: 01-query-planning-docs.
Local branch: codex/01-query-planning-docs-115091ddc531.
Implementation commit: pending commit/publication.

Result and scope: README and architecture/reference explanations now distinguish
deterministic bounded retrieval planning from excluded agent/task planning.
Existing planning/execution status fields retain their protocol meaning; the
reference now agrees with the implemented optional local vector route. Acceptance
is supported by the code review and passing planner/MCP/CLI and Python checks.
The final diff contains only five documentation files, this plan, and task status.

Publication and resumption: ready for a normal fast-forward push after fetching
origin/main. Set completed and record the full implementation SHA only after
verification, then publish a follow-up bookkeeping commit. Stop after this task;
do not start the next queue entry.
