# Implement executable assembled-product acceptance drivers

Requirements: R26

## Acceptance

- Implement every completion-* Make target listed in ACCEPTANCE.md with real assertions and failure exits.
- Native scenarios must fail or explicitly block on wrong hardware, never silently skip and pass.
- Add JSON evidence/report schema, fixture generators and fault injection; tests initially expose missing product behaviours.

## Decisions

Inspect current implementation before selecting changes.

## Progress

Pending.

## Scenario scope

Run existing core/web gates plus scoped product scenarios with `gate NAME --milestone 02-product-test-driver`. Full assembled/native gates are final-candidate requirements. Record deferred native checks explicitly.

## Actual validation

Record commands, receipts, platform and full tested commit. No checks run yet.

## Independent review

Separate reviewer report and corrected commit required.

## Blockers

None established; native requirements need actual hardware/CI.

## Handoff

Persist implementation SHA, publication evidence and next work. Continue mission.
