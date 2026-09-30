# 02-variable-estate: Replace exactly-25 product restriction with a bounded variable estate

Status: completed. See the queue for authoritative status and dependencies.

## Baseline and resumption

Seeded from c285773. No product implementation has been performed by the harness.
Read AGENTS.md and the queue; fetch origin/main and verify completed predecessor
commits before starting. Reinspect current code because predecessors change it.

internal/catalog and internal/mirror/registry.go both enforce exactly 25. CLI/bootstrap and public docs assume this. Acceptance schema and private gold-corpus constraints intentionally benchmark 25. Routine proposed product bound: 1..100 repositories; assess resource budgets and record the chosen finite bound before implementation.

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

## Session decisions

User requested all tasks; the one-task stop rule is superseded. Work remains on
scoped sequential branches with verified predecessor publication. Use the existing
catalog.MaxRepositories=100 bound for both catalog and mirror validation. Keep
25-repository acceptance and private-estate constraints unchanged. Matching IDs
and catalog-atomic activation already exist in app/ingest.go and remain mandatory.

## Validated implementation handoff

Result: catalog and mirror registry accept 1–100 repositories with the shared
catalog.MaxRepositories cap. Matching IDs, strict JSON and atomic bootstrap
remain enforced. Public docs describe this bound; the 25-repository fixture and
private gold corpus remain unchanged. Reviewed final scoped diff.

Actual checks (Linux amd64, Go 1.26.1/JDK 21):
- make harness-setup passed, including npm ci and pinned model via Git LFS.
- make harness-validate passed; log /workspace/scratch/02-core-final.log.
- go test ./internal/catalog ./internal/mirror ./internal/app ./cmd/aios passed;
  log /workspace/scratch/02-targeted.log. Tests cover 1,3,25,100; zero/101 and
  mismatched counts/IDs fail, and 1/3-repository bootstrap activates coherently.
- Two fresh make acceptance-v1 runs passed at /workspace/scratch/02-acceptance-a
  and -b. Both semantic_fingerprint values are
  f75584b3be0c5e3750e72144fc072c55af56235e37d8218a39bf2ff8ef7656ea.
- A first test run failed cleaning its read-only owned snapshot fixtures;
  cleanup now restores permissions on fixture directories only; rerun passed.
- git diff --check passed. No native/vector or private-estate verification claimed.

Implementation commit: 383207888a75c2b7abe060c49ce97a917e030a5b.
Verified on fetched origin/main after exact-object GitHub API publication with
force=false. Completed status recorded in this follow-up bookkeeping commit.
No remaining blocker for this task. User requested the whole queue; continue
with the next eligible task after verifying this bookkeeping publication.
