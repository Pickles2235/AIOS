# Deliver honest Demo and My Knowledge Benchmark Lab

Requirements: R22

## Acceptance

- Demo fixtures isolated from user KB; own-repository known-answer cases persist and replay as regressions.
- Same immutable snapshot for KB and grep; disclose baseline invocation, warm/cold/index cost and estimated token assumptions.
- Publish correctness including losses/unknowns, source reads/context size/time and advanced metrics without favourable weighting.
- Test cases where grep wins and KB is wrong; machine-readable per-query exports.

## Decisions

Reuse existing fixture engine where sound; deliver dedicated live Lab/API with isolated Demo and optional durable My Knowledge regressions. Lease/capture the same immutable source snapshot for both KB and literal baseline; report actual measured costs and explicit estimates. Unknown never counts as answer-correct. Preserve source nonmutation, user KB, generation/security/resource limits.

## Progress

Started on `codex/completion-benchmark-lab` from verified fetched main `e8037c48a513540296516041a2e8fa2b864b685f`. Task09/10 dependencies completed and reachable. Principal delivery/integration and separate Senior acceptance mapped; manager owns metadata/publication.

## Scenario scope

Run existing core/web gates plus scoped product scenarios with `gate NAME --milestone 13-benchmark-lab`. Full assembled/native gates are final-candidate requirements. Record deferred native checks explicitly.

## Actual validation

Record commands, receipts, platform and full tested commit. No checks run yet.

## Independent review

Separate reviewer report and corrected commit required.

## Blockers

None established; native requirements need actual hardware/CI.

## Handoff

Persist implementation SHA, publication evidence and next work. Continue mission.

Acceptance preflight: existing CLI runner is not Lab implementation; ActiveFiles warm read is not query warm cost, in-process Contains is not rg invocation, unknown correctness and missing byte counters need explicit repair/measurement. Review fixture answers must be declared before scoring. All source/UI/API gates plus scoped benchmark scenario and paired KB acceptance if retrieval/ingestion changes. No task15/native scale claim.

Architecture checkpoint: Principal `/root/benchmark_lab_delivery` (gpt-6-astra/medium/forknone) owns isolated Lab engine/API, live component, existing CLI measurement corrections, tests/scenarios/docs/assets. Senior `/root/benchmark_lab_acceptance` (gpt-6-sol/medium/forknone) fixes expected holdout evidence before scoring and audits actual snapshot parity/isolation, costs, wrong/unknown outcomes, durable cases and browser faults. Demo embedded fixtures use owned temporary DB; My Knowledge captures pinned canonical bytes for an isolated comparison DB, with coverage/provenance/differences disclosed. Instrumented in-process literal baseline is explicitly labelled, never misrepresented as rg. Files/bytes read, context/token estimates, capture/index/first/repeat costs stay separate. Manager requires bounded cases/reports, crash-safe writes, admission/cancellation and no source/user generation mutation. Existing CLI correction stays with delivery to avoid coupled schema writers. No retrieval changes intended; paired acceptance required if that changes. Start queue regression passed11legacy/14completion at e1a19a4 checkpoint.
