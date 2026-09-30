# 03-knowledge-instance: Persist a named knowledge instance with stable identity, optional logo and seed colour

Status: completed. See the queue for authoritative status and dependencies.

## Baseline and resumption

Seeded from c285773. No product implementation has been performed by the harness.
Read AGENTS.md and the queue; fetch origin/main and verify completed predecessor
commits before starting. Reinspect current code because predecessors change it.

Inspect store schema migration and web session/API before designing persistence. Current active main.tsx does not provide durable instance branding. Prefer a bounded raster logo stored locally, no remote URL loading; record final metadata contract and compatibility decisions.

## Decisions

Initial design guidance above is provisional until checked against current code.
Record each decision, alternatives, compatibility implications and reason here.
No unresolved public contract decision is currently blocking harness setup.

## Progress

- Harness seed only; task is not started.
- Next: verify eligibility, inspect scoped code/tests/docs, mark in_progress,
  implement the smallest complete change satisfying queue acceptance.

## Actual validation

Not run for this product task. Record command, environment, exit/result, artifact
path and any failure here. Distinguish passed, failed, and not run; include reasons
and native/private-corpus limitations. Harness validation belongs in setup.md.

## Blockers and recovery

None assessed yet. Add exact symptoms and a concrete resume instruction if blocked.

## Handoff

No task commit yet. Record branch, implementation commit, inspected diff summary,
acceptance evidence, publication result, remaining limitations and next action.
Publish validated scoped commits directly to main with a normal fast-forward
push, then verify origin/main contains them. Set completed and completion_commit
in a follow-up bookkeeping commit only after that verification. Stop after this
task. Publishing failures leave the task ready with exact recovery instructions.

## Session design

Persist owner-only instance.json separately from canonical index.db so resets and
projection rebuilds retain identity. Lazily migrate existing data to a random stable
ID, Homefold name and default colour without changing IR format. Accept only bounded
PNG/JPEG logos re-encoded locally; no remote URLs or SVG. Edits use the authenticated
same-origin CSRF browser API; MCP remains read-only.

## Validated implementation handoff

Result: owner-only instance.json persists a random stable ID, editable name,
seed colour and optional normalized PNG/JPEG logo. Browser session/Origin/CSRF
protect edits; the read-only MCP and canonical IR format are unchanged. Metadata
survives restart, incremental generations, projection rebuilds and IR resets.
Existing directories lazily receive safe Homefold defaults. Logos reject SVG,
URLs, malformed bytes and oversized bytes/dimensions; missing logos have an
accessible initial. The actual main.tsx displays and edits the persisted values.
Reviewed source and generated asset diffs, built assets using make web-build.

Actual Linux validation:
- make harness-validate passed (/workspace/scratch/03-core-final.log).
- go test ./internal/store ./internal/webui ./cmd/aios passed
  (/workspace/scratch/03-targeted-final.log).
- PLAYWRIGHT_CHANNEL=chromium make harness-validate-web passed, including
  unit tests, production build and 8 live-entry browser tests at both viewports
  (/workspace/scratch/03-web.log). Added optional channel selection because the
  worker cannot install system Chrome; locally installed Chrome for Testing ran.
- Persistence tests exercise existing-data defaults, rename/stable ID, raster
  logo, restart, incremental generation/rebuild, IR reset, permissions and
  symlink rejection. API tests reject missing CSRF and unauthenticated reads.
- Initial persistence test lacked an active catalog for the rebuild; fixed its
  fixture to publish a canonical generation, then all checks passed.
- git diff --check passed. No native macOS or private-corpus claim.

Implementation commit: 962a3f093e1ae9c8b0a6d8276e452a40d96ef298.
Verified on fetched origin/main after exact-object GitHub API publication with
force=false. Completed status recorded in this follow-up bookkeeping commit.
No remaining blocker for this task. User requested the whole queue; continue
with the next eligible task after verifying this bookkeeping publication.
