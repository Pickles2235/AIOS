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
