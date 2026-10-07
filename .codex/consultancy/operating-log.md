# Consultancy operating state

## Current outcome

Finish the approved installable V1 completion mission. Acceptance is the full 15-milestone harness, all R01–R26 requirements, final native Apple Silicon gates, checksummed installed candidate, independent review, and `completion_harness.py accept` passing. Public release remains the stakeholder's decision.

## Decisions and constraints

- Follow `.codex/completion/PRODUCT.md`, `MISSION.md`, `ACCEPTANCE.md`, `tasks.json`, and each milestone plan.
- Preserve canonical IR, evidence semantics, browser security, source nonmutation, and truthful native evidence. Never claim a deferred check passed.
- Task 09 implementation `f28e413a8637d4561d358ff659ade3df90b5116c` was published in `50211f88ec62fc066b121d42212b87a96b7dac90`; follow-up completion bookkeeping is `c9637783e9cca819b3e2c0afb683683e63b92f8b`.
- Task 10 runs on `codex/completion-investigation-search`, from fetched main `c9637783e9cca819b3e2c0afb683683e63b92f8b`.

## Team map

| Owner | Level and role | Scope | Status |
|---|---|---|---|
| `/root` | Manager / integration, senior | Mission scope, plan/state, integration, publication, final sign-off | Active |
| Pending fresh agent | Senior delivery lead | Task 10 implementation and focused checks; code/tests only | To hire |
| Reserved fresh agent | Mid or senior independent reviewer | Inspect task 10 corrected commit, evidence, and acceptance behavior | Required after candidate |

## Efficiency and workflow

Keep one owner per write scope. Share existing durable evidence paths, not transcripts or private raw logs. Coordinate full gates to avoid duplicate expensive runs; use fresh configured agents for distinct assignments and a reviewer independent of implementation. Live token/cost telemetry is unavailable in this thread; no usage-efficiency claims are made.

## Workflow candidates and lessons

The completion mission and consultancy workflow have been applied to task 09 with an independent exact-commit review and recorded scoped gates. Do not create a skill from this single mission. Continue recording only stable lessons demonstrated by repeated work.
