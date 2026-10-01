# AgentOS installable V1 completion harness

This is the delivery contract and executable workflow produced from stakeholder
discovery on 2026-10-01. It supersedes the old single-task mission, not its history.
Read PRODUCT.md for 26 approved requirements; tasks.json maps 15 dependency-ordered
milestones to acceptance criteria; ACCEPTANCE.md defines 13 assembled-product gates.
Current implementation must be inspected before each milestone. This harness is
ready to drive development; it does not declare the product complete.

## Start

Use a clean checkout on Apple Silicon for complete native verification. Configure
the existing development environment using `.codex/README.md`/`make harness-setup`.
The development CLI already needs its own authentication; the installed product
will not need Codex. No new model, provider or credentials are configured here.

```sh
git fetch origin main
python3 scripts/completion_harness.py check
python3 scripts/completion_harness.py status
python3 scripts/completion_harness.py run
```

`run` starts one Codex mission instructed to continue through the finite backlog,
review each substantial milestone and finish assembled acceptance. It inherits
the configured model/auth and uses the existing workspace-write sandbox. Explicit
permissions/network/native hardware remain real environment constraints. The
launcher cannot manufacture access or keep a finite-context process alive forever;
MISSION.md requires checkpoints/resumption. If interrupted, preserve the checkout,
inspect logs/plans and resume with the prompt in a new session after resolving
the precise blocker. Do not discard work to get a clean checkout.

Without a CLI, give the mission to a coding-agent session:

```sh
python3 scripts/completion_harness.py prompt
```

`next` returns the next pending/in-progress milestone whose completed predecessors
are published on fetched main and reviewed. Update tasks.json and each plan as
work progresses. Ready/blocked work requires explicit inspection/resumption; do
not blindly rerun it. Full implementation authority includes decisions and reviewer
delegation; public release still belongs to the stakeholder.

## Evidence and exit behaviour

```sh
python3 scripts/completion_harness.py gate core --output /owned/checks/core.json
python3 scripts/completion_harness.py accept
python3 -m unittest discover -s scripts -p 'test_completion_harness.py'
```

Gate commands are registry-controlled argv arrays, not user-supplied shell text.

Use `gate NAME --milestone ID --output /owned/scoped.json` for the selected
milestone's implementation scenarios. This avoids requiring later features early
and allows portable work while native integration proof is explicitly deferred.
Full native/assembled gates remain mandatory for the final candidate, which rejects
scoped receipts. See ACCEPTANCE.md for the dispatcher contract.
Product changes must be committed before receipts are collected. Only subsequent
evidence/plans/task-status bookkeeping is compatible with final receipts. Missing
new product Make targets intentionally fail until milestone 02 implements them.
Its target-driver self-tests can pass while product scenario targets expose missing
features; do not make all missing-feature scenarios part of core test discovery
until their milestones implement them.

Native receipts require actual Darwin arm64; Linux cannot stand in for macOS UX.
Logs may contain development/source information; keep them private and redact
before sharing. Independent reviewer reports are required, and records do not
cryptographically establish that a reviewer was independent: the operator/agent
must actually start a separate review session, as instructed in REVIEWER.md.

The shared Git-common-directory lock prevents old/new launchers running together.
Logs are in `<git-common-dir>/aios-completion-runs/`. CLI exit zero alone is not
success: `run` checks the final evidence and fails when the candidate is incomplete.
`accept` verifies published milestone/review records, fresh full gate receipts,
requirements mapping and package digest. Final code still needs genuine tests and
review; JSON consistency cannot validate the substance of tests by itself.

Completion means a verified candidate ZIP/binary with install/uninstall/update
tooling and guides, handed to the stakeholder for release acceptance. It does not
mean merely a green queue, screenshot mockup or printed installation instructions.
