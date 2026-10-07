# Run V1 benchmarks

Use a fresh private directory per fixture:

```sh
aios benchmark \
  --fixture scripts/benchmark/fixtures/retrieval-v1.json \
  --data-dir /private/benchmark/retrieval \
  --output /private/benchmark/retrieval.json

aios benchmark \
  --fixture scripts/benchmark/fixtures/compiler-v1.json \
  --data-dir /private/benchmark/compiler \
  --output /private/benchmark/compiler.json
```

Reports include exact, lexical, structural, graph, hybrid, grep-baseline, and
context-package results; Precision@1, Recall@5, MRR, nDCG, correctness,
provenance, latency, index/update time, context size, and deterministic
LLM-free answer rate. They also include result-state correctness for positive,
supported-negative, and unknown cases; negative-evidence accuracy; coverage
completeness; and unknown rate. Treat a `not_found` as valid only when its
coverage basis is complete. Results prove only their checked-in gold cases.

An optional reviewed 25-repository, 150-case corpus can provide additional coverage;
see [`../acceptance-handoff.md`](../acceptance-handoff.md). It may use generic fixtures
or voluntarily supplied repositories. No private estate is a release prerequisite.
The ordinary benchmark does not establish assembled-product readiness: the
[completion acceptance contract](../../.codex/completion/ACCEPTANCE.md) defines that gate.

The live [Benchmark Lab](benchmark-lab.md) adds isolated Demo and persistent My
Knowledge regressions. The older CLI fixture runner labels its baseline as an
in-process case-insensitive fixed-string scan of frozen owned bytes, not ripgrep.
Its `manifest_read_ms` is an `ActiveFiles` read cost, not warm indexing. Unknown
state classification is separate from answer correctness. See each report's
measurement notes before comparing costs.
