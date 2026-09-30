AIOS had no repository-local workflow for independent Codex sessions. This adds
root agent instructions, seven dependency-ordered product tasks with persistent
plans, repeatable dependency setup and validation targets, an optional one-task
CLI launcher, offline regression tests, CI and a trunk handoff template
(with an optional PR template for separately requested PR work).

The launcher creates a dedicated branch, refuses dirty worktrees, requires
completed predecessor commits in origin/main and HEAD, prevents overlapping linked
worktree runs, inherits the configured model/authentication and retains failure
logs. No product task is implemented and all seven remain pending. No automatic
merge, model scheduling or continuous loop is introduced.

Validation on Linux amd64:

- Harness regression tests: 11 passed; all Python contracts: 25 passed.
- Go vet, all tests, race tests and native build passed after providing Go/JDK.
- Web unit tests: 27 passed; embedded production build passed with no asset diff.
- Two fresh 25-repository fixture gates: accepted, 12 cases passed each, identical
  semantic fingerprint f75584b3be0c5e3750e72144fc072c55af56235e37d8218a39bf2ff8ef7656ea.
- Browser E2E assertions blocked: Chrome is missing and installation requires
  unavailable root access. Apple Silicon first-run/vector execution and the
  private real-estate gate were not run in this Linux environment.

Final diff inspected; implementation preserves existing canonical IR,
retrieval/provenance/result-state, source non-mutation, read-only MCP and browser
security behavior. CI configuration is included; remote CI has not been verified.
Independent tasks publish directly to main with normal fast-forward pushes and
only become completed after their implementation commits are verified on main.
Full decisions, results, artifacts and publishing/resumption instructions are in
`.codex/plans/setup.md`. After publication on main, start the first independent task with
`python3 scripts/codex_harness.py run 01-query-planning-docs` from a clean checkout.
