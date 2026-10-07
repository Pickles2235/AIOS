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
