# 07-macos-first-run: Verify macOS Apple Silicon installation and first-run experience

Status: pending. See the queue for authoritative status and dependencies.

## Baseline and resumption

Seeded from c285773. No product implementation has been performed by the harness.
Read AGENTS.md and the queue; fetch origin/main and verify completed predecessor
commits before starting. Reinspect current code because predecessors change it.

Native CI already includes macos-15 arm64 with Git LFS and fixture acceptance, but CI compilation is not manual first-run experience. The pinned model is a Git LFS object, with bundled Darwin arm64 helper; capture OS, architecture, toolchain, archive hash and observations without secrets.

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
