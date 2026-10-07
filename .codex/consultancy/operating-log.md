# Consultancy operating state

## Current outcome

Finish the approved installable V1 completion mission. Acceptance is the full 15-milestone harness, all R01–R26 requirements, final native Apple Silicon gates, checksummed installed candidate, independent review, and `completion_harness.py accept` passing. Public release remains the stakeholder's decision.

## Decisions and constraints

- Follow `.codex/completion/PRODUCT.md`, `MISSION.md`, `ACCEPTANCE.md`, `tasks.json`, and each milestone plan.
- Preserve canonical IR, evidence semantics, browser security, source nonmutation, and truthful native evidence. Never claim a deferred check passed.
- Task 09 implementation `f28e413a8637d4561d358ff659ade3df90b5116c` was published in `50211f88ec62fc066b121d42212b87a96b7dac90`; follow-up completion bookkeeping is `c9637783e9cca819b3e2c0afb683683e63b92f8b`.
- Task 10 reviewed source `4d176cdd4587381d4491526dc09137b40631a77d` is published and completion bookkeeping verified in `0c3fba23fd64b68e5ab548779dcb45c40ffd3413`. Task 11 now runs on `codex/completion-volumetric-cloud` from that fetched main.

## Team map

| Owner | Level and role | Scope | Status |
|---|---|---|---|
| `/root` | Manager / integration | Scope, plans/evidence, publication and mission sign-off | Active |
| `/root/volumetric_cloud_delivery` | Principal; `gpt-6-astra` / `medium` / `fork_turns=none` | Task 11 API, renderer, safe source actions and motion integration; may delegate one bounded contributor | Active |
| `/root/volumetric_cloud_acceptance` | Principal; `gpt-6-astra` / `medium` / `fork_turns=none` | Independent R18-R20 evidence, security and native rendering acceptance | Active |
| `/root/volumetric_cloud_delivery/cloud_backend` | Senior backend contributor; `gpt-6-sol` / `medium` / `fork_turns=none` | Go-only canonical cloud API, membership and safe source actions; reports to delivery lead | Active |
| Completed task 10 delivery/review | Senior delivery and independent review | Task 10 passed and published; reviewed source4d176cd, publication6e16126, completion0c3fba2 | Completed |
| Completed cloud gap auditor | Principal read-only audit | Task 11 current gap map, retained below | Completed |

Staffing rationale: task 11 spans canonical aggregate semantics, renderer behavior, source-action security and native GPU evidence, requiring principal integration judgment and separate principal acceptance. One delivery lead keeps these contracts coordinated; one optional disjoint contributor is allowed only after the lead defines interfaces. Manager retains plan/state/evidence writes.

## Efficiency and workflow

Keep one owner per write scope. Share existing durable evidence paths, not transcripts or private raw logs. Coordinate full gates to avoid duplicate expensive runs; use fresh configured agents for distinct assignments and a reviewer independent of implementation. Live token/cost telemetry is unavailable in this thread; no usage-efficiency claims are made.

## Workflow candidates and lessons

The completion mission and consultancy workflow have been applied to task 09 with an independent exact-commit review and recorded scoped gates. Do not create a skill from this single mission. Continue recording only stable lessons demonstrated by repeated work.

Task 11 audit: live `web/src/knowledge-cloud.tsx` is a 2D SVG ring wired by `main.tsx`; canonical evidence handles are generation-bound, but `internal/knowledge/read.go` projects only one repository/20 nodes and 21 globally selected claims. It has no estate/package/file hierarchy, exact aggregate membership/counts, volumetric controls, semantic legend, or source actions. Discovery/staging events exist, but not cloud transitions, promotion/delta motion, or query highlighting; ranked results are not proof of relationship paths. Before task11 implementation, define an active-catalog read API with exact member IDs/count scope/generation and bounded expansion; derive aggregates from canonical IR and source files; validate source-action path/revision on the backend; animate only real activation diffs and describe search hits as results unless a path is proven. Tests need exact membership, promotion/removal/stale cases, safe links, keyboard/reduced motion/context loss, dense data; native GPU evidence must measure rendering under load and include visual review. No task11 code/tests were changed or run by the auditor.

## Task 10 recovery checkpoint

Current source `4092163670030770823585e372c7370e7c01f4fe` includes typed coverage, faithful relationship citations, independently reviewed gold cases, stale evidence handling, configuration coverage repair and bounded Playwright runner. The final serial web run failed seven maintenance/management cases: actual battery power deferred initial jobs beyond the fixture poll deadline. Delivery owns a private AC-power command fixture for only those disposable test backends. Reviewer independently audited its isolation and retained behavioral assertions; this synthetic fixture is not native power evidence. Production resource policy remains unchanged. Recovery completed: clean exact-SHA core/web/retrieval gates and paired 14-case acceptance passed at 4d176cd; independent report10-review.json passed, and milestone10 was published/completed. Prior passing reports and rejected/stalled runs remain historical evidence.

Task 11 initial architecture: canonical cloud API pinned to an active generation set; bounded exact hierarchy membership/pages and claim-backed edges; actual WebGL perspective/depth with deterministic positions; DOM evidence alternative and backend validated source actions. Path-derived directories must be labelled as directories rather than inferred modules. Delivery lead owns UI/integration and will staff one fresh backend contributor with a disjoint contract. Reviewer owns independent criteria/evidence checks.

Task 10 bookkeeping validation: default macOS Python 3.9 `make harness-test` returned exit 2 solely from TemporaryDirectory cleanup (`Directory not empty: info`) in a synthetic harness regression after its behavior assertions. Rerun with Python 3.12 passed exit 0 (11 legacy and 14 completion harness tests), log `/tmp/aios-task10-completed-harness-python312.log`. The rejected cleanup run is retained at `/tmp/aios-task10-completed-harness.log`; no claim that it passed. Use explicit Python 3.12 for subsequent manager checks.

Task 11 interface clarification: total_claims describes the complete pinned scope; visible edge_count describes only the returned page, with omissions disclosed. Extracted package/module declarations take precedence; directory fallbacks are explicit. Reviewer requires API-to-IR reconciliation and actual selection/control tests. Native dense GPU proof must rotate/draw actual dense symbol data and retain rendered captures, declared hardware/renderer and target misses; an idle animation-frame measurement is insufficient.

Task 11 integration audit: first exact-SHA core/cloud passed at21c1d60 but independent review blocked stale inspector/query when selected file disappears. Delivery is fixing that exact recovery path, preserving a real browser deletion regression, before collecting corrected evidence. Scope cloud excludes native GPU; standalone native evidence needs retained captures plus actual Finder and source nonmutation proof. Gate provenance rule: await clean build terminal and verify exact Go VCS revision/modified=false before dispatch; no simultaneous dependent builds or any-file writes/HEAD changes during final checks.
