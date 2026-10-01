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

Independent review found and reproduced an owned-source overlap defect at historical source11d54c: a local Mirror remote could be another owned mirror, and removing its provider would delete that approved source. Fixed before publication: canonical/symlink/file-URL/symmetric ancestor boundary checks before preview/configure/add and in shared Sync/maintenance; target-specific purge checks before removal intent protect legacy approvals while allowing the invalid dependent to be withdrawn first. A pending legacy intent has a protected same-ID/mode source-URL correction path; Build stays blocked until cleanup completes. Actual regression coverage includes provider HEAD preservation, alias rejection, pre-intent failure, dependent withdrawal, persisted restart recovery and corrected URL. All11d receipts are historical and superseded; rerun final clean-source gates on the corrected commit.

Reviewer also reproduced Git local clone hardlinks changing source object nlink/ctime. Clone now explicitly uses --no-hardlinks; actual loose-object tests verify separate source/owned inodes, and the full process scenario compares source Git object nlink+ctime across add, polling, rebuild, purge and restart. Target-specific purge checks now cover Direct paths too, including aliases changed since approval.

Corrected backend source f4ece45a68e151881f55475c63de7387f92e474a passed core, the expanded purge/rebuild process scenario and two fresh 12-case/25-repository hermetic runs with equal semantic fingerprint f75584b3be0c5e3750e72144fc072c55af56235e37d8218a39bf2ff8ef7656ea. Its web gate failed one actual Direct desktop case: build completion refresh cleared a successful query/entity/evidence from the current generation. Preserve these historical failure receipts. Fixed generation-aware refresh, status re-read if query/inspection changes while status is in flight, and indexing state reset on unconfigured/ready. A controlled delayed-completion/promotion regression checks current evidence survives and retired evidence clears; real Direct/Mirror browser cases now remove the last repository and assert the build card disappears. Repeated focused browser checks passed 12 cases at both viewports. New exact-source formal gates follow; no final approval claimed yet.

Source6279442 passed all portable formal gates (core91.26s, web79.36s/27 unit/32 browser cases, process23.38s) and twice fresh acceptance. Its actual Apple Silicon CI run36941435978 failed the independent-object fixture because Git's transient .git/objects/maintenance.lock disappeared after enumeration. Restrict this inode test to real loose object hash names and disable automatic gc/maintenance in the synthetic fixture before committing; retain nonempty-object and every-inode independence assertions. Twenty actual local repetitions pass. Native failure is recorded rather than waived; collect final evidence on corrected source. Distinct portable binaries are recorded honestly: formal maintenance/reviewer used trimpath frozen31d7d618..., hermetic make acceptance used its clean worktree's non-trimpath build.

Final engineering implementation:4249522a36d52a1e4b3113306df242821c0cd5b7. Linux/x86_64 Go1.27.1/JDK21/Node24 formal core18.58s PASS, web66.21s PASS27unit/32browser cases, scoped maintenance15.61s PASS807actual assertions over Direct/Mirror. Receipts06-management-{core,web,maintenance,scenarios,acceptance}. Two fresh hermetic 12-case/25-repository acceptances both accepted with matching f75584b3be0c5e3750e72144fc072c55af56235e37d8218a39bf2ff8ef7656ea. Frozen scenario binary SHAa7245b130e7397e40fe1d47e83205eb0a6f1d7c9d7ab0e49f49daebe2bbed700, hermetic binary36514fe9039a46dd08cac5b850eafc8da1d3529e5bf36d65606a11f29f6b6933; both clean GoVCS4249522, frozen unchanged after suite. Raw logs /workspace/scratch/repo-management-evidence/*-4249-0.log; scenario/tmp/aios-product-evidence-9lwu52l2/maintenance.json. Independent reviewer/root/baseline_reviewer PASS exactsource06-management-review.json after actual race tests and rendered keyboard/reduced-motion removal on frozen4249. Generated assets built via make web-build; scoped diff checked. Native engineering run36941818885/job110634843335 passes prior failing Go object fixture and continues installed integration; full native assembled/wake/final artifact evidence remains task15. Any later contradictory native failure reopens this milestone. Ready for normal fast-forward publication; completion only after verified main reachability.
