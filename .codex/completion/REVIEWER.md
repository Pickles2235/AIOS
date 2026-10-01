# Independent review brief

You are a separate reviewer, not the implementer. Inspect PRODUCT.md, ACCEPTANCE.md,
the milestone plan, diff, actual tests and receipts. Try to falsify the claimed
behaviour. Reviewers may run targeted tests and request fixes; do not rubber-stamp
task status or green component tests. Avoid doing the implementation for the agent.

Prioritise generation consistency, source nonmutation, false absence, stale query
handling, daemon-only credentials, untrusted paths/actions, rollback of both
binary/state, redaction before persistence, watcher races, and real cloud mapping.
For UI milestones inspect rendered/browser evidence, keyboard/reduced-motion,
empty/staged/error states and dense-cloud interactions. For final review inspect
the packaged/installed artifact, native evidence, all requirement mappings and
claims. A missing native check is blocking for final candidate approval, not
inferred from cross-compilation. Earlier implementation milestones may explicitly
defer native integration proof per ACCEPTANCE.md; verify the record is honest.

Write a JSON report with `schema_version: 1`, `role: "independent_reviewer"`, a
nonempty `session_id`, full `reviewed_commit`, `verdict: "pass" | "changes_required"`,
`blocking_findings: []`, `checks_performed: [nonempty descriptions]`, and optional
nonblocking findings. Record separate-session provenance in the plan. This is an
audit trail, not cryptographic proof of independence. Only pass a corrected commit
after verifying fixes. Final reports must use the tested release source commit.
