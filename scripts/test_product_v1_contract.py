import re
import unittest
from pathlib import Path


ROOT = Path(__file__).resolve().parents[1]
ALLOWED_IMPLEMENTATION_VERSIONS = ("Nomic v1.5", "knowledge-ir-v9", "knowledge-ir-v10")


class ProductV1ContractTest(unittest.TestCase):
    def test_product_surfaces_do_not_advertise_later_releases(self):
        paths = [ROOT / "README.md", ROOT / "CHANGELOG.md", *sorted((ROOT / "docs").rglob("*.md"))]
        forbidden = re.compile(r"\bV(?:1\.5|[2-9][0-9]*(?:\.[0-9]+)*)\b", re.IGNORECASE)
        violations = []
        for path in paths:
            for number, line in enumerate(path.read_text(encoding="utf-8").splitlines(), 1):
                scrubbed = line
                for allowed in ALLOWED_IMPLEMENTATION_VERSIONS:
                    scrubbed = scrubbed.replace(allowed, "")
                if forbidden.search(scrubbed):
                    violations.append(f"{path.relative_to(ROOT)}:{number}:{line.strip()}")
        self.assertEqual(violations, [])

    def test_public_json_contracts_are_version_one(self):
        acceptance = (ROOT / "scripts/v1_acceptance.py").read_text(encoding="utf-8")
        schema = (ROOT / "docs/reference/acceptance-report.schema.json").read_text(encoding="utf-8")
        self.assertIn("SCHEMA_VERSION = 1", acceptance)
        self.assertIn('\"schema_version\": {\"const\": 1}', schema)
        self.assertNotRegex((ROOT / "internal/mcp/v1.go").read_text(encoding="utf-8"), r"\}\{2, provenanceEnvelope")


if __name__ == "__main__":
    unittest.main()
