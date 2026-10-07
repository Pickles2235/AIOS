# Consultancy operating state

## Current outcome

Finish the approved installable V1 completion mission. Acceptance is the full 15-milestone harness, all R01–R26 requirements, final native Apple Silicon gates, checksummed installed candidate, independent review, and `completion_harness.py accept` passing. Public release remains the stakeholder's decision.

## Decisions and constraints

- Follow `.codex/completion/PRODUCT.md`, `MISSION.md`, `ACCEPTANCE.md`, `tasks.json`, and each milestone plan.
- Preserve canonical IR, evidence semantics, browser security, source nonmutation, and truthful native evidence. Never claim a deferred check passed.
- Task 09 implementation `f28e413a8637d4561d358ff659ade3df90b5116c` was published in `50211f88ec62fc066b121d42212b87a96b7dac90`; follow-up completion bookkeeping is `c9637783e9cca819b3e2c0afb683683e63b92f8b`.
- Task 10 runs on `codex/completion-investigation-search`, from fetched main `c9637783e9cca819b3e2c0afb683683e63b92f8b`.

## Team map

| Owner | Level and role | Scope | Status |
|---|---|---|---|
| `/root` | Manager / integration, senior | Mission scope, plan/state, integration, publication, final sign-off | Active |
| `/root/investigation_search_delivery` | Senior delivery lead; `gpt-6-sol` / `medium` / `fork_turns=none` | Task 10 implementation and exact-SHA gates; current source `4092163`, deterministic browser power fixture repair in progress | Active |
| `/root/investigation_search_acceptance` | Senior independent reviewer; `gpt-6-sol` / `high` / `fork_turns=none` | Task 10 independent source/corpus review; repairs and gold audited, final corrected-SHA gates pending | Active |
| `/root/cloud_gap_audit` | Principal read-only audit; `gpt-6-sol` / `high` / `fork_turns=none` | Task 11 baseline/gap analysis, no writes; completed audit, delivery dependency-blocked |
| Reserved fresh agents | Task 11 delivery and independent acceptance | New assignment after task 10 is reviewed and published | Pending dependency |

## Efficiency and workflow

Keep one owner per write scope. Share existing durable evidence paths, not transcripts or private raw logs. Coordinate full gates to avoid duplicate expensive runs; use fresh configured agents for distinct assignments and a reviewer independent of implementation. Live token/cost telemetry is unavailable in this thread; no usage-efficiency claims are made.

## Workflow candidates and lessons

The completion mission and consultancy workflow have been applied to task 09 with an independent exact-commit review and recorded scoped gates. Do not create a skill from this single mission. Continue recording only stable lessons demonstrated by repeated work.

Task 11 audit: live `web/src/knowledge-cloud.tsx` is a 2D SVG ring wired by `main.tsx`; canonical evidence handles are generation-bound, but `internal/knowledge/read.go` projects only one repository/20 nodes and 21 globally selected claims. It has no estate/package/file hierarchy, exact aggregate membership/counts, volumetric controls, semantic legend, or source actions. Discovery/staging events exist, but not cloud transitions, promotion/delta motion, or query highlighting; ranked results are not proof of relationship paths. Before task11 implementation, define an active-catalog read API with exact member IDs/count scope/generation and bounded expansion; derive aggregates from canonical IR and source files; validate source-action path/revision on the backend; animate only real activation diffs and describe search hits as results unless a path is proven. Tests need exact membership, promotion/removal/stale cases, safe links, keyboard/reduced motion/context loss, dense data; native GPU evidence must measure rendering under load and include visual review. No task11 code/tests were changed or run by the auditor.

## Task 10 recovery checkpoint

Current source `4092163670030770823585e372c7370e7c01f4fe` includes typed coverage, faithful relationship citations, independently reviewed gold cases, stale evidence handling, configuration coverage repair and bounded Playwright runner. The final serial web run failed seven maintenance/management cases: actual battery power deferred initial jobs beyond the fixture poll deadline. Delivery owns a private AC-power command fixture for only those disposable test backends. Reviewer independently audited its isolation and retained behavioral assertions; this synthetic fixture is not native power evidence. Production resource policy remains unchanged. Milestone stays in progress until clean exact-SHA core/web/retrieval gates, paired acceptance and final independent review pass. Prior passing reports and rejected/stalled runs remain historical evidence.
