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

Candidate `102b8eef5d47f38451086770c6bb5ab65b84b589` passed clean-tree scoped core, web (36/36 browser tests), and retrieval gates on Darwin arm64. Receipts are `.codex/completion/evidence/10-initial-{core,web,retrieval}.json`. The independent reviewer approved the 187-case/25-repository gold corpus after auditing source spans/predicates and independently running Go, benchmark and browser checks.

The first broad acceptance run at `/tmp/aios-task10-accept-102b8ee-a/acceptance-report.json` failed architecture_slice only; it is not passing evidence. Delivery traced structural coverage treating lexical-only configuration `orders-service/build.gradle` as unsupported source syntax. A bounded repair will distinguish configuration from unsupported source files (including proto), then rerun affected gates and two fresh acceptance runs on the corrected SHA.

## Independent review

Reviewer `/root/investigation_search_acceptance` found and verified repairs for typed intent coverage, sentence unknowns, source/predicate scoring, background stale handling, relationship citations and keyboard behavior. Final review is pending the architecture-slice regression repair and exact-commit gate/paired acceptance evidence.

## Blockers

Internal regression under repair: architecture_slice acceptance failed on 102b8ee; configuration coverage must not imply missing structural source extraction. No external blocker established.

## Handoff

Persist implementation SHA, publication evidence and next work. Continue mission.

## Resumption checkpoint

Coverage repair is `8857ec0c16838301c2290c51a6bde7729f72f996`; current tested HEAD is `4f4194f481acfca8cfcad23f5b03ec689fda6aa4` (only evidence/plan added after product repair). Independent review verified configuration-vs-unsupported-source coverage boundary. Both fresh acceptance reports `/tmp/aios-task10-accept-8857ec0-a/acceptance-report.json` and `/tmp/aios-task10-accept-4f4194f-b/acceptance-report.json` record exact HEAD4f4194f, accepted14/14, semantic fingerprint `6c2507e8994d4d53e0ec7fb25cf5eed8ad3179b9915bdac3a6517a584d9ffcd0`. Final web gate has stalled under both Homebrew Node26 and configured Node22; Node26 alone is ruled out. No passing final receipt is claimed. Delivery owns live-run diagnosis and bounded serial runner recovery; reviewer awaits final receipts before verdict. Keep milestone in_progress; final candidate milestones11–15 remain pending.

Runner recovery commit `4092163670030770823585e372c7370e7c01f4fe` adds validated worker count and15minute global deadline, independently reviewed statically. Full serial web gate is live at `/tmp/aios-task10-web-4092163.json` (log `-0.log`, PID70394 at last observation) using Node22/Python3.12/port4174/workers1. It is now producing actual named failures: mirror maintenance and local repository management, while local maintenance passed. Delivery owns traces and precise diagnosis after terminal run; no pass receipt exists yet. Preserve unrelated localhost4173 preview. No publication or completion status until these are repaired and exact-commit gates/review pass.

## Serial web diagnosis (2026-10-07)

The run at `/tmp/aios-task10-web-4092163.json` terminated with exit 2 after 437.9 seconds: 29/36 passed, 7 maintenance/repository-management cases failed waiting 30 seconds for initial jobs to become idle. The receipt reports `clean_product_tree:false` and is diagnostic only. Delivery observed actual host battery power; native resource policy bounds deferral at 5 minutes, beyond the 30 second fixture polls. These browser fixtures test maintenance/management, so delivery is adding an explicit AC-power signal on a private PATH for their owned backend. Production native policy and native resources acceptance remain unchanged. Independent reviewer is auditing this boundary. New source requires committed clean exact-SHA core/web/retrieval receipts and paired acceptance before readiness.
