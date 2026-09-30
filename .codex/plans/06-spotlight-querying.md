# 06-spotlight-querying: Complete Spotlight-first querying and truthful knowledge-cloud feedback

Status: ready. See the queue for authoritative status and dependencies.

## Baseline and resumption

Seeded from c285773. No product implementation has been performed by the harness.
Read AGENTS.md and the queue; fetch origin/main and verify completed predecessor
commits before starting. Reinspect current code because predecessors change it.

web/src/components.tsx and graph helpers are separate from main.tsx. Current query notices do not clearly surface unknown. Existing browser tests exercise the simpler live evidence UI; graph component/layout unit tests do not establish live Spotlight/cloud wiring. Preserve those live browser contracts while extending coverage.

## Decisions

Initial design guidance above is provisional until checked against current code.
Record each decision, alternatives, compatibility implications and reason here.
No unresolved public contract decision is currently blocking harness setup.

## Progress

- Harness seed only; task is not started.
- Next: verify eligibility, inspect scoped code/tests/docs, mark in_progress,
  implement the smallest complete change satisfying queue acceptance.

## Actual validation

Not run for this product task. Record command, environment, exit/result, artifact
path and any failure here. Distinguish passed, failed, and not run; include reasons
and native/private-corpus limitations. Harness validation belongs in setup.md.

## Blockers and recovery

None assessed yet. Add exact symptoms and a concrete resume instruction if blocked.

## Handoff

No task commit yet. Record branch, implementation commit, inspected diff summary,
acceptance evidence, publication result, remaining limitations and next action.
Publish validated scoped commits directly to main with a normal fast-forward
push, then verify origin/main contains them. Set completed and completion_commit
in a follow-up bookkeeping commit only after that verification. Stop after this
task. Publishing failures leave the task ready with exact recovery instructions.

## Session design

Wire native dialog Spotlight (Cmd/Ctrl+K, Escape, keyboard results, focus restoration)
and an SVG cloud of actual active projection entities/claims into main.tsx. Retain
the simple evidence list and search. Render generation/commit/hash/coverage/bounds
from server data; stale handles revalidate before evidence. Fix the existing browser
query service to require complete coverage for not_found, without expanding query
inputs or adding model/agent behavior. Add coverage metadata and bounded trace states.

## Validated implementation handoff

Live main.tsx now offers native-dialog Spotlight with Cmd/Ctrl-K, Escape, keyboard
selection and focus restoration, while retaining the ordinary search/entity list.
The SVG cloud draws only current-page canonical entities and claims, labels counts,
generation and bounds, and never invents endpoints. Repository selection and
replacement pagination preserve generation boundaries; stale cursors are rejected.
Evidence revalidates active handles and shows commit/hash/generation/lines, clearing
old excerpts on failures. Queries report distinct empty/unsupported/projection
outage/coverage unknown/covered absence/bounded-positive states. Negative answers
require complete applicable coverage; output is capped and reads have a time budget.
GET session recovery uses an existing HttpOnly session, no-store responses and
same-origin validation; one-use capabilities and mutation CSRF checks remain.
Generated embedded assets match the live source. Scoped diff inspected.

Validation on Linux amd64, Go 1.26.1, JDK 21, Node 24, Python 3.12.14:
- make harness-validate passed (queue/Python, vet, all Go tests, race, binary build).
  Log: /workspace/scratch/06-core.log.
- PLAYWRIGHT_CHANNEL=chromium make harness-validate-web passed: 27 unit tests and
  16 browser cases at 1440 and 2560, including real mirrored/local first run,
  Spotlight focus/results/evidence and session refresh. Log: 06-web-final.log.
  Initial browser run failed on ambiguous test selectors; corrected dialog scope.
- go test ./internal/planner ./internal/knowledge ./internal/mcp ./internal/webui
  passed, including coverage gaps, outage, stale cursor and session security cases.
  Log: /workspace/scratch/06-targeted-final.log.
- Fresh make acceptance-v1 ACCEPTANCE_OUTPUT=/workspace/scratch/06-acceptance-a
  and /workspace/scratch/06-acceptance-c accepted with identical fingerprint:
  f75584b3be0c5e3750e72144fc072c55af56235e37d8218a39bf2ff8ef7656ea.
  The second attempt at 06-acceptance-b was interrupted during environment
  transition before producing a report; it was replaced with fresh run c.
- git diff --check passed.

Remote native run 36760467766 (task 05 bookkeeping) exposed macOS temp-directory
alias fixture failures. Repairing and verifying native fixtures belongs to task 07;
Linux gates do not establish manual macOS UX or private-estate acceptance. No such
claim is made here. User authorized continuation through the entire task queue.
