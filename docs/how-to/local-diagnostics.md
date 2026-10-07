# Local diagnostics

Open **Diagnostics** in the authenticated browser UI. The panel shows the
product/IR/Go versions, active and configured repository counts, projection
health, maintenance stage counts, aggregate operation timings and local
telemetry health. It does not display source or query text.

**Download diagnostic archive** creates the default ZIP. It contains aggregate
status and health only. To include individual operation spans and structured
operation logs, select **Include expanded developer diagnostics**, read the
warning, then explicitly acknowledge it. Review any archive before sharing.
Both modes omit source/excerpts, query text, credentials, source paths and Git
remotes. Identifiers in individual records are opaque hashes. The expanded
choice does not turn off redaction.

Telemetry uses local OpenTelemetry SDK spans and a local metric counter with no
remote exporter. Redacted JSONL operation records are written under the owned
`data/diagnostics` directory. Root-level upgrade/recovery records are kept
under the installation root and are read without creating files during
read-only health checks. Each sink retains at most three private 256 KiB files;
archives are at most 1 MiB compressed. Old records rotate out. A full, unsafe,
or unavailable sink increments the dropped-write count or reports unavailable;
it never falls back to an outbound collector. Setting OTLP environment
variables does not enable an exporter.

Existing operational SQLite diagnostic rows are redacted on the next writer
open before the new policy is marked current. A busy checkpoint causes a
retryable writer error. This cleanup does not remove canonical compiler
diagnostics, source evidence or coverage: those are part of the local knowledge
base and are excluded from diagnostic exports. Normal evidence and local Copy
actions can include source content; they are separate user-requested actions.

If the panel reports diagnostics unavailable, check owner permissions and free
space on the owned installation/data directory, then restart the daemon.
Do not move diagnostic files into a shared directory or replace them with
symlinks or hard links.
