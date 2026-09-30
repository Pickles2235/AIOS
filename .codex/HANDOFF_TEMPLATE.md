# One-task trunk handoff

Task ID, local branch and implementation commit:

## Result and scope

Describe the concrete problem and resulting behavior. Map observable task
acceptance criteria to evidence and explain relevant contract/migration decisions.
Confirm that the final diff was inspected and only scoped changes were committed.

## Actual validation

Record exact commands, environment, results and artifact paths. Distinguish
passed, failed and unrun checks with reasons. Fixture checks do not prove native
macOS first-run experience or private-estate readiness.

## Publication and resumption

Record the normal fast-forward push to main and verification against fetched
origin/main. Mark ready after validation; mark completed and record the full
implementation SHA as completion_commit only after verified publication. Publish
status/plan bookkeeping in a follow-up commit. If publication fails, preserve the
branch, ready status and precise blocker with recovery commands. Link the task's
persistent plan and state the next action. Stop; do not start another task.
