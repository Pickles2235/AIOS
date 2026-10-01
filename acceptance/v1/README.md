# V1 acceptance output

Acceptance reports are generated artifacts, not checked-in release claims.
Run `make acceptance-v1 ACCEPTANCE_OUTPUT=/private/acceptance-run` and retain
the resulting `acceptance-report.json` and logs with the CI run.

Generic fixture acceptance must not be presented as assembled-product readiness.
No private corpus is required; full candidate gates are defined in
[the completion contract](../../.codex/completion/ACCEPTANCE.md).
