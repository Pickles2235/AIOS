# Autonomous completion mission

Read AGENTS.md, PRODUCT.md, ACCEPTANCE.md, tasks.json and CURRENT-STATE.md in this
directory. These decisions are approved; do not conduct another requirements
interview. Your goal is the complete installable candidate, not one task or a plan.

1. Inspect current main, status and existing contracts/tests. Preserve unrelated
   work; use an isolated branch/worktree if needed. Reconcile the older harness
   restrictions using the explicit completion override in AGENTS.md.
2. Run `python3 scripts/completion_harness.py check` and `next`. Select the next
   eligible milestone. Inspect working implementation before replacing anything.
   Persist status/decisions/checks/blockers/handoff in its plan and tasks.json.
3. Implement end-to-end behaviour, including the live browser entry point and
   embedded release assets. Add meaningful fault/recovery tests. Complete the
   milestone's acceptance scenarios, not just helper/component tests.
4. Run relevant existing gates plus milestone-scoped product scenarios. Full
   assembled gates run in the final milestone; do not require a later milestone's
   feature to pass an earlier one. Native proof may be deferred to the final gate
   with explicit pending records so portable implementation can continue. Record actual commands,
   exit codes, platform/hardware, revision and artifacts. Commit scoped work.
5. Start a SEPARATE reviewer agent/session with REVIEWER.md, the contract, selected
   milestone and actual diff/receipts. The implementation agent cannot attest its
   own independent review. Fix every blocking finding and rerun affected checks;
   reviewer assesses the corrected commit. If delegation is unavailable, launch a
   separate configured CLI review session or mark review blocked. Never fabricate.
6. Publish normal fast-forward commits to main following repository trunk policy.
   Fetch/reconcile concurrent changes; never force-push or reset unrelated work.
   Mark completed only after real checks/review and verified publication. Record
   full implementation SHA, review report path and check receipts in the plan.
7. Continue through every eligible milestone in the SAME mission; do not stop at
   the old one-task handoff. Independent nonblocked milestones can proceed around
   a hardware/access blocker. Stop only for a genuine external blocker or when
   assembled product gates pass and candidate handoff is ready. Persist resumable
   state before context/session boundaries; resumption starts from evidence.
8. Final milestone runs all product gates against one built artifact/current
   implementation, not scattered historical receipts. Independent reviewer checks
   release manifest, evidence and contract. `completion_harness.py accept` checks
   evidence consistency; it does not replace the reviewer or subjective UI QA.

Use the existing model/authentication configuration. Development model calls are
explicitly authorised; do not add model calls to the product. Full implementation
authority is not authority to weaken requirements, mark unrun checks passed or
publish a public release without stakeholder acceptance.

## Evidence format

Use `completion_harness.py gate GATE --output /owned/path.json` to execute gates
and write a receipt. Passing receipts capture commands, exact tested commit,
product-tree cleanliness, platform, times and output digest. Keep raw logs private.
Copy suitable redacted receipts/reviewer reports into `.codex/completion/evidence`
and commit them; never commit source-bearing dumps or secrets. Plans must cite
test cases and artefact paths/hashes, not just say 'all passed'.

The final `release.json` schema is shown in ACCEPTANCE.md. Record the candidate
artifact SHA256, actual tested source commit, all gate receipts, reviewer session,
and any nonblocking limitations. A receipt for a dirty product tree cannot prove
the final artifact. After final gates and review pass, publish the final task's
completion bookkeeping and run accept. If assembled acceptance fails, reopen the
final task and repair the failure; do not hand off a candidate-ready claim.
