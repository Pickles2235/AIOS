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
