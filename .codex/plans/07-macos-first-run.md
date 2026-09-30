# 07-macos-first-run: Verify macOS Apple Silicon installation and first-run experience

Status: in_progress. See the queue for authoritative status and dependencies.

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

## Session assessment

Task 06 and its bookkeeping are verified on origin/main. User requested all tasks,
so proceed through this final task. Host is Linux x86_64; no interactive Apple
Silicon session was provided. GitHub macos-15 arm64 runners are accessible for
automated checks. Run 36760467766 failed because test temporary paths traversed
macOS /var aliases; normalize owned fixture paths, retaining production symlink
rejection. Extend native arm64 CI with core/race/browser gates, two fresh fixture
acceptance runs, candidate archive/checksum/clean-install checks and artifacts.
Native CI evidence cannot replace the explicit manual Mac first-run checklist.
Windows baseline/native CI failures are outside this Mac task; do not claim its
platform verified. Final status stays blocked if interactive verification cannot
be completed, with full recovery instructions and no completion SHA.

## Prepared native verification implementation

Branch: codex/07-macos-first-run. Canonicalized owned Go/browser fixture paths
for macOS aliases, without changing production path policy. Added JDK 21 to
native CI, native arm64 release/checksum/installed-browser/dual-acceptance/offline
archive verification, and uploaded operator evidence. The candidate version is
1.0.0-harness, for test packaging only. No release/tag/signing claim was made.
Added docs/how-to/verify-macos-first-run.md with exact native commands and the
manual identity/logo/permissions/interrupt/retry/source-nonmutation checklist.

Passed locally on Linux x86_64, Go 1.26.1/JDK21/Node24/Python3.12.14:
- make harness-validate (queue/Python, Go tests/vet/race/build): 07-core.log.
- PLAYWRIGHT_CHANNEL=chromium make harness-validate-web: 27 unit/16 browser tests
  passed, 07-web.log. After adding the explicit stable-ID assertion, the four real
  mirror/local browser cases at both viewports passed again: 07-browser-final.log.
- sh -n scripts/verify-macos-first-run.sh passed; running it on Linux refused
  native verification with exit 2, 07-native-refusal.log.
- git diff --check passed; final scoped source/fixture/workflow/docs diff inspected.
All local logs are under /workspace/scratch; private logs are not committed.

Not yet run on the repaired revision: native core/race/browser/release/checksum,
two native acceptance gates and installed offline archive acceptance. Publish
this validated fixture/automation repair normally to main, then inspect its exact
Apple Silicon job and record native results. No interactive Mac session is
available; manual checklist and private-estate readiness remain unverified.
This is an in-progress implementation publication, not task completion.

## Native failure and repair

Published preparation commit e3c9f1882b3cc8e15c277a101e15f7c56efbb16a was verified
on fetched origin/main. Native run 36765310784, Apple Silicon job 110057869504,
failed local snapshot/app tests on directory rename permission; canonical fixture
normalization succeeded and the pinned native Nomic test passed (38.979 seconds).
Intel macOS reproduced the same failure. No downstream release/browser/acceptance
step ran, and none is claimed as passed.

Darwin requires writable permission on the source directory during rename. The
repair reopens only the owned staging root for publication, renames it, then
seals the final root to 0500 before returning it to any compiler. Nested files
stay 0400 and directories 0500. Sealing failures clean up the renamed owned
staging directory; no source path is touched or generation activated. Added
explicit root/file sealing assertions. Native browser checks now press Command-K
on Darwin (Ctrl-K on Linux). New JDK setup uses supported setup-java@v5.

Repair validation on Linux: make harness-validate passed (07-core-repair.log),
PLAYWRIGHT_CHANNEL=chromium make harness-validate-web passed 27 unit/16 browser
checks (07-web-repair.log), two fresh make acceptance-v1 runs at
/workspace/scratch/07-acceptance-a and -b accepted with equal fingerprint
f75584b3be0c5e3750e72144fc072c55af56235e37d8218a39bf2ff8ef7656ea.
Scoped diff and git diff --check inspected. Native rerun remains required.

## Native vector investigation and bounded-runtime repair

Snapshot repair e6d78a7412c039d620a57f8e26e25d9de1bdd3af is verified on main.
Run 36766560106 / Apple Silicon job 110062085772 passed Go tests/vet, Python
contracts, all 27 web units and native compilation. Native acceptance failed only
route_vector; the other 13 cases passed. The summary omitted the failure details,
so no specific cause is asserted as proven. Downstream archive/browser/offline
checks were skipped by the failing preliminary step. Windows CI also failed;
no Windows platform verification is claimed.

To investigate and preserve the bounded contract, use CPU execution with two
threads and no helper warmup (supported flags confirmed in the pinned helper),
retaining pinned model/hash, mean pooling and 768 dimensions. Pass context through
native embeddings, projection builds and vector reads. MCP gives vector work the
remaining existing query deadline, terminates the helper on exhaustion and reports
unknown with time_budget, without claiming a lost index or recommending rebuild.
No public budget, fixture expectation or evidence rule was relaxed. Added actual
cancellation/deadline regressions and native runtime permission assertions.

Acceptance failure summaries now include trace/uncertainty/budget diagnostics.
Native CI uploads the preliminary acceptance report even on failure and attempts
installed-candidate verification independently, collecting whatever native stages
actually pass. Archive evidence now records the digest itself as well as checksum
verification. Native results for this repair are pending publication/rerun.

Local Linux repair checks passed: make harness-validate (07-vector-core.log),
go test ./internal/semantic ./internal/vector ./internal/store ./internal/mcp
(07-vector-targeted-final.log), sh -n and git diff --check. Two fresh hermetic
runs at /workspace/scratch/07-vector-acceptance-a and -b accepted with the same
f75584b3be0c5e3750e72144fc072c55af56235e37d8218a39bf2ff8ef7656ea fingerprint.
Native model execution and performance remain unverified on the repaired revision
until its exact Apple Silicon CI job completes. Manual observations still pending.

## Startup asset preparation repair

Commit 7049dcb73f2d3c83dd4662bb54b3ed92cf83acbc is verified on main. Native run
36769552202 / Apple Silicon job 110072170307 passed core/vet/Python/native build,
then native acceptance failed route_vector with unknown/vector_time_budget at
1626 ms. Independent native verification passed make harness-validate (including
race), candidate archive/checksum and all 27 units/16 installed-binary browser
cases, then acceptance-a failed the same vector budget at 1391 ms. Acceptance-b
and installed offline acceptance therefore did not run. This is partial native
evidence, not accepted native release verification.

Artifact 11122614444 (macos-first-run-evidence) was downloaded and inspected.
Actual host: macOS 15.7.9 build 24G830, Darwin arm64; Go1.25.0, Node22.23.2,
Python3.14.7, Temurin21.0.12.1. Candidate archive SHA256:
9888f508a48ee17a7cd8b48a5233af77f67bd8691cba2990f4bfd3ceeef23154.
Reports and environment/checksum/build metadata are retained outside the repo at
/workspace/scratch/07-native-7049-evidence. No logs or tokens were committed.

Prepare/verify owned model and helper assets during MCP startup, before accepting
requests. Individual retrievals retain the same deadline and still terminate
native inference on exhaustion. Reusing already-correct asset modes avoids
unnecessary cached-executable metadata changes. Optional runtime setup failures
leave canonical retrieval available. No inference or source query is hidden in
startup, and no time budget or native acceptance expectation was changed.

Repair validation: make harness-validate passed (07-startup-core.log); two fresh
Linux make acceptance-v1 runs at 07-startup-acceptance-a/-b accepted with identical
f75584b3be0c5e3750e72144fc072c55af56235e37d8218a39bf2ff8ef7656ea fingerprint.
Scoped diff and git diff --check inspected. Native rerun and manual checks remain.
