# Complete repository management and rebuild/purge lifecycle

Requirements: R09, R10

## Acceptance

- Add/remove/rules/retry/rebuild/health/revision surfaces wired to actual backend.
- Removal cancels jobs and purges all owned facts/projections/history/cache/mirrors; no resurrection or source deletion.
- KB rebuild retains identity/config and stages healthy replacement; concurrent queries remain consistent.

## Decisions

Inspect current implementation before selecting changes.

## Progress

Pending.

## Scenario scope

Run existing core/web gates plus scoped product scenarios with `gate NAME --milestone 06-repo-management`. Full assembled/native gates are final-candidate requirements. Record deferred native checks explicitly.

## Actual validation

Record commands, receipts, platform and full tested commit. No checks run yet.

## Independent review

Separate reviewer report and corrected commit required.

## Blockers

None established; native requirements need actual hardware/CI.

## Handoff

Persist implementation SHA, publication evidence and next work. Continue mission.
