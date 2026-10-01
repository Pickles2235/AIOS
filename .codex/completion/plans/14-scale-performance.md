# Validate 1–100 repositories and large knowledge clouds

Requirements: R25

## Acceptance

- Realistic mixed-language fixtures at 1/25/100 repos and dense symbol/edge datasets; cold/incremental/query budgets measured.
- Native reference hardware: target 60fps and common warm sub-second queries; detail degrades gracefully and misses disclosed.
- Long-run maintenance/restart/query soak, bounded storage/telemetry and false-absence correctness.
- Review gold corpus independently: do not tune expected answers to observed implementation.

## Decisions

Inspect current implementation before selecting changes.

## Progress

Pending.

## Scenario scope

Run existing core/web gates plus scoped product scenarios with `gate NAME --milestone 14-scale-performance`. Full assembled/native gates are final-candidate requirements. Record deferred native checks explicitly.

## Actual validation

Record commands, receipts, platform and full tested commit. No checks run yet.

## Independent review

Separate reviewer report and corrected commit required.

## Blockers

None established; native requirements need actual hardware/CI.

## Handoff

Persist implementation SHA, publication evidence and next work. Continue mission.
