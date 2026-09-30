# Verify Apple Silicon installation and first run

Task 07 needs actual macOS arm64 evidence and operator observations. Linux tests,
cross compilation and automated CI do not establish the complete manual first-run
experience. Keep the task blocked until every required observation is recorded.
Use a clean account or owned test directory and approved fixture repositories;
production/private-estate readiness has its separate acceptance gate.

## Native automated evidence

On an Apple Silicon Mac, record `sw_vers`, `uname -sm`, the exact Git commit and
versions of Go (1.25+), Node (22+), Python (3.10+), Git/Git LFS, JDK 21 and the C
compiler. Install Xcode command-line tools. Fetch the repository and pinned LFS
objects, then run `make harness-setup`. Review the candidate version; the CI-only
`1.0.0-harness` version is an unpublished test archive, not a release declaration.

From a clean checkout of the reviewed commit:

```sh
native_work=$(mktemp -d "$HOME/Library/Caches/homefold-native.XXXXXX")
native_work=$(cd "$native_work" && pwd -P)
npm exec --prefix web -- playwright install chromium
PLAYWRIGHT_CHANNEL=chromium ./scripts/verify-macos-first-run.sh \
  --version 1.0.0-harness --output-dir "$native_work/evidence"
```

The script refuses non-Darwin/non-arm64 hosts and existing output directories.
It runs `make harness-validate`, builds a native archive with `make release`,
checks the adjacent SHA256 file, extracts a clean installation and records its
Mach-O architecture and build metadata. It runs `make harness-validate-web`
against that installed binary, including real mirror/local onboarding, Spotlight
keyboard selection, page refresh, renaming, server restart and projection rebuild.
Go integration tests cover permissions, interrupted setup/cancellation/retry,
dirty-source failure, concurrent edits and preservation of active generations.
Only owned fixture paths are canonicalized; production symlink rejection remains.

Two fresh `make acceptance-v1` runs must report `accepted` and equal
`semantic_fingerprint` values. An additional installed-binary acceptance runs
under macOS `sandbox-exec` with network access denied and must match the same
fingerprint. The pinned Nomic model/helper are bundled in the binary; the native
semantic test executes the helper and validates the model hash, 768 dimensions
and owner-only directory/model/helper permissions. The helper uses CPU execution
with two threads and no warmup to avoid per-process GPU startup costs. Native
runtime assets are prepared at MCP startup before accepting requests. Native
embedding receives the caller deadline; budget exhaustion remains `unknown`
rather than implying missing evidence or a lost projection.
If sandbox execution is unavailable, record the exact failure and arrange an
actual offline run before claiming no-download installation verified.

The `macos-15` arm64 job in `v1-native.yml` runs this procedure and uploads
`macos-first-run-evidence`: environment, archive checksum, installed-binary
metadata, fingerprints, three acceptance reports and browser failure traces.
Inspect the exact commit's job, including its native semantic test; an overall
workflow result from another platform is not the arm64 result.

## Manual clean-install observations

Use the verified installed binary and a new canonical owned data directory:

```sh
native_data="$native_work/manual-data"
mkdir -m 700 "$native_data"
"$native_work/evidence/install/aios-1.0.0-harness-macos-arm64/bin/aios" \
  ui serve --data-dir "$native_data"
```

Open the printed one-use link locally. Keep its token and cookies out of the
handoff. Record pass/fail and evidence for each observation:

- Clean extraction and native launch work through macOS security controls. Record
  quarantine/Gatekeeper behavior and any required remediation; do not claim
  signing/notarization. A reused launch token fails; refresh retains the session.
- Rename the instance, choose a seed colour and upload a bounded PNG/JPEG logo;
  verify its accessible fallback without a logo. Save the stable ID privately
  from owned `instance.json`; restart, reopen the new link, and rebuild projections.
  ID, name, colour and logo persist independently of canonical evidence.
- Configure one approved mirror using an explicit ID and full Git ref, then
  several approved repositories. Validation, actual synchronization/ingestion
  stages and successful activation appear. An invalid remote reports its error.
  Cancel or stop during setup, restart, observe interruption, and explicitly retry.
  A failed attempt leaves the prior active evidence queryable.
- Capture a clean local Git workspace through **Clean local Git workspaces**.
  Record source `git status --porcelain=v1`, tracked/untracked byte hashes, modes
  and `.git/index` hash before and after success and failure. Dirty, nonignored
  untracked and symlink inputs are rejected without source changes. Ignore-only
  files are not captured. Local evidence cites its captured commit/hash/generation.
- Command-K opens Spotlight and focuses its input; Up/Down and Enter open evidence;
  Escape closes and restores focus. Ordinary query/list navigation also works.
  The cloud shows returned counts, bounds and generation; it never invents nodes.
  Check empty/unsupported inputs, unknown coverage, covered absence, unavailable
  projections, bounded positive results and stale selections with distinct feedback.
- With runtime network access disabled after installation, verify bundled Nomic
  extraction/embedding and inspect owner-only runtime model/helper permissions.
  Source evidence remains canonical; vectors do not become assertions.

## Handoff and completion

Record commit, OS/architecture/toolchains, archive and binary SHA256, runner/job
URL, acceptance dispositions and fingerprints, browser results, manual observations,
source non-mutation hashes, permissions, failures and remediation. Store tokens,
credentials and private corpus data outside committed documentation.

After all native/manual checks pass, publish any scoped repairs to main and verify
the implementation SHA is reachable from fetched `origin/main`. Only then mark
`07-macos-first-run` completed with that full SHA in a follow-up bookkeeping
commit. If hardware or interactive access is missing, mark blocked with the exact
unperformed checks and this checklist; leave `completion_commit` null. Resume on
an actual Mac from the published task branch/commit and rerun against that exact
revision before changing the status.
