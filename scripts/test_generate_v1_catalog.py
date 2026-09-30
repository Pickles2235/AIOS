import json
import os
import subprocess
import tempfile
import unittest
from pathlib import Path
import sys

sys.path.insert(0, str(Path(__file__).parent))
from generate_v1_catalog import INCLUDE, build_outputs, verify_remotes


def inventory(count: int = 25) -> dict:
    return {
        "version": 1,
        "repositories": [
            {"id": f"repo-{index:02d}", "url": f"file:///approved/repo-{index:02d}.git", "ref": "refs/heads/main"}
            for index in range(count)
        ],
    }


class CatalogGenerationTest(unittest.TestCase):
    def test_approved_inventory_generates_current_v1_contracts(self):
        catalog, registry = build_outputs(inventory())
        self.assertEqual(catalog["version"], 1)
        self.assertEqual(len(catalog["sources"]), 25)
        self.assertEqual(catalog["sources"][0]["kind"], "repository")
        self.assertEqual(registry["repositories"][0]["ref"], "refs/heads/main")
        for extension in ("**/*.js", "**/*.jsx", "**/*.kt", "**/*.kts", "**/*.py"):
            self.assertIn(extension, INCLUDE)
        self.assertNotIn("root", catalog["sources"][0])

    def test_inventory_is_an_explicit_approval_guard(self):
        with self.assertRaisesRegex(ValueError, "expected 25 approved repositories, found 24"):
            build_outputs(inventory(24))
        value = inventory()
        value["repositories"][1]["id"] = value["repositories"][0]["id"]
        with self.assertRaisesRegex(ValueError, "duplicated"):
            build_outputs(value)

    def test_remote_ref_must_exist(self):
        with tempfile.TemporaryDirectory() as directory:
            remote = Path(directory) / "remote.git"
            work = Path(directory) / "work"
            subprocess.run(["git", "init", "--bare", str(remote)], check=True, capture_output=True)
            subprocess.run(["git", "init", "-b", "main", str(work)], check=True, capture_output=True)
            subprocess.run(["git", "-C", str(work), "config", "user.email", "fixture@example.invalid"], check=True)
            subprocess.run(["git", "-C", str(work), "config", "user.name", "Fixture"], check=True)
            (work / "README.md").write_text("fixture\n", encoding="utf-8")
            subprocess.run(["git", "-C", str(work), "add", "README.md"], check=True)
            subprocess.run(["git", "-C", str(work), "commit", "-m", "fixture"], check=True, capture_output=True)
            subprocess.run(["git", "-C", str(work), "push", str(remote), "HEAD:refs/heads/main"], check=True, capture_output=True)
            verify_remotes({"repositories": [{"id": "fixture", "url": str(remote), "ref": "refs/heads/main"}]})
            with self.assertRaisesRegex(ValueError, "unavailable"):
                verify_remotes({"repositories": [{"id": "fixture", "url": str(remote), "ref": "refs/heads/missing"}]})

    def test_cli_writes_owner_only_files_beneath_data_dir(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            source, data = root / "inventory.json", root / "data"
            source.write_text(json.dumps(inventory()), encoding="utf-8")
            completed = subprocess.run(
                [sys.executable, str(Path(__file__).with_name("generate_v1_catalog.py")), "--inventory", str(source), "--data-dir", str(data), "--skip-remote-check"],
                check=True, text=True, capture_output=True,
            )
            result = json.loads(completed.stdout)
            self.assertEqual(Path(result["catalog"]), data / "catalog.json")
            self.assertEqual(os.stat(data).st_mode & 0o777, 0o700)
            self.assertEqual(os.stat(data / "catalog.json").st_mode & 0o777, 0o600)
            self.assertEqual(os.stat(data / "mirrors.json").st_mode & 0o777, 0o600)


if __name__ == "__main__":
    unittest.main()
