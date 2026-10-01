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

Corrected c6f8802797c9f23f644d56396c9a46f76e9fe065 passed core/web26livecases and twice-fresh acceptance; its long maintenance run was deliberately interrupted and recorded failed when review identified that a five-second read wait could exceed the existing one-second query budget. Final policy is five seconds for writers/migration and100milliseconds for all read-only/backup-verification connections. Actual EXCLUSIVE write lease tests bound positive/negative/cancelled queries, prevent false absence and verify last-good recovery after unlock. New final-source checks are required.

Final source a19211f6e26d241fce3683690160acd72d1825eb passed clean core and27UI unit/26live browser cases. A concurrent acceptance build replaced shared bin/aios after allowed evidence files were added, changing its GoVCS modified flag during the long maintenance run. That run completed PASS but its mutable executable provenance is historical and is not the final receipt. The isolated clean detached checkout /workspace/scratch/aios-maintenance-a192-clean produces the fixed owned executable /workspace/scratch/aios-maintenance-evidence/aios-a192-frozen, mode0500, GoVCS exacta192/modifiedfalse, SHA256e67193ca6fc58079b7f03f3979ee5992898c7c661aef5b772e520a8da3eea3f9. New formal maintenance uses AIOS_COMPLETION_BINARY to that file. Two clean-checkout acceptance builds passed12cases/25repositories with equal semantic fingerprint and the exact same executable digest. Final gate must verify this digest again after completion. Subsequent milestones/final15 must freeze executable/package paths before concurrent gates and must not rebuild the executable being tested.

The frozen final maintenance run passed all five actual scoped scenarios in469.84seconds: dirty/untracked capture48assertions; real restart/missed-change recovery123; Mirror isolation/coalescing/exhaustion/restart9191(452.95seconds); continuous edits/projection fault+repair225; healthy unavailable-member bootstrap44. After execution, digest and GoVCS metadata still matched the frozen executable. Verified redacted receipts/reports: evidence/05-maintenance-core.json,05-maintenance-web.json,05-maintenance-gate.json,05-maintenance-scenarios.json,05-maintenance-acceptance.json. Actual raw log/report hashes are retained in those files; private logs are /workspace/scratch/aios-maintenance-evidence, scoped report /tmp/aios-product-evidence-68_v_z2k/maintenance.json. Native CI36934207023 on sourcea192 passed Go unit/watch integration, vet, native login Git/mDNS authentication smoke and native deterministic acceptance; installed candidate checks remained in progress at this record. Full native scoped maintenance, real sleep/wake and final same-artifact candidate proof remain pending15.

Native CI36934207023/job110610520487 subsequently completed SUCCESS. Verified artifact11198550372 ZIP SHA25696768d628fec5dd7f646f3059bbd1ab993d9f9ef92aa0c4e5398e7c97da45d11, retained /workspace/attachments/81342190-b101-4185-80e8-862ab68a4854/maintenance-a192-native.zip. Native daemon/onboarding reports passed actual Darwinarm64/sourcea192, including installed per-user launchd, external TLS Git helper/login context, namespace transition/recovery, native UI stop/restart and source nonmutation. Native baseline/fresha/freshb/installed14case acceptance runs all accepted with equal fingerprint6eeab4379b06b9176ddf374e503050290aa89fb3a1690e03a5c6bb5c2caa693a. Installed GoVCS sourcea192/modifiedfalse; installed package SHA256953ea5dcc2c2e9b47cd82ec4ba499ede65714374345c7a36dc644d7712ae89c2. Actual macOS15.7.9arm64,3CPUs,7516192768bytesRAM. Redacted summary evidence/05-maintenance-native.json preserves report hashes/case statuses and explicit final deferrals. This verifies native engineering smoke, not the native full maintenance/wake/final candidate gate.

## Independent review

Separate session /root/baseline_reviewer performed early read-only audit and independent targeted race tests. Fixed its retained retry deadline, revisited delta epoch, excluded oversized/symlink input, descendant-held Git pipe, unchanged metadata/coverage and post-activation cancellation findings with adjacent actual regressions. Formal report against committed source and clean gate receipts remains required.

Separate reviewer /root/baseline_reviewer wrote evidence/05-maintenance-review.json, independently verified corrected sourcea192, actual race/fault/query-budget/migration/journal-recovery regressions, browser integration and final frozen receipts/digest/twice-fresh acceptance. Engineering verdict PASS with no blocking findings. Native artifact provenance is reviewed as a final evidence addendum; full native wake/maintenance/assembled acceptance remain explicitly pending15.

## Blockers

None established; native requirements need actual hardware/CI.

## Handoff

Persist implementation SHA, publication evidence and next work. Continue mission.

Implementation SHA a19211f6e26d241fce3683690160acd72d1825eb. Main before publication was verified746c1a19b002fbdbb627f13a2197ba98d920938c; publication and completion bookkeeping must be checked before06 becomes eligible. Scratch preparation for06: /workspace/scratch/repo-management-purge.go,repo-management-purge_test.go,repo-management-store.go,repo-management-overlay.json. Its actual SQLite prototype preserves a survivor, rolls back injected purge failure, compacts deleted pages and removes the last repository using an empty catalog fingerprint. This is preparation, not completed06; still implement durable cancel/withdraw/purge/restart lifecycle, force rebuild, actual API/live UI and independent reviewed product scenarios.
