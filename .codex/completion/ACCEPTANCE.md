# Assembled product acceptance

The initial harness deliberately has no fake green product checks. Milestone 02
implements the missing Make targets below as executable scenarios. Until actual
behaviour exists they fail; no print-only success, blanket skips or source-string
tests substitute for product assertions. Gates are run by completion_harness.py.
Each scenario uses owned temporary data, deterministic generic fixtures and real
backend/UI boundaries. Native lifecycle cases use disposable per-user service
names and explicit test roots, not the operator's live installation.

| Gate / Make target | Required scenario coverage |
| --- | --- |
| core / harness-validate | Existing queue/Python/Go/vet/race/build checks, plus new completion harness tests. Two fresh hermetic KB runs with equal semantic fingerprints for changed ingestion/retrieval. |
| web / harness-validate-web | Unit/build plus live authenticated browser E2E; no component-only completion. Production embedded assets verified. |
| install / completion-install | Actual arm64 install without dev tools/runtime downloads, bundle checksums/assets, launchd login/headless/start/stop/restart, sources unchanged, localhost recovery. |
| onboarding / completion-onboarding | Multi-repo validation, saved branding/local logo, exclusive Mirror/Direct, actual daemon Git auth, auth failure remediation, namespace collision/selection/origin handling, immediate empty main UI, staged build and partial failures. |
| maintenance / completion-maintenance | Scheduled/watched dirty/untracked changes, ignores, continuous-edit debounce, immutable capture races, offline/stale retries, wake/restart/lost events, failed staged projections, concurrent reads, per-repo isolation, remove versus in-flight job, full purge, safe rebuild. Include real native wake smoke in final run. |
| upgrade / completion-upgrade | Upgrade from preserved compatible prior fixture, migration/projection version change, binary/state rollback at every fault/crash boundary, next-start journal recovery, identity/IR retention, reject incompatible release before mutation, uninstall preserve/delete. |
| privacy / completion-privacy | OTEL local-only; correlate bounded jobs/queries/upgrades; planted PII/credentials/source/query/path/remote content absent from persisted logs/traces/metrics/default archives. Warned expanded archive still redacted; no outbound telemetry. |
| resources / completion-resources | Actual native power/load signals plus injectable policy tests, bounded/fair queues/workers/disk; eventual freshness under constraints, cancellation, disk-full, measurable normal/constrained/idle transitions. |
| retrieval / completion-retrieval | Independent reviewed generic gold questions for symbols/change/callers/events/log causes/routes/config/impact/negative verification, ambiguity/unsupported/stale/truncated. Copy payload evidence/generation parity, commands/history/clear; deterministic baseline and repeat fingerprints. |
| cloud / completion-cloud | Native browser/GPU rendering evidence: every selected aggregate/entity/edge maps to IR, all semantic zoom levels, colour legend, rotation/pan/zoom, revision-safe links, staged/promotion/delta/query animations, empty/reduced motion/keyboard, module drag/reset/reconnect and visual QA screenshots/video. |
| benchmark / completion-benchmark | Isolated Demo + My Knowledge cases, immutable source parity, explicit costs and unbiased correctness/context/source-read/time metrics. Include demonstrated grep wins and wrong/unknown KB outcomes; persist per-query report. |
| scale / completion-scale | 1/25/100 mixed repositories, substantial dense symbol sets, long-run update/query/restart soak, CPU/RAM/disk/latency/frame measurements on declared reference hardware, explain missed targets and graceful LOD. |
| release / completion-release | One package built from tested commit, install/open/query/update/uninstall the package itself, dependencies/licenses/assets/checksums; concise guide/release notes/recovery/diagnostics. All requirement scenarios cross-referenced and no unresolved blocking review findings. |

Native gates must check `Darwin` + arm64 and capture hardware. Linux may run their
portable subtests, but cannot emit passing native receipts. CI should make macOS
arm64 the supported product gate; other developer smoke targets may remain optional.
Browser screenshots/video/manual engineering checks must be recorded honestly.
An automated semantic assertion alone does not certify polished UI.

## Milestone scenarios versus final gates

`gate NAME --milestone ID` executes `make completion-milestone` with the fixed task
and gate identifiers. Milestone 02 implements that dispatcher using each task's
acceptance criteria only. For example, maintenance in task05 excludes task06's
purge/rebuild; cloud in task11 excludes task12's module behaviour. Portable scenario
tests verify implementation and fault handling; native smoke runs when available.
Engineering milestone completion may defer native integration proof with an
explicit pending record, enabling later portable work. It must never assert that
launchd/GPU/mDNS works natively without executing it. Final task15 runs every FULL
gate without `--milestone` on real Apple Silicon. Scoped receipts are rejected by
final acceptance. No native omission survives candidate-ready acceptance.

## Gold corpus and benchmark rules

Start from generic heterogeneous fixtures, including unsupported/ambiguous inputs.
Use at least 150 independently reviewed known-answer questions across the planner
families and at least four per repository in the 25-repo retrieval corpus. This is
a fixture/regression minimum, not proof on arbitrary user code. Keep My Knowledge
optional; never require private employer repositories. Define expected evidence,
negative/unknown classifications and retrieval budgets before scoring. Retain
existing stricter useful thresholds after inspection; record rationale for changes
and never lower them merely to pass. Accuracy/provenance regressions block release.
Grep may be faster for literal matches. Compare and display such outcomes fairly.

## Release evidence schema

Write `.codex/completion/evidence/release.json`:

```json
{
  "schema_version": 1,
  "tested_commit": "FULL_40_HEX_SOURCE_COMMIT",
  "artifact": {"path": "owned/path/to/aios-macos-arm64.zip", "sha256": "SHA256"},
  "receipts": [".codex/completion/evidence/core.json", "...all gates..."],
  "review": ".codex/completion/evidence/final-review.json",
  "requirements_report": ".codex/completion/evidence/requirements.json",
  "candidate_ready": true,
  "stakeholder_release": "pending"
}
```

Requirements report: JSON object mapping every R01–R26 to nonempty `scenarios`,
`evidence` and `result: "pass"`. Reviewer checks report truth against actual outputs.
Final tested commit must be reachable from fetched main and HEAD. Only subsequent
evidence/plan/bookkeeping edits are allowed without rerunning final product gates;
any product/tooling code change invalidates receipts. All receipts must refer to
that source commit and clean product tree; gate collection itself may add evidence.
All gates must pass, native gate receipts must be actual Darwin arm64, artifact
bytes must match the digest, separate-session review must pass that commit, all
milestones must be published and reviewed. `accept` validates those records and
returns nonzero for missing/stale/failed evidence. It cannot prove honest authorship
or test depth by itself; independent review remains mandatory.

If artifact/evidence lives outside the checkout, retain it as a CI artifact with
verified digest and materialise it before `accept`; inaccessible artifacts cannot
pass acceptance. Do not claim ready from just successful packaging or task status.

Bootstrap exceptions: task01 uses existing core gates and recorded baseline/doc
assertions, since the dispatcher is introduced by task02. Task02 tests the dispatcher
itself and scenario failure reporting; full feature scenarios remain later work.
All final receipts, including portable core/web/maintenance suites, must be from
Apple Silicon at the same tested revision. Linux receipts remain useful development
evidence but cannot make the final candidate pass.
