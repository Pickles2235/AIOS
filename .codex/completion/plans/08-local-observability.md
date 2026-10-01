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

Pending.

## Scenario scope

Run existing core/web gates plus scoped product scenarios with `gate NAME --milestone 08-local-observability`. Full assembled/native gates are final-candidate requirements. Record deferred native checks explicitly.

## Actual validation

Record commands, receipts, platform and full tested commit. No checks run yet.

## Independent review

Separate reviewer report and corrected commit required.

## Blockers

None established; native requirements need actual hardware/CI.

## Handoff

Persist implementation SHA, publication evidence and next work. Continue mission.
