# Finish contextual draggable operational UI

Requirements: R21

## Acceptance

- Health/storage/performance/coverage/jobs/query/indexing modules show when relevant and expose truthful detail.
- Persist positions, bound to viewport, accessible movement/reset; errors visible without clutter.
- Live runtime event sequencing/reconnection coalesces updates without lost or fake activity.

## Decisions

Reuse actual jobs/resources/activity/status and canonical query/coverage state. Wire contextual floating modules into main.tsx; idle view remains uncluttered, useful warnings remain visible. Persist viewport-bounded placement with accessible move/reset. Event sequencing/reconnect must reconcile durable state and disclose gaps without fake activity. Delivery owns architecture and full implementation choices within R21.

## Progress

Started on `codex/completion-contextual-modules` from fetched main `780109d`. Dependencies 09 and 11 completed, independently reviewed and reachable from HEAD/main. Senior delivery/integration owner and separate senior independent acceptance mapped before staffing. Manager owns plan/tasks/evidence/publication.

## Scenario scope

Run existing core/web gates plus scoped product scenarios with `gate NAME --milestone 12-contextual-modules`. Full assembled/native gates are final-candidate requirements. Record deferred native checks explicitly.

## Actual validation

Record commands, receipts, platform and full tested commit. No checks run yet.

## Independent review

Separate reviewer report and corrected commit required.

## Blockers

None established; native requirements need actual hardware/CI.

## Handoff

Persist implementation SHA, publication evidence and next work. Continue mission.

Architecture checkpoint: `/root/contextual_modules_delivery` owns a unified actual runtime snapshot poller over jobs/resources/onboarding/activity/status and seven contextual floating modules. Existing activity sequence is in-process and bounded to 512 retained events; explicitly handle restart/reset/history gaps and reconnect, never infer lost events as healthy inactivity. Preserve maintenance controls in health detail and generation/query freshness. Independent `/root/contextual_modules_acceptance` is reviewing before final source freeze. Both senior gpt-6-sol/medium/forknone; manager owns metadata.

Independent draft review identified header drag initiation, overlapping cards on narrow viewports, missing stream epoch/retention boundary, and initial build state before the first poll. Delivery is repairing these and strengthening live scenarios to cover all seven modules rather than the original one-card placement smoke. No frozen candidate or passing final receipt yet. Canonical runtime state, unavailable-versus-empty distinctions and retained errors remain required.

## Committed source checkpoint

Delivery source `a8d66bb5ff54b6585b6e8738a8a4c8a99afbfa01` is clean and contains all seven modules, actual runtime state integration, explicit activity stream/retention metadata, draggable fixed detachment with persisted clamped placement/reset, contextual query/coverage/error handling, docs and built assets. Default document-flow dock avoids covering navigation; actual pointer and keyboard detachment remains required and tested.

Preflight logs: `/tmp/aios-task12-focused-build.log` web build pass; `/tmp/aios-task12-focused-go.log` activity retention contract pass; `/tmp/aios-task12-focused-unit.log` four helper tests pass; `/tmp/aios-task12-focused-product-browser.log` one fresh embedded-backend scenario pass (16.3s test/17.9s run) including all seven modules, pointer/keyboard/reset/reload/clamp, bad coordinates, direct/Spotlight state, transient error, retention gap plus concurrent setup error, epoch and reconnect. Prior legacy run 18/20 passed, two alert-selector failures corrected and rerun 2/2 at `/tmp/aios-task12-focused-spotlight.log`. Initial Go run overlapped generated-asset rebuild and failed; serial rerun passed. Failed/transient runs remain historical, not final passes.

Independent source review is next, followed by clean exact-source core/web/scoped cloud gates and independent live visual/runtime acceptance. No ingestion/retrieval behavior changed; no paired hermetic KB runs are required for this UI/activity metadata scope. No formal final receipts yet. Freeze all file/HEAD writes during formal gate collection.

Independent source review blocked checkpoint48555f2: Promise.all discarded fresh successful job/setup failures when one endpoint failed, and job error/watch/retention priority masked concurrent issues. Corrected product `aede19254bcdf3f1a43801cf8baa35a6acd480c7` reconciles endpoints independently with explicit stale-read warnings and separate concurrent failures. Fresh built embedded browser regression passed 1/1 (16.1s test/17.7s run), including activity503 concurrent with fresh job/setup errors and independent jobs/resource outages; log `/tmp/aios-task12-correction-browser.log`, build log `/tmp/aios-task12-correction-build.log`. A duplicate nested Health alert selector initially failed and was corrected without suppressing alerts. No formal gates started; this is preflight evidence only. Independent corrected-source clearance is required before freezing core/web/scopedcloud runs.
