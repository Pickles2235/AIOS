# 07-macos-first-run: Verify macOS Apple Silicon installation and first run

Status: blocked. The queue is authoritative; completion_commit is null.
Branch: codex/07-macos-first-run. Tasks 01–06 are completed and their full
implementation SHAs are verified on fetched origin/main and working HEAD.
The user requested all tasks, overriding the one-task session stop convention.

## Decisions and result

Implemented native candidate installation evidence and repaired failures found on
actual Darwin arm64 CI. The workspace is Linux x86_64; GitHub macos-15 arm64
runners provide real native automated validation. No interactive Mac was provided.
CI browser automation does not replace the required manual operator observations.

- Normalize only owned test/browser temporary paths through macOS /var aliases;
  retain production rejection of unsafe/noncanonical/symlink source paths.
- During immutable local snapshot publication, reopen only the owned staging root
  for Darwin rename, then seal the final root to 0500 before compiler access.
  Nested directories/files remain 0500/0400; failed sealing removes owned staging.
  Source bytes, status, index, permissions and active generations stay unchanged.
- Native Nomic inference retains the pinned model/hash, 768 dimensions and mean
  pooling. Use CPU execution, no warmup and available CPU count capped at four;
  preserve caller cancellation and the existing one-second MCP query deadline.
  Exhaustion reports unknown/time_budget without claiming missing evidence.
- Prepare/verify owned model/helper assets at MCP startup, before requests,
  with no hidden inference or query. Correct cached modes avoid metadata churn.
  Optional setup failure keeps deterministic canonical retrieval available.
- The native script builds an unpublished 1.0.0-harness candidate, verifies its
  archive checksum, extracts a clean install and runs real browser checks against
  that installed binary. Two fresh native acceptance gates must match; additional
  installed-binary acceptance runs under sandbox-exec with network access denied.
- Native CI records OS/CPU/memory/toolchains, binary/archive hashes, fingerprints
  and reports in macos-first-run-evidence, including preliminary/failure reports.
  It attempts installed checks independently of preliminary acceptance failures.
  Detailed failure summaries preserve the existing acceptance/fingerprint contract.
- Command-K is exercised on Darwin (Ctrl-K elsewhere); actual mirror/local UI
  cases cover querying, evidence, reload, rename, stable ID, restart and rebuild.

Exact native procedure and manual checklist:
[verify-macos-first-run.md](../../docs/how-to/verify-macos-first-run.md).
No release/tag, signing/notarization or private-corpus readiness claim was made.

## Progress and published implementation

All listed commits were published as exact Git objects using the authenticated
GitHub Git API, force=false, with the parent/main checked before update. Ordinary
Git upload returned HTTP 401. Fetching origin/main verified each publication.
Scoped diffs were inspected; no logs, credentials or private corpus were committed.

1. e3c9f1882b3cc8e15c277a101e15f7c56efbb16a: canonical fixtures, native automation,
   JDK21 and checklist. Run 36765310784 / arm64 job 110057869504 passed native
   Nomic execution but exposed local snapshot rename permissions; downstream
   release/browser/acceptance did not run.
2. e6d78a7412c039d620a57f8e26e25d9de1bdd3af: Darwin snapshot publication repair.
   Run 36766560106 / job 110062085772 passed Go/vet/Python, 27 web units and native
   build. Acceptance failed only route_vector; no diagnostic cause was proven.
3. 7049dcb73f2d3c83dd4662bb54b3ed92cf83acbc: CPU/helper cancellation/deadlines,
   contextual vector build/read and actual deadline regressions; failure reports.
   Run 36769552202 / job 110072170307 passed native core/race, archive/checksum,
   27 units and 16 installed-browser cases. Preliminary vector query failed at
   1626 ms; acceptance-a failed at 1391 ms, so b/offline did not run. Artifact
   11122614444 was inspected at /workspace/scratch/07-native-7049-evidence.
4. 6b9d413ef0cbb6a1913fef4eec69561871e37d17: move asset setup before requests.
   Run 36773168666 / job 110084392270 passed preliminary acceptance, native
   core/race, archive/checksum, 27 units and 16 installed-browser cases. Native
   acceptance-a/b both accepted with matching fingerprint
   6eeab4379b06b9176ddf374e503050290aa89fb3a1690e03a5c6bb5c2caa693a.
   Installed offline acceptance failed only route_vector at 1003 ms. Artifact
   11125586899 was downloaded/inspected at
   /workspace/scratch/07-native-startup-evidence. Actual candidate archive SHA256:
   f660fec8c279c6c6c0dfdfbec2d5d24bc6e6e2752e2556b694d843165b4da3ca;
   installed arm64 Mach-O binary SHA256:
   ad2a725f5525ea9c3eb584ac6472f89fb8060a5e582edf46d0b16f533ac15fcf.
   Host: macOS15.7.9 build24G830, Go1.25.0, Node22.23.2, Python3.14.7,
   Temurin21.0.12.1 and Git LFS3.8.0. Offline success was not established.
5. e0c35b3a0d1002146a82ef88db688f22c443340f: use available bounded CPU parallelism
   rather than fixing every host at two threads; record CPU/memory. Native
   run 36775888433 / Apple Silicon job 110093554943 succeeded. Current
   implementation SHA is verified on main; complete native evidence follows.

Linux amd64/arm64 and Intel Mac jobs also passed on the bounded-CPU repair.
Windows job 110093555058 failed; job 110084391606 on the prior revision showed
existing canonical ownership failures ("path must be owned by
the current user"). Overall multi-platform CI is not green; Windows is unverified.

## Accepted native candidate evidence

Exact implementation: e0c35b3a0d1002146a82ef88db688f22c443340f.
[Apple Silicon job](https://github.com/Pickles2235/AIOS/actions/runs/36775888433/job/110093554943)
completed successfully. Overall workflow failed because Windows failed; the
Apple Silicon job and Linux/Intel Mac jobs succeeded.

- Actual host: macOS15.7.9 build24G830, Darwin arm64, 3 CPUs, 7516192768 bytes
  memory. Go1.25.0, Node22.23.2, Python3.14.7, Temurin21.0.12.1, Git LFS3.8.0.
  The C compiler version was not captured; record it in the manual handoff.
- Native make harness-validate passed including Go tests/vet/race/build and
  harness/Python contracts. The pinned native Nomic execution/permissions test
  passed. Candidate make release and adjacent checksum verification passed.
- Installed Mach-O arm64 binary: make harness-validate-web passed all 27 units
  and 16 real/mock browser cases, including native Command-K, mirror/local
  onboarding, evidence, reload, rename, stable ID, restart and rebuild.
- Preliminary acceptance, two fresh make acceptance-v1 runs, and additional
  installed-binary acceptance with network denied all accepted: 14/14 cases each.
  All four actual reports were downloaded and independently compared; they share
  semantic_fingerprint:
  6eeab4379b06b9176ddf374e503050290aa89fb3a1690e03a5c6bb5c2caa693a.
  Native CPU inference is verified offline within the unchanged query budget.
- Candidate archive SHA256:
  0d84850189a2c9e8d88b8bbb7a27ffe1f88e8670268fc840b670b55b8bb5c1e8.
  Installed binary SHA256:
  03f70c7563bbeaf8ad348c11281b4d883fb2c33040bc52c302795de83b0ff9a1.
- Artifact 11127060810, macos-first-run-evidence, was downloaded and inspected.
  Receipts are retained outside the repo at /workspace/scratch/07-native-cpu-evidence;
  ZIP: /workspace/attachments/9ea7f529-5278-4a04-baf8-9b666d59153d/macos-first-run-evidence.zip.
  Archive checksum, installed build metadata, environment, fingerprints and actual
  reports are recorded; no private logs/tokens are committed.
- Harness CI run 36775888360 also succeeded for this exact implementation.

## Actual validation

Linux x86_64, Go1.26.1/JDK21/Node24/Python3.12.14. Logs and fixtures are outside
source repositories under /workspace/scratch; no fixture proves native UX.

- make harness-validate passed after native preparation, snapshot repair,
  contextual vector repair, startup preparation and bounded-CPU repair:
  07-core.log, 07-core-repair.log, 07-vector-core.log, 07-startup-core.log,
  07-cpu-core.log. Includes harness/Python contracts, Go tests/vet/race/build.
- PLAYWRIGHT_CHANNEL=chromium make harness-validate-web passed 27 units and 16
  real/mock browser cases at both viewports: 07-web.log and 07-web-repair.log.
  After the stable-ID assertion, four real mirror/local cases passed again:
  07-browser-final.log. Browser asset builds ran separately from Go core gates.
- go test ./internal/semantic ./internal/vector ./internal/store ./internal/mcp
  passed: 07-vector-targeted-final.log, including actual cancellation/deadline
  regressions and correct unknown/budget feedback.
- Two fresh make acceptance-v1 runs after each ingestion/retrieval repair accepted
  with identical Linux fingerprint
  f75584b3be0c5e3750e72144fc072c55af56235e37d8218a39bf2ff8ef7656ea.
  Directories: 07-acceptance-a/-b, 07-vector-acceptance-a/-b,
  07-startup-acceptance-a/-b and 07-cpu-acceptance-a/-b.
- sh -n scripts/verify-macos-first-run.sh and git diff --check passed. Native script
  correctly refused Linux with exit2 (07-native-refusal.log).

## Blockers and remaining observations

No interactive Apple Silicon session is available. Manual Gatekeeper/quarantine
launch, visual/accessibility branding/logo/colour/fallback, interruption/retry UX,
keyboard/focus observations, restart/rebuild identity and operator-recorded source
byte/mode/index hashes have not been performed. Automated cases cover overlapping
behavior but do not establish these explicit manual checks. Private-estate gate
was not run: no reviewed private corpus was supplied. Signing/notarization was
not assessed or promised. Resolve every required native/manual check before
marking completed or recording a completion_commit.

## Handoff and recovery

The explicit manual Mac checklist remains blocked by missing interactive Apple
Silicon access. Do not mark completed from CI automation. All automated native
checks on the published implementation passed. This follow-up changes only task
status and evidence; completion_commit stays null until manual checks pass.

To resume on an actual interactive Apple Silicon Mac:

1. Fetch origin/main in a clean checkout and verify all six completion SHAs are
   ancestors. Preserve unrelated work. Read AGENTS.md, the queue and this plan.
2. Explicitly return task 07 from blocked to pending and record available Mac
   access in this plan. Validate with make harness-test, publish that bookkeeping
   normally and verify it on fetched main before using the clean-tree launcher.
3. Run python3 scripts/codex_harness.py prompt 07-macos-first-run (or the authorized
   one-shot run when Codex CLI is installed). Use a dedicated task branch and mark
   in_progress. Install required native toolchains, run make harness-setup, and
   follow docs/how-to/verify-macos-first-run.md against the reviewed exact commit.
4. Execute the native procedure and every manual observation on the installed
   candidate. Retain private observations/source hashes outside commits. Record
   actual OS/tools, archive/binary checksums, acceptance reports/fingerprints,
   operator results and any remediation. Revalidate any repaired behavior.
5. Publish scoped repairs without force, fetch and verify the implementation SHA
   on main. Only after every native/manual requirement passes, record completed
   and that full implementation SHA in a follow-up bookkeeping commit. Publish
   and verify the bookkeeping too. No release/signing/private-estate claims follow
   automatically from task completion.

Final bookkeeping validation: make harness-test passed all 11 tests after restoring
the required capitalized plan headings (07-final-harness.log). git diff --check
passed. The final diff contains only this plan and the blocked queue status; it
was inspected. All six completed task SHAs are ancestors of HEAD and fetched
origin/main; HEAD matches main before bookkeeping. Publish this scoped follow-up
through the same exact-object/force=false fallback, then fetch and verify it on
main. Preserve the task branch and blocked/manual handoff if publication fails.
