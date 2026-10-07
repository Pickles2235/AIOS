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
in place. The full core and browser validation commands pass; committed-source
scoped gates and independent review remain pending.

## Scenario scope

Run existing core/web gates plus scoped product scenarios with `gate NAME --milestone 08-local-observability`. Full assembled/native gates are final-candidate requirements. Record deferred native checks explicitly.

## Actual validation

Focused `go test ./internal/observability ./internal/store ./internal/app
./internal/webui ./internal/lifecycle` passed after the ledger change; direct
task08 product scenarios and the live Chrome diagnostics browser flow passed.
On Darwin arm64, `PATH=/opt/homebrew/bin:$PATH GIT_CONFIG_COUNT=2
GIT_CONFIG_KEY_0=maintenance.auto GIT_CONFIG_VALUE_0=false
GIT_CONFIG_KEY_1=gc.auto GIT_CONFIG_VALUE_1=0 make harness-validate` passed
Python, vet, Go, race and build. A first full run had an isolated timing failure
in mirror's pipe helper; that test passed alone and the exact full rerun passed.
`PATH=/opt/homebrew/bin:$PATH make harness-validate-web` passed 27 unit and
36 live Chrome E2E cases with rebuilt embedded assets. Earlier full browser
runs caught the Direct-source choice race and a variable native namespace
collision; the final unchanged default gate passed after the targeted fix.
`make acceptance-v1 ACCEPTANCE_OUTPUT=/tmp/aios-task08-acceptance-1-20261007T1212`
and the same command with `...-2-20261007T1215` each accepted 14 cases from
fresh external directories with equal semantic fingerprint
`6c2507e8994d4d53e0ec7fb25cf5eed8ad3179b9915bdac3a6517a584d9ffcd0`.
Formal committed-source core/privacy receipts and independent attestation
remain pending; add their exact results after the implementation commit.

## Independent review

Separate reviewer report and corrected commit required.

## Blockers

No current environment blocker. The earlier cloud outage and unpublished source
loss remain historical evidence below. Native assembled-product acceptance
remains a milestone 15 gate.

## Handoff

Persist implementation SHA, publication evidence and next work. Continue mission.

## Historical implementation and executor outage

Task08 started from published main 3ae2efd369322a8b92d6a33ab6b3bb7f50285615. Uncommitted OTEL/local diagnostics/redaction work was left on codex/completion-observability; provisional adjacent Go and separate reviewer fault reproductions passed for selected corrected surfaces. No formal task08 gate or review attestation existed. Canonical compiler/coverage/source rows are approved KB inputs and must remain intact; diagnostic exporters exclude them. The old patch was absent from the restored checkout, so its described fixes were treated as requirements, not recovered code.

At the outage, the selected cloud environment reported starting/offline, and shell calls including pwd stalled for both agents. A readiness wait returned ready without restoring shell/current observed state. This was an external execution blocker; no unvalidated task08 source was published. The original resumable state is in .codex/completion/evidence/08-environment-handoff.json. The present local checkout is healthy and milestone 08 has been rebuilt from current source; assembled candidate-ready/public release remains unclaimed.

Task07 exactB native engineering workflow 36955124703 completed both native and Linux jobs successfully; actual native A/B package/state mechanics uses portable service callbacks and does not prove full launchd fault/realENOSPC/powerloss/restored-A controls. See 07-upgrade-native.json. Its artifacts are remotely retained, not locally materialised during outage. Full native upgrade proof remains a milestone 15 requirement.
