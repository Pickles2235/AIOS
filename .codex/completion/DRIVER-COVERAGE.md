# Executable acceptance driver and remaining depth

`scenarios.json` maps all 26 requirements to executable product tests and owner
milestones. Full gates run the union; scoped gates run only the owner's scenarios.
Milestone 02 certifies dispatcher integrity, not later feature acceptance.

Run `make completion-privacy`, or
`python3 scripts/completion_driver.py privacy --milestone 08-local-observability
--output /owned/report.json`. Use `completion_harness.py gate` for retained outer
receipts. A report follows `scenario-report.schema.json`; errors retain only type
and hash. No source, query, capability or raw exception text belongs in receipts.
Synthetic unit suites have purpose `driver_fixture`; only the fixed live suite
is callable from the CLI. Neither synthetic nor scoped evidence is final proof.

Build on a clean committed checkout. The executable's Go VCS metadata must match
the tested HEAD and disclose an unmodified build. `AIOS_COMPLETION_BINARY` selects
an artifact but cannot select an unrelated build. `AIOS_COMPLETION_PACKAGE` selects
the candidate ZIP. Release probes require exhaustive checksums, tested source SHA
and identical packaged/gate binary bytes. Native upgrade also requires a distinct
`AIOS_COMPLETION_PRIOR_PACKAGE`; no empty-state identity comparison is accepted.
Reference hardware is declared with `AIOS_COMPLETION_REFERENCE_HARDWARE`.

Linux is developer smoke. Wrong native hosts block before tests run. Native service
tests use disposable labels and owned roots; uninstall cleanup is registered before
installation to cover partial failures. The implementation must make cleanup
idempotent. Backend startup reads bounded bytes with a deadline; termination has a
bounded TERM/KILL fallback. Browser suites use real embedded assets/authentication,
never a mocked backend; they live outside ordinary core/web smoke discovery.

## Required expansions before feature acceptance

The initial live probes deliberately expose missing APIs. API policy flags alone
are insufficient. Explicit `PENDING` assertions prevent those provisional probes
from certifying a feature after a stub implements flags. Replace each with actual
outcome/fault assertions in the owning milestone; deleting the guard without the
corresponding tests is an acceptance regression.

| Owner | Remaining required depth |
| --- | --- |
| 03 | Installed launchd login/headless and disconnect smoke on real Apple Silicon; runtime assets/toolchain absence. Portable runtime/plist probes are separate. |
| 04 | Auth failure/remediation under actual daemon Git environment; collision suggestions and explicitly selected namespace; partial batch/staged build. |
| 05 | Guarded: changed generations under durable retries, restart/wake/lost events, burst debounce/coalescing, capture races, projection faults, concurrent readers and per-repo isolation. |
| 06 | Remove racing queued/in-flight jobs; inspect owned canonical/projection/history/cache/mirror deletion, preserve sources. |
| 07 | Guarded: distinct prior package populated with nonempty IR; matching binary/state rollback at six FAULTS boundaries, actual killed-process crashes and next-start recovery, compatibility rejection before mutation. Native cleanup always runs. |
| 08 | Guarded: enumerate every persisted telemetry sink; plant credentials/PII/source/query/path/remote through jobs/errors/upgrades; observe no outbound telemetry. Archive probes already inspect actual ZIP bytes, not export flags. |
| 09 | Guarded: injected normal/constrained/idle transitions; observe bounded queues/workers/storage, fair eventual generations, cancellation and disk-full outcomes. Native power signals need real hardware. |
| 10 | Independent pre-scoring 150-case/25-repo gold corpus; evidence/generation copy parity and two fresh semantic fingerprints. Corpus self-report is not review provenance. |
| 11 | Node/aggregate/edge-to-IR exact membership and revision mapping at every level; staged/promotion/delta/query animation, pan/zoom/rotation, keyboard/reduced motion; native GPU visual QA. |
| 12 | Actual module contextual behaviour, persistence/reset/viewport/reconnect; visual QA. |
| 13 | Demo/My Knowledge actual isolated roots/source byte parity and persisted rows; predeclared losses, incorrect/unknown cases, source/context/token/time metrics. |
| 14 | Dense mixed 1/25/100 fixtures already generated/indexed by probe; add cold/incremental/query budgets and bounded 30-minute update/restart/query soak, memory/CPU/frame measurements and honest misses. |
| 15 | Guarded: open/query/update/uninstall exact installed ZIP, all full gates at one native source/artifact plus independent manifest/requirements/UI review. |

Every owner must expand adjacent fault/race scenarios from ACCEPTANCE.md and record
actual outcomes before being reviewed complete. This table is a coverage handoff,
not evidence of those unimplemented scenarios passing.
