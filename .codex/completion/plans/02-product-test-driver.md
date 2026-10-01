# Implement executable assembled-product acceptance drivers

Requirements: R26

## Acceptance

- Implement every completion-* Make target listed in ACCEPTANCE.md with real assertions and failure exits.
- Native scenarios must fail or explicitly block on wrong hardware, never silently skip and pass.
- Add JSON evidence/report schema, fixture generators and fault injection; tests initially expose missing product behaviours.

## Decisions

Keep existing core/web gates unchanged. Add a fixed-argv, fail-closed live product dispatcher, machine-readable report schema and 26-requirement scenario registry. Milestone 02 tests only driver bootstrap; later owner milestones deepen acceptance as recorded in DRIVER-COVERAGE.md. No API self-attestation certifies compliance.

## Progress

Implemented all eleven completion-* product targets, scoped dispatcher, deterministic dense Git fixtures, six upgrade fault boundaries and real authenticated browser suite. Thirteen bootstrap tests cover missing/empty/duplicate suites, skips/zero assertions/subtest failures, invalid hardware, binary provenance, dirty source, redacted errors and bounded partial-line startup. Native cleanup is registered before installation and uses disposable labels. Explicit PENDING assertions prevent provisional fault/policy probes passing before owner expansions.

## Scenario scope

Run existing core/web gates plus scoped product scenarios with `gate NAME --milestone 02-product-test-driver`. Full assembled/native gates are final-candidate requirements. Record deferred native checks explicitly.

## Actual validation

Linux x86_64 developer smoke: precommit `make harness-validate` passed; `PLAYWRIGHT_CHANNEL=chromium make harness-validate-web` passed (27 unit tests, 16 browser cases). Bootstrap `python3 -m unittest discover -s scripts -p test_completion_driver.py` passed 13 cases. Formal committed receipts and deliberate live feature-failure/native-block probes follow below. Native integration/GPU/install proof is pending task15 and is not claimed.

## Independent review

Separate session /root/baseline_reviewer identified binary/artifact provenance, privacy-byte inspection, flag-only assertions and cleanup gaps; corrected before formal review. Final report/commit pending.

## Blockers

No portable driver blocker. Native full gates require actual Darwin arm64; no native receipt is claimed. Later feature APIs intentionally fail until implemented.

## Handoff

After formal checks/review and exact publication, continue 03-daemon-install. DRIVER-COVERAGE.md specifies mandatory fault/race/native expansions before each owner can complete.
