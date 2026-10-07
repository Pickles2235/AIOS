# Implement resource-aware fair bounded background work

Requirements: R14, R25.

## Acceptance

- Native battery/load/idle policy transitions are real, testable and visible; heavy work is deferred with eventual progress.
- Bound memory, queues, workers and disk retention; preserve last good state on cancellation and full storage.
- Record the measurement envelope before tuning; report only observed outcomes.

## Decisions and implementation

The durable maintenance scheduler remains a single worker with at most 100 repository jobs and coalesced followups. A native Darwin probe reads `pmset -g batt`, `sysctl -n vm.loadavg` and `ioreg -c IOHIDSystem -d 4` under a two-second deadline. Injected probes prove normal, battery/load constrained and AC idle transitions. Battery or load defers heavy jobs for at most five minutes from a durable pending timestamp; retry waits and restarts retain a finite deadline. Unknown signals run normal work and are explicitly marked unavailable. The authenticated resources API, CLI JSON and repository-health UI expose policy, queue age, deferral, cancellation, and measured storage.

Owned-data admission counts every regular file beneath the agent data directory, including SQLite main/WAL, snapshots, models, caches, telemetry and retained history, without following links. It refuses new heavy work when measured bytes exceed 100 GiB minus an 8 GiB build reserve or free volume is below 16 GiB (8 GiB reserve plus 8 GiB for the next capture). These are **start-work admission thresholds**, not a hard cap on bytes written by an already running build. The maintenance journal separately has an enforced 512 KiB bound. Source input retains its existing per-repository file and byte limits. After a successful activation, the existing transactional `Store.Retain` keeps the active catalog and, by default, two prior catalog revisions; its failure is a warning and cannot retract a committed active generation. No source workspace is deleted.

Running cancellation cancels the job context; a late returned result cannot update maintenance identity. Store activation checks cancellation immediately before commit. Journal, stage and activation write-boundary ENOSPC probes verify rollback and last-good/source preservation. Cancellation after an already committed activation is too late to undo that commit; the UI says “cancelling,” and canonical reconciliation remains authoritative.

## Measurement envelope

Reference hardware captured 2026-10-07 on real Apple Silicon: MacBook Pro Mac15,6, M3 Pro (11 cores), 36 GB memory, macOS 26.7.1, AC power. Initial host load averages were 1.55 / 2.05 / 3.04. The native commands returned AC, positive load and a positive HID idle value; no battery identifier or personal data is recorded. `/usr/bin/time -l go test ./internal/maintenance -run '^TestDurableQueueSupportsOneHundredAndRejectsOverflowBeforeMutation$' -count=1 -v` passed: test body 0.03 s, command wall 12.50 s, maximum resident set 970,915,840 bytes and peak memory footprint 31,064,736 bytes. The command includes Go compilation/runtime startup, so those figures are **not** isolated scheduler or UI performance metrics. No 60 fps, query-latency, 1/25/100 dense-cloud or sustained battery benchmark claim is made here; those need later scale/final evidence. Policy thresholds were not tuned from this measurement.

## Progress

Implementation, focused unit/integration tests and embedded UI asset rebuild are prepared for the task09 candidate commit. Resource completion scenario now checks the live API/queue/source outcome and runs named deterministic injected policy, storage, cancellation and rollback tests instead of a pending assertion. Existing task08 and other milestone behavior is preserved.

## Validation

Focused `go test ./internal/resourcepolicy ./internal/maintenance ./internal/store ./internal/app ./internal/webui ./cmd/aios` passed after task09 edits. `npm --prefix web test` passed 27/27; `make web-build` passed. More exact committed-source core/resources/web receipts remain to run. The test source has not yet been committed; no product gate is claimed.

## Review and blocker audit

Independent review is pending. The global owned-data admission and finite generation retention prevent new work from starting above the budget and bound retained catalog count, but do not provide a hard byte cap during an in-flight build. Reviewer/manager must decide whether that is sufficient for R14; a hard post-build byte invariant would require a safe quota across SQLite, snapshots, model/cache and projections without deleting canonical last-good evidence. Do not mark this milestone completed or published on this evidence alone.

## Handoff

Commit scoped implementation after focused validation; run exact scoped core/resources and full web gates on the clean commit. Do not push; manager integrates and publishes after review.
