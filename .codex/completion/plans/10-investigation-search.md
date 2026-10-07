# Complete Spotlight and portable investigation payload

Requirements: R15, R16, R17

## Acceptance

- Question/symbol/path/log/event/route/config families have reviewed gold results and honest unknowns.
- Slash/@ commands, keyboard/cancel/focus/history/clear plus stale/truncated/unsupported UI states tested.
- Versioned compact Copy payload matches canonical UI evidence, source locations and generation/coverage metadata.
- No external model calls; unsupported depth disclosed; source copy and diagnostic privacy remain distinct.

## Decisions

The approved product contract requires a deterministic, versioned investigation payload and actual Spotlight UI behavior; no runtime model calls are in scope. Preserve the canonical IR/planner evidence semantics and keep diagnostic redaction separate from user-requested source copy. Task 09 was published and its completion bookkeeping verified before this task started.

## Progress

Started 2026-10-07 on dedicated branch `codex/completion-investigation-search`, based on fetched `origin/main` at `c9637783e9cca819b3e2c0afb683683e63b92f8b`. Delivery owner is conducting a current implementation/gap audit and will own code and tests. Manager owns task/plan/evidence integration. Independent reviewer will inspect the corrected committed result and evidence before readiness.

## Scenario scope

Run existing core/web gates plus scoped product scenarios with `gate NAME --milestone 10-investigation-search`. Full assembled/native gates are final-candidate requirements. Record deferred native checks explicitly.

## Actual validation

Final tested source: `4d176cdd4587381d4491526dc09137b40631a77d`, clean Darwin arm64, Node 22.22.1 and Python 3.12.13. Exact-source receipts `.codex/completion/evidence/10-{core,web,retrieval}.json` all pass with exit 0. Core includes harness/Python contracts, Go tests/vet/race/build. Web includes 27/27 unit tests, production assets and 36/36 browser tests; port 4174 and one worker preserve the preexisting port 4173 preview. Retrieval executes scoped product assertions and the independently reviewed 187-case, 25-repository corpus.

Two fresh acceptance runs, `/tmp/aios-task10-accept-4d176cd-{a,b}/acceptance-report.json`, accepted all 14 cases, including architecture slice, negative/unknown/stale and corruption recovery. Both record the exact tested source and fingerprint `6c2507e8994d4d53e0ec7fb25cf5eed8ad3179b9915bdac3a6517a584d9ffcd0`. Redacted summary/digests: `.codex/completion/evidence/10-acceptance.json`; source-bearing reports remain outside the checkout.

Historical failures and recovery below are not final passing proof. The rejected retrieval dispatch on 4f81e87 executed zero scenarios and reported ValueError. Its precise rejected binary metadata was not retained; rebuilding clean at 4d176cd resolved the provenance rejection. Do not claim a proven exact cause.

## Independent review

Reviewer `/root/investigation_search_acceptance` independently audited typed coverage, source/predicate gold scoring, relationship citations, keyboard behavior, stale invalidation, coverage repair and the child-local synthetic AC fixture. The reviewer verified final gate source/platform/cleanliness and raw-log hashes; final report `.codex/completion/evidence/10-review.json` passes exact source with no blocking findings. Independent targeted Go/browser checks and fresh gold scoring passed. Raw hybrid precision@1 is 0.134: evidence/state success does not imply the first raw ranked hit is usually the answer.

## Blockers

No external blocker. Task 10 completed after independent pass and verified normal publication. Native final installed-product and GPU acceptance remain task 15 obligations; scoped fixture checks do not prove them.

## Handoff

Branch `codex/completion-investigation-search`; implementation/tested source `4d176cdd4587381d4491526dc09137b40631a77d`. Only scoped task 10 product, fixture, plan and evidence changes are included. Publish after independent pass, verify fetched main, then record completion in follow-up bookkeeping. Continue approved mission with task 11 after dependency publication.

## Resumption checkpoint

Coverage repair is `8857ec0c16838301c2290c51a6bde7729f72f996`; current tested HEAD is `4f4194f481acfca8cfcad23f5b03ec689fda6aa4` (only evidence/plan added after product repair). Independent review verified configuration-vs-unsupported-source coverage boundary. Both fresh acceptance reports `/tmp/aios-task10-accept-8857ec0-a/acceptance-report.json` and `/tmp/aios-task10-accept-4f4194f-b/acceptance-report.json` record exact HEAD4f4194f, accepted14/14, semantic fingerprint `6c2507e8994d4d53e0ec7fb25cf5eed8ad3179b9915bdac3a6517a584d9ffcd0`. Final web gate has stalled under both Homebrew Node26 and configured Node22; Node26 alone is ruled out. No passing final receipt is claimed. Delivery owns live-run diagnosis and bounded serial runner recovery; reviewer awaits final receipts before verdict. Keep milestone in_progress; final candidate milestones11–15 remain pending.

Runner recovery commit `4092163670030770823585e372c7370e7c01f4fe` adds validated worker count and15minute global deadline, independently reviewed statically. Full serial web gate is live at `/tmp/aios-task10-web-4092163.json` (log `-0.log`, PID70394 at last observation) using Node22/Python3.12/port4174/workers1. It is now producing actual named failures: mirror maintenance and local repository management, while local maintenance passed. Delivery owns traces and precise diagnosis after terminal run; no pass receipt exists yet. Preserve unrelated localhost4173 preview. No publication or completion status until these are repaired and exact-commit gates/review pass.

## Serial web diagnosis (2026-10-07)

The run at `/tmp/aios-task10-web-4092163.json` terminated with exit 2 after 437.9 seconds: 29/36 passed, 7 maintenance/repository-management cases failed waiting 30 seconds for initial jobs to become idle. The receipt reports `clean_product_tree:false` and is diagnostic only. Delivery observed actual host battery power; native resource policy bounds deferral at 5 minutes, beyond the 30 second fixture polls. These browser fixtures test maintenance/management, so delivery is adding an explicit AC-power signal on a private PATH for their owned backend. Production native policy and native resources acceptance remain unchanged. Independent reviewer is auditing this boundary. New source requires committed clean exact-SHA core/web/retrieval receipts and paired acceptance before readiness.

## Corrected final validation

Fixture repair `4f81e8771d555d0e267790153da00d8d2ce1ee71` passed the focused maintenance/management suite (8/8). Final clean web receipt `.codex/completion/evidence/10-web.json` passes on that exact source, Darwin arm64: 36/36 browser tests, exit 0, 184.6 seconds. Independent reviewer verified exact source, cleanliness and raw-log digest `45bdf45a6974b6c20a00cb95bee17536d44b2b7f645e7124cd07804c6708ef6f`. Core, retrieval, paired acceptance and final review remain pending. Synthetic AC fixture evidence is not native power-signal certification.

Ready-state bookkeeping checks: `python3 scripts/completion_harness.py check` and `make harness-test` passed (exit 0), log `/tmp/aios-task10-ready-harness.log`. Synthetic failure probes inside harness regressions are expected test fixtures, not final product failures.

Publication: normal fast-forward `git push origin HEAD:main` published validated task 10 in `6e16126`; fetched origin/main verified that head and reviewed implementation `4d176cdd4587381d4491526dc09137b40631a77d` as an ancestor. Follow-up bookkeeping records task completion and retains the independently reviewed implementation SHA. Next eligible milestone: 11-volumetric-cloud.
