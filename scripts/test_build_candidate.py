import hashlib
import json
from pathlib import Path
import tempfile
import unittest
import zipfile
import build_candidate


class CandidatePackageTests(unittest.TestCase):
    def test_actual_zip_inventory_bytes_and_checksum(self):
        with tempfile.TemporaryDirectory() as temp:
            root = Path(temp)
            stage = root / 'stage'; stage.mkdir()
            (stage / 'bin').mkdir()
            (stage / 'bin/aios').write_bytes(b'fixture executable bytes')
            (stage / 'bin/aios').chmod(0o700)
            for name in ('install.sh', 'uninstall.sh', 'update.sh', 'LICENSES.txt', 'USER-GUIDE.md', 'RELEASE-NOTES.md'):
                (stage / name).write_text('fixture ' + name)
            output = root / 'output'; output.mkdir()
            archive = build_candidate.package(stage, output, '1.0.0-test', 'a' * 40, 'darwin', 'arm64')
            with zipfile.ZipFile(archive) as bundle:
                manifest_name = next(n for n in bundle.namelist() if n.endswith('/manifest.json'))
                manifest = json.loads(bundle.read(manifest_name))
                self.assertEqual(manifest['source_commit'], 'a' * 40)
                self.assertEqual(manifest['knowledge_ir_format'], 'knowledge-ir-v10')
                self.assertEqual(set(manifest['sha256']), set(bundle.namelist()) - {manifest_name})
                for name, digest in manifest['sha256'].items():
                    self.assertEqual(hashlib.sha256(bundle.read(name)).hexdigest(), digest)
                info = bundle.getinfo('aios-1.0.0-test-darwin-arm64/bin/aios')
                self.assertEqual((info.external_attr >> 16) & 0o777, 0o700)
            self.assertEqual(archive.stat().st_mode & 0o777, 0o600)
            self.assertEqual(archive.with_suffix('.zip.sha256').read_text().split()[0], build_candidate.digest_file(archive))
            with self.assertRaisesRegex(ValueError, 'already exists'):
                build_candidate.package(stage, output, '1.0.0-test', 'a' * 40, 'darwin', 'arm64')


if __name__ == '__main__':
    unittest.main()
