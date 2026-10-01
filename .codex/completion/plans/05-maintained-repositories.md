# Implement durable polling, debounced watches and safe repair

Requirements: R06, R07, R08

## Acceptance

- 15-minute configurable polling, Check now, bounded backoff, stale warnings, independent repo recovery.
- Dirty/untracked Direct snapshots, ignore rules, one-second debounce with maximum wait, event loss and concurrent edit checks.
- Wake/restart reconciles missed changes; query serves last good generations under failed/interrupted rebuilds.
- Demonstrate delta-only work and no source modifications across success, cancellation and failure.

## Decisions

Inspect current implementation before selecting changes.

## Progress

Pending.

## Scenario scope

Run existing core/web gates plus scoped product scenarios with `gate NAME --milestone 05-maintained-repositories`. Full assembled/native gates are final-candidate requirements. Record deferred native checks explicitly.

## Actual validation

Record commands, receipts, platform and full tested commit. No checks run yet.

## Independent review

Separate reviewer report and corrected commit required.

## Blockers

None established; native requirements need actual hardware/CI.

## Handoff

Persist implementation SHA, publication evidence and next work. Continue mission.
