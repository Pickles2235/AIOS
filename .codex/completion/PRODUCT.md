# Installable AgentOS V1 — stakeholder contract

Approved 2026-10-01. This contract supersedes older product assumptions and the
old one-task stopping rule for the completion mission. Historical task records
remain evidence of their original scope, not proof of this expanded product.

## Product and authority

Deliver a finished, stakeholder-testable, generic repository knowledge appliance
for macOS Apple Silicon. The daemon owns state and runs without an open browser.
V1 includes KB compilation/retrieval, maintained repositories, honest benchmarks,
and the polished interactive UI. No external LLM connection, embedded generative
reasoning, agents at runtime, non-repository connectors, or corporate hierarchy.
Development agents and independent reviewers are permitted and required tooling.
Existing read-only MCP code may remain as a compatible internal boundary; no MCP
connection/setup is a V1 product gate or onboarding step.

The coding agent has full implementation authority: refactoring, dependencies,
schemas with migrations, APIs, architecture, and routine visible UX details.
Use the contract to resolve choices without asking the stakeholder. Escalate only
irreconcilable requirements, unavailable access/hardware, or a proposed change to
the approved scope, privacy, or data-loss guarantees. An inability to verify is
a blocker, never a reason to weaken a gate. Do not rewrite a working KB engine.
Preserve existing work, source nonmutation, browser security and evidence semantics.

## Requirements

| ID | Required outcome |
| --- | --- |
| R01 | Generic product: remove obsolete estate-specific namespaces, defaults, prerequisites and documentation assumptions from the tracked current project. No private estate is required. Support 1–100 arbitrary Git repositories; extractor limits stay explicit. |
| R02 | Deliver an Apple Silicon binary or ZIP with install script; bundle production UI and required local assets. Installation must not require development toolchains. Detect optional compiler coverage honestly. Signing/notarization is not a prerequisite for the first candidate; document actual macOS first-run behaviour. |
| R03 | First-run onboarding persists stable instance ID, name, optional uploaded or locally generated logo, UI seed colour and chosen namespace. Local logo generation needs no cloud/LLM. Choose Mirror OR Direct globally, never mix repositories from both. Validate multiple URLs/directories, detect languages/frameworks and preview scope before Build. |
| R04 | On Build enter the main UI immediately. An empty KB has no cloud. Real discovery/staging activity progressively appears with an unobtrusive indexing card. Staged knowledge must be visibly distinguished and cannot answer as active evidence before atomic promotion. |
| R05 | Mirror uses machine Git/SSH/credential helpers and optional installed gh configuration, not an embedded provider API or token store. The user configures credentials outside AgentOS. Noninteractive daemon auth must work with its actual login environment. Explain failures and recommend Direct mode without silently switching modes. |
| R06 | Mirror checks every 15 minutes by default, configurable with Check now. Coalesce jobs, bound retries with exponential backoff, expose last success/revision/next attempt and stale state. One failing repository must not disable healthy repositories or stop the daemon. |
| R07 | Direct watches local Git workspaces with approximately one-second quiet-period debounce and bounded maximum delay under continuous edits. Include tracked edits and eligible untracked files, honour excludes/ignore rules, safely snapshot immutable owned input and label working-tree provenance. Never write/reset/stash/checkout sources. Detect concurrent capture changes rather than publish mixed evidence. |
| R08 | Sleep/wake checks mirrors promptly and reconciles missed Direct events. Restart restores durable scheduler/watch state and identity. Interrupted builds resume/retry safely; failed staged revisions retain previous known-good active knowledge while bounded repair runs in background. |
| R09 | Repository management supports add/remove, include/exclude, retry, force rebuild, indexing health and active/attempted revisions. Removal purges all owned repository knowledge/history/cache/mirror and references, never the source workspace. In-flight jobs cannot resurrect removed repositories. |
| R10 | Rebuild KB preserves identity/configuration and recompiles safely. Prefer keeping last good state until replacement validates; if an explicit reset must discard it, communicate that before the action. All generation/projection activation is atomic and consistency-aware. |
| R11 | Per-user launchd lifecycle starts at login and operates headlessly. Do not require a root LaunchDaemon to reuse user Git credentials. One instance on a Mac for V1, with stable identities/layout that permit later multiple instances. Installer, start/stop/status/open and uninstall tooling are supported. UI has Stop daemon with accurate disconnected/restart instructions; a menu bar app is optional. |
| R12 | A namespaced local URL is delivered, including collision detection, human-selected alternatives and persistence. Do not silently rename. Resolve the actual macOS addressing/port contract and document it: mDNS hostnames alone do not route HTTP ports. Preserve loopback/local-only access and browser origin/capability/CSRF controls; localhost is a recovery address, not a substitute for the namespace feature. |
| R13 | User-invoked updates use a crash-safe staged binary/state transaction, declared disk compatibility, migration and health validation. Automatically restore matching previous binary AND state on failure, including power loss. Preserve onboarding/config/IR; rebuild only changed projections unless unavoidable and explicitly diagnosed. No background self-download/update mechanism. Test at least one actual supported upgrade; future V2 compatibility is a contract to implement in V2, not something V1 can prove today. |
| R14 | Resource-aware bounded workers, memory, queues and disk retention. Normal/constrained/idle opportunity policies reduce heavy indexing/projection/benchmark work on battery/load but eventually process freshness and edits. Fairness, cancellation, disk-full behaviour and telemetry must be observable. |
| R15 | Canonical Knowledge IR remains authority; graph/FTS/structural/vector/UI are rebuildable projections. Vectors discover candidates and never constitute evidence. Preserve bounded deterministic planning, provenance, confidence, coverage and found/not_found/unknown. Unsupported languages/ambiguity/budget exhaustion must not imply absence. |
| R16 | Spotlight handles natural developer questions, symbols, paths, logs, config keys, routes and event names; support documented slash or @ commands/filtering. Keyboard focus, cancel, stale/unknown/truncated states and coverage remain clear. History is short and local, persists across restarts, has Clear history, and is excluded from telemetry/default exports. |
| R17 | Primary answer is a versioned compact investigation payload: query/intent, source generations/freshness, findings, relationships/conditions, source excerpts/locations, canonical evidence, confidence/coverage, unknowns and budget/truncation metadata. Copy is faithful to UI evidence and future LLM context. No connected LLM or promise that all LLMs generate identical answers. Local user-requested copy may include source; diagnostics policy is separate. |
| R18 | The cloud never lies: every node/edge represents an actual entity/relationship or explicitly labelled aggregate. No decorative particles, invented knowledge, fake progress, fabricated synapses or misleading complete counts. Aggregates preserve membership/count meaning, provenance and generation identity; disclose sampling/hidden/truncated data. |
| R19 | Real interactive volumetric rendering, rotation, pan, zoom, selection and semantic zoom: estate → repository → package/module → file → symbol/entity. Seed colour styles the UI; semantic node/edge/state colours have keys and accessible alternatives. Inspect identity/path/type/relationships/evidence; support applicable editor/Finder/remote source links safely, with revision-aware destinations. |
| R20 | Search navigates/dims/highlights actual relevant knowledge and animates the investigation path. Initial build shows real events; updates animate only deltas. Animation never delays work or invents causality. Reduced-motion and keyboard/accessible evidence navigation are required. Motions have documented timings; fast facts may remain visible briefly after completion, correctly labelled. |
| R21 | Contextual draggable floating modules for indexing, health, storage/size, performance, coverage, scheduled jobs and active query. Appear when useful; normal idle UI remains uncluttered. Persist reasonable placement and avoid hiding errors/accessibility/navigation offscreen. |
| R22 | Dedicated Benchmark Lab with isolated Demo fixtures and optional My Knowledge known-answer cases/regression suite. Demo data never silently contaminates the user's cloud/KB. Compare correctness, timing, files/bytes read, context bytes and estimated tokens first; advanced metrics in detail. Same immutable source snapshot, transparent baseline/configuration and cold/warm/indexing costs; show losses and incorrect results honestly. Unknown is not counted as correct. Do not bias scores to win. Retain machine-readable per-query results so future routing can learn limitations; no routing engine in V1. |
| R23 | Local-only OpenTelemetry-backed structured logs, bounded metrics/traces and diagnostic dumps, correlated across ingestion/query/jobs/upgrades. No remote exporter/runtime telemetry calls. Apply a default PII/secret redaction pipeline before persistence/export, not only archive cleanup. Redact freeform error paths/URLs/secrets/query/source content as needed. |
| R24 | Default diagnostic bundle excludes source/excerpts, credentials, query text and identifying paths/remotes; includes versions, config shape, health, stages, timings, coverage and errors. Optional expanded developer dump requires explicit warning/selection and still passes default redaction pipeline; never raw credentials. Bound size/retention and test adversarial planted PII/secrets across logs/traces/errors/archives. |
| R25 | Smooth polished UI, not functional placeholders. Target 60fps on declared Apple Silicon reference hardware, degrade detail before responsiveness, common warm indexed queries ideally sub-second. These are measured targets, not invented universal guarantees. Declare dataset, hardware, memory/disk envelope and measurement methodology before tuning; report misses. Test 1, 25 and 100 repositories plus dense large-symbol clouds, not merely 100 tiny empty fixtures. |
| R26 | Uninstall stops service/jobs and offers preserve or delete owned config/KB. Manual update/install/recovery documentation, release notes, user guide and diagnostic instructions accompany checksummed candidate artifacts. All required implementation/product gates and separate independent review pass before candidate-ready. Stakeholder alone releases V1 after using it; later defects can be patches, but known failing mandatory gates are not waived. |

## Engineering defaults

Retain Go backend and React/Vite; prefer existing StyleX where useful. Renderer is
an implementation choice. Choose stable IDs and versioned contracts over UI names.
Keep extraction language-agnostic/extensible and document exact supported depth.
Use vetted pinned dependencies; isolate untrusted source and never run arbitrary
repository hooks/builds as part of discovery. Preserve loopback sessions, origin
checks and bounded access when implementing namespaced URLs and editor actions.

Acceptance is against generic reviewed fixtures and optional user corpora. Prior
private-estate gates may remain opt-in generic tooling after removing assumptions;
they must not block candidate delivery or be claimed as run without access.

## Completion semantics

Queue completion is insufficient. Final evidence must cover the installed release
artifact, current code, native macOS lifecycle, UI, failure recovery and upgrade.
Linux/core evidence does not prove launchd, mDNS, power policy, Gatekeeper or GPU
UX. A missing native worker is an environment blocker: finish portable work and
prepare/run native CI where possible, then report remaining actual evidence.
Do not substitute stakeholder subjective approval for engineering verification.
