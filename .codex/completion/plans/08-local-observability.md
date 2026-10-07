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
the corrected source commit is `353de6a43dde4e1998636d47a80730c90343d307`.
Task remains in progress pending reviewer recheck and disposition of a variable
full-browser failure. No publication or candidate-ready claim has been made.

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

## Independent review

Independent reviewer identified the ID-prefix privacy bypass and required a
corrected commit. Re-review of `353de6a43dde4e1998636d47a80730c90343d307`
and its receipts is pending. Reviewer must also decide whether the broader
browser timing failure blocks task08; the task is not ready yet.

## Blockers

The current blocker is review disposition of the corrected privacy fix and
the committed-source full browser failure. The earlier cloud outage and
unpublished source loss remain historical evidence below. Native
assembled-product acceptance remains a milestone 15 gate.

## Handoff

Implementation SHA: `353de6a43dde4e1998636d47a80730c90343d307` on
`codex/completion-observability-resume`. Inspect the corrected commit, copied
receipts, scenario report, full web log and fresh acceptance reports. Obtain
independent re-review before changing status. No push, publication, or
completion bookkeeping was performed.

## Historical implementation and executor outage

Task08 started from published main 3ae2efd369322a8b92d6a33ab6b3bb7f50285615. Uncommitted OTEL/local diagnostics/redaction work was left on codex/completion-observability; provisional adjacent Go and separate reviewer fault reproductions passed for selected corrected surfaces. No formal task08 gate or review attestation existed. Canonical compiler/coverage/source rows are approved KB inputs and must remain intact; diagnostic exporters exclude them. The old patch was absent from the restored checkout, so its described fixes were treated as requirements, not recovered code.

At the outage, the selected cloud environment reported starting/offline, and shell calls including pwd stalled for both agents. A readiness wait returned ready without restoring shell/current observed state. This was an external execution blocker; no unvalidated task08 source was published. The original resumable state is in .codex/completion/evidence/08-environment-handoff.json. The present local checkout is healthy and milestone 08 has been rebuilt from current source; assembled candidate-ready/public release remains unclaimed.

Task07 exactB native engineering workflow 36955124703 completed both native and Linux jobs successfully; actual native A/B package/state mechanics uses portable service callbacks and does not prove full launchd fault/realENOSPC/powerloss/restored-A controls. See 07-upgrade-native.json. Its artifacts are remotely retained, not locally materialised during outage. Full native upgrade proof remains a milestone 15 requirement.
