# Starting evidence and gaps

Baseline main: `619c0b0bc83e956ed2e84225df2b01562b1f54cd`, inspected via GitHub
2026-10-01. This is a targeted harness baseline, not a newly executed code audit.

Existing boundaries: cmd/aios, internal/app, adapter, mirror, store, compiler,
extract, planner, knowledge/context/slice, webui and React web. Reuse the canonical
IR and generation system. Existing Makefile has core, web and 25-fixture acceptance
gates. The old seven-task queue records six completed tasks and native manual
first-run task blocked. Those records do not satisfy the expanded contract.

Directly inspected gaps: `web/src/knowledge-cloud.tsx` is an SVG ring layout with
one seed colour; it does not meet volumetric rendering, semantic zoom or semantic
colour requirements. Old launcher runs one task then stops; no whole-product
completion mission. Old native matrix covers five platforms; new delivery is
Apple Silicon only. Existing public docs make MCP connection central; it is not a
V1 user journey now. Current module path is historical; choose intentional identity
during cleanup rather than incidental rename. Inspect namespace remnants broadly.

Do not assume daemon scheduling/watchers/install transactions/PII pipeline or
Benchmark Lab are implemented simply because packages or prior audit prose exist.
Milestone 01 establishes a current code-to-requirement gap map with exact paths,
tests and reuse/extension/removal choices. Do not restart completed useful work.

Environment of harness authoring: Linux scratch with connector reads/writes, no
full checkout or Apple Silicon hardware. Offline harness verification is separate
from application/core/native verification. No real user repositories are needed
to build this candidate. Existing private-estate proof is optional user validation.
