# AgentOS candidate installation

The supported product is macOS Apple Silicon, installed for your login user.
Extract the candidate ZIP and verify its adjacent SHA256 file with
`shasum -a 256 -c aios-VERSION-darwin-arm64.zip.sha256`. No Go, Node, Python or JDK
is needed to install or run. Required UI, structural extractors and local embedding
assets are bundled. Git is required for repository sources. Semantic Java and
TypeScript compilers are optional; the runtime screen reports detection separately
from successfully covered source evidence. Unsupported extraction remains unknown.

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
Stop unregisters the currently running service while retaining its login plist so
it can restart on the next login. Uninstall removes that plist. A foreground
developer `ui serve` session reports that it must be stopped in its terminal.

The candidate is currently unsigned/unnotarized. macOS may block a downloaded
binary. Use macOS Privacy & Security to approve the binary only after verifying
the artifact and its source; do not disable Gatekeeper globally. Signing and a
menu-bar application are outside this first candidate's prerequisites.

Choose exactly one source mode in Repository setup: Mirror creates owned mirrors;
Direct selects local workspaces. Reuse of machine Git credentials in the daemon
is still task04 work: current mirror discovery disables credential helpers/global
Git configuration. Configure credentials outside AgentOS; authentication remediation
and namespace support are not yet certified. Source checkouts must never be reset, stashed, built or written by
AgentOS. Knowledge and UI links carry generation/provenance and coverage; an unknown
result is not proof of absence. The completion mission adds maintenance, namespace,
resource policy and investigation features in later milestones.

To uninstall, choose explicitly: `"$AIOS_BIN" uninstall --preserve-data --json`
retains configuration/KB for reinstall; `--delete-data` deletes owned state. Both
stop the service and remove the installed binary. The original source workspaces
are preserved. Retain the extracted candidate's binary or reinstall script to
reinstall preserved data. Custom acceptance installs use `--root` and disposable
`--service-label`; do not point a test at your live installation.

Manual atomic updates and diagnostic export are being implemented in their owner
milestones. Until those pass, this package is an engineering candidate, not a
candidate-ready release. Do not replace binaries/state manually to simulate an update.

Manifest `disk_schema: 1` describes the installation layout/metadata only. The
canonical database is `knowledge-ir-v9`; `compatible_from` is a declaration, not
proof of a supported upgrade until the upgrade milestone passes. Complete
transitive dependency/native helper attribution is required before final packaging.
