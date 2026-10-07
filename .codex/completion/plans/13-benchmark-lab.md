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

Record commands, receipts, platform and full tested commit. Preflight below; formal exact-source gates pending.

## Independent review

Separate reviewer report and corrected commit required.

## Blockers

None established; native requirements need actual hardware/CI.

## Handoff

Persist implementation SHA, publication evidence and next work. Continue mission.

Acceptance preflight: existing CLI runner is not Lab implementation; ActiveFiles warm read is not query warm cost, in-process Contains is not rg invocation, unknown correctness and missing byte counters need explicit repair/measurement. Review fixture answers must be declared before scoring. All source/UI/API gates plus scoped benchmark scenario and paired KB acceptance if retrieval/ingestion changes. No task15/native scale claim.

Architecture checkpoint: Principal `/root/benchmark_lab_delivery` (gpt-6-astra/medium/forknone) owns isolated Lab engine/API, live component, existing CLI measurement corrections, tests/scenarios/docs/assets. Senior `/root/benchmark_lab_acceptance` (gpt-6-sol/medium/forknone) fixes expected holdout evidence before scoring and audits actual snapshot parity/isolation, costs, wrong/unknown outcomes, durable cases and browser faults. Demo embedded fixtures use owned temporary DB; My Knowledge uses a SQLite-consistent VACUUM INTO private copy preserving canonical IR/projections/coverage; copied source bytes are verified and materialized for the literal baseline. Instrumented in-process literal baseline is explicitly labelled, never misrepresented as rg. Files/bytes read, context/token estimates, capture/index/first/repeat costs stay separate. Manager requires bounded cases/reports, crash-safe writes, admission/cancellation and no source/user generation mutation. Existing CLI correction stays with delivery to avoid coupled schema writers. No retrieval changes intended; paired acceptance required if that changes. Start queue regression passed11legacy/14completion at e1a19a4 checkpoint.

## Product checkpoint and review recovery

Scoped implementation `bf595d5c8160f2709951ae278b8398136068e3f3` is clean. Live main.tsx Lab includes primary case editor with repository selection, durable save/replay/clear/export, prior report comparison, actual immutable source manifests and independent equal-weight per-query outcomes. Fixed-workspace deletion was replaced by unique marker-owned workspaces; private durable writes sync parent directories; latest/export remain readable during active work. Saved cases bind snapshots, configured unindexed repositories retain incomplete coverage, source hash/size/path inputs and report/queue bounds are explicit. Baseline is an instrumented literal scan, not falsely labelled rg. Unknown gets no answer credit; classification correctness is separate.

Early review rejected post-hoc duplicate target choices. Demo preserves ambiguous state mismatch and explicit unsupported operator miss; the unique `Client->>Ledger: refund` expected evidence is source-grounded but selected after observation and labelled exploratory/adversarial, not independent gold. No demonstrated wrong factual found-answer is claimed. Exact case/path/line/snippet/outcome assertions replaced weak incorrect-flag-only proof. Independent reviewer holdouts remain fixed.

The operator probe exposed a real false-absence bug: knowledge.Query now returns unsupported unknown for tokenless punctuation after exact canonical lookup, with an adjacent regression. This actual retrieval change requires two fresh hermetic acceptance-v1 runs with matching fingerprints on the final source.

Actual preflight: latest focused `go test ./internal/benchmark ./internal/webui ./internal/knowledge` exit0, `/tmp/aios-lab-focused.log` (benchmark5.049s, webui40.485s, knowledge cached). Web build exit0 `/tmp/aios-lab-web-build.log`. Prior-draft actual embedded browser `web/e2e/benchmark-backend.spec.ts` passed1/1 in15.9s, `/tmp/aios-lab-browser-preflight.log`, exercising Demo isolation, onboarding, primary editor, save/replay/export, reload and clear/source nonmutation. Earlier sessions31893/28853/53838 terminal exit0; unredirected early output is preflight only. All delivery processes terminal before manager checkpoint; preflight is not final exact-source gate evidence.

Reviewer source audit found no remaining blocking backend defect; independent live/API/fault/visual acceptance and clean exact-source core/web/scoped benchmark receipts remain pending. No milestone completion claim yet. Copy bounds256MiB DB/64MiB source/10k files/40 cases/2min are explicit unavailable outcomes, not partial global absence. Task14 must measure25/100 datasets and implement a faithful bounded large-estate path if these limits prevent required comparisons; sustained/native assembled acceptance remains unproven.
