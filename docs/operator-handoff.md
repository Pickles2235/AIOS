# Operator handoff

## Release disposition

The deterministic 25-repository fixture gate is the software acceptance gate.
It does not assess the actual production repositories. Record production
readiness separately using the checklist in
[`acceptance-handoff.md`](acceptance-handoff.md); do not reuse fixture results as
production evidence.

## Recovery

All builds stage immutable derived state and activate it only after validation.
An interrupted build, failed projection, or invalid activation therefore leaves
the last known good generation queryable. Retry the failed command after fixing
the diagnostic. A missing optional vector projection does not block canonical,
exact, lexical, graph, or structural/path retrieval; rebuild it with
`projections rebuild --kind vector`.

For stale caches or handles, repeat the query against the active generation.
If derived data itself is unrecoverable, stop readers, move the data directory
aside, create a fresh owner-only directory, then sync mirrors and run
`ingest --all`. Preserve the previous directory until the replacement passes
acceptance.

## Upgrade and rollback

Install each archive into a new immutable directory and verify its adjacent
`.sha256` file.
Keep the prior binary and data directory until acceptance succeeds. Roll back
by restoring both prior pointers; source repositories are never modified.
