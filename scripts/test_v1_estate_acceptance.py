import copy
import unittest

from scripts.v1_estate_acceptance import GateError, evaluate_report, semantic_fingerprint, validate_corpus


CATEGORIES = ["exact_identifier", "structural_locate", "trace_cause", "cross_repository_impact", "supported_negative"]


def corpus() -> dict:
    repositories = [{"id": f"repo-{index:02d}", "path": f"repo-{index:02d}", "revision": f"{index:040x}"} for index in range(25)]
    cases = []
    for index in range(150):
        query_class = CATEGORIES[index % 5]
        case = {"id": f"case-{index:03d}", "query": f"gold-query-{index:03d}-unique", "query_class": query_class}
        if query_class == "supported_negative":
            case.update({"expected_state": "not_found", "expected_repository": f"repo-{index % 25:02d}"})
        else:
            case.update({"expected_repository": f"repo-{index % 25:02d}", "expected_path": f"src/Case{index}.java"})
        cases.append(case)
    return {"repositories": repositories, "cases": cases}


def report(value: dict) -> dict:
    results = []
    for index, case in enumerate(value["cases"]):
        cross = case["query_class"] == "cross_repository_impact"
        structural = case["query_class"] == "structural_locate"
        retrievers = {}
        for name in ("exact_only", "lexical_only", "structural_only", "graph_only", "hybrid"):
            correct = True
            if cross and name != "hybrid" and index % 5 == 3:
                correct = False
            if structural and name == "structural_only" and index % 50 == 1:
                correct = False
            retrievers[name] = {"correct": correct, "rank": 1 if correct else 0, "precision_at_1": 1 if correct else 0, "latency_ms": 5}
        results.append({
            "id": case["id"], "revision": "a" * 40,
            "canonical": {"correct": True, "provenance_correct": True},
            "retrievers": retrievers, "state_correct": True,
            "result_state": case.get("expected_state", "found"),
            "context_package": {"estimated_tokens": 500},
        })
    return {
        "results": results,
        "quality": {"llm_free_answer_rate": .9, "hybrid_correctness": .9, "grep_baseline_correctness": .8},
        "incremental_indexing": {"changed_file_visibility_ms": 100},
    }


class EstateAcceptanceTest(unittest.TestCase):
    def test_corpus_contract_is_exact_and_balanced(self):
        self.assertEqual(validate_corpus(corpus()), {name: 30 for name in ("exact", "structural", "trace_cause", "cross_repo_impact", "negative_unknown")})
        invalid = corpus()
        invalid["cases"].pop()
        with self.assertRaisesRegex(GateError, "150"):
            validate_corpus(invalid)

    def test_report_thresholds_and_fingerprint_are_deterministic(self):
        value = corpus()
        measured = report(value)
        evaluation, accepted = evaluate_report(value, measured)
        self.assertTrue(accepted, evaluation)
        self.assertEqual(semantic_fingerprint(measured), semantic_fingerprint(copy.deepcopy(measured)))
        measured["quality"]["llm_free_answer_rate"] = .69
        evaluation, accepted = evaluate_report(value, measured)
        self.assertFalse(accepted)
        self.assertFalse(evaluation["checks"]["llm_free"])


if __name__ == "__main__":
    unittest.main()
