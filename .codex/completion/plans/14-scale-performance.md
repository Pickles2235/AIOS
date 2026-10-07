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

## Required inherited checks

Before final performance certification, correct task11 native RAF first-frame baseline; exclude no invalid initial interval silently and distinguish CPU submission from GPU completion. Measure actual dense rotating rendering, not idle RAF.

Task13 My Knowledge currently uses complete private canonical/projection copy with256MiB DB,64MiB source,10k files bounds. Evaluate Lab on declared1/25/100 mixed dense fixtures, record unavailable/latency/disk outcomes. Implement and independently validate a faithful bounded large-estate copy/cohort path if limits block required comparisons; never claim global not_found from a subset or weaken provenance to fit. Final installed acceptance must include any resulting source changes.
