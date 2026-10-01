# Deliver honest Demo and My Knowledge Benchmark Lab

Requirements: R22

## Acceptance

- Demo fixtures isolated from user KB; own-repository known-answer cases persist and replay as regressions.
- Same immutable snapshot for KB and grep; disclose baseline invocation, warm/cold/index cost and estimated token assumptions.
- Publish correctness including losses/unknowns, source reads/context size/time and advanced metrics without favourable weighting.
- Test cases where grep wins and KB is wrong; machine-readable per-query exports.

## Decisions

Inspect current implementation before selecting changes.

## Progress

Pending.

## Scenario scope

Run existing core/web gates plus scoped product scenarios with `gate NAME --milestone 13-benchmark-lab`. Full assembled/native gates are final-candidate requirements. Record deferred native checks explicitly.

## Actual validation

Record commands, receipts, platform and full tested commit. No checks run yet.

## Independent review

Separate reviewer report and corrected commit required.

## Blockers

None established; native requirements need actual hardware/CI.

## Handoff

Persist implementation SHA, publication evidence and next work. Continue mission.
