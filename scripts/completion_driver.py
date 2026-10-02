#!/usr/bin/env python3
"""Fail-closed dispatcher for installed-product acceptance scenarios.

Scenario tests live in scripts/completion_scenarios, outside ordinary core
discovery. A missing suite/test, zero assertions, failed assertion, timeout or
wrong native host is failure, never a blanket skip. No caller-supplied shell.
"""
from __future__ import annotations

import argparse
import hashlib
import importlib.util
import json
import os
from pathlib import Path
import platform
import subprocess
import sys
import tempfile
import time
import unittest
from completion_harness import product_dirty

ROOT = Path(__file__).resolve().parents[1]
SPEC_PATH = ROOT / '.codex/completion/scenarios.json'
NATIVE = {'install', 'onboarding', 'upgrade', 'resources', 'cloud', 'scale', 'release'}


def registry(path=SPEC_PATH):
    data = json.loads(Path(path).read_text())
    if data.get('schema_version') != 1 or not isinstance(data.get('gates'), dict):
        raise ValueError('invalid scenario registry')
    expected = {'install', 'onboarding', 'maintenance', 'upgrade', 'privacy',
                'resources', 'retrieval', 'cloud', 'benchmark', 'scale', 'release'}
    if set(data['gates']) != expected:
        raise ValueError('every product gate must have scenarios')
    seen, requirements = set(), set()
    tasks = {t['id']: t for t in json.loads((ROOT / '.codex/completion/tasks.json').read_text())['tasks']}
    for gate, cases in data['gates'].items():
        if not isinstance(cases, list) or not cases:
            raise ValueError('empty gate scenarios')
        for case in cases:
            name = case.get('test', '')
            if not name.startswith('test_') or name in seen or not case.get('requirements'):
                raise ValueError('invalid/duplicate scenario')
            if any(r not in {f'R{i:02}' for i in range(1, 27)} for r in case['requirements']):
                raise ValueError('unknown scenario requirement')
            if not isinstance(case.get('milestones'), list) or not case['milestones']:
                raise ValueError('missing scenario scope')
            if any(m not in tasks or gate not in tasks[m]['product_gates'] for m in case['milestones']):
                raise ValueError('scenario scope must match milestone product gates')
            requirements.update(case['requirements'])
            seen.add(name)
    if requirements != {f'R{i:02}' for i in range(1, 27)}:
        raise ValueError('scenario requirements mapping incomplete')
    return data['gates']


class RecordedResult(unittest.TestResult):
    """Keep machine-readable failure provenance, never source/error contents."""
    def __init__(self):
        super().__init__()
        self.records = []
        self.started = {}

    def startTest(self, test):
        super().startTest(test)
        self.started[test.id()] = time.monotonic()

    def record(self, test, result, error=None):
        row = {'test': getattr(test, '_testMethodName', test.id()), 'result': result,
               'seconds': time.monotonic() - self.started.get(test.id(), time.monotonic()),
               'assertions': getattr(test, 'assertion_count', 0)}
        if error:
            row['error_type'] = error[0].__name__
            row['error_sha256'] = hashlib.sha256(str(error[1]).encode()).hexdigest()
        measurements = getattr(test, 'measurements', [])
        if measurements:
            # Metrics only: no query/source/path/credential values in receipts.
            if not all(isinstance(m, dict) and all(isinstance(k, str) and isinstance(v, (int, float))
                                                  for k, v in m.items()) for m in measurements):
                raise ValueError('scenario measurements must be numeric metrics')
            row['measurements'] = measurements
        self.records.append(row)

    def addSuccess(self, test):
        if getattr(test, 'assertion_count', 0) < 1:
            error = (AssertionError, AssertionError('scenario executed zero recorded assertions'), None)
            self.addFailure(test, error)
            return
        super().addSuccess(test)
        self.record(test, 'pass')

    def addFailure(self, test, error):
        super().addFailure(test, error)
        self.record(test, 'fail', error)

    def addError(self, test, error):
        super().addError(test, error)
        self.record(test, 'error', error)

    def addSkip(self, test, reason):
        # Native omissions and unavailable later features never certify success.
        super().addSkip(test, reason)
        self.record(test, 'blocked')

    def addSubTest(self, test, subtest, error):
        super().addSubTest(test, subtest, error)
        if error:
            self.record(test, 'fail', error)


def native_host():
    return platform.system() == 'Darwin' and platform.machine().lower() in {'arm64', 'aarch64'}


def binary_provenance(commit):
    """Bind the executable under test to clean, committed Go build metadata."""
    binary = Path(os.environ.get('AIOS_COMPLETION_BINARY', ROOT / 'bin/aios')).resolve()
    details = subprocess.run(['go', 'version', '-m', str(binary)], capture_output=True,
                             text=True, timeout=15, check=True).stdout
    settings = dict(line.strip().removeprefix('build\t').split('=', 1)
                    for line in details.splitlines() if line.strip().startswith('build\t') and '=' in line)
    if settings.get('vcs.revision') != commit or settings.get('vcs.modified') != 'false':
        raise ValueError('binary must be built from the clean tested commit')
    return {'sha256': hashlib.sha256(binary.read_bytes()).hexdigest(),
            'source_commit': commit, 'vcs_modified': False}


def validate_report(report):
    if report.get('schema_version') != 1 or not isinstance(report.get('passed'), bool):
        raise ValueError('invalid scenario report')
    if report['passed']:
        if report.get('clean_product_tree') is not True:
            raise ValueError('passing report requires clean unchanged product source')
        if report.get('purpose') != 'driver_fixture':
            binary = report.get('binary', {})
            if binary.get('source_commit') != report.get('tested_commit') or binary.get('vcs_modified') is not False or len(binary.get('sha256', '')) != 64:
                raise ValueError('passing product report requires matching executable provenance')
        if not report.get('scenarios') or report.get('blocked') or report.get('dispatcher_error'):
            raise ValueError('passing report cannot omit/skip scenarios')
        if any(r.get('result') != 'pass' or r.get('assertions', 0) < 1 for r in report['scenarios']):
            raise ValueError('passing report requires actual scenario assertions')
        if report.get('native_required') and (report.get('platform') != 'Darwin' or
                report.get('architecture', '').lower() not in {'arm64', 'aarch64'}):
            raise ValueError('passing native report requires Darwin arm64')


def load_cases():
    path = ROOT / 'scripts/completion_scenarios/product.py'
    spec = importlib.util.spec_from_file_location('completion_product_scenarios', path)
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module.ProductScenarios


def execute(gate, output, milestone=None, case_class=None):
    gates = registry()
    if gate not in gates:
        raise ValueError('unknown product gate')
    cases = gates[gate]
    if milestone:
        queue = json.loads((ROOT / '.codex/completion/tasks.json').read_text())['tasks']
        task = next((t for t in queue if t['id'] == milestone), None)
        if not task or gate not in task['product_gates']:
            raise ValueError('unrelated milestone/gate')
        cases = [c for c in cases if milestone in c['milestones']]
        if not cases:
            raise ValueError('milestone has no executable scenarios')
    output = Path(output)
    output.parent.mkdir(parents=True, exist_ok=True)
    report = {'schema_version': 1, 'gate': gate, 'milestone': milestone,
              'platform': platform.system(), 'architecture': platform.machine(),
              'native_required': gate in NATIVE and milestone is None,
              'timestamp': time.time(), 'passed': False, 'scenarios': []}
    report['purpose'] = 'driver_fixture' if case_class else 'product'
    report['tested_commit'] = subprocess.check_output(['git', 'rev-parse', 'HEAD'], cwd=ROOT, text=True).strip()
    before = report['tested_commit']
    if product_dirty(ROOT):
        report['blocked'] = 'commit product changes before scenario execution'
    elif report['native_required'] and not native_host():
        report['blocked'] = 'actual macOS Apple Silicon required'
    else:
        try:
            if not case_class:
                report['binary'] = binary_provenance(before)
            cls = case_class or load_cases()
            result = RecordedResult()
            suite = unittest.TestSuite()
            for case in cases:
                if not callable(getattr(cls, case['test'], None)):
                    raise ValueError('missing executable scenario: ' + case['test'])
                probe = cls(case['test'])
                probe.completion_milestone = milestone
                suite.addTest(probe)
            suite.run(result)
            report['scenarios'] = result.records
            expected = {c['test'] for c in cases}
            actual = {r['test'] for r in result.records}
            report['passed'] = (result.wasSuccessful() and not result.skipped and
                                expected == actual and len(result.records) == len(cases) and
                                all(r['result'] == 'pass' and r['assertions'] > 0 for r in result.records))
        except (ValueError, OSError, ImportError, subprocess.SubprocessError) as error:
            report['dispatcher_error'] = type(error).__name__
            report['error_sha256'] = hashlib.sha256(str(error).encode()).hexdigest()
    after = subprocess.check_output(['git', 'rev-parse', 'HEAD'], cwd=ROOT, text=True).strip()
    report['clean_product_tree'] = not product_dirty(ROOT) and before == after
    if not report['clean_product_tree']:
        report['passed'] = False
        report['blocked'] = 'product source changed or is dirty'
    # No raw traceback/query/capability/source values enter the retained receipt.
    validate_report(report)
    output.write_text(json.dumps(report, indent=2) + '\n')
    output.chmod(0o600)
    print(f"{gate}: {'PASS' if report['passed'] else 'FAIL'}; report {output}")
    return 0 if report['passed'] else 1


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('gate')
    parser.add_argument('--milestone')
    parser.add_argument('--output', type=Path)
    args = parser.parse_args()
    if args.milestone == '02-product-test-driver':
        if args.gate != 'core':
            raise ValueError('driver bootstrap supports core only')
        # Actual dispatcher/fixture/negative-report tests, not feature success.
        return subprocess.run([sys.executable, '-m', 'unittest', 'discover', '-s',
                               'scripts', '-p', 'test_completion_driver.py'], cwd=ROOT).returncode
    if args.gate in {'core', 'web'}:
        target = 'harness-validate' if args.gate == 'core' else 'harness-validate-web'
        return subprocess.run(['make', target], cwd=ROOT).returncode
    output = args.output or Path(tempfile.mkdtemp(prefix='aios-product-evidence-')) / f'{args.gate}.json'
    return execute(args.gate, output, args.milestone)


if __name__ == '__main__':
    try:
        sys.exit(main())
    except (ValueError, OSError, KeyError, TypeError) as error:
        print('completion driver: ' + str(error), file=sys.stderr)
        sys.exit(2)
