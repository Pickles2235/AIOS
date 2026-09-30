# Repository-local development harness

This harness was built against `c285773` after inspecting code, tests, docs, and
native CI. No installation ZIP was available. It changes development tooling only.

## Setup and checks

Requirements: real Go 1.25+ (the `go` command must be the Go language toolchain),
CGO and a C compiler, Git + Git LFS, Python 3.10+, Make, Node 22+ with npm,
and a JDK providing java/javac (17+ recommended). The existing full Go tests
require Java compiler coverage even though compiler helpers are optional for
product ingestion. npm ci supplies the repository TypeScript helper.

```sh
make harness-setup          # go mod download, npm ci, pinned model via Git LFS
make harness-test           # offline harness regression tests and queue validation
make harness-validate       # harness, all Python contracts, Go tests/vet/race/build
make harness-validate-web   # web unit tests, production build, Chrome browser tests
make acceptance-v1 ACCEPTANCE_OUTPUT=/absolute/fresh/acceptance-a
make acceptance-v1 ACCEPTANCE_OUTPUT=/absolute/fresh/acceptance-b
```

The LFS model is required by the existing checksum test on every platform even
though native embeddings run only on macOS arm64. Setup does not install system
packages or change authentication. For browser tests, explicitly install the
configured Chrome channel if absent: `cd web && npx playwright install chrome`.
That command can require system package privileges. Acceptance outputs must be
fresh, outside source repositories. Compare report `semantic_fingerprint` values.
The private real-estate gate requires an external reviewed corpus; fixture results
do not substitute for it. Linux cannot verify the Apple Silicon first-run UX.

## Start one independent task

Use a clean checkout containing the harness published on main:

```sh
git fetch origin main
python3 scripts/codex_harness.py next
python3 scripts/codex_harness.py run 01-query-planning-docs
```

The optional launcher calls `codex exec --sandbox workspace-write` once, inherits the user's
configured model and authentication, and passes a task-specific prompt on stdin.
It sets no model, credentials, login, or config override. No scheduled or repeated
model calls occur. Codex CLI must already be installed and authenticated.
A new `codex/<task>-<unique-id>` branch starts at the current HEAD; dependency
completion commits must be ancestors of both HEAD and the locally fetched `origin/main`.
Do not run from a branch carrying unrelated unpublished product work.

Without the CLI, print the exact prompt and give it to a fresh Codex session:

```sh
python3 scripts/codex_harness.py prompt 01-query-planning-docs
```

The launcher refuses tracked, untracked, staged, or submodule dirt, checks queue
eligibility before branching, and holds an exclusive directory lock under the
Git common directory, shared by linked worktrees. Logs and prompt are under
`<git-common-dir>/aios-codex-runs/<run-id>/`, including failures. The branch and
edits remain for inspection. Interrupts terminate and wait for the child before
releasing the lock. A hard kill can leave a lock; verify no launcher/Codex process
is active before manually removing `<git-common-dir>/aios-codex.lock`. Never remove
a live lock. Logs can include repository content; inspect before sharing.

## Queue and persistent plans

`tasks.json` is ordered, schema-versioned, and checked for missing fields, cycles,
invalid statuses, missing plans, and invalid completion records. Each task has scope,
non-goals, observable acceptance criteria, checks, dependencies, and status.
Plans in `.codex/plans/` persist decisions, progress, actual validation, blockers,
and handoffs. Read earlier plans for context without implementing their scope.

Status lifecycle: `pending` → `in_progress` → `ready` → `completed`;
`blocked` requires a recorded reason and explicit return to `pending` before retry.
Use trunk-based publishing: fetch origin/main, reconcile any concurrent trunk
changes, rerun affected validation, then `git push origin HEAD:main` without force.
Only after verifying the implementation commit on fetched origin/main, set
`completed` and `completion_commit` to its full SHA in a follow-up bookkeeping
commit, which also lands directly on main. The implementation SHA refers to the
preceding commit, avoiding a self-referential hash. Record the commit and final
handoff in the plan using `.codex/HANDOFF_TEMPLATE.md`.

If Git transport authentication fails while the configured GitHub Git API has
write access, that API may publish the same Git trees/commits and update
refs/heads/main with force=false. Preserve exact commit hashes, including parent,
message, author/committer and timestamps; verify tree/commit hashes before updating
the ref. Check the current remote head and never overwrite concurrent trunk work.
Re-fetch origin/main to verify publication. This environment used that fallback
successfully; normal Git upload still returned HTTP 401.

A publishing failure leaves the validated task `ready` with its local commits
and blocker preserved. Failed runs require inspection and manual resumption;
the launcher accepts only pending tasks and does not automatically retry. No PR
is required, and no automatic merge or next-task launch occurs. Predecessors
must actually be on main; local completion claims do not satisfy dependencies.

Use `make harness-test` after queue edits. All seven product tasks were seeded
as pending; setting up this harness does not complete any of them.
