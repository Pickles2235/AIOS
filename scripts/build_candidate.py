#!/usr/bin/env python3
"""Build an owned checksummed candidate ZIP; never publish a release."""
import argparse
import hashlib
import json
import os
from pathlib import Path
import platform
import re
import subprocess
import tempfile
import zipfile

ROOT = Path(__file__).resolve().parents[1]


def digest_file(path):
    with path.open('rb') as stream:
        return hashlib.file_digest(stream, 'sha256').hexdigest()


def package(stage, output, version, commit, system, architecture):
    prefix = f'aios-{version}-{system}-{architecture}'
    files = {f'{prefix}/{p.relative_to(stage).as_posix()}': p for p in stage.rglob('*') if p.is_file()}
    manifest = {'schema_version': 1, 'version': version, 'source_commit': commit,
                'platform': system, 'architecture': architecture, 'disk_schema': 1,
                'knowledge_ir_format': 'knowledge-ir-v10',
                'update_protocol': 1, 'compatible_ir_formats': ['knowledge-ir-v10'],
                'compatible_from': [1], 'sha256': {name: digest_file(p)
                                                 for name, p in sorted(files.items())}}
    path = output / (prefix + '.zip')
    if path.exists():
        raise ValueError('candidate output already exists')
    temporary = path.with_suffix('.zip.partial')
    try:
        with zipfile.ZipFile(temporary, 'x', compression=zipfile.ZIP_DEFLATED, compresslevel=6) as archive:
            for name, source in sorted(files.items()):
                archive.write(source, name)
            archive.writestr(prefix + '/manifest.json', json.dumps(manifest, indent=2) + '\n')
        temporary.chmod(0o600)
        os.rename(temporary, path)
    finally:
        temporary.unlink(missing_ok=True)
    with path.open('rb') as data:
        digest = hashlib.file_digest(data, 'sha256').hexdigest()
    checksum = path.with_suffix('.zip.sha256')
    checksum.write_text(f'{digest}  {path.name}\n')
    checksum.chmod(0o600)
    return path


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--version', required=True)
    parser.add_argument('--output-dir', type=Path, required=True)
    parser.add_argument('--developer-smoke', action='store_true')
    args = parser.parse_args()
    if not re.fullmatch(r'1\.[A-Za-z0-9._-]+', args.version):
        raise ValueError('candidate version must be a safe V1 identifier')
    native = platform.system() == 'Darwin' and platform.machine() == 'arm64'
    if not native and not args.developer_smoke:
        raise ValueError('product package requires native Apple Silicon; use --developer-smoke for Linux validation only')
    if subprocess.check_output(['git', 'status', '--porcelain'], cwd=ROOT):
        raise ValueError('build candidate from a completely clean committed checkout')
    commit = subprocess.check_output(['git', 'rev-parse', 'HEAD'], cwd=ROOT, text=True).strip()
    output = args.output_dir.resolve()
    output.mkdir(mode=0o700, parents=True, exist_ok=True)
    output.chmod(0o700)
    subprocess.run(['make', 'web-build'], cwd=ROOT, check=True)
    if subprocess.check_output(['git', 'status', '--porcelain'], cwd=ROOT):
        raise ValueError('production build changed committed assets; commit rebuilt assets first')
    with tempfile.TemporaryDirectory(prefix='aios-candidate-') as temp:
        stage = Path(temp)
        (stage / 'bin').mkdir(mode=0o700)
        binary = stage / 'bin/aios'
        subprocess.run(['go', 'build', '-trimpath', '-o', str(binary), './cmd/aios'], cwd=ROOT,
                       env=dict(os.environ, CGO_ENABLED='1'), check=True)
        binary.chmod(0o700)
        for action in ('install', 'uninstall', 'update'):
            script = stage / (action + '.sh')
            if action == 'install':
                command = 'install'
            elif action == 'uninstall':
                command = 'uninstall'
            else:
                command = 'upgrade apply'
            script.write_text('#!/bin/sh\nset -eu\n' +
                'candidate_dir=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd -P)\n' +
                f'exec "$candidate_dir/bin/aios" {command} "$@"\n')
            script.chmod(0o700)
        for name in ('USER-GUIDE.md', 'RELEASE-NOTES.md'):
            (stage / name).write_bytes((ROOT / 'docs' / name).read_bytes())
        # Engineering inventory only; complete transitive/native helper notices
        # are a required final-candidate obligation, not certified by this builder.
        licenses = sorted((ROOT / 'cmd/aios/notices').glob('*')) + sorted((ROOT / 'internal/semantic/assets').glob('LICENSE*'))
        (stage / 'LICENSES.txt').write_text('Engineering candidate: complete transitive dependency and llama.cpp helper attribution pending final acceptance. Nomic model uses Apache-2.0.\n\n' + '\n\n'.join(p.name + '\n' + p.read_text() for p in licenses if p.is_file()))
        system = 'darwin' if native else platform.system().lower()
        arch = 'arm64' if native else 'amd64' if platform.machine() == 'x86_64' else platform.machine()
        print(package(stage, output, args.version, commit, system, arch))


if __name__ == '__main__':
    main()
