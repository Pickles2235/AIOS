# Implement crash-safe explicit update and uninstall

Requirements: R13, R26

## Acceptance

- Release compatibility manifest and supported upgrade fixture preserve identity/IR without repeat onboarding.
- Interrupt/fail each install/migration/activation/health boundary; restore matching old binary/state and restart.
- Atomic filesystem/disk-full/power-loss recovery covered with durable journal; no background self-update.
- Uninstall stops jobs/service; preserve/delete choice operates only on owned data.

## Decisions

Inspect current implementation before selecting changes.

## Progress

Protocol1 implementation and portable fault regressions are in progress. Real
filesystem transaction tests cover 20 updater failure boundaries and 20 externally
SIGKILLed updater processes, initial install for fresh/preserved data (18 failure
and 18 killed-process cases), and preserve/delete uninstall (10 failure and 10
killed-process cases). These use synthetic executable bytes and service callbacks;
they prove portable transaction mechanics, not actual installed native lifecycle.
Authenticated health tests exercise live HTTP reads, mutation rejection, unchanged
owned-file digests and absent maintenance/namespace workers. No formal clean-source
task07 receipt, supported A/B fixture or native upgrade pass is claimed yet.

## Scenario scope

Run existing core/web gates plus scoped product scenarios with `gate NAME --milestone 07-atomic-upgrades`. Full assembled/native gates are final-candidate requirements. Record deferred native checks explicitly.

## Actual validation

Record commands, receipts, platform and full tested commit. No checks run yet.

## Independent review

Separate reviewer report and corrected commit required.

## Blockers

None established; native requirements need actual hardware/CI.

## Handoff

Persist implementation SHA, publication evidence and next work. Continue mission.

## Session design

Started from verified published main9dfe196001efb8f12ef2b1ed0cda25f87675543d; task03 dependency and task06 engineering review are reachable. Implement explicit checksummed local-package update protocol1, declared installation-layout and canonical-format compatibility. Stop verified service before snapshot; serialize updates with an owned root lock and retain writer/lifetime leases through consistent copying. Stage candidate and complete owned state on the same filesystem, migrate only the staged copy, compare canonical identities/facts/config/instance, rebuild only incompatible derived projections and validate candidate health. Fsync journal before every mutation and files/directories at publication boundaries. A stable owned launchd recovery launcher remains outside the replaceable current/state paths, restoring previous matching binary/state before next daemon startup for any uncommitted transaction; committed cleanup resumes idempotently. No downloads/self-update. Extend explicit uninstall to recover pending transactions before preserve/delete and remove only owned state.

The actual supported prior fixture will be a validated committed protocol1
engineering candidate A followed by distinct candidate B. Store v9→v10 staged
migration is separately tested from reconstructed genuine old schema; it is not
claimed as a transactional upgrade from the published protocol0 task04 binary.
No native upgrade or hardware power-loss proof is claimed from portable callback
or externally killed-process tests. Retain actual native A/B ZIPs for final gates.

Independent design review identified that pre-protocol04 CLI ownership checks cannot control a rollback left behind a protocol1 recovery launchd wrapper. That published04 choice was a session implementation decision, not a required compatibility promise. Establish an explicit protocol1 compatibility boundary: protocol0 engineering installations are rejected before stopping or modifying state; document preserving data and reinstalling through a separately tested migration path. The actual supported upgrade fixture will be a real committed, validated protocol1 engineering candidate A, then distinct candidate B with a substantive lookup-projection completeness/read-boundary correction (builderv2). A is not a historical/public release. Freeze A only after protocol fault/recovery checks and separate review; retain actual native A/B packages, checksums/GoVCS/source commits and A-populated state. Native successful/failed/killed upgrades must retain canonical rows, active generations, identity/config, only rebuild changed lookup, preserve all other projections and invoke A's restored controls. Actual storev9→v10 migration remains separate evidence and is not advertised as a protocol0 transactional upgrade.

Review corrections in progress: common install/update/uninstall root lease and authority re-read; durable intent before stop; recovery interpreter checksum authority before its rename/metadata/plist boundaries; allocated recovery reserve released before rollback journal writes; committed cleanup tolerates legitimate later ingestion; every attempted candidate startup is stopped/waited before rollback. Dedicated health constructor omits all setup/removal/namespace restoration, maintenance and user mutations, and exits when updater lease disappears. No passing task07 source receipts yet.


## Writer, source and durability review corrections

Separate reviewer `/root/upgrade_reviewer` reproduced killed-owner writer access,
late-approved source deletion, approved plist-workspace deletion and approved
current-workspace deletion. Corrected central store writer/reset fence checks the
same complete owned journal schema as recovery before mkdir/migration/reset;
precommit intent survives SIGKILL, while verified committed binary/identity permits
legitimate ingestion. Stable writer leases cover old data, missing publication
paths and staged candidate inodes. UI serve holds the daemon lifetime lease for
its full serving period. Private validate-state accepts only an existing owned
new-data slot matching durable install/update authority.

Uninstall intent is in a private hashed persistent sibling on the installation
filesystem, independent of transient flock directories and root deletion. Newly
created directory entries and their ancestors are fsynced before intent/mutation.
After stopping, source approvals are refreshed under writer/lifetime authority
before cleanup. Approval/replay/update/uninstall protect the full installation,
external plist and external durable authority. Snapshot copying rejects source
path replacement after copying as well as in-place size/mtime changes.

Development checks (dirty source, no formal passing receipt): complete Go suite
passed with Go1.27.1/JDK21; adjacent20 update ENOSPC+20 real SIGKILL cases and18
initial-install ENOSPC+18 SIGKILL cases now assert writer/reset refusal before
recovery. Postcommit test ingests a genuinely changed canonical generation and
verifies cleanup retains it. Uninstall10 failure+10 kill cases cover preserve
and delete, durable original choice and partial cleanup; external intent survives
transient lock-directory deletion. Separate reviewer overlays and expanded race
suite passed after corrections; exact-source final review/gates still pending.

Scoped upgrade driver now runs real Go JSON transaction tests with explicit
required case/subcase counts and records native_lifecycle_proven=0. It is portable
engineering mechanism evidence, using synthetic executable bytes/service callbacks.
Full upgrade fault gate continues to fail explicitly pending actual native A/B
fault/disk-full/installed-controls scenarios; no native gate is silently skipped.
Candidate A freeze/build/review precedes candidate B's substantive lookup builder
correction. Workflow retains checksummed actual native protocol1 engineering ZIPs.
