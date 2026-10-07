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

Record commands, receipts, platform and full tested commit. No checks run yet.

## Independent review

Separate reviewer report and corrected commit required.

## Blockers

None established; native requirements need actual hardware/CI.

## Handoff

Persist implementation SHA, publication evidence and next work. Continue mission.
