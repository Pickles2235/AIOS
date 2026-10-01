# Reconcile current code and generic V1 contract

Requirements: R01, R15. Implementation branch: `codex/completion-01-baseline-contract`.
Baseline: `2e24f0c` (fetched `origin/main`, 2026-10-01).

## Acceptance and decisions

- Complete inspected requirement-to-code/test/gap map is in `../BASELINE-MAP.md`.
- Preserve canonical IR/planner/generation/provenance and the compatibility Go module
  coordinate. No data-format or query changes. Default product name becomes AgentOS;
  existing saved names/IDs are read unchanged.
- Inventory generator accepts 1–100 generic repositories with no language allowlist;
  explicit patterns and secure deny rules/limits remain. Example catalog has one source.
- Supported installable product is Apple Silicon; Linux CI is developer smoke.
  Optional reviewed 25-repo/150-case acceptance preserves thresholds but is not an
  employer/private-estate prerequisite. Historical task records remain unchanged.
- Browser onboarding needs no MCP/LLM/Codex. Correct current security/upgrade/archive
  claims; native daemon/namespace/upgrades/3D are explicitly pending, not claimed done.

## Progress

In progress. Read AGENTS, current/legacy harness contracts, queue, plan and code/tests.
Implemented generic defaults/docs/CI and rebuilt live UI branding assets. Independent
review session `/root/baseline_reviewer` is inspecting boundaries separately.

## Scenario scope

Bootstrap uses existing core gate and real inventory/default/persistence assertions.
UI changes also run production build/unit/live authenticated E2E. Dispatcher is task02.
Final native proof remains mandatory for task15; no scoped Linux check proves it.

## Actual validation

- `git fetch origin main`: exit 0, baseline unchanged.
- `python3 scripts/completion_harness.py check`: exit 0 (15 milestones/26 requirements).
- `python3 scripts/completion_harness.py next`: selected 01-baseline-contract.
- User-requested `python3 scripts/completion_harness.py run`: exit 1. CLI child failed
  authentication before work: expired/logged-out login, refresh rejected (401).
  Private logs: `.git/aios-completion-runs/1790868515244505414/agent.log`.
  Continuing this authorized mission in the existing session, without login/config changes.
- Environment: Linux x86_64, Node 24.19.0, Python 3.12.14, Git LFS 3.6.1.
  `/usr/bin/go` is an unrelated game, not Go. Installed verified Go 1.27.1 locally at
  `/workspace/scratch/toolchains/go`; downloaded SHA256 checked against official metadata.
  JRE lacked javac; extracted checksum-verified Debian OpenJDK 21.0.12.1 JDK at
  `/workspace/scratch/jdk/usr/lib/jvm/java-21-openjdk-amd64`, merged existing matching JRE.
  No system/auth changes. Commands use both bin directories prepended to PATH.
- `make harness-setup`: exit 0 after local toolchain setup; model/assets and npm ready.
- `python3 -m unittest discover -s scripts -p test_generate_v1_catalog.py`: exit 0,
  five tests include accepted/bounded generic inventory and explicit patterns/output.
- `git diff --check`: exit 0.
- `make harness-validate`: exit 0 (Python contracts, Go vet/tests/race/build); private log `/workspace/scratch/aios-baseline-core.log`.
- First browser gate: exit 2 (12 passed/4 failed); real backend E2E could not spawn
  missing `bin/aios` because the web target did not build it. Fixed `web-e2e` to build
  production assets then backend; removed an unrelated absolute JDK PATH injection.
- `PLAYWRIGHT_CHANNEL=chromium make harness-validate-web`: exit 0: 40 unit tests, production build and 16 browser E2E
  (two viewports, real mirror/local capture, identity/restart, nonmutation, Spotlight); private log
  `/workspace/scratch/aios-baseline-web.log`. Chromium installed locally for E2E.
- No native checks run: actual Darwin arm64 hardware is unavailable in this workspace.
  Native install/launchd/Git login/mDNS/wake/power/GPU/installed-upgrade and scale proof
  remain pending full final gates; portable work may continue per ACCEPTANCE.md.

## Independent review

Actual separate reviewer: `/root/baseline_reviewer`; formal reviewed commit/report pending.
Reviewer already audited current code and flagged stale security/default/generator
claims, now corrected. No self-review substituted for required independent review.

## Blockers

CLI launcher authentication blocks the child process, not this existing coding session.
Actual main publication and formal core/browser gates remain to verify. Native final
hardware is unavailable; not an excuse to skip remaining portable implementation.

## Handoff

Pending validated implementation SHA, formal receipts, corrected-commit review and
verified main publication. Continue task02 after predecessor evidence permits it.
