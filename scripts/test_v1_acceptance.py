#!/usr/bin/env python3

import copy
import sys
import unittest
from pathlib import Path

sys.path.insert(0, str(Path(__file__).parent))
from v1_acceptance import semantic_fingerprint, validate_report


def report() -> dict:
    return {
        "schema_version": 1,
        "fixture_version": "fixture-v1",
        "timestamp": "2026-01-01T00:00:00+00:00",
        "engine_revision": "a" * 40,
        "platform": {},
        "commands": {"run": {"elapsed_seconds": 1.0}},
        "active_generation": "generation-a",
        "repositories": {f"repo-{index:02d}": {"baseline": "b" * 40, "active": "c" * 40} for index in range(25)},
        "cases": [{"id": "exact", "status": "pass", "expected": {"route": "exact"}, "actual": {"planner_route": ["route:exact:selected"], "locations": ["src/A.java"], "evidence_ids": ["volatile"]}, "diagnostics": []}],
        "readiness": {"fixture": {"status": "ready"}, "production_estate": {"status": "not_assessed"}},
        "disposition": "accepted",
    }


class AcceptanceReportTest(unittest.TestCase):
    def test_fingerprint_ignores_declared_volatile_fields(self) -> None:
        first = report()
        second = copy.deepcopy(first)
        second["timestamp"] = "2026-01-02T00:00:00+00:00"
        second["active_generation"] = "generation-b"
        second["commands"]["run"]["elapsed_seconds"] = 99.0
        second["cases"][0]["actual"]["evidence_ids"] = ["another-volatile-id"]
        self.assertEqual(semantic_fingerprint(first), semantic_fingerprint(second))

    def test_fingerprint_changes_for_semantic_result(self) -> None:
        first = report()
        second = copy.deepcopy(first)
        second["cases"][0]["actual"]["locations"] = ["src/B.java"]
        self.assertNotEqual(semantic_fingerprint(first), semantic_fingerprint(second))

    def test_report_validation_requires_exactly_25_repositories(self) -> None:
        candidate = report()
        validate_report(candidate)
        candidate["repositories"].pop("repo-00")
        with self.assertRaisesRegex(RuntimeError, "25 repositories"):
            validate_report(candidate)


if __name__ == "__main__":
    unittest.main()
