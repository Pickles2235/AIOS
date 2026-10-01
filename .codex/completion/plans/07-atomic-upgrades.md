# Implement crash-safe explicit update and uninstall

Requirements: R13, R26

## Acceptance

- Release compatibility manifest and supported upgrade fixture preserve identity/IR without repeat onboarding.
- Interrupt/fail each install/migration/activation/health boundary; restore matching old binary/state and restart.
- Atomic filesystem/disk-full/power-loss recovery covered with durable journal; no background self-update.
- Uninstall stops jobs/service; preserve/delete choice operates only on owned data.

## Decisions

Inspect current implementation before selecting changes.

## Progress

Pending.

## Scenario scope

Run existing core/web gates plus scoped product scenarios with `gate NAME --milestone 07-atomic-upgrades`. Full assembled/native gates are final-candidate requirements. Record deferred native checks explicitly.

## Actual validation

Record commands, receipts, platform and full tested commit. No checks run yet.

## Independent review

Separate reviewer report and corrected commit required.

## Blockers

None established; native requirements need actual hardware/CI.

## Handoff

Persist implementation SHA, publication evidence and next work. Continue mission.
