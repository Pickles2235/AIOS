#!/usr/bin/env python3
"""Real disposable Apple Silicon launchd/package smoke, never a full final gate."""
import argparse
import json
import os
from pathlib import Path
import platform
import subprocess
import tempfile
import time

ROOT = Path(__file__).resolve().parents[1]


def cli(binary, *args, expected=0):
    result = subprocess.run([str(binary), *map(str, args)], capture_output=True, timeout=60)
    if result.returncode != expected:
        raise AssertionError('native CLI boundary failed: ' + str(args[0]))
    return json.loads(result.stdout) if result.stdout and expected == 0 else None


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--output', type=Path, required=True)
    args = parser.parse_args()
    if platform.system() != 'Darwin' or platform.machine() != 'arm64':
        raise ValueError('actual Darwin arm64 required')
    source = subprocess.check_output(['git', 'rev-parse', 'HEAD'], cwd=ROOT, text=True).strip()
    report = {'schema_version': 1, 'tested_commit': source, 'platform': 'Darwin', 'architecture': 'arm64',
              'purpose': 'milestone_daemon_smoke', 'passed': False}
    args.output.parent.mkdir(parents=True, exist_ok=True)
    try:
        with tempfile.TemporaryDirectory(prefix='aios-native-daemon-') as temporary:
            root = Path(temporary).resolve()
            subprocess.run(['python3', 'scripts/build_candidate.py', '--version', '1.0.0-daemon-smoke',
                            '--output-dir', str(root / 'package')], cwd=ROOT, check=True)
            archive = next((root / 'package').glob('*.zip'))
            binary = ROOT / 'bin/aios'
            label = 'dev.aios.completion.native.' + str(os.getpid())
            install = root / 'install'
            # Cleanup is registered before any native mutation and always uses
            # the same disposable root/label. No default live service is touched.
            try:
                result = cli(binary, 'install', '--package', archive, '--root', install, '--service-label', label, '--json')
                installed = Path(result['binary'])
                state = cli(installed, 'daemon', 'start', '--root', install, '--json')
                assert state['running'] and not state['browser_required'] and state['instance_id']
                identity = state['instance_id']; pid = state['pid']
                cli(installed, 'daemon', 'stop', '--root', root / 'foreign', '--service-label', label, '--json', expected=1)
                assert cli(installed, 'daemon', 'status', '--root', install)['running']
                assert not (root / 'foreign').exists()
                cli(installed, 'daemon', 'stop', '--root', install, '--json')
                deadline = time.monotonic() + 10
                while time.monotonic() < deadline and cli(installed, 'daemon', 'status', '--root', install)['running']:
                    time.sleep(.1)
                assert not cli(installed, 'daemon', 'status', '--root', install)['running']
                state = cli(installed, 'daemon', 'start', '--root', install, '--json')
                assert state['running'] and state['instance_id'] == identity and state['pid'] != pid
                cli(installed, 'daemon', 'open', '--root', install, '--json')
                cli(binary, 'uninstall', '--root', install, '--service-label', label, '--preserve-data', '--json')
                assert (install / 'data/instance.json').is_file() and not installed.exists()
                report['passed'] = True
                report['checks'] = ['checksummed_zip_install', 'native_headless_launchd', 'foreign_root_stop_rejected',
                                    'native_stop_start_identity', 'native_browser_open', 'preserve_data_uninstall']
            finally:
                cli(binary, 'uninstall', '--root', install, '--service-label', label, '--delete-data', '--json')
                assert not install.exists()
    finally:
        # An exception, including cleanup failure, must invalidate the retained
        # artifact even if the earlier lifecycle assertions succeeded.
        import sys
        if sys.exc_info()[0] is not None:
            report['passed'] = False
        args.output.write_text(json.dumps(report, indent=2) + '\n')
        args.output.chmod(0o600)


if __name__ == '__main__':
    main()
