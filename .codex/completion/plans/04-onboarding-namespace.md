# Complete onboarding, exclusive source modes and local namespace

Requirements: R03, R04, R05, R12

## Acceptance

- Validate batch sources and preview include/exclude/language scope; persist identity/logo/colour/namespace.
- Collision suggestions require human selection; native namespaced browser origin and recovery localhost are secure.
- Actual launchd Git credential environment tested; auth errors recommend Direct without switching.
- Build enters main UI with empty cloud and real visibly staged progress; no queryable unvalidated state.

## Decisions

Reuse existing instance/immutable snapshot/atomic catalog contracts. Add strict exclusive source validation, bounded real scope preview with saved include/exclude rules, local deterministic PNG logos and context-derived real discovery/staging/promotion events. Main UI enters indexing state immediately and never treats staged evidence as active. Mirror subprocesses use trusted machine credential configuration in a noninteractive bounded environment; sanitized failures recommend Direct without switching. Namespaces use owned leases plus native local-only DNS-SD A records pointing at loopback with an explicit HTTP port; recovery localhost remains available, collision alternatives require human selection and every allowed origin remains Host/session/CSRF bound. Linux namespace behavior is explicitly developer-only. Native proof will use disposable sources/service/name/config, never default live user state.

## Progress

Started after task03 reviewed source/evidence and completion bookkeeping published/fetched as38d8bc87829d25ff26eedd458a950197e2345f4c. Harness next selects04. Existing code silently discards mixed-mode arrays, suppresses machine credential helpers/global config, lacks preview/logo generation/namespace/activity, and holds users in setup until completion. Extend it rather than replace canonical IR.

## Scenario scope

Run existing core/web gates plus scoped product scenarios with `gate NAME --milestone 04-onboarding-namespace`. Full assembled/native gates are final-candidate requirements. Record deferred native checks explicitly.

## Actual validation

Precommit Linux core and web passed (27 UI unit cases, 22 browser cases at two desktop sizes); scoped live helper/TLS, branding/collision/partial batch and staging probes passed. Targeted race tests pass for interrupted stage/query isolation, resume, unchanged batch members, same-revision scope changes, named-origin migration/registration-loss retry, scope exclusion/source nonmutation and actual external helper auth/error sanitation. Process regressions verify context cancellation kills pipe-holding helper descendants and bounds an exited parent's inherited-pipe wait. Final implementation4cab95f464761b26456b360197437f71153d5a07 passes clean core/web/scoped-onboarding gates, recorded in evidence/04-onboarding-{core,web,onboarding}.json. Web verifies27unit and22browser cases across1440/2560. Both fresh12-case,25-repository acceptance runs pass with identical semantic fingerprint f75584b3be0c5e3750e72144fc072c55af56235e37d8218a39bf2ff8ef7656ea; redacted summary is evidence/04-onboarding-acceptance.json. Raw logs and source-bearing reports remain private under /workspace/scratch/aios-onboarding-evidence. Actual callback interruption checks retain old queryable catalog until atomic activation; actual external helper TLS/401 checks retain last-good evidence and source mode.

Native DNS-SD independent ownership conflicts and complete system-resolver loopback answers are mandatory native tests. verify_onboarding_native.py installs an owned disposable service and exercises launchd external Git profile/CA helper success, auth failure retaining last-good state/mode, real named-origin migration, localhost recovery and restart. CI retains its actual report. Native installed-service restart failed at b83/c025. Fixed external stop/uninstall to wait for final lifetime cleanup; Start now verifies loaded ownership before returning status and waits before bootstrap after UI Stop. UI Stop requests shutdown without waiting on its own lock. A canceled daemon does not report Running. Native0ac installed helper/TLS, named browser origin, rename, recovery and different-process/stable-ID restart preflight passes; latest4cab installed-service native preflight also passes, including actual UI Stop/disconnection/restart. Actual run36919253078/job110560882820 at4cab completed that mandatory preflight successfully at20:11:57UTC, recorded in evidence/04-onboarding-native-step.json from fetched GitHub run/job data. The remainder of the native CI job and its report artifact are still pending; no full-job, hardware measurements or final assembled acceptance claim. No login reboot/GPU/final candidate claim.

## Independent review

Separate session /root/baseline_reviewer requested and verified fixes for stale SSH socket persistence, revoked-host rename navigation, automatic Git launcher execution, helper-status honesty, stalled helper descendants, independent DNS ownership conflicts, full loopback resolver answers, unhealthy same-name retry, sealed-tree deletion and restart lifetime barriers. Its independent race checks pass. Independent report evidence/04-onboarding-review.json passes exact4cab95f464761b26456b360197437f71153d5a07 after checking final receipts and independently fetching the successful current native preflight. No blocking findings; this is engineering milestone review, not final candidate acceptance. Native report artifact must be fetched and its ZIP digest verified when the current job finishes; API-observed successful preflight is explicit milestone proof, not the final native gate.

## Blockers

None established; native requirements need actual hardware/CI.

## Handoff

Implementation4cab95f464761b26456b360197437f71153d5a07 and independent review/receipts published as0a256bf75fad4f1373fa2f6384b92fe215bd66fd, verified by fetching actual GitHub main and recording its exact object locally. Marked completed only after source/review were reachable from fetched main. Continue task05-maintained-repositories, implementing durable polling, watches and safe repair. Harvest current native CI report/artifact when available and retain its verified digest; full assembled native gates remain15.
