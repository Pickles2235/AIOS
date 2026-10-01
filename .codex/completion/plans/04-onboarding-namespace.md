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

Precommit Linux core and web passed (27 UI unit cases, 22 browser cases at two desktop sizes); scoped live helper/TLS, branding/collision/partial batch and staging probes passed. Targeted race tests pass for interrupted stage/query isolation, resume, unchanged batch members, same-revision scope changes, named-origin migration/registration-loss retry, scope exclusion/source nonmutation and actual external helper auth/error sanitation. Process regressions verify context cancellation kills pipe-holding helper descendants and bounds an exited parent's inherited-pipe wait. Formal clean-source receipts and twice-fresh acceptance fingerprints remain to collect after committing the product.

Native DNS-SD independent ownership conflicts and complete system-resolver loopback answers are mandatory native tests. verify_onboarding_native.py installs an owned disposable service and exercises launchd external Git profile/CA helper success, auth failure retaining last-good state/mode, real named-origin migration, localhost recovery and restart. CI retains its actual report. Native certification is pending actual run; no login reboot/GPU/final candidate claim.

## Independent review

Separate reviewer report and corrected commit required.

## Blockers

None established; native requirements need actual hardware/CI.

## Handoff

Persist implementation SHA, publication evidence and next work. Continue mission.
