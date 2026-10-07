# Implement resource-aware fair bounded background work

Requirements: R14, R25

## Acceptance

- Native battery/load/idle policy transitions are real, testable and visible; heavy work deferred with eventual progress.
- Bound memory/queues/workers/disk, cancellation and fairness; disk-full retains last good state.
- Record measurement envelope and reference hardware before tuning; no invented performance assertions.

## Decisions

Inspect the maintenance engine, app workers and resource endpoints before
choosing changes. Keep source snapshots and canonical activation semantics
unchanged. Do not tune performance thresholds without a declared baseline.

## Progress

Started on `codex/completion-observability-resume` after task08 was verified on
fetched `origin/main`. The completion harness exposes a deliberately failing
task09 resources scenario that currently asserts only policy flags before an
explicit pending assertion; replace it with observed transitions, queue bounds,
starvation, cancellation and disk-full outcomes.

Initial reference envelope captured 2026-10-07 on Apple Silicon: MacBook Pro
Mac15,6, Apple M3 Pro (11 cores), 36 GB memory, macOS 26.7.1, on AC power; load
averages 1.55 / 2.05 / 3.04 at capture. This is machine metadata, not a
performance benchmark. Record actual benchmark workload and measurements before
any tuning claim.

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
