# Implement resource-aware fair bounded background work

Requirements: R14, R25.

## Acceptance

- Native battery/load/idle policy transitions are real, testable and visible; heavy work is deferred with eventual progress.
- Bound memory, queues, workers and disk retention; preserve last good state on cancellation and full storage.
- Record the measurement envelope before tuning; report only observed outcomes.

## Decisions and implementation

The durable maintenance scheduler remains a single worker with at most 100 repository jobs and coalesced followups. A native Darwin probe reads `pmset -g batt`, `sysctl -n vm.loadavg` and `ioreg -c IOHIDSystem -d 4` under a two-second deadline. Injected probes prove normal, battery/load constrained and AC idle transitions. Battery or load defers heavy jobs for at most five minutes from a durable pending timestamp; retry waits and restarts retain a finite deadline. Unknown signals run normal work and are explicitly marked unavailable. The authenticated resources API, CLI JSON and repository-health UI expose policy, queue age, deferral, cancellation, and measured storage.

Owned-data admission counts every regular file beneath the agent data directory, including SQLite main/WAL, snapshots, models, caches, telemetry and retained history, without following links. It refuses new heavy work when measured bytes exceed 100 GiB minus an 8 GiB build reserve or free volume is below 16 GiB (8 GiB reserve plus 8 GiB for the next capture). These are **start-work admission thresholds**, not a hard cap on bytes written by an already running build. The maintenance journal separately has an enforced 512 KiB bound. Source input retains its existing per-repository file and byte limits. After a successful activation, transactional retention keeps the active catalog and, by default, two prior catalog revisions. Committed prune candidates are durable in SQLite. Owned snapshots of removed generations are reclaimed after the ingest releases its per-repository lease; a restart retries unfinished cleanup. Cleanup validates each ancestor, never follows symlinks, checks published and staged references under an exclusive lease, and removes empty revision/repository parents. The lease file count is bounded by the 100-repository catalog limit. No source workspace is deleted. Local and mirror capture hold shared leases through ingestion; mirror archive extraction streams from Git with transport, extracted-byte and file-count limits before each write. An oversized archive fails without replacing the last-good generation.

Running cancellation cancels the job context; a late returned result cannot update maintenance identity. Store activation checks cancellation immediately before commit. Journal, stage and activation write-boundary ENOSPC probes verify rollback and last-good/source preservation. Cancellation after an already committed activation is too late to undo that commit; the UI says “cancelling,” and canonical reconciliation remains authoritative.

## Measurement envelope

Reference hardware captured 2026-10-07 on real Apple Silicon: MacBook Pro Mac15,6, M3 Pro (11 cores), 36 GB memory, macOS 26.7.1, AC power. Initial host load averages were 1.55 / 2.05 / 3.04. The native commands returned AC, positive load and a positive HID idle value; no battery identifier or personal data is recorded. `/usr/bin/time -l go test ./internal/maintenance -run '^TestDurableQueueSupportsOneHundredAndRejectsOverflowBeforeMutation$' -count=1 -v` passed: test body 0.03 s, command wall 12.50 s, maximum resident set 970,915,840 bytes and peak memory footprint 31,064,736 bytes. The command includes Go compilation/runtime startup, so those figures are **not** isolated scheduler or UI performance metrics. No 60 fps, query-latency, 1/25/100 dense-cloud or sustained battery benchmark claim is made here; those need later scale/final evidence. Policy thresholds were not tuned from this measurement.

## Progress

The first candidate `8823a5b` failed independent review for unbounded snapshot history and buffered Git archive output. Candidate `28cc707` fixed these but review found failed-ingest orphan roots and the eligible limit incorrectly applied to the full archive. The current corrective patch queues every captured root before indexing and separates full-capture limits from eligible limits. Focused tests prove six successive local activations reclaim old snapshots, repeated failed captures do not accumulate, staged failures remain protected until discard, restart retries cleanup, excluded bytes/files do not consume eligible limits, source and active history remain, in-use captures defer cleanup, symlink ancestors are rejected, lease metadata stays bounded, and a crash after deletion is acknowledged on retry. Resource completion scenario now checks the live API/queue/source outcome and runs named deterministic injected policy, storage, cancellation and rollback tests instead of a pending assertion. Existing task08 and other milestone behavior is preserved.

## Validation

On committed candidate `28cc707`, `PATH=/opt/homebrew/opt/python@3.12/libexec/bin:$PATH make harness-validate` passed (54 Python tests, vet, all Go tests, all race tests, build), but that source failed independent review. The new corrective patch has focused app/adapter tests passing; exact committed-source core/resources/web and two fresh acceptance runs remain. Earlier web unit 27/27 and embedded asset build passed on the previous task09 commit; no current-source web gate is claimed.

## Review and blocker audit

Independent review is pending. The prior review blockers are addressed in the corrective patch. A global hard byte cap during an in-flight build is still unproven: admission plus per-repository extraction bounds, durable snapshot GC, finite canonical retention and 512 KiB journal bound constrain growth, but SQLite/projection/model writes do not share one atomic byte quota. This limitation remains explicit for independent review; do not call the admission threshold a hard cap. Do not mark this milestone completed or published on this evidence alone.

## Handoff

Commit scoped implementation after focused validation; run exact scoped core/resources and full web gates on the clean commit. Do not push; manager integrates and publishes after review.
