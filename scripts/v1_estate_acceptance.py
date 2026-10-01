#!/usr/bin/env python3
"""Validate and run the optional reviewed 25-repository V1 effectiveness gate."""

from __future__ import annotations

import argparse
import hashlib
import json
import os
import statistics
import subprocess
import sys
from pathlib import Path
from typing import Any

SCHEMA_VERSION = 1
CATEGORIES = ("exact", "structural", "trace_cause", "cross_repo_impact", "negative_unknown")


class GateError(RuntimeError):
    pass


def category(case: dict[str, Any]) -> str:
    query_class = str(case.get("query_class", "")).lower()
    if case.get("expected_state") in ("not_found", "unknown") or "negative" in query_class or "unknown" in query_class:
        return "negative_unknown"
    if any(token in query_class for token in ("cross", "impact", "producer", "consumer")):
        return "cross_repo_impact"
    if any(token in query_class for token in ("trace", "cause", "caller", "event")):
        return "trace_cause"
    if any(token in query_class for token in ("structural", "locate", "route", "path")):
        return "structural"
    if any(token in query_class for token in ("exact", "identifier", "symbol", "configuration")):
        return "exact"
    raise GateError(f"case {case.get('id', '<unknown>')} has an unclassified query_class")


def validate_corpus(corpus: dict[str, Any], source_root: Path | None = None) -> dict[str, int]:
    repositories, cases = corpus.get("repositories"), corpus.get("cases")
    if not isinstance(repositories, list) or len(repositories) != 25:
        raise GateError("reviewed corpus must declare exactly 25 repositories")
    if not isinstance(cases, list) or len(cases) != 150:
        raise GateError("reviewed corpus must declare exactly 150 cases")
    repository_ids: set[str] = set()
    for repository in repositories:
        identifier, revision = repository.get("id"), repository.get("revision")
        if not isinstance(identifier, str) or not identifier or identifier in repository_ids:
            raise GateError("repository IDs must be non-empty and unique")
        if not isinstance(revision, str) or len(revision) != 40 or any(character not in "0123456789abcdef" for character in revision):
            raise GateError(f"repository {identifier} must pin a lowercase 40-character revision")
        if not isinstance(repository.get("path"), str) or not repository["path"]:
            raise GateError(f"repository {identifier} path is required")
        repository_ids.add(identifier)
    identifiers, categories = set(), {name: 0 for name in CATEGORIES}
    per_repository = {identifier: 0 for identifier in repository_ids}
    queries: list[str] = []
    for case in cases:
        identifier, query = case.get("id"), case.get("query")
        if not isinstance(identifier, str) or not identifier or identifier in identifiers:
            raise GateError("case IDs must be non-empty and unique")
        if not isinstance(query, str) or not query.strip():
            raise GateError(f"case {identifier} query is required")
        identifiers.add(identifier)
        queries.append(query)
        categories[category(case)] += 1
        expected_state = case.get("expected_state", "found")
        if expected_state not in ("found", "not_found", "unknown"):
            raise GateError(f"case {identifier} has invalid expected_state")
        repository = case.get("expected_repository")
        if expected_state == "found":
            if repository not in repository_ids or not case.get("expected_path"):
                raise GateError(f"found case {identifier} needs an approved repository and path")
        if repository:
            if repository not in repository_ids:
                raise GateError(f"case {identifier} references unknown repository {repository}")
            per_repository[repository] += 1
    if categories != {name: 30 for name in CATEGORIES}:
        raise GateError(f"cases must be balanced at 30 per category: {categories}")
    underrepresented = sorted(identifier for identifier, count in per_repository.items() if count < 4)
    if underrepresented:
        raise GateError(f"every repository needs at least four cases: {underrepresented}")
    if source_root is not None:
        production = "\n".join(path.read_text(encoding="utf-8", errors="ignore") for base in (source_root / "internal", source_root / "cmd") for path in base.rglob("*.go"))
        leaked = sorted(query for query in queries if len(query) >= 12 and query in production)
        if leaked:
            raise GateError(f"gold queries appear in production source: {leaked[:5]}")
    return categories


def percentile(values: list[float], quantile: float) -> float:
    if not values:
        return 0.0
    ordered = sorted(values)
    index = max(0, min(len(ordered) - 1, int((len(ordered) * quantile + 0.999999)) - 1))
    return ordered[index]


def correctness(results: list[dict[str, Any]], retriever: str) -> float:
    if not results:
        return 0.0
    return sum(bool(result["retrievers"][retriever]["correct"]) for result in results) / len(results)


def evaluate_report(corpus: dict[str, Any], report: dict[str, Any]) -> tuple[dict[str, Any], bool]:
    cases = {case["id"]: case for case in corpus["cases"]}
    results = report.get("results", [])
    if len(results) != 150 or {result.get("id") for result in results} != set(cases):
        raise GateError("benchmark report does not cover the complete gold corpus")
    grouped = {name: [] for name in CATEGORIES}
    for result in results:
        grouped[category(cases[result["id"]])].append(result)
    exact = grouped["exact"]
    structural = grouped["structural"]
    cross = grouped["cross_repo_impact"]
    hybrid_cross = correctness(cross, "hybrid")
    best_single_cross = max(correctness(cross, name) for name in ("exact_only", "lexical_only", "structural_only", "graph_only"))
    metrics = {
        "provenance_completeness": sum(bool(result["canonical"]["provenance_correct"]) for result in results) / len(results),
        "state_correctness": sum(bool(result["state_correct"]) for result in results) / len(results),
        "exact_precision_at_1": sum(float(result["retrievers"]["exact_only"]["precision_at_1"]) for result in exact) / len(exact),
        "structural_correctness": correctness(structural, "structural_only"),
        "llm_free_answer_rate": float(report["quality"]["llm_free_answer_rate"]),
        "hybrid_correctness": float(report["quality"]["hybrid_correctness"]),
        "grep_correctness": float(report["quality"]["grep_baseline_correctness"]),
        "cross_repo_hybrid_correctness": hybrid_cross,
        "cross_repo_best_single_correctness": best_single_cross,
        "cross_repo_uplift": hybrid_cross - best_single_cross,
        "exact_latency_p95_ms": percentile([float(result["retrievers"]["exact_only"]["latency_ms"]) for result in exact], .95),
        "hybrid_latency_p95_ms": percentile([float(result["retrievers"]["hybrid"]["latency_ms"]) for result in results], .95),
        "context_median_estimated_tokens": statistics.median(float(result["context_package"]["estimated_tokens"]) for result in results),
        "changed_file_visibility_ms": float(report["incremental_indexing"]["changed_file_visibility_ms"]),
    }
    checks = {
        "provenance_complete": metrics["provenance_completeness"] == 1.0,
        "states_correct": metrics["state_correctness"] == 1.0,
        "exact_precision": metrics["exact_precision_at_1"] >= .95,
        "structural_correctness": metrics["structural_correctness"] >= .85,
        "llm_free": metrics["llm_free_answer_rate"] >= .70,
        "cross_repo_uplift": metrics["cross_repo_uplift"] >= .15,
        "hybrid_beats_grep": metrics["hybrid_correctness"] >= metrics["grep_correctness"],
        "exact_latency": metrics["exact_latency_p95_ms"] < 100,
        "hybrid_latency": metrics["hybrid_latency_p95_ms"] < 750,
        "bounded_context": metrics["context_median_estimated_tokens"] < 1500,
        "incremental_visibility": metrics["changed_file_visibility_ms"] < 2000,
    }
    return {"metrics": metrics, "checks": checks}, all(checks.values())


def semantic_fingerprint(report: dict[str, Any]) -> str:
    stable = {
        "results": [{
            "id": result["id"], "revision": result["revision"], "state": result["result_state"],
            "canonical_correct": result["canonical"]["correct"],
            "canonical_provenance": result["canonical"]["provenance_correct"],
            "retrievers": {name: {"correct": value["correct"], "rank": value.get("rank", 0)} for name, value in sorted(result["retrievers"].items())},
        } for result in sorted(report["results"], key=lambda value: value["id"])],
        "quality": report["quality"],
    }
    return hashlib.sha256(json.dumps(stable, sort_keys=True, separators=(",", ":")).encode()).hexdigest()


def run(binary: Path, corpus_path: Path, output_dir: Path) -> dict[str, Any]:
    corpus = json.loads(corpus_path.read_text(encoding="utf-8"))
    validate_corpus(corpus, Path(__file__).resolve().parents[1])
    output_dir.mkdir(mode=0o700, parents=True, exist_ok=False)
    reports, fingerprints = [], []
    for index in (1, 2):
        data, report_path = output_dir / f"data-{index}", output_dir / f"benchmark-{index}.json"
        completed = subprocess.run([str(binary), "benchmark", "--fixture", str(corpus_path), "--data-dir", str(data), "--output", str(report_path)], text=True, capture_output=True)
        (output_dir / f"benchmark-{index}.log").write_text(completed.stdout + completed.stderr, encoding="utf-8")
        if completed.returncode != 0:
            raise GateError(f"benchmark run {index} failed; see {output_dir / f'benchmark-{index}.log'}")
        report = json.loads(report_path.read_text(encoding="utf-8"))
        reports.append(report)
        fingerprints.append(semantic_fingerprint(report))
    evaluation, passed = evaluate_report(corpus, reports[0])
    stable = fingerprints[0] == fingerprints[1]
    gate = {
        "schema_version": SCHEMA_VERSION,
        "corpus": str(corpus_path),
        "repositories": 25, "cases": 150,
        **evaluation,
        "semantic_fingerprint": fingerprints[0],
        "reproducible": stable,
        "vector": {"required": False, "state": "separately_gated_when_runtime_available"},
        "disposition": "accepted" if passed and stable else "not_accepted",
    }
    (output_dir / "estate-acceptance-report.json").write_text(json.dumps(gate, indent=2, sort_keys=True) + "\n", encoding="utf-8")
    return gate


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--binary", required=True, type=Path)
    parser.add_argument("--corpus", required=True, type=Path)
    parser.add_argument("--output-dir", required=True, type=Path)
    args = parser.parse_args()
    try:
        gate = run(args.binary.resolve(strict=True), args.corpus.resolve(strict=True), args.output_dir.resolve())
    except (OSError, ValueError, json.JSONDecodeError, GateError) as error:
        print(f"error: {error}", file=sys.stderr)
        return 1
    print(json.dumps(gate, sort_keys=True))
    return 0 if gate["disposition"] == "accepted" else 1


if __name__ == "__main__":
    raise SystemExit(main())
