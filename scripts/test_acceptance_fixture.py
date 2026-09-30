import json
import sys
import tempfile
import unittest
from pathlib import Path

sys.path.insert(0, str(Path(__file__).parent))
from acceptance_fixture import DELTA, FEATURE_REPOSITORIES, REPOSITORY_IDS, advance, create, semantic_digest


class AcceptanceFixtureTest(unittest.TestCase):
    def test_estate_is_complete_and_reproducible(self):
        with tempfile.TemporaryDirectory() as first_dir, tempfile.TemporaryDirectory() as second_dir:
            first, second = Path(first_dir) / "estate", Path(second_dir) / "estate"
            create(first)
            create(second)
            self.assertEqual(25, len(REPOSITORY_IDS))
            self.assertEqual(semantic_digest(first), semantic_digest(second))
            catalog = json.loads((first / "catalog.json").read_text())
            registry = json.loads((first / "registry.json").read_text())
            self.assertEqual(list(REPOSITORY_IDS), [item["id"] for item in registry["repositories"]])
            self.assertEqual(25, len(catalog["sources"]))

    def test_delta_has_required_change_shapes(self):
        self.assertEqual(set(FEATURE_REPOSITORIES), set(DELTA))
        self.assertIsNone(DELTA["shared-contracts"]["contracts/order-created.json"])
        self.assertIn("contracts/order-submitted.json", DELTA["shared-contracts"])
        self.assertIsNone(DELTA["gateway-service"]["src/main/java/example/OrderCreatedListener.java"])
        self.assertIn("src/main/java/example/OrderSubmittedListener.java", DELTA["gateway-service"])
        self.assertIsNone(DELTA["customer-portal"]["src/routes/orders.test.tsx"])

    def test_advance_moves_only_approved_ref(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory) / "estate"
            manifest = create(root)
            advance(root)
            for repository_id, values in manifest["repositories"].items():
                import subprocess
                active = subprocess.check_output(["git", "-C", values["remote"], "rev-parse", "refs/heads/main"], text=True).strip()
                self.assertEqual(values["delta"] if repository_id in FEATURE_REPOSITORIES else values["baseline"], active)


if __name__ == "__main__":
    unittest.main()
