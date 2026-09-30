# Install and operate V1

Requirements are Go 1.25, CGO with a C compiler, Git, Node/npm for rebuilding
the UI, and Java/Node/TypeScript only when compiler-level coverage is desired.
No runtime service is required.

Build a release archive:

```sh
npm ci --prefix web
./scripts/build-v1-release.sh --version 1.0.0 --output-dir /private/releases
(cd /private/releases && shasum -a 256 -c aios-1.0.0-*.sha256)
```

On Windows, run `scripts/build-v1-release.ps1 -Version 1.0.0 -OutputDir C:\releases`
and verify the adjacent `.sha256` file with `Get-FileHash`.

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
