# 05-local-snapshots: Add direct indexing through immutable snapshots of local Git workspaces

Status: completed. See the queue for authoritative status and dependencies.

## Baseline and resumption

Seeded from c285773. No product implementation has been performed by the harness.
Read AGENTS.md and the queue; fetch origin/main and verify completed predecessor
commits before starting. Reinspect current code because predecessors change it.

The supported CLI only ingests immutable mirror archives; internal/app has older root-index helpers which are not proof of safe public local ingestion. Routine initial policy: committed clean tracked Git trees, explicitly reject dirty/untracked inputs; expand only with a reviewed deterministic snapshot contract. Never run source Git hooks.

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

Approve a bounded local registry separately from catalog policy. Require canonical
Git top-level paths, clean tracked/staged state and no nonignored untracked files;
ignored files remain uncaptured. Reject symlinks, submodules and partial clones.
Read pinned Git tree/blob objects with optional locks and lazy fetch disabled,
not mutable files or source hooks. Bound all captured tracked bytes/files by catalog
limits and seal the owned snapshot before compilation. Explicit CLI and browser
local routes retain catalog-atomic activation and stable caller-approved IDs.

## Validated implementation handoff

Result: explicit local ingest CLI and authenticated browser source selector accept
approved matching bounded local registries. Clean tracked commits are copied from
pinned Git tree/blob objects into read-only owned snapshots before compilation.
Dirty/staged/nonignored untracked inputs, partial clones, symlinks/submodules,
unsafe or overlapping paths, case collisions and exceeded capture budgets fail.
Ignored untracked files remain untouched. Optional locks and lazy fetch are disabled;
no source hooks/builds run. Reuse verifies snapshot content against the pinned tree.
Source IDs remain explicitly approved IDs; query excerpts expose captured Git commit,
SHA256 and generation. Existing mirror/bootstrap/activation semantics remain intact.
Identical content retains existing canonical generation and its original captured
revision; revision discovery records the new commit separately. Reviewed final diff.

Actual Linux checks:
- make harness-validate passed (/workspace/scratch/05-core.log).
- go test ./internal/adapter ./internal/discover ./internal/provenance
  ./internal/mirror ./internal/app ./internal/store ./cmd/aios ./internal/webui
  ./internal/knowledge passed (/workspace/scratch/05-targeted-final.log).
- PLAYWRIGHT_CHANNEL=chromium make harness-validate-web passed: unit tests,
  build and 14 browser cases (/workspace/scratch/05-web.log). Real-server tests
  cover mirror and local first run at both viewports, queries and cited source.
- Snapshot tests cover source file modes and Git-index bytes unchanged, dirty and
  untracked rejection without edits, snapshot reuse, bounds and symlink rejection.
  A concurrent-edit test mutates a workspace after capture staging begins and
  proves no snapshot is published and the user's concurrent edit is retained.
- Local ingestion integration checks captured-commit/hash provenance, dirty failure
  retaining the catalog, source bytes/modes retained, and committed delta activation.
- Two fresh make acceptance-v1 runs passed at /workspace/scratch/05-acceptance-a
  and -b with equal fingerprints:
  f75584b3be0c5e3750e72144fc072c55af56235e37d8218a39bf2ff8ef7656ea.
- git diff --check passed. No native macOS or private-estate claim.

Implementation commit: f3bcc832fafd49eaea1f311d86a7b050496268e3.
Verified on fetched origin/main after exact-object GitHub API publication with
force=false. Completed status recorded in this follow-up bookkeeping commit.
No remaining blocker for this task. User requested the whole queue; continue
with the next eligible task after verifying this bookkeeping publication.
