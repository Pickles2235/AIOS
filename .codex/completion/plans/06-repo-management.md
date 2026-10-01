# Complete repository management and rebuild/purge lifecycle

Requirements: R09, R10

## Acceptance

- Add/remove/rules/retry/rebuild/health/revision surfaces wired to actual backend.
- Removal cancels jobs and purges all owned facts/projections/history/cache/mirrors; no resurrection or source deletion.
- KB rebuild retains identity/config and stages healthy replacement; concurrent queries remain consistent.

## Decisions

- Durable owner-only removal intent is fsynced before withdrawal; startup repeats withdrawal/atomic purge before restoring workers. Pending cleanup blocks evidence/projection reads and exposes an explicit recovery control. Sources are never deletion arguments.
- Cancellation waits for build completion and closes its completion channel before maintenance reacquires the transition lock. Repository purge creates survivor catalog/projections transactionally, removes all canonical/staged/retired repository history and shared negative caches, then VACUUMs under the writer lease. Owned snapshots/mirror/shared derived cache/source-bearing IR migration backups and interrupted staging are removed through an os.Root boundary, with parent fsyncs before completing the intent.
- Force rebuild freshly extracts every selected immutable file, including unchanged files; global rebuild validates all approved members before one catalog promotion. Active identity/config and last-good generations remain through failures.
- Protected add/scope/retry/rebuild/remove/purge-status APIs and live management/health controls use the current global source mode. Setup cannot bypass purge by omitting IDs. UI removal clears queries, cloud selection and retired evidence; recoverable cleanup is visible after restart.
- Full native assembled maintenance/wake/installed candidate proof remains explicitly pending task15.

## Progress

In progress on `codex/completion-06-repository-management`, from verified fetched main `4f6ef6b17ce40956cf58b68d999662664ad36e1b`. Task05 source/review/receipts are published. Implement durable repository withdrawal/purge, force recompilation, backend controls and live UI with rollback/restart scenarios.

## Scenario scope

Run existing core/web gates plus scoped product scenarios with `gate NAME --milestone 06-repo-management`. Full assembled/native gates are final-candidate requirements. Record deferred native checks explicitly.

## Actual validation

Preliminary Linux checks (not final source receipts): targeted/full package race tests passed, including umask022; four real backend browser management cases passed at 1440/2560 for Direct/Mirror; expanded process scenario passed both modes with concurrent rebuild reads, actual projection failure, add/scope/retry, in-flight removal, populated migration-backup deletion, process restart, failed physical purge and next-process recovery, last-repository empty catalog and source fingerprints. Final clean-source gates and twice hermetic acceptance are next.

Two preliminary command invocations used the wrong test import/cwd and failed before reaching product behavior; corrected PYTHONPATH and web cwd invocations passed. Raw logs remain in /workspace/scratch/repo-management-*.log.

## Independent review

Separate reviewer report and corrected commit required.

## Blockers

None established; native requirements need actual hardware/CI.

## Handoff

Persist implementation SHA, publication evidence and next work. Continue mission.
