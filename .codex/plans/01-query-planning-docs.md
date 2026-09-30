# 01-query-planning-docs: Clarify deterministic query planning versus agent/task planning

Status: pending. See the queue for authoritative status and dependencies.

## Baseline and resumption

Seeded from c285773. No product implementation has been performed by the harness.
Read AGENTS.md and the queue; fetch origin/main and verify merged predecessor
commits before starting. Reinspect current code because predecessors change it.

README.md currently says no planning while also advertising bounded hybrid planning; internal/planner implements deterministic query algebra. Disabled capability labels refer to future agent/task behavior.

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

No task PR or commit yet. Record branch, commit, draft PR, reviewed diff summary,
acceptance evidence, remaining limitations and next action. Stop after this task.
After merge, record the reachable merge/squash SHA in the queue and this plan.
