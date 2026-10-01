# Deliver daemon lifecycle and installable Apple Silicon package

Requirements: R02, R11

## Acceptance

- Native per-user launchd install/start/stop/status/open works without browser or dev toolchains.
- Bundled UI/model assets work offline; report optional external compiler coverage accurately.
- Restart/login persists identity/state; UI Stop daemon reflects disconnection and restart instructions.

## Decisions

Retain embedded UI and local Nomic assets. Add per-user launchd lifecycle with an owned control socket for fresh one-use launch links, stable instance identity, safe ZIP installer and authenticated Stop daemon. Native launchd integration is deferred honestly to real Apple Silicon CI.

## Progress

Implementation started after independent task02 review and publication. Task02 final bookkeeping published via connected GitHub Git API as 22a98cfc16edfb19dd37a56044dd4dc8840f4b87 after CLI credentials expired. API-fetched raw commit reproduced exact SHA locally (API timestamps normalize the original +0100 offset); main ref observed/verified through the connector. Original local bookkeeping 4532bec retained on prior branches; no resets/force.

## Scenario scope

Run existing core/web gates plus scoped product scenarios with `gate NAME --milestone 03-daemon-install`. Full assembled/native gates are final-candidate requirements. Record deferred native checks explicitly.

## Actual validation

Linux x86_64 precommit developer checks: make harness-validate passed; Go race tests for lifecycle/webui/CLI passed; full web gate passed 27 unit + 20 browser cases, including actual headless foreground process, close/reopen browser, restart identity, Stop/disconnected and failed-stop/session-expiry UX. Portable archive adversarial tests cover unsafe names/special files/duplicates/incomplete hashes/tamper/path replacement/size overflow; control tests cover strict bounded payloads, owner-only socket and exclusive instance lock. Exact committed receipts follow. Native launchd/install/login/open remains pending real CI, not claimed from Linux.

## Independent review

Separate /root/baseline_reviewer WIP review required service ownership before bootout, archive descriptor/size integrity, symlink pre-mutation rejection/UID checks, stop failure observability and session vs transport distinction; corrected with adjacent tests. Final committed review follows.

## Blockers

None established; native requirements need actual hardware/CI.

## Handoff

Persist implementation SHA, publication evidence and next work. Continue mission.

## Implementation and native handoff

Per-user launchd plan, user-only install/start/stop/status/open, secure local UNIX control, stable owned identity and signal cleanup implemented. Foreground smoke is explicitly distinguished from managed launchd. ZIP builder/manifest/checksum/scripts/guide added; manual atomic upgrades remain task07; full licenses final15. Native smoke workflow exercises a disposable root/label with cleanup registered before mutation. Login-at-load is planned via actual plist but manual login proof and final native installed/GPU acceptance remain final15 requirements. No source workspace is a lifecycle output.
