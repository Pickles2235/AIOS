# 04-mirror-onboarding: Add first-run onboarding for mirrored repositories

Status: ready. See the queue for authoritative status and dependencies.

## Baseline and resumption

Seeded from c285773. No product implementation has been performed by the harness.
Read AGENTS.md and the queue; fetch origin/main and verify completed predecessor
commits before starting. Reinspect current code because predecessors change it.

Current webui tests reject /api/v1/onboarding/index and the live UI says to index the catalog first. Mirror registry and catalog are strict separate files. Resolve setup mutations on a narrowly scoped authenticated browser/operator surface while keeping all knowledge MCP operations read-only.

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

Fresh ui serve may omit --config and initialize only owned durable state. Approved
mirror IDs/URLs/full refs are a bounded typed browser request. Persist configuration
and job stage atomically under owned data; no caller-selected write path. Background
sync/ingestion uses fixed controlled staging and the existing atomic catalog route.
Setup mutations require Origin/session/CSRF. Restart marks unfinished jobs interrupted;
retry is explicit and retains the last published generation. Fix JSON casing at the
knowledge browser boundary so real API replies match the live UI (not just mocks).

## Validated implementation handoff

Result: fresh ui serve --data-dir initializes owned state without requiring a
catalog. The live browser configures 1–100 approved mirror IDs/URLs/full refs,
starts sync and atomic immutable-snapshot ingestion, reports actual stages and
errors, and permits explicit retry/cancel. Restart marks unfinished work
interrupted and restores active catalog identity. All setup mutations require
session/Origin/CSRF and cannot choose write paths. Mirrors reject remote helpers
and embedded credentials and refuse silently changed remotes. Knowledge MCP
remains read-only. Fixed lowercase browser JSON fields, max_lines request
binding and empty arrays, which real-server E2E exposed. Stopped tracking the
already-ignored generated Playwright run-state file. Reviewed scoped source,
documentation and production-generated asset diffs.

Actual Linux validation:
- make harness-validate passed (/workspace/scratch/04-core-verified.log).
- go test ./internal/webui ./internal/app ./internal/mirror ./cmd/aios passed
  (/workspace/scratch/04-targeted-final.log).
- PLAYWRIGHT_CHANNEL=chromium make harness-validate-web passed: unit tests,
  build and 12 browser cases at two viewports (/workspace/scratch/04-web-passed.log).
  Real Go-server browser tests created approved fixture remotes, authenticated
  with a one-use capability, configured/synced/ingested, queried and read cited
  source evidence, and verified unchanged source Git status.
- API integration covers missing CSRF, unsafe remote helpers, real bootstrap,
  canonical evidence reads, cancellation retaining the generation and restart
  recovery. Earlier real-browser failures exposed JSON binding/null arrays;
  fixed and reran. A concurrent web rebuild briefly removed embedded dist files
  during a race build; reran the core gate after web builds completed.
- Two fresh make acceptance-v1 runs passed at /workspace/scratch/04-acceptance-a
  and -b; both fingerprints equal
  f75584b3be0c5e3750e72144fc072c55af56235e37d8218a39bf2ff8ef7656ea.
- git diff --check passed. Native macOS/private-estate verification not claimed.
