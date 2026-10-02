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


## Frozen protocol1 fixture A

Exact source `6619f662ad87a8b88719e0eecd0c93d882766082`, tree
`9a617ad70e2be0746910ffa6119cdbc9c7b4106d`, materialized from checked Git API
objects; remote branch codex/completion-protocol1-fixture-a retains source while
main remains cf896eda85b568ca1b5fd2fe83e896ca89a858c9. Formal Linux core
126.999s PASS; web with installed Chromium58.265s PASS27unit/32browser cases;
scoped upgrade5.658s PASS3scenarios/32recorded assertions plus118portable Go cases.
Receipt measurements explicitly native_lifecycle_proven=0; no native lifecycle
proof inferred. First web attempt failed missing Chrome distribution; corrected
Chromium run passes and failed raw attempt remains private.

Two clean detached worktrees each accepted12cases/25repos, equal semanticFP
f75584b3be0c5e3750e72144fc072c55af56235e37d8218a39bf2ff8ef7656ea. Actual Linux A ZIP
/workspace/scratch/upgrade-fixture-a-package/aios-1.0.0-protocol1-a-linux-amd64.zip
SHA b68310ead11a612c5f97664da734bae84eaeccbdf2f0ecd2bef75a614841a829.
Frozen actual archive executable0500 SHA
18aaae55f4493460988d41151746ea2eb7558d49c1d6620ea41e0b3653af3526,
unchanged after scoped suite, GoVCS source A/modifiedfalse. Full archive inventory,
inspection, paired acceptance and command/scenario receipts retained as
07-protocol1-fixture-a-*.json; raw logs/private artifacts remain outside commits.

Real native workflow36952287178/job110667628343 on Apple Silicon is SUCCESS;
the actual native evidence and remaining A/B obligations are recorded below.
Linux job110667628492 failed existing scope polling helper's SQLITE_BUSY at
repositories_test.go338; unlike add polling it treats bounded transient reads as
fatal. Separate reviewer classifies this as nonblocking for narrow protocolA
fixture freeze and requires bounded polling correction in B while retaining final
included/excluded assertions. No whole-CI success claim. A is an engineering
compatibility fixture, not historical/public release. Distinct B will now change
lookup builder/read completeness; native A/B/fault/disk-full/controls proof remains
explicit until actual execution and mandatory for final assembled acceptance.


## Candidate B correction in progress

Lookup builderv2 verifies complete bidirectional active canonical entity/evidence
membership at build/provenance validation and exact-read boundaries. Exact lookup
runs version/catalog/health/completeness/result reads in one SQLite read transaction.
Changed-only migration expects lookupv2 and all other ordinary buildersv1, rebuilding
only the changed lookup while preserving canonical rows, active generations and
unchanged projection IDs/fingerprints. Vector retains its separate builder identity
and schema contract. Adjacent missing-row/wrong-evidence regressions reject exact
reads and negative knowledge returns unknown; rebuild restores found and preserves
canonical fingerprints. Genuine previous-builder regression proves changed-only
lookup replacement. No formal B receipt/commit yet.

Added completionfixture-tagged actual A/B package integration: requires distinct
retained checksummed/source-bound archives, installs real A with portable service
callbacks, populates nonempty IR using A's own installed local-ingest CLI, executes
real staged A/B candidate validators, faults at health then verifies actual restored
A executable/state/config/identity/projection IDs and A's own status CLI, and proves
successful B rebuilds only lookup. This is actual binary/state validation on Linux;
callbacks do not claim installed launchd controls. Scoped B compatibility requires
this executed fixture, not identical-package inspection. Native success/fault/kill,
real disk-full and A's restored start/stop/uninstall remain final15 obligations.

Corrected the existing scope polling test to retry bounded transient ActiveGeneration
read errors within its30s deadline, like its existing add polling. Included/excluded
symbol and generation-replacement assertions are retained. This responds to actual
A Linux CI SQLITE_BUSY failure; no product timeout or correctness threshold relaxed.


A native engineering job110667628343/run36952287178 actually completedSUCCESS
on Apple Silicon (source6619f662); native Go/vet/Python/frontend/install/launchd/
onboarding/browser and four14case25repo acceptance runs passed, matching semanticFP
6eeab4379b06b9176ddf374e503050290aa89fb3a1690e03a5c6bb5c2caa693a.
Small evidence ZIP973b7fd7a2cee41def4de2350c5f57cd164e26c804aa0de9d12ad31d90cfe5b8
retained privately, summarized07-protocol1-fixture-a-native.json. Actual native
protocol1 package ZIP is retained by GitHub artifact11205375980 (271909285bytes,
GitHub declared archiveSHAeafd39dd81d7c33d26251e18e4c757264a8f313c5f0f5456d8e2997a113fa537,
expires2026-12-31); local download_file refuses>32MiB and direct network transfer
was forbidden. Thus no local actual ZIP-byte/inventory/executable verification is
claimed. Native B/finalCI must download that retained actual package and verify
its source/manifest/inventory/GoVCS before use. Successful TAR-installed binary
SHA63ae744527cbf62048cac2b8cb9483a737a77363ab68ffe881a826d9647e84f3
is separate from the protocol ZIP; TAR bytes were not retained. Whole workflow
remainsFAIL from recordedLinux polling defect; nativejob isSUCCESS. No A/B native
upgrade/power-loss or final candidate acceptance claim from these engineering checks.

Independent review measured repeated lookup certification at 50k canonical functions
exhausting the existing1s query budget. Query now reads active snapshot metadata
instead of full diagnostics and relies on ExactCandidates' single transactional
completeness/read validation; no certification cache or weakened safety check.
Dense corrected measurements and exact B gates remain pending. Actual native
A/B package mechanics workflow retrieves retained sourceA artifact36952287178,
verifies SHA256 and executes nonempty actual package tests with explicit portable
service callbacks; no launchd/fault/native final gate is implied by those callbacks.

Actual Linux A/B installed binaries both passed nonempty upgrade/fault/state
assertions. Initial fixture cleanup failed on A's immutable snapshot directory;
corrected fixture restores owned directory write permission after assertions only.
Provisional corrected suitePASS2.132s, final exact-source execution pending.
Lookup completeness uses exact expected entity/evidence coverage plus equal counts
under PK uniqueness in one snapshot, safely avoiding the second full joined scan.
Regressions cover extra inactive rows and equal-count substitution. This follows
independent review's dense50k query budget finding; no changed time budget/cache.


## Final B engineering validation

Implementation source17631ea9ac6bb18a16aaa855c01a55c5eebfc6e6/tree
d0f285718e3f2e3dcdcccab1b7dffd63e1ca10c5 is committed and clean.
Actual frozen Linux archive549cf26d8597d8ad8d8e8fa976943a13d5253f04c497f15f0359ceb4b5fb3628
and executable75d40ebf5bdc330413ea784dd9e0bf6b3809f1f02c51797d2b5b23ec614982e7
verified against every manifest inventory hash and GoVCS fullsource/modifiedfalse;
executable unchanged0500 before/after scenarios. SourceA6619f662 archive is an
actual distinct preserved protocol1 engineering fixture, not historicalrelease.

Formal core142.140s PASS (rawdigest6d2a8fe0d16967761837a62ab181e2c0f51ce7f6b12900bf3051b7c5131b7941);
web Chromium59.771s PASS27unit/32browser (c3790d90c38bf11edbf35a361c9959987839f80e841a587db5fbd6c6bcc7ff64).
Scoped upgrade10.064s PASS3scenarios/37recordedassertions, actualABparent+2subcases
and118portabletransaction cases (cb8e02f4bf7cc6a47e92d46a7d16b3e80ca582292a9b22a68ca6ce6818dccef9).
Actual A own installedCLI populated nonempty canonical knowledge. Health fault
restored matching A binary/state/config/instance/projection IDs and A ownstatusCLI;
successful B preserves canonicalFP/generations/config/ID, rebuildsonlylookup.
Portable service callbacks explicitly native_lifecycle_proven=0; nativeactual
launchd upgrade/fault/kill/diskfull/restoredcontrols remain mandatory15.

Fresh detached exactB worktrees each accepted12cases/25repos with equalsemanticFP
f75584b3be0c5e3750e72144fc072c55af56235e37d8218a39bf2ff8ef7656ea, verified
actual report/binary/log hashes and cleanGoVCS. Receipts07-upgrade-{core,web,
package,acceptance,scenarios}.json and07-upgrade.json retained in evidence;
private rawfiles/artifacts under/workspace/scratch/upgrade-b-evidence.

Independent dense fixture found valid50k negative requests can exhaust1s budget
under concurrent browser load, truthfully returningunknown. Idle corrected runs
known365–403ms/missing800–884ms, allcorrect; underload oneunknown1.003s is a
recorded performance miss, never a correctnegative. Task14/15 must measure and
tune the declared native scale/load envelope. Lookup completeness still validates
in one snapshot; no certificationcache and noquerybudget relaxed.

Native engineering workflow36955124703 targets exactB. Linuxjob110676301956
isSUCCESS, including corrected bounded scopepoll. Nativejob110676301767 is
inprogress; no nativeB PASS claim yet. Existing A native engineering evidence
is separate and does not certify B finalcandidate. Full current-code/native
assembled proof, actual diskfull and installed A restoredcontrols, licensing and
referencehardware/GPU/wake remain explicitly pending final15.


## Independent review and publication handoff

Separate reviewer /root/upgrade_reviewer passed corrected exactsource17631ea9
in07-upgrade-review.json with no blocking task07 findings after reproducing and
verifying fixes, actual archives andA/Bexecution, exact-source race overlays,
formal receipts/twicefreshacceptance and dense querycorruption/count-proof tests.
Idleafterweb dense50k negatives799.9–886.5ms pass, concurrentload1003msunknown
remains measured14/15 obligation. Native full upgrade/powerloss/actualENOSPC/
Acontrols and protocol0preservedv9 native migration remain final15 obligations.
Task07 is ready for normal fast-forward publication; completion only after
verifiedmain and follow-upbookkeeping. Continue08 localobservability next.
