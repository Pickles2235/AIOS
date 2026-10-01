# Inspected implementation against the installable V1 contract

Milestone 01, inspected 2026-10-01 from `2e24f0c` and its baseline changes.
This is a code-to-requirement gap map, not passing assembled acceptance. Existing
test names below identify reusable assertions; executed checks and reviewed source
commit belong in the milestone plan/receipts. All native final proof is pending.

## Preservation decisions

Keep the Go backend, React/Vite entry point, immutable canonical IR, generation
activation, typed provenance, deterministic planner and optional local vector
retrieval. Extend their existing boundaries instead of replacing the engine.
Retain `github.com/AdamNi-7080/AIOS` as the compatibility Go module identity;
it is an import/build coordinate, not an estate namespace or product requirement.
The product name/default is AgentOS; CLI/binary and owned artifact names stay `aios`.
Saved user names and stable IDs are preserved without migration or forced renaming.

The example catalog has one generic repository. Inventory generation accepts 1–100
and has no language allowlist. Existing secure file-deny rules, explicit caller
patterns and bounded discovery remain. Fixed 25-repository/150-case optional corpus
counts preserve benchmark thresholds, not product prerequisites. Historical task
plans, completion records and CHANGELOG evidence remain historical. The generic
`estate` fixture name/IR topology term does not identify an employer or user estate.

macOS Apple Silicon is the supported product. The CI matrix labels Linux as
developer smoke and packages only the Apple Silicon candidate. Current archive
scripts for other platforms remain development tools. CI and old first-run evidence
do not prove the expanded installed daemon, namespace, upgrades or GPU experience.
MCP remains a compatible read-only integration; browser onboarding requires no
MCP client, external model, Codex or private employer repositories.

## Requirement map

| Requirement | Actual implementation and reusable assertions | Gap and implementation owner |
| --- | --- | --- |
| R01 generic 1–100 repositories | `internal/catalog/catalog.go`, `internal/mirror/registry.go`, `internal/app/ingest.go`; `TestVariableSourceEstate`, `TestVariableEstateBounds`, `TestVariableEstateMatchingAndAtomicBootstrap`. Milestone 01 removes Homefold defaults and generator's exactly-25/language policy; `scripts/test_generate_v1_catalog.py` checks 1/3/24/25/100, zero/101, explicit scope and private output. | Preserve catalog/registry matching and extractor limits. Generic requirements and supported-platform docs aligned in 01; scale proof remains 14. |
| R02 installable bundled Apple Silicon artifact | `scripts/build-v1-release.sh`, `internal/webui/webui.go` embeds dist, `internal/semantic/nomic.go` embeds pinned model/helper on arm64. `TestBundledNomicEmbedsLocally`, compiler Java/TypeScript coverage tests, `scripts/verify-macos-first-run.sh` inspect the native archive. | Current archive is a tarball without installer; no finished toolchain-free daemon ZIP/install transaction. Implement 03 and installed-artifact proof 15. Optional compilers remain honest coverage gaps. |
| R03 identity/branding/exclusive source choice | `internal/store/instance.go`, `internal/webui/instance.go`, `internal/webui/setup.go`, `web/src/instance-brand.tsx`, `mirror-setup.tsx`; `TestInstanceLegacyPersistenceResetAndRebuild`, `TestInstanceMutationRequiresCSRFAndPersistsIdentity`, real mirror/local E2E in `web/e2e/runtime-backend.spec.ts`. Current setup selects mirror OR clean local capture. | No persisted namespace, local logo generation, batch discovery/scope preview; mixed request fields currently discarded rather than rejected. Extend 04, preserve identity/logo validation and authenticated mutation. |
| R04 immediate main UI/staged activity | `setup.go` persists actual syncing/ingesting/completed states and activates through `internal/app`; `main.tsx` polls setup and reloads on ready. `TestAuthenticatedMirrorOnboardingRetryAndRestart`; live first-run E2E. | Current main entry keeps setup visible during build, lacks progressive staged discovery/visual distinction and per-repository activity stream. Extend 04/11; staged data never answers active evidence. |
| R05 machine Git credentials | `internal/mirror/registry.go` validates explicit URLs/refs and invokes fixed Git commands; `scripts/generate_v1_catalog.py:verify_remotes` is optional inventory tooling. `TestSyncWritesOnlyAgentOwnedMirror`. | Both commands disable global config/credential helpers and construct restricted environments; no actual launchd login-context authentication or remediation proof. Extend mirror auth in 04, without embedded token store or executing source hooks. |
| R06 polling/backoff/per-repo resilience | `internal/reconcile/reconcile.go` is an in-memory full-scan reconciler with one coalescing signal channel and serial direct callbacks; `reconcile_test.go` tests change/restart/coalescing behaviour. `mirrors sync` is currently explicit CLI/setup work. | Reconciler defaults 30s polling/250ms debounce, exits on scan/callback error and is not a durable mirror scheduler. Implement 15-minute configurable polling, Check now, isolated retry/backoff/stale state in 05. |
| R07 dirty/untracked safe Direct capture | `internal/adapter/local.go` reads pinned Git objects into read-only owned snapshots; `TestLocalSnapshotsArePinnedBoundedAndNonMutating`, `TestConcurrentWorkspaceEditNeverPublishesMixedSnapshot`, `TestLocalIngestCitesCapturedCommitAndRetainsGenerationOnFailure`. | Clean committed only: dirty/nonignored untracked inputs rejected. No watcher/one-second quiet-period bounded max delay or working-tree provenance. Extend capture/reconciliation in 05; never reset/stash/write sources. |
| R08 wake/restart/repair | `setup.go:restoreSetup` converts interrupted setup to retryable state; `internal/store/ingestion.go` persists revision selection; `TestRevisionSelectionIsDurableAndIdempotent`, `TestDiscardStagedRepositoryPreservesActiveGeneration`. | No daemon wake integration/durable scheduler/watch recovery or bounded automatic repair. Extend 05. Existing last-good atomic activation is reusable, not proof of native wake. |
| R09 repository management/purge | Existing setup/configuration and `internal/store/store.go:Retain` remove unreferenced historical generations; `TestRetainPurgesOnlyUnreferencedHistoricalGenerations`. | No end-to-end add/remove/rules/retry/force-rebuild surface, complete purge or in-flight removal tombstones. Implement 06 including cache/mirror/history/reference cleanup and source nonmutation. |
| R10 safe full rebuild | `internal/store/store.go:RebuildProjections`, `ResetDerivedData`, staged `ActivateCatalog`; `TestProjectionRebuildIsDeterministicAndKeepsPriorBuildOnFailure`, `TestInvalidActivationPreservesActiveCatalog`, instance reset/rebuild test. | Projection rebuild is implemented; full configuration/identity-preserving KB replacement and UI action missing. Explicit reset removes database files and needs honest destructive-state messaging. Extend 06. |
| R11 headless per-user lifecycle | `cmd/aios/main.go` owns `ui serve`/stdio `serve`; loopback server lifetime is currently the invoking foreground process. | No launchd installer/login service, lifecycle commands or UI stop/restart flow. Implement 03. A foreground server or optional development launcher is not a daemon. |
| R12 namespaced local URL/security | `internal/webui/webui.go` binds ephemeral IPv4 loopback and checks origin, capability/session/CSRF; `TestUIRequiresReadOnlyStoreAndLoopback`, `TestSessionIsOneUseAndCSRFProtected`, refresh-session tests. | No namespace resolution, collision/alternative selection, persisted addressing/port contract. Extend 04; namespace must remain local-only and include actual port routing, not just an mDNS name. |
| R13 atomic explicit upgrades | `internal/store/store.go:verifyFormat` rejects incompatible schema; `TestExistingLegacyDatabaseRequiresCleanReindex`. Current docs describe manual binary/data preservation. | No compatibility manifest, migrations, durable update journal, matching binary+state rollback or supported-upgrade fixture. Implement 07; no background updater. Manual pointers are not crash-safe update proof. |
| R14 bounded resource policies | `internal/catalog`, `discover`, `planner`, `cache` bound source/query/cursor work; `internal/reconcile` invokes callbacks serially; local vector execution caps native threads. `TestValidateHardCatalogAndPatternLimits`, planner budget tests. | No native power/load/idle signals, durable fair queues/eventual freshness, disk-full/resource telemetry policies. Implement 09 after maintained jobs/observability; publish measurement envelope before tuning. |
| R15 canonical evidence/coverage/determinism | `internal/store/{schema.sql,store.go,coverage.go,query.go,cross.go,vector.go}`, `internal/planner`, `knowledge`, `mcp`, `context`, `slice`. Canonical/projection validation tests, `TestQueryNegativeNotFoundAndUnknownCoverage`, `TestVectorQueryDeadlineReturnsUnknownWithoutClaimingIndexLoss`, cache coverage invalidation tests. | Reuse rather than rewrite. MCP has the richer deterministic planner; browser `knowledge.Query` is currently exact identity then lexical fallback, not full intent parity. Native vector proof remains pending; richer browser integration 10. |
| R16 Spotlight/commands/history | `web/src/spotlight.tsx`, `main.tsx`, `runtime-types.tsx`; E2E covers keyboard focus, selection, stale/unknown/negative/truncated feedback. Request IDs suppress stale UI completion. | No slash/@ commands or persisted short local history/Clear history. Closing ignores late responses but does not cancel backend fetch/work. Extend 10 with richer intents, coverage and actual cancellation. |
| R17 compact investigation/copy | `internal/context/package.go`, `slice/slice.go`, MCP provenance envelopes and browser canonical excerpt fields; `TestContextReturnsBoundedCanonicalPackage`, bounded generation-aware slice tests. | No versioned browser investigation payload/Copy evidence parity contract; current UI renders selected entities/excerpts. Extend 10. Source copy may include source; default diagnostics may not. |
| R18 truthful cloud membership | `internal/knowledge/read.go` returns bounded active canonical nodes/claims; `web/src/knowledge-cloud.tsx` draws only page entities and edges with visible endpoints, disclosing counts/generation/truncation. `TestProjectionAndEvidenceAreBoundedAndGenerationBacked`; graph-workspace E2E. | No hierarchy aggregates with traced membership/sample semantics. Preserve truthful empty/partial states while adding aggregates in 11. |
| R19 actual volumetric/semantic navigation | Current live `knowledge-cloud.tsx` is SVG ring layout and one instance seed colour; main entry has canonical evidence inspector. `graph-layout.ts`/other component helpers are not proof of live 3D integration. | No volumetric render/rotate/pan/zoom, semantic zoom hierarchy/legend or revision-safe source actions. Implement 11 with accessible evidence alternative and native GPU proof. |
| R20 real event/query/delta animation | Current live UI shows actual polled stage/result state; no invented nodes/progress. | No sequenced ingestion/promotion/delta/query-path events or corresponding truthful animations. Implement 11; document timing/reduced motion and avoid implying causality. |
| R21 contextual modules | `main.tsx` has fixed status, setup, evidence and projection panels; backend status reports active repositories/projection health. | No draggable contextual operational modules, durable safe placement/reset, live coalesced reconnect stream. Implement 12. Helpers/components alone do not satisfy live entry point. |
| R22 honest isolated Benchmark Lab | `internal/benchmark/benchmark.go`/tests measure fixture retrieval, literal baseline, provenance, context size and index/update costs; optional `v1_estate_acceptance.py` preserves fixed-corpus thresholds. | No live Demo/My Knowledge Lab isolation, persisted user gold cases, explicit immutable baseline parity/source bytes or honest loss/unknown presentation. Existing quality uplift gates cannot substitute for unbiased Lab outcomes. Extend 13. |
| R23 local OTEL/persistence redaction | `internal/store/diagnostics.go` persists bounded diagnostic fields and rejects unsafe metadata keys; `TestDiagnosticLedgerIsRedactedAndResolvable`. `internal/cache` excludes query text/source bodies. | No OTEL collector/traces/metrics correlation/retention. Key rejection is not a value-redaction pipeline: freeform remediation/scope/resolution can retain paths/PII. Implement 08 before disk persistence, no remote exporter. |
| R24 default/expanded diagnostics | Existing bounded status/doctor and diagnostic ledger expose versions/coverage/errors. | No archive/export flow or explicit expanded selection/warning and adversarial all-channel redaction checks. Implement 08, including size/retention limits and UI. Never confuse source Copy with safe export. |
| R25 declared native scale/UX targets | Catalog permits 100; `app/estate_test.go` validates variable sets; benchmark records retrieval timings. These are not dense clouds, sustained maintenance or frame measurements. | No declared reference hardware/data/envelope, 1/25/100 dense mixed corpus, long soak, 60fps/sub-second target measurements or graceful LOD proof. Implement 09/14, with actual native evidence and disclosed misses. |
| R26 candidate handoff/uninstall/review | `completion_harness.py` validates queue, receipts/review provenance and candidate consistency; harness regression tests reject false/stale acceptance. Existing archives include docs/notices/checksums. | `completion-*` scenario targets, installer/uninstaller/update tooling and final manifest/requirements evidence missing. Implement driver 02, lifecycle 03/07 and full artifact handoff 15. Stakeholder release stays pending after engineering acceptance. |

## Extractor and privacy limits to preserve

`internal/extract/extract.go` has Tree-sitter grammars for Java, Kotlin,
JavaScript/JSX and TypeScript/TSX. Java and TypeScript optional compiler frontends
add resolved relationships; documentation/configuration provide lexical facts.
Arbitrary repository languages are permitted inputs, not a promise of structural
coverage. Computed routes/events, reflection/runtime registration and unavailable
compiler resolution must remain coverage gaps/unknown, never fabricated absence.

Full source text lives in the owner-only canonical database. Filename denylists
are a backstop, not general PII detection. Source hooks/builds are never executed
for discovery. Existing code must not be represented as satisfying default export
redaction or source sandboxing that has not actually been implemented/tested.

## Validation and continuation

Milestone 01 uses the existing core gate and real inventory/catalog/default identity
assertions. UI copy changes require rebuilt embedded production assets and live
authenticated E2E. Native final evidence is deferred explicitly, not skipped/pass.
Milestone 02 creates executable scenario drivers; its self-tests must not mistake
missing later product behaviour for completed acceptance. Continue in dependency
order after reviewed implementation is verified on fetched main.
