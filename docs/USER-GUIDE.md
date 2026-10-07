# AgentOS candidate installation

The supported product is macOS Apple Silicon, installed for your login user.
Extract the candidate ZIP and verify its adjacent SHA256 file with
`shasum -a 256 -c aios-VERSION-darwin-arm64.zip.sha256`. No Go, Node, Python or JDK
is needed to install or run. Required UI, structural extractors and local embedding
assets are bundled. Git is required for repository sources. Semantic Java and
TypeScript compilers are optional. The authenticated `/api/v1/runtime` endpoint
reports detection separately from successfully covered source evidence; a runtime
settings screen is still pending. Unsupported extraction remains unknown.

From the extracted directory run `./install.sh --package /absolute/candidate.zip
--json`. Use your own login account, without sudo. The default owned installation
is `~/Library/Application Support/AgentOS`; sources stay in their original locations.
Define a convenient shell variable:

```sh
AIOS_BIN="$HOME/Library/Application Support/AgentOS/current/bin/aios"
"$AIOS_BIN" daemon start
"$AIOS_BIN" daemon status
"$AIOS_BIN" daemon open
```

Start registers a per-user LaunchAgent that starts at login and restarts after an
unexpected exit. The browser can close while work continues. Open obtains a fresh
one-use secure browser link through an owner-only local control socket. The URL is
loopback only; there is no remote account or LLM connection. Keep launch links private.
Use the browser's Stop daemon button or `"$AIOS_BIN" daemon stop`; start/open reconnects.
For a browser you choose yourself, `"$AIOS_BIN" daemon link` prints a fresh private
launch link. Do not paste this capability into logs or reports. Long data-directory
paths use a hashed socket in an owner-only system temporary directory to respect
macOS's Unix socket length limit; knowledge stays in the configured owned root.
Stop unregisters the currently running service while retaining its login plist so
it can restart on the next login. Uninstall removes that plist. A foreground
developer `ui serve` session reports that it must be stopped in its terminal.

The candidate is currently unsigned/unnotarized. macOS may block a downloaded
binary. Use macOS Privacy & Security to approve the binary only after verifying
the artifact and its source; do not disable Gatekeeper globally. Signing and a
menu-bar application are outside this first candidate's prerequisites.

Choose exactly one source mode in Repository setup: Mirror creates owned mirrors;
Direct captures current local working trees, including tracked edits and eligible
untracked files, without modifying them. Git ignore rules and approved scope
patterns are applied before source bytes are copied. Preview a
batch of 1–100 sources and adjust include/exclude patterns before Build. Language
and framework-manifest discovery describe scope; they do not certify compiler or
dependency availability. Build enters the main UI immediately. Discovery and staged
generations are progress only; validated catalog promotion makes evidence queryable.
A failed or cancelled build keeps the last good active knowledge.

After Build, Direct watches approximately one second of quiet edits with a
five-second maximum delay. Periodic reconciliation repairs missed events. Mirror
checks every 15 minutes by default. Expand Repository health to change the Mirror
interval or Check now for one repository. The panel reports actual last success,
active revision, next check and stale warnings. Failed checks retry with bounded
exponential backoff; healthy repositories continue independently. Restart restores
the owned durable queue and checks for missed changes. Native sleep/wake evidence
remains part of final candidate validation.

Git uses your machine's external configuration and credential helpers without
terminal/password prompts. AgentOS does not store helper responses or tokens.
Configure credentials outside AgentOS. Installation retains only explicit external
Git configuration and CA-file paths; SSH agent sockets are inherited from the
current login environment. `"$AIOS_BIN" daemon credentials` explicitly probes the
running daemon's configuration; helper detection alone is not proof a remote can
be authenticated. Authentication failure explains how to retry or explicitly choose
Direct mode while leaving your selected mode unchanged.

Choose a local namespace yourself. Native macOS registers a DNS-SD name only on
this Mac, resolves it to IPv4/IPv6 loopback and uses an explicit HTTP port shared by
127.0.0.1 and ::1 listeners. No wildcard or LAN listener is opened. Collision suggestions
are never saved until you select one. Changing the name navigates with a fresh private
launch link. `"$AIOS_BIN" daemon link --recovery` obtains a localhost link if the name
is unavailable. Saved names are retained on restart; unavailable registration is
reported rather than automatically renamed. Linux names use a developer `.localhost`
address and do not certify native DNS integration. Instance name, colour and optional
locally generated PNG logo preserve the same durable identity.

Knowledge and UI links carry generation, provenance and coverage; an unknown result
is not proof of absence. Source checkouts must never be reset, stashed, built or written
by AgentOS. Native login Git/namespace certification remains pending the retained
macOS checks; later milestones add investigation features.

Repository health shows background resource state, queue age, why a build is
deferred, and a Cancel update control. On battery or high load, background
updates wait up to five minutes; an old queued update then gets a turn. AC idle
time is an opportunity for immediate work. If the owned data admission budget
or free volume reserve is exhausted, new heavy builds wait until space is
available. Existing active knowledge and source workspaces remain untouched.
`aios resources status --json` shows native power, load and idle observations.
The resource panel reports measured owned usage, free volume space and the
100 GiB start-work admission cap (with 8 GiB reserved for a build). This does
not cap bytes written during an already running build. The maintenance journal
is separately capped at 512 KiB. By default, activation retains the active
catalog and two earlier catalog revisions; the configured retention count can
change that finite history window. Retention also reclaims owned snapshots of superseded generations after
captures finish; it never deletes the source workspace or active generation.
Mirror archives stream through the configured per-repository byte/file limits.
Free owned space or reduce approved scope to resume a deferred build.

To uninstall, choose explicitly: `"$AIOS_BIN" uninstall --preserve-data --json`
retains configuration/KB for reinstall; `--delete-data` deletes owned state. Both
stop the service and remove the installed binary. The original source workspaces
are preserved. Retain the extracted candidate's binary or reinstall script to
reinstall preserved data. Custom acceptance installs use `--root` and disposable
`--service-label`; do not point a test at your live installation.

From the newly extracted candidate, inspect an explicit local update with
`./bin/aios upgrade inspect --package /absolute/candidate.zip --json`, then apply
it with `./update.sh --package /absolute/candidate.zip --json`. Add `--root` for
a custom installation. There are no automatic downloads or background updates.
Finish any pending repository removal before updating. The updater stops the
owned service, stages the executable and a consistent state copy, validates
migration and read-only health, and commits the matching binary/state together.
During validation, browser mutations are paused. Before commit, failure restores
the previous pair; the next startup recovers an interrupted transaction. After a
durable commit, newly ingested knowledge is retained: a restart or cleanup error
requests `./bin/aios upgrade recover --json` from the extracted candidate instead
of discarding that knowledge. Do not replace binaries/state manually.

Protocol1 updates require an installed manifest declaring `update_protocol: 1`
and matching declared layout and canonical-format compatibility. Earlier protocol0
engineering installations are rejected before stopping or changing their state.
For those prototypes, use their installed binary to uninstall with
`--preserve-data`, then use the new candidate to install at the same root. That
reinstall stages supported canonical-format migration while retaining instance
identity, configuration and knowledge. It is a separate manual migration path.

If initial installation is interrupted, repeat the install command or use
`upgrade recover` before installing again. An interrupted uninstall retains its
original preserve/delete choice; repeat that same command using the extracted
candidate binary. A small private hashed lock/cleanup authority remains outside
the installation to coordinate deletion and reinstall safely. It contains no
source or query content. Do not delete transaction control files to bypass an
error. Use [Local diagnostics](how-to/local-diagnostics.md) for a bounded redacted
support archive. Full native installed-upgrade proof remains a release-candidate
gate; this is still an engineering candidate.

Manifest `disk_schema: 1` describes the installation layout/metadata only. The
canonical database is `knowledge-ir-v10`; `compatible_from` declares layout
compatibility and `compatible_ir_formats` declares state compatibility. Supported
upgrade evidence must bind distinct actual packages and source revisions. Complete
transitive dependency/native helper attribution is required before final packaging.
