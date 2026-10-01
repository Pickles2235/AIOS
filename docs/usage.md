# Install and operate V1

The supported installable product targets macOS Apple Silicon. Installer/launchd,
headless lifecycle and transactional updates are pending completion milestones 03
and 07; the commands here describe current development archives, not a finished installer.

Building from source requires Go 1.25+, CGO with a C compiler, Git and Node/npm.
A built archive bundles the UI and local assets; Go/npm/C tools are not runtime
requirements. Machine Git is used for repository capture. Java/Node/TypeScript
are optional compiler coverage, with explicit diagnostics when unavailable. No
cloud service, Codex installation or LLM connection is a product dependency.

Build a release archive:

```sh
npm ci --prefix web
./scripts/build-v1-release.sh --version 1.0.0 --output-dir /private/releases
(cd /private/releases && shasum -a 256 -c aios-1.0.0-*.sha256)
```

Other-platform archive scripts remain developer smoke tooling, not supported
installable-product releases.

Extract the archive into a new immutable installation directory. Keep the
previous directory until acceptance succeeds, then point the operator's stable
launcher or symlink at the new binary. Rollback changes that pointer back; the
source repositories are never part of installation or rollback.

Validate the bounded 1–100-source V1 catalog, synchronise its approved mirrors, then
ingest into a private data directory:

```sh
aios catalog validate --config /private/catalog.json
aios doctor --config /private/catalog.json --data-dir /private/data
aios mirrors sync --registry /private/mirrors.json --data-dir /private/data
aios ingest --config /private/catalog.json --registry /private/mirrors.json --data-dir /private/data --repo frontend
aios status --data-dir /private/data
```

Upgrades reuse a compatible V1 data directory. If the command reports an old,
invalid, or corrupt derived database, stop readers, move that data directory
aside, create a fresh owner-only directory, and reingest approved mirrors. Derived data is
rebuildable; never delete or modify a catalogued repository to recover it.
