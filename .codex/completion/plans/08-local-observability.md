# Deliver local OTEL and PII-redacted diagnostics

Requirements: R23, R24

## Acceptance

- Trace/query/ingest/job/upgrade correlation, bounded retention and local-only collectors.
- Redaction before log/trace/metric persistence and bundle export; planted PII/secret/query/source fixtures do not leak.
- Default redacted archive plus warned optional expanded dump; expanded still never emits raw secrets.
- UI diagnostic details/export include versions, coverage, stages and timings without source content.

## Decisions

Inspect current implementation before selecting changes.

## Progress

In progress; blocked by the execution environment outage described below.

## Scenario scope

Run existing core/web gates plus scoped product scenarios with `gate NAME --milestone 08-local-observability`. Full assembled/native gates are final-candidate requirements. Record deferred native checks explicitly.

## Actual validation

Only provisional checks on the uncommitted implementation have run; see the handoff. Formal receipts and an independent attestation remain pending.

## Independent review

Separate reviewer report and corrected commit required.

## Blockers

Current execution environment observations report starting/offline. Shell commands stall. Native final acceptance remains mandatory after access returns.

## Handoff

Persist implementation SHA, publication evidence and next work. Continue mission.

## In-progress implementation and executor outage

Task08 started from published main 3ae2efd369322a8b92d6a33ab6b3bb7f50285615. Uncommitted OTEL/local diagnostics/redaction work was left on codex/completion-observability; provisional adjacent Go and separate reviewer fault reproductions passed for selected corrected surfaces. No formal task08 gate or review attestation exists. Canonical compiler/coverage/source rows are approved KB inputs and must remain intact; diagnostic exporters exclude them. Current operational legacy cleanup, opaque-key purge, inherited trace IDs/exemplars, accurate export outcomes, root recovery integration, real outbound/sink probes and live browser controls still need completion.

The selected cloud environment now reports current starting/offline, and shell calls including pwd stall for both agents. A readiness wait returned ready without restoring shell/current observed state. This is an external execution blocker; no unvalidated task08 source was published. Full resumable state and unapplied corrections are in .codex/completion/evidence/08-environment-handoff.json. Preserve all local dirty/untracked work and reconcile this metadata-only remote update after access returns. Resume task08, then the finite remaining mission09–15; assembled candidate-ready/public release remains unclaimed.

Task07 exactB native engineering workflow 36955124703 completed both native and Linux jobs successfully; actual native A/B package/state mechanics uses portable service callbacks and does not prove full launchd fault/realENOSPC/powerloss/restored-A controls. See 07-upgrade-native.json. Its artifacts are remotely retained, not locally materialised during outage. Full native upgrade proof remains a milestone 15 requirement.
