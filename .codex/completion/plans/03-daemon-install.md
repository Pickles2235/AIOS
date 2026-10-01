# Deliver daemon lifecycle and installable Apple Silicon package

Requirements: R02, R11

## Acceptance

- Native per-user launchd install/start/stop/status/open works without browser or dev toolchains.
- Bundled UI/model assets work offline; report optional external compiler coverage accurately.
- Restart/login persists identity/state; UI Stop daemon reflects disconnection and restart instructions.

## Decisions

Inspect current implementation before selecting changes.

## Progress

Pending.

## Scenario scope

Run existing core/web gates plus scoped product scenarios with `gate NAME --milestone 03-daemon-install`. Full assembled/native gates are final-candidate requirements. Record deferred native checks explicitly.

## Actual validation

Record commands, receipts, platform and full tested commit. No checks run yet.

## Independent review

Separate reviewer report and corrected commit required.

## Blockers

None established; native requirements need actual hardware/CI.

## Handoff

Persist implementation SHA, publication evidence and next work. Continue mission.
