# Benchmark Lab

Open **Benchmark Lab** in the running browser UI. **Run Demo** builds a private
five-file corpus and compares the product's knowledge query with a literal file
scan. Demo never adds repositories or generations to your knowledge instance.

For **My Knowledge**, add a query and its expected outcome. Found cases require a
repository and file path; a line and source snippet make the answer more precise.
Save the cases, then replay them. Expectations are declared by you, never inferred
from the result. The first replay binds unbound cases to the captured indexed
snapshot. Later replays flag changed snapshots; old expectations remain visible
until you deliberately revise them. Advanced JSON supports a `repository` filter
(applied to both methods) and a `declared_snapshot` SHA256 binding.

Cases and the latest report survive restart, privately in the owned data folder.
**Clear saved cases and report** removes them. **Export per-query JSON** includes
queries, paths, expected snippets and returned source excerpts: review it before
sharing. It is a separate explicit export; diagnostic bundles exclude this state.

## What the comparison measures

My Knowledge uses SQLite `VACUUM INTO` under a bounded read lease to copy the
active canonical database, including immutable generations, coverage and existing
projections. It queries that exact copy, not a new index with different extraction
coverage. SHA256 and size are checked against every copied source's actual bytes.
The literal baseline reads private materialized copies of the same bytes; both
manifest hashes are independently calculated. Live workspaces are never read or
modified. Indexed generations may lag the workspace.

The baseline is `aios.literal.v1`, a Go fixed-string, case-sensitive line scan in
repository/path/line order, not an invocation of ripgrep. It scans every captured
file in the selected repository scope. Logical file/byte counters count completed
reads; physical I/O and SQLite page/byte reads are unavailable. KB source-file
reads are zero because it reads SQLite. These are different counters, not a claim
that knowledge retrieval incurs zero I/O.

KB timing includes retrieval and canonical evidence reads. First and repeat query
wall times are measured; OS and SQLite caches are uncontrolled. Demo reports the
actual temporary build cost. My Knowledge reports snapshot copy cost and marks
original cold/warm indexing cost unavailable. No original indexing time is
invented. Context bytes are the UTF-8 JSON representation of query results plus
excerpts, or literal matching rows. Tokens are explicitly `ceil(bytes / 4)`, not
an actual tokenizer or billed model usage.

Answer correctness scores the first returned answer against the declared
repository/path/line/snippet. Recall@5, MRR and NDCG expose later matches. Complete
coverage is required for correct absence. Unknown, unsupported queries and errors
never count as correct answers. State classification correctness is reported
separately. Cases have equal weight, including losses. Literal output retains up
to 100 matching rows and explicitly marks truncation; full match counts and ranks
are still measured. Evidence items exceeding 16 KiB make the KB query unavailable.

The Demo's punctuation operator case was selected as an exploratory capability
probe after initial fixture inspection, with source and expectation fixed before
scoring. It is not independent holdout evidence. Its exact `->>` source match
illustrates the literal baseline's advantage over unsupported punctuation-only
lexical queries. The duplicate-name case declares ambiguity, rather than assigning
an arbitrary sole correct file; a `found` response is a classification miss.

## Bounds and recovery

One comparison per browser service runs at a time, with a two-minute budget and an explicit cancel
button. The last saved report remains available during a run. Storage admission
reserves 1 GiB of capture space above the standard free-space reserve. The complete
indexed database must fit 256 MiB; captured active sources must fit 64 MiB and
10,000 files. Cases are limited to 40, queries to 256 bytes and reports to 8 MiB.
Larger instances receive an unavailable error, never a subset absence claim.
These are current Lab limits, not proof of 100-repository Lab performance; the
scale milestone must measure realistic estates and revise the bounded strategy
where needed. Use a smaller separately indexed comparison instance if necessary.

Temporary copies are private and removed after success, failure or cancellation.
A later run cleans interrupted directories only when they have the Lab's explicit
ownership marker and its owning process has stopped. Saved files use atomic replacement and file/directory syncing.
