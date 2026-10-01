# Deliver daemon lifecycle and installable Apple Silicon package

Requirements: R02, R11

## Acceptance

- Native per-user launchd install/start/stop/status/open works without browser or dev toolchains.
- Bundled UI/model assets work offline; report optional external compiler coverage accurately.
- Restart/login persists identity/state; UI Stop daemon reflects disconnection and restart instructions.

## Decisions

Retain embedded UI and local Nomic assets. Add per-user launchd lifecycle with an owned control socket for fresh one-use launch links, stable instance identity, safe ZIP installer and authenticated Stop daemon. Native launchd integration is exercised through real Apple Silicon CI; full final gate and login proof remain task15 obligations.

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

Native CI first attempt: workflow36896016764, Apple Silicon job110483087639, source9a61a0e88543b71b5f1e8a824c58df4ff0936b97 failed a real Unix socket length limit under /private/var/folders. No native pass is claimed. Corrected product routes long canonical data paths through unique hashed owner-only short control directories, validates directory/socket ownership before read-only dialing, bounds the resulting path, cleans only empty per-instance directories, and rejects shared/symlink/missing endpoints. Added explicit long-root/routing/nonmutation regressions and private daemon link CLI. New exact-source native CI follows; final receipt collection must use that source. Earlier passing portable86dca9 receipts are historical after this product correction.

## Corrected source evidence

Reviewed/tested source: `5f96ef154df7cfef3bf043718a2f3b6adb4a78eb` (exact connector-created Git commit, locally reconstructed/hash verified; prior drafts preserved on branches). Fresh Linux core/web/scoped install receipts are retained in evidence/03-daemon-{core,web,install}-source.json. Core checks passed; web passed 27 unit and 20 live browser cases. Real developer ZIP inventory and SHA receipt: evidence/03-package-source.json; archive `/workspace/scratch/aios-daemon-package-final/aios-1.0.0-daemon-final-linux-amd64.zip`, SHA256 `09e181ae7db26ba87eea96b8a985f71bdeb0f7264bd36b28c74e98b52a228737`. Scoped install ran its actual extracted clean-source binary, not merely checkout output. Raw gate logs remain private at `/workspace/scratch/aios-daemon-evidence`.

Native final-source workflow `36898649485`, Apple Silicon job `110491952812` is still running. Its real Go/Python/frontend tests have passed, including the corrected long-root socket test; launchd/package/browser checks pending. The disposable native install path includes a space, exercising real launchctl argument formatting. Separate reviewer independently checked source, receipts/log hashes, ZIP inventory, clean binary provenance and reran the scoped artifact probe; final report follows native results. No final release/candidate-ready claim.

Reviewer native audit correction: report javac_command_detected, because macOS may provide only a launcher stub. Never invoke it to prompt a runtime download. Source revision and fresh receipts will follow this correction. Earlier receipts above remain historical until replaced.

## Final engineering evidence

Final corrected implementation source: `837f2c9ea58100d80b1562b825d9c3451b7c353d`. Fresh clean core/web/scoped install receipts: evidence/03-daemon-{core,web,install}-commands.json. Real extracted candidate-binary scoped install passes. ZIP SHA256 `e1d4b714e791067fadb1c8e277dfb0faad11ed854dd43e95243bc9d3bd0eca63`, path `/workspace/scratch/aios-daemon-package-commands/aios-1.0.0-daemon-commands-linux-amd64.zip`, exhaustive inventory/source verification in evidence/03-package-commands.json. Raw logs stay private in scratch.

Actual native workflow36897762577/job110488961085 passed at source3df36c2 (earlier source, not837). Downloaded artifact11181212247 matches GitHub SHA2566f0cf0d49b3c004a52b11c81f79fe355128dc53d0264f56ca3edf3882fedbc22. Retained actual daemon-native.json as evidence/03-daemon-native-3df.json; CI/source/hardware binding in evidence/03-daemon-ci-3df.json. It proves checksummed ZIP install, headless launchd, foreign-root stop rejection, stop/start persistent identity, native browser open and preserve-data uninstall; cleanup deletion also succeeded. Native 15.7.9 arm64,3logicalCPUs,7516192768RAM. Two fresh native semantic fingerprints and installed sandbox-denied-network acceptance all matched and accepted. This is engineering smoke, not full final installed-product proof.

5f96ef1 native space-path fixture and837 native rerun still pending. Native lifecycle code matches successful3df;837 changes only runtime field honesty plus historical evidence bookkeeping since5f96. Latest-source native integration, actual login and all final assembled one-source/one-artifact gates remain task15 obligations. Independent reviewer report binds837 portable evidence and explicitly distinguishes earlier native proof. Continue around this allowed native deferral; reopen on any observed native defect. No public release or candidate-ready claim.

Actual5f96 native workflow36898649485/job110491952812 now SUCCESS. Downloaded artifact11181700873 checksum0feda6d76fe6251925c21b101578ac3ad396270654e5e26cc6892721e17faedb verified; retained daemon-native report and CI binding as evidence/03-daemon-{native,ci}-5f96.json. Actual service_path_with_spaces assertion passed. Native installed offline acceptance and two fresh fingerprints accepted/equal.837 latest runtime-report-only rerun and fullfinal login/gates remain pending; no latest-source native receipt fabricated.
