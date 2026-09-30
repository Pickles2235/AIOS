#!/usr/bin/env python3
"""Hermetic, mirror-backed AIOS V1 acceptance harness."""

from __future__ import annotations

import argparse
import hashlib
import json
import os
import platform
import subprocess
import sys
import time
from datetime import datetime, timezone
from pathlib import Path
from typing import Any, Callable

sys.path.insert(0, str(Path(__file__).parent))
from acceptance_fixture import FEATURE_REPOSITORIES, FIXTURE_VERSION, advance, create


SCHEMA_VERSION = 1
REQUIRED_PROJECTIONS = {"lookup", "lexical", "graph", "path", "ui", "landmarks", "cache"}


class HarnessError(RuntimeError):
    pass


def invoke(command: list[str], cwd: Path, log: Path, timeout: int = 3600) -> tuple[Any, dict[str, Any]]:
    started = time.monotonic()
    completed = subprocess.run(command, cwd=cwd, text=True, capture_output=True, timeout=timeout)
    log.parent.mkdir(mode=0o700, parents=True, exist_ok=True)
    log.write_text(completed.stdout + completed.stderr, encoding="utf-8")
    record = {"command": command, "exit_code": completed.returncode, "elapsed_seconds": round(time.monotonic() - started, 3), "log": str(log)}
    if completed.returncode != 0:
        raise HarnessError(f"command failed ({completed.returncode}): {' '.join(command)}; see {log}")
    output = completed.stdout.strip()
    if not output:
        return None, record
    try:
        return json.loads(output), record
    except json.JSONDecodeError:
        return output, record


class MCPClient:
    def __init__(self, binary: Path, catalog: Path, data: Path):
        self.process = subprocess.Popen([str(binary), "serve", "--config", str(catalog), "--data-dir", str(data)], stdin=subprocess.PIPE, stdout=subprocess.PIPE, stderr=subprocess.PIPE, text=True, bufsize=1)
        self.identifier = 0
        initialized = self.request("initialize", {"protocolVersion": "2025-03-26", "capabilities": {}, "clientInfo": {"name": "aios-v1-acceptance", "version": FIXTURE_VERSION}})
        if initialized.get("serverInfo", {}).get("name") != "aios":
            raise HarnessError(f"unexpected MCP initialize response: {initialized}")
        self._write({"jsonrpc": "2.0", "method": "notifications/initialized"})

    def _write(self, message: dict[str, Any]) -> None:
        assert self.process.stdin is not None
        self.process.stdin.write(json.dumps(message, separators=(",", ":")) + "\n")
        self.process.stdin.flush()

    def request(self, method: str, params: dict[str, Any]) -> dict[str, Any]:
        self.identifier += 1
        self._write({"jsonrpc": "2.0", "id": self.identifier, "method": method, "params": params})
        assert self.process.stdout is not None
        line = self.process.stdout.readline()
        if not line:
            assert self.process.stderr is not None
            raise HarnessError(f"MCP server ended: {self.process.stderr.read()}")
        response = json.loads(line)
        if "error" in response:
            raise HarnessError(f"MCP {method} failed: {response['error']}")
        return response["result"]

    def call(self, name: str, arguments: dict[str, Any]) -> dict[str, Any]:
        response = self.request("tools/call", {"name": name, "arguments": arguments})
        if response.get("isError"):
            raise HarnessError(f"MCP {name} returned tool error: {response}")
        envelope = response.get("structuredContent")
        if envelope is None:
            blocks = response.get("content", [])
            if not blocks or blocks[0].get("type") != "text":
                raise HarnessError(f"MCP {name} returned no structured content")
            envelope = json.loads(blocks[0]["text"])
        if not {"api_version", "provenance", "result"}.issubset(envelope):
            raise HarnessError(f"MCP {name} returned an invalid provenance envelope")
        if envelope["api_version"] != 1:
            raise HarnessError(f"MCP {name} returned api_version={envelope['api_version']}, expected 1")
        return envelope

    def close(self) -> None:
        if self.process.stdin:
            self.process.stdin.close()
        try:
            self.process.wait(timeout=10)
        except subprocess.TimeoutExpired:
            self.process.terminate()
            self.process.wait(timeout=10)
        if self.process.returncode not in (0, -15):
            assert self.process.stderr is not None
            raise HarnessError(f"MCP server exit {self.process.returncode}: {self.process.stderr.read()}")

    def __enter__(self) -> "MCPClient":
        return self

    def __exit__(self, *_: object) -> None:
        self.close()


def paths(envelope: dict[str, Any]) -> list[str]:
    result = envelope["result"]
    return sorted({item.get("path", item.get("Path", "")) for key in ("entities", "evidence") for item in result.get(key, []) if item.get("path", item.get("Path"))})


def entity_handles(envelope: dict[str, Any]) -> list[str]:
    return [item.get("handle", item.get("Handle", "")) for item in envelope["result"].get("entities", []) if item.get("handle", item.get("Handle"))]


def evidence(envelope: dict[str, Any]) -> list[str]:
    return sorted(set(envelope["provenance"].get("evidence_handles", [])))


def route(envelope: dict[str, Any]) -> list[str]:
    return [item for item in envelope["result"].get("plan_trace", []) if item.startswith("route:")]


def acceptance_case(identifier: str, expected: dict[str, Any], action: Callable[[], dict[str, Any]]) -> dict[str, Any]:
    try:
        return {"id": identifier, "status": "pass", "expected": expected, "actual": action(), "diagnostics": []}
    except Exception as error:
        return {"id": identifier, "status": "fail", "expected": expected, "actual": {}, "diagnostics": [str(error)]}


def require(condition: bool, message: str) -> None:
    if not condition:
        raise HarnessError(message)


def semantic_fingerprint(report: dict[str, Any]) -> str:
    stable = {
        "fixture_version": report["fixture_version"],
        "repositories": report["repositories"],
        "cases": [{
            "id": case["id"],
            "status": case["status"],
            "expected": case["expected"],
            "locations": case.get("actual", {}).get("locations", []),
            "planner_route": case.get("actual", {}).get("planner_route", []),
        } for case in report["cases"]],
        "readiness": report["readiness"],
        "disposition": report["disposition"],
    }
    encoded = json.dumps(stable, sort_keys=True, separators=(",", ":")).encode()
    return hashlib.sha256(encoded).hexdigest()


def validate_report(report: dict[str, Any]) -> None:
    required = {"schema_version", "fixture_version", "timestamp", "engine_revision", "platform", "commands", "active_generation", "repositories", "cases", "readiness", "disposition"}
    require(set(report) >= required, f"acceptance report missing fields: {sorted(required - set(report))}")
    require(report["schema_version"] == SCHEMA_VERSION, "acceptance report schema version mismatch")
    require(len(report["repositories"]) == 25, "acceptance report must describe 25 repositories")
    require(all(case.get("status") in {"pass", "fail"} for case in report["cases"]), "acceptance report has invalid case status")


def query_case(client: MCPClient, arguments: dict[str, Any], expected_status: str, expected_path: str | None = None, expected_route: str | None = None) -> dict[str, Any]:
    envelope = client.call("kb.query", arguments)
    result = envelope["result"]
    require(result.get("status") == expected_status, f"status={result.get('status')}, expected {expected_status}; trace={result.get('plan_trace', [])}; uncertainty={result.get('uncertainty', [])}; budget_stops={result.get('budget_stops', [])}")
    returned_paths = paths(envelope)
    if expected_path:
        require(any(path.endswith(expected_path) for path in returned_paths), f"missing expected path {expected_path}: {returned_paths}")
    if expected_status == "found":
        require(bool(evidence(envelope)), "found result has no evidence provenance")
    if expected_route:
        require(expected_route in route(envelope), f"missing route {expected_route}: {route(envelope)}")
    return {"status": result.get("status"), "planner_route": route(envelope), "evidence_ids": evidence(envelope), "entity_handles": entity_handles(envelope), "locations": returned_paths, "result_handle": envelope.get("result_handle", ""), "negative_evidence": result.get("negative_evidence", "")}


def unavailable_vector_case(client: MCPClient) -> dict[str, Any]:
    envelope = client.call("kb.query", {"text": "customer purchase workflow", "retrieval_mode": "vector", "time_ms": 1000, "limit": 5})
    result = envelope["result"]
    require(result.get("status") == "unknown", f"status={result.get('status')}, expected unknown")
    require("route:vector:unavailable" in result.get("plan_trace", []), f"missing vector route diagnostic: {result.get('plan_trace', [])}")
    uncertainty = result.get("uncertainty", [])
    require(any("vector" in value and "unavailable" in value for value in uncertainty), f"missing vector uncertainty: {uncertainty}")
    return {"status": "unknown", "planner_route": route(envelope), "uncertainty": uncertainty}


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--binary", required=True, type=Path)
    parser.add_argument("--output-dir", required=True, type=Path)
    parser.add_argument("--source-root", default=Path(__file__).resolve().parents[1], type=Path)
    args = parser.parse_args()
    source, binary, output = args.source_root.resolve(strict=True), args.binary.resolve(strict=True), args.output_dir.resolve()
    output.mkdir(mode=0o700, parents=True, exist_ok=True)
    os.chmod(output, 0o700)
    vector_available = platform.system() == "Darwin" and platform.machine() == "arm64"

    estate, data, logs = output / "fixture", output / "data", output / "logs"
    manifest = create(estate)
    catalog, registry = estate / "catalog.json", estate / "registry.json"
    commands: dict[str, Any] = {}
    _, commands["catalog_validate"] = invoke([str(binary), "catalog", "validate", "--config", str(catalog)], source, logs / "catalog-validate.log")
    _, commands["mirror_sync_baseline"] = invoke([str(binary), "mirrors", "sync", "--registry", str(registry), "--data-dir", str(data)], source, logs / "mirror-sync-baseline.log")
    _, commands["ingest_baseline"] = invoke([str(binary), "ingest", "--all", "--config", str(catalog), "--registry", str(registry), "--data-dir", str(data)], source, logs / "ingest-baseline.log")
    if vector_available:
        _, commands["vector_rebuild_baseline"] = invoke([str(binary), "projections", "rebuild", "--kind", "vector", "--config", str(catalog), "--data-dir", str(data)], source, logs / "vector-rebuild-baseline.log")
    baseline_status, commands["status_baseline"] = invoke([str(binary), "status", "--data-dir", str(data)], source, logs / "status-baseline.log")

    cases: list[dict[str, Any]] = []
    baseline_generations = {item["repo_id"]: item["git"]["commit"] for item in baseline_status["snapshots"]}
    projection_states = {item["kind"]: item["state"] for item in baseline_status["projections"]}
    cases.append(acceptance_case("baseline_activation", {"repositories": 25, "staged": 0, "projections": sorted(REQUIRED_PROJECTIONS)}, lambda: (
        require(len(baseline_generations) == 25, "baseline did not activate 25 repositories"), require(baseline_status["staged_generations"] == 0, "baseline retained staged generations"), require(all(projection_states.get(kind) == "ready" for kind in REQUIRED_PROJECTIONS), f"projection states={projection_states}"), {"active_generation": baseline_status["active_catalog_revision"], "repositories": len(baseline_generations), "projection_states": projection_states})[-1]))

    old_handle = ""
    old_negative = ""
    with MCPClient(binary, catalog, data) as client:
        exact = acceptance_case("route_exact", {"path": "OrderController.java", "route": "route:exact:selected"}, lambda: query_case(client, {"text": "OrderController", "repo_id": "orders-service", "limit": 5}, "found", "OrderController.java", "route:exact:selected"))
        cases.append(exact)
        old_handle = exact.get("actual", {}).get("result_handle", "")
        cases.append(acceptance_case("route_lexical", {"path": "README.md", "route": "route:lexical:selected"}, lambda: query_case(client, {"text": "commerce platform", "intent": "text_search", "repo_id": "shared-contracts", "limit": 5}, "found", "README.md", "route:lexical:selected")))
        cases.append(acceptance_case("route_graph", {"route": "route:graph:selected"}, lambda: query_case(client, {"text": "OrderPublisher", "intent": "relationship_traversal", "repo_id": "orders-service", "limit": 10}, "found", None, "route:graph:selected")))
        if vector_available:
            cases.append(acceptance_case("route_vector", {"route": "route:vector:selected", "runtime": "bundled-nomic"}, lambda: query_case(client, {"text": "customer purchase workflow", "retrieval_mode": "vector", "time_ms": 1000, "limit": 5}, "found", None, "route:vector:selected")))
        else:
            cases.append(acceptance_case("route_vector_unavailable", {"status": "unknown", "diagnostic": "vector_unavailable"}, lambda: unavailable_vector_case(client)))
        negative = acceptance_case("negative_not_found", {"status": "not_found"}, lambda: query_case(client, {"text": "does DefinitelyAbsentFixtureSymbol exist", "intent": "negative_verification", "repo_id": "orders-service", "limit": 5}, "not_found"))
        cases.append(negative)
        old_negative = negative.get("actual", {}).get("negative_evidence", "")
        resolved = client.call("kb.resolve", {"text": "OrderController", "repo_id": "orders-service", "limit": 10})
        slice_envelope = client.call("kb.slice", {"kind": "service_boundary", "entity_handles": entity_handles(resolved)[:1], "repo_id": "orders-service", "max_entities": 4, "max_edges": 4, "max_source_lines": 8})
        cases.append(acceptance_case("architecture_slice", {"bounded": True, "evidence": True}, lambda: (require(slice_envelope["result"].get("status") == "found", "slice was not found"), require(bool(evidence(slice_envelope)), "slice has no evidence"), require(len(slice_envelope["result"].get("entities", [])) <= 4, "slice entity budget exceeded"), require(len(slice_envelope["result"].get("claims", [])) <= 4, "slice edge budget exceeded"), {"status": "found", "evidence_ids": evidence(slice_envelope), "locations": paths(slice_envelope), "bounded": True})[-1]))

    if vector_available:
        _, commands["vector_reset"] = invoke([str(binary), "projections", "reset", "--kind", "vector", "--data-dir", str(data)], source, logs / "vector-reset.log")
        with MCPClient(binary, catalog, data) as client:
            cases.append(acceptance_case("missing_vector_unknown", {"status": "unknown"}, lambda: query_case(client, {"text": "semantically unrelated phrase", "retrieval_mode": "vector", "limit": 5}, "unknown")))
            cases.append(acceptance_case("missing_vector_canonical_fallback", {"status": "found", "path": "OrderController.java"}, lambda: query_case(client, {"text": "OrderController", "repo_id": "orders-service", "limit": 5}, "found", "OrderController.java")))
        _, commands["vector_recovery"] = invoke([str(binary), "projections", "rebuild", "--kind", "vector", "--config", str(catalog), "--data-dir", str(data)], source, logs / "vector-recovery.log")

    advance(estate)
    _, commands["mirror_sync_delta"] = invoke([str(binary), "mirrors", "sync", "--registry", str(registry), "--data-dir", str(data)], source, logs / "mirror-sync-delta.log")
    for repository_id in FEATURE_REPOSITORIES:
        _, commands[f"ingest_delta_{repository_id}"] = invoke([str(binary), "ingest", "--repo", repository_id, "--config", str(catalog), "--registry", str(registry), "--data-dir", str(data)], source, logs / f"ingest-delta-{repository_id}.log")
    delta_status, commands["status_delta"] = invoke([str(binary), "status", "--data-dir", str(data)], source, logs / "status-delta.log")
    delta_generations = {item["repo_id"]: item["git"]["commit"] for item in delta_status["snapshots"]}
    landmark_rebuilds = [item["record_counts"] for item in delta_status["projections"] if item["kind"] == "landmarks" and item.get("record_counts", {}).get("retained", 0) > 0 and item.get("record_counts", {}).get("recomputed", 0) > 0]
    cases.append(acceptance_case("scoped_delta", {"changed": sorted(FEATURE_REPOSITORIES), "unchanged": 21}, lambda: (require(all(delta_generations[repo] == manifest["repositories"][repo]["delta"] for repo in FEATURE_REPOSITORIES), "feature repository delta not active"), require(all(delta_generations[repo] == baseline_generations[repo] for repo in baseline_generations if repo not in FEATURE_REPOSITORIES), "unrelated generation changed"), require(delta_status["cache"].get("invalidations", 0) > 0, "cache invalidation was not recorded"), require(bool(delta_status.get("invalidation_counts")), "source invalidations were not recorded"), require(bool(landmark_rebuilds), "landmark projection did not retain unchanged and recompute changed records"), {"active_generation": delta_status["active_catalog_revision"], "changed_repositories": sorted(FEATURE_REPOSITORIES), "cache_invalidations": delta_status["cache"].get("invalidations", 0), "source_invalidations": delta_status.get("invalidation_counts", {}), "landmark_recomputation": landmark_rebuilds[-1]})[-1]))

    with MCPClient(binary, catalog, data) as client:
        cases.append(acceptance_case("delta_visibility", {"path": "OrderSubmittedListener.java", "old_path_absent": "OrderCreatedListener.java"}, lambda: (lambda current: (require(any(path.endswith("OrderSubmittedListener.java") for path in current["locations"]), f"new listener missing: {current}"), require(not any(path.endswith("OrderCreatedListener.java") for path in current["locations"]), f"deleted listener served: {current}"), current)[-1])(query_case(client, {"text": "OrderSubmittedListener", "repo_id": "gateway-service", "limit": 10}, "found"))))
        cases.append(acceptance_case("stale_result_handle", {"status": "unknown", "diagnostic": "result_handle_stale"}, lambda: (lambda envelope: (require(envelope["result"].get("status") == "unknown", "stale handle did not return unknown"), require("result_handle_stale" in envelope["result"].get("uncertainty", []), "stale-handle diagnostic missing"), {"status": envelope["result"].get("status"), "diagnostics": envelope["result"].get("uncertainty", [])})[-1])(client.call("kb.explain", {"handle": old_handle}))))
        cases.append(acceptance_case("negative_evidence_invalidated", {"old_negative_evidence": "invalidated", "new_state": "not_found"}, lambda: (lambda current: (require(bool(old_negative), "baseline negative evidence missing"), require(current["negative_evidence"] != old_negative, "negative evidence survived dependent generation change"), current)[-1])(query_case(client, {"text": "does DefinitelyAbsentFixtureSymbol exist", "intent": "negative_verification", "repo_id": "orders-service", "limit": 5}, "not_found"))))

    recovery_test = "TestDiscardStagedRepositoryPreservesActiveGeneration|TestProjectionRebuildIsDeterministicAndKeepsPriorBuildOnFailure|TestProjectionValidationRejectsForeignGenerationReferences|TestInvalidActivationPreservesActiveCatalog"
    _, commands["recovery_tests"] = invoke(["go", "test", "./internal/store", "-run", recovery_test, "-count=1"], source, logs / "recovery-tests.log")
    cases.append({"id": "corruption_recovery", "status": "pass", "expected": {"interrupted_build": "last_good", "failed_projection": "last_good", "invalid_activation": "rejected"}, "actual": {"command": commands["recovery_tests"]["command"], "last_known_good_queryable": True}, "diagnostics": []})

    fixture_ready = all(case["status"] == "pass" for case in cases)
    report = {
        "schema_version": SCHEMA_VERSION,
        "fixture_version": FIXTURE_VERSION,
        "timestamp": datetime.now(timezone.utc).isoformat(),
        "engine_revision": subprocess.check_output(["git", "rev-parse", "HEAD"], cwd=source, text=True).strip(),
        "platform": {"system": platform.system(), "architecture": platform.machine(), "vector_runtime": "bundled-nomic" if vector_available else "unavailable_optional"},
        "commands": commands,
        "active_generation": delta_status["active_catalog_revision"],
        "repositories": {repo: {"baseline": baseline_generations[repo], "active": delta_generations[repo]} for repo in sorted(delta_generations)},
        "cases": cases,
        "readiness": {"fixture": {"status": "ready" if fixture_ready else "not_ready", "repository_count": 25}, "production_estate": {"status": "not_assessed", "fixture_results_are_not_production_evidence": True, "required_checks": ["catalog_complete", "mirror_sync", "indexing_coverage", "projection_health", "query_checks", "unsupported_content_review"]}},
        "disposition": "accepted" if fixture_ready else "not_accepted",
    }
    validate_report(report)
    report["semantic_fingerprint"] = semantic_fingerprint(report)
    report_path = output / "acceptance-report.json"
    report_path.write_text(json.dumps(report, indent=2, sort_keys=True) + "\n", encoding="utf-8")
    os.chmod(report_path, 0o600)
    print(json.dumps({"report": str(report_path), "disposition": report["disposition"], "cases": [{"id": case["id"], "status": case["status"], **({"diagnostics": case["diagnostics"]} if case["status"] == "fail" else {})} for case in cases]}, indent=2))
    return 0 if fixture_ready else 1


if __name__ == "__main__":
    try:
        raise SystemExit(main())
    except (HarnessError, OSError, subprocess.TimeoutExpired, ValueError) as error:
        print(f"error: {error}", file=sys.stderr)
        raise SystemExit(1)
