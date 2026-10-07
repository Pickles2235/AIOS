# Deliver local OTEL and PII-redacted diagnostics

Requirements: R23, R24

## Acceptance

- Trace/query/ingest/job/upgrade correlation, bounded retention and local-only collectors.
- Redaction before log/trace/metric persistence and bundle export; planted PII/secret/query/source fixtures do not leak.
- Default redacted archive plus warned optional expanded dump; expanded still never emits raw secrets.
- UI diagnostic details/export include versions, coverage, stages and timings without source content.

## Decisions

Rebuilt the lost unpublished patch from current main. Use a direct OTEL SDK
provider with a synchronous allowlisted exporter and no global/remote provider.
Hash all trace, span, parent and identifying source attributes before file
output. Keep the default archive aggregate-only; require explicit warning
acknowledgement for redacted individual operation records. Keep canonical
compiler/coverage/source evidence intact and exclude it from diagnostics.
The new synchronous request telemetry exposed an onboarding mount-read race:
an in-flight status response could overwrite an explicit Direct-source choice.
The narrow `web/src/mirror-setup.tsx` guard preserves the user's choice while
still loading saved values when untouched; a delayed-response browser probe
keeps this task08 integration repair covered.

## Progress

In progress on real Darwin arm64 in the restored local checkout. OTEL/local
sink, operational ledger scrub, query/setup/job/ingest/upgrade correlation,
safe API/UI and embedded assets are implemented. Fault and planted probes are
in place. Independent review found a raw diagnostic-ID prefix bypass in the
first implementation commit `8a734bf38f50f0895dbcf0984618f33cadd7914d`;
the corrected product commit is `353de6a43dde4e1998636d47a80730c90343d307`.
The final task08 candidate source commit is
`e47c369b3ea32c08ac110e5d7b6a6aacd65f5c4e`, adding only an E2E wait for
rebuild jobs to settle. Task remains in progress pending independent re-review.
No publication or assembled candidate-ready claim has been made.

## Scenario scope

Run existing core/web gates plus scoped product scenarios with `gate NAME --milestone 08-local-observability`. Full assembled/native gates are final-candidate requirements. Record deferred native checks explicitly.

## Actual validation

On Darwin arm64, corrected commit `353de6a43dde4e1998636d47a80730c90343d307`
was built with `make build`; `go version -m bin/aios` showed that exact revision
and `vcs.modified=false`. Focused adversarial store command
`go test ./internal/store -run 'TestFreshOperationalIDThatLooksOpaqueIsHashed|TestLegacyOperationalLedgerScrubPreservesCanonicalEvidence|TestBusyOperationalCheckpointKeepsPendingRetry' -count=1 -v`
passed. The fresh-write and legacy-migration tests plant `diag-h_` followed by
a secret; migration still preserves the canonical source/compiler/coverage
fingerprint and rows.

`PATH=/opt/homebrew/bin:$PATH GIT_CONFIG_COUNT=2
GIT_CONFIG_KEY_0=maintenance.auto GIT_CONFIG_VALUE_0=false
GIT_CONFIG_KEY_1=gc.auto GIT_CONFIG_VALUE_1=0 python3
scripts/completion_harness.py gate core --milestone 08-local-observability
--output /tmp/aios-task08-final-evidence/core.json` passed Python, vet, Go,
race and build at the corrected commit. `PATH=/opt/homebrew/bin:$PATH python3
scripts/completion_harness.py gate privacy --milestone 08-local-observability
--output /tmp/aios-task08-final-evidence/privacy.json` passed both task08
scenarios (52 and 45 assertions) with exact clean-binary provenance. Copies of
the two receipts and scenario report are in `.codex/completion/evidence/08-final-*.json`.

Sequential fresh `PATH=/opt/homebrew/bin:$PATH make acceptance-v1` runs with
`ACCEPTANCE_OUTPUT=/tmp/aios-task08-final-acceptance-1-20261007T1242` and
`ACCEPTANCE_OUTPUT=/tmp/aios-task08-final-acceptance-2-20261007T1244` each
accepted all 14 cases. Both report `engine_revision` equal to the corrected
commit and semantic fingerprint
`6c2507e8994d4d53e0ec7fb25cf5eed8ad3179b9915bdac3a6517a584d9ffcd0`.

`npm --prefix web run test:e2e -- --grep 'diagnostic|delayed onboarding read'
--workers=1` passed all four corrected-commit Chrome cases. The exact
`PATH=/opt/homebrew/bin:$PATH make harness-validate-web` passed once at the
first implementation commit (27 unit, 36 browser cases), but the corrected
commit's final run exited 2 with 27 unit and 35/36 browser cases passing.
Its sole failure was the existing desktop-2560 mirror repository-management
query after rebuild: Evidence inspector stayed at “Select an entity” rather
than showing `OriginalWorker` within five seconds. Full log:
`/tmp/aios-task08-final-evidence/web.log`. Earlier browser attempts varied
among native namespace collision, repository-management timing and one hung
run terminated after 34/36 passes; the last hung-run log is
`/tmp/aios-task08-web-final-20261007T1235.log`. Preserve these failures for
review rather than treating the prior web pass as corrected-commit proof.

The mirror failure was reproduced in 2/10 isolated desktop-2560 repeats at
five Playwright workers, while 5/5 serial repeats passed. In the failing trace,
the query returned HTTP 200 `found` with three `fixture` entities from
generation `g1-ba4b...`; the repository advanced to `g1-fa4c...` before the
entity read, which correctly returned HTTP 409 stale handle. At that point
`/api/v1/jobs` reported one running and one pending job. The test's prior
generation-change poll observed a provisional intermediate state. The
test-only commit `e47c369b3ea32c08ac110e5d7b6a6aacd65f5c4e` additionally
waits for both jobs to be idle before the same positive `OriginalWorker`
query/evidence assertion. Ten desktop-2560 repeats at five workers then
passed. Pre/post logs: `/tmp/aios-task08-mirror-parallel-repeat-20261007.log`
and `/tmp/aios-task08-mirror-parallel-fixed-20261007.log`.

At final candidate `e47c369b3ea32c08ac110e5d7b6a6aacd65f5c4e`, clean
`make build` reported the exact GoVCS revision and `vcs.modified=false`.
Sequential fresh acceptance runs at
`/tmp/aios-task08-candidate-e47c369-a-20261007/acceptance-report.json` and
`/tmp/aios-task08-candidate-e47c369-b-20261007/acceptance-report.json`
each accepted 14/14 cases with matching `engine_revision` and the same
fingerprint `6c2507e8994d4d53e0ec7fb25cf5eed8ad3179b9915bdac3a6517a584d9ffcd0`.
The commands were `PATH=/opt/homebrew/bin:$PATH make acceptance-v1
ACCEPTANCE_OUTPUT=/tmp/aios-task08-candidate-e47c369-a-20261007` and the
same command with `...-b-20261007`. Exact scoped core and privacy gates passed
on Darwin arm64 with `PATH=/opt/homebrew/bin:$PATH GIT_CONFIG_COUNT=2
GIT_CONFIG_KEY_0=maintenance.auto GIT_CONFIG_VALUE_0=false
GIT_CONFIG_KEY_1=gc.auto GIT_CONFIG_VALUE_1=0 python3
scripts/completion_harness.py gate core --milestone 08-local-observability
--output /tmp/aios-task08-candidate-e47c369-evidence/core.json` and
`PATH=/opt/homebrew/bin:$PATH python3 scripts/completion_harness.py gate
privacy --milestone 08-local-observability --output
/tmp/aios-task08-candidate-e47c369-evidence/privacy.json`. Receipts and the
97-assertion privacy report are copied to
`.codex/completion/evidence/08-candidate-{core,privacy,privacy-scenarios}.json`.
The exact `PATH=/opt/homebrew/bin:$PATH make harness-validate-web` exited 0:
27 unit tests and 36 live Chrome E2E tests passed in 1.5 minutes. Full log:
`/tmp/aios-task08-candidate-e47c369-evidence/web.log`.

## Independent review

Independent reviewer identified the ID-prefix privacy bypass and required a
corrected commit. Trace inspection then identified a separate test timing
race, repaired without changing product behavior or weakening assertions.
Re-review of exact candidate `e47c369b3ea32c08ac110e5d7b6a6aacd65f5c4e`
and its receipts is pending; the task is not ready yet.

## Blockers

No implementation or environment blocker remains on the current candidate.
Independent re-review is pending before a ready transition. The earlier cloud
outage and unpublished source loss remain historical evidence below. Native
assembled-product acceptance remains a milestone 15 gate.

## Handoff

Candidate source SHA: `e47c369b3ea32c08ac110e5d7b6a6aacd65f5c4e` on
`codex/completion-observability-resume`. Inspect the corrected product commit
`353de6a43dde4e1998636d47a80730c90343d307`, the test-only follow-up,
copied candidate receipts, scenario report, final web log and matching fresh
acceptance reports. Obtain independent re-review before changing status. No
push, publication, or completion bookkeeping was performed.

## Historical implementation and executor outage

Task08 started from published main 3ae2efd369322a8b92d6a33ab6b3bb7f50285615. Uncommitted OTEL/local diagnostics/redaction work was left on codex/completion-observability; provisional adjacent Go and separate reviewer fault reproductions passed for selected corrected surfaces. No formal task08 gate or review attestation existed. Canonical compiler/coverage/source rows are approved KB inputs and must remain intact; diagnostic exporters exclude them. The old patch was absent from the restored checkout, so its described fixes were treated as requirements, not recovered code.

At the outage, the selected cloud environment reported starting/offline, and shell calls including pwd stalled for both agents. A readiness wait returned ready without restoring shell/current observed state. This was an external execution blocker; no unvalidated task08 source was published. The original resumable state is in .codex/completion/evidence/08-environment-handoff.json. The present local checkout is healthy and milestone 08 has been rebuilt from current source; assembled candidate-ready/public release remains unclaimed.

Task07 exactB native engineering workflow 36955124703 completed both native and Linux jobs successfully; actual native A/B package/state mechanics uses portable service callbacks and does not prove full launchd fault/realENOSPC/powerloss/restored-A controls. See 07-upgrade-native.json. Its artifacts are remotely retained, not locally materialised during outage. Full native upgrade proof remains a milestone 15 requirement.
