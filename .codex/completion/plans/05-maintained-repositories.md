# Implement durable polling, debounced watches and safe repair

Requirements: R06, R07, R08

## Acceptance

- 15-minute configurable polling, Check now, bounded backoff, stale warnings, independent repo recovery.
- Dirty/untracked Direct snapshots, ignore rules, one-second debounce with maximum wait, event loss and concurrent edit checks.
- Wake/restart reconciles missed changes; query serves last good generations under failed/interrupted rebuilds.
- Demonstrate delta-only work and no source modifications across success, cancellation and failure.

## Decisions

Retain canonical IR and atomic catalog activation. Direct captures eligible tracked/modified/untracked working files into owned sealed immutable snapshots; bounded reads, two-pass content/metadata/list/state validation reject concurrent changes and never mutate source. Use actual OS directory watches with one-second quiet debounce/five-second maximum and periodic lost-event reconciliation. A serialized fair worker owns bounded/coalesced per-repository durable jobs, default15-minute Mirror polling, explicit Check now, bounded exponential retries and restart recovery. Source failures remain independent and retain last-good evidence. Current committed-only capture, batch-wide source validation and failed pending-revision selection must be extended safely. Native wake proof remains mandatory in final15; portable clocks/event-loss and actual native watcher/launchd smoke are distinct evidence.

## Progress

Started after reviewed04 source4cab95f464761b26456b360197437f71153d5a07 and completion bookkeeping published as3d835efe46e8b6d9c9d445aa4c1b287d343a0be4. Late04 verified native artifact/reviewer addendum published and fetched as746c1a19b002fbdbb627f13a2197ba98d920938c. Implemented scoped immutable working capture, independent maintained ingestion, durable serialized/coalesced queue, real fsnotify watches, lost-event and clock-gap repair, API and live browser health/polling/Check now controls. Canonical queue/delta completion commits atomically with catalog/projections; post-activation cancellation cannot leave identities behind. Metadata/coverage-only refreshes create a new evidence epoch with unchanged analysis reused.

Repeated A→B→A→B exposed the old canonical content uniqueness constraint. Explicit knowledge-ir-v10 removes it, preserving distinct epochs. The owned writer upgrades populated v9 with a verified/fsynced owner-only SQLite backup and transactional table replacement; injected migration failure keeps old format, active IDs and facts. This narrow store migration does not certify the later complete installed-package upgrade.

## Scenario scope

Run existing core/web gates plus scoped product scenarios with `gate NAME --milestone 05-maintained-repositories`. Full assembled/native gates are final-candidate requirements. Record deferred native checks explicitly.

## Actual validation

Precommit Linux x86_64 debug checks passed core vet/unit/race/Python/build and27UI unit/26browser cases at1440/2560. Targeted actual integration probes passed dirty/untracked/ignore/excluded capture, process restart/missed edits, continuous production debounce, graph-projection transaction fault/recovery and healthy partial bootstrap. A456.7-second real Mirror probe reached5attempt exhaustion with30/60-second backoff,100request coalescing and healthy updated queries; its final assertion hit an optional JSON-key error that is fixed. Formal exact-commit maintenance rerun remains required. Debug logs: /workspace/scratch/maintenance-core-debug3.log and maintenance-web-debug2.log. Actual native watch/launchd/wake proofs for05 remain pending until native CI/final15; portable injected clock gaps are not native sleep/wake evidence.

Committed d5946f2e696bd5479636c27153f55f9d570c3783 passed clean core/web, twice-fresh accepted12case/25repo acceptance with equal fingerprint f75584b3be0c5e3750e72144fc072c55af56235e37d8218a39bf2ff8ef7656ea, and all five scoped maintenance cases in469.9seconds. The initial scoped invocation correctly rejected a binary built from the preceding local source commit; rebuild corrected that provenance before the passing run. Actual CI36931545494 found test fixtures inherited shared TempDir permissions under its umask. Source ownership checks stayed strict; a2560adff40dc88ac614724f6bf294e34efa6b41 explicitly sets only disposable fixture roots0700. Its core and all five maintenance cases passed under umask022, as did twice-fresh acceptance. The first a256 web run failed initial Direct maintenance idle after successful Build; 25othercases passed. A full rerun passed26cases and six repeated real maintenance browser cases passed, but the failure remained open for diagnosis.

A real100ms SQLite reader lease reproduced immediate SQLITE_BUSY on a maintenance write. Every writer/read/backup-verification connection now has a bounded five-second busy timeout via the DSN. Actual short-reader contention succeeds; cancellation with a held reader returns after the bounded five-second handler, and subsequent writes recover. Every pooled read connection has the policy. Actual canonical replacement with a deliberately stale exhausted journal now has an adjacent restore/reopen regression. Independent reviewer verified both diagnostics under race before incorporation. Fresh corrected-source receipts remain required; all earlier receipts are historical, not proof of the newest source.

## Independent review

Separate session /root/baseline_reviewer performed early read-only audit and independent targeted race tests. Fixed its retained retry deadline, revisited delta epoch, excluded oversized/symlink input, descendant-held Git pipe, unchanged metadata/coverage and post-activation cancellation findings with adjacent actual regressions. Formal report against committed source and clean gate receipts remains required.

## Blockers

None established; native requirements need actual hardware/CI.

## Handoff

Persist implementation SHA, publication evidence and next work. Continue mission.
