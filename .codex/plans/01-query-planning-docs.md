# 01-query-planning-docs: Clarify deterministic query planning versus agent/task planning

Status: completed. See the queue for authoritative status and dependencies.

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
- Committed the implementation as dd3ec9f0a1858eaa0330d941e1bebf215a06a09d.
  Verified that fetched origin/main contains that exact commit, then recorded
  completed and completion_commit in this follow-up bookkeeping change.

## Actual validation

Linux amd64, Python 3.12.14, Go 1.26.1, JDK 21.0.12.1. Installed the real Go
language toolchain and JDK under /workspace/toolchains outside the repository.
Required validation commands used PATH=/workspace/toolchains/go/bin:/workspace/toolchains/jdk/bin:$PATH.

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

The nested launcher authentication failure was bypassed by completing its
printed prompt in the active session. Plain Git push lacked credentials; a
command-scoped gh credential helper reached GitHub but returned HTTP 401. The
authenticated gh API succeeded. Published exact Git blobs/tree/commit, preserving
local SHA, parent, message, author/committer, and timestamps, then updated main
with force=false after rechecking its head. No remaining implementation,
validation, or publication blocker.

If Git transport still fails for future tasks, use the authenticated GitHub Git
API with exact-object/hash checks and force=false; fetch to verify publication.
The external /workspace/scratch/publish_aios_commit.py helper was used for this
single-parent documentation commit and is not a product or harness change.

## Handoff

Task ID: 01-query-planning-docs.
Local branch: codex/01-query-planning-docs-115091ddc531.
Implementation commit: dd3ec9f0a1858eaa0330d941e1bebf215a06a09d.

Result and scope: README and architecture/reference explanations now distinguish
deterministic bounded retrieval planning from excluded agent/task planning.
Existing planning/execution status fields retain their protocol meaning; the
reference now agrees with the implemented optional local vector route. Acceptance
is supported by the code review and passing planner/MCP/CLI and Python checks.
The final diff contains only five documentation files, this plan, and task status.

Publication and resumption: fetched origin/main before publishing and confirmed
it matched the implementation parent (63e34d763f4f8553baa036b064e57543c6c6ffe3).
Normal Git transport failed as described above. The authenticated GitHub Git API
published the exact implementation with tree 3313cf90aea3c4eb69c4beb1f7ba9cd0a575ae8b
and a non-forced fast-forward ref update. A subsequent git fetch origin main
and git merge-base --is-ancestor confirmed the implementation on origin/main.
Task status and its full implementation SHA are recorded in this follow-up
bookkeeping commit, to be published by the same verified API path if Git
transport continues to fail. make harness-test passed again for this queue
update (11 tests, valid completed status and implementation SHA).

Final handoff: both acceptance criteria are satisfied by the scoped docs and
code-grounded review; all required checks passed. The selected task is completed.
No runtime behavior changed. Stop here; do not start another queue entry.
