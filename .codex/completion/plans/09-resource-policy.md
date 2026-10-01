# Implement resource-aware fair bounded background work

Requirements: R14, R25

## Acceptance

- Native battery/load/idle policy transitions are real, testable and visible; heavy work deferred with eventual progress.
- Bound memory/queues/workers/disk, cancellation and fairness; disk-full retains last good state.
- Record measurement envelope and reference hardware before tuning; no invented performance assertions.

## Decisions

Inspect current implementation before selecting changes.

## Progress

Pending.

## Scenario scope

Run existing core/web gates plus scoped product scenarios with `gate NAME --milestone 09-resource-policy`. Full assembled/native gates are final-candidate requirements. Record deferred native checks explicitly.

## Actual validation

Record commands, receipts, platform and full tested commit. No checks run yet.

## Independent review

Separate reviewer report and corrected commit required.

## Blockers

None established; native requirements need actual hardware/CI.

## Handoff

Persist implementation SHA, publication evidence and next work. Continue mission.
