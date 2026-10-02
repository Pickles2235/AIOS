"""Dispatcher/fixture tests only; synthetic cases never certify the product."""
import importlib.util
import json
from pathlib import Path
import subprocess
import tempfile
import sys
import time
import unittest
from unittest.mock import patch

import completion_driver as driver


class DriverTests(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory()
        self.addCleanup(self.tmp.cleanup)
        self.output = Path(self.tmp.name) / 'report.json'
        clean = patch.object(driver, 'product_dirty', return_value=[])
        clean.start()
        self.addCleanup(clean.stop)

    def fake_class(self, outcome='pass'):
        def method(test):
            test.assertion_count = 1
            if outcome == 'zero':
                test.assertion_count = 0
            elif outcome == 'fail':
                test.fail('secret@example.invalid sk-private-planted-value')
            elif outcome == 'skip':
                test.skipTest('missing platform or feature')
            elif outcome == 'subtest':
                with test.subTest(boundary='activation'):
                    test.fail('rollback failure')
        names = [c['test'] for c in driver.registry()['privacy']]
        return type('SyntheticScenarioFixture', (unittest.TestCase,), {name: method for name in names})

    def test_registry_has_executable_requirement_scopes(self):
        gates = driver.registry()
        cls = driver.load_cases()
        self.assertEqual(len(gates), 11)
        requirements = set()
        for cases in gates.values():
            for case in cases:
                self.assertTrue(callable(getattr(cls, case['test'], None)), case)
                requirements.update(case['requirements'])
        self.assertEqual(requirements, {f'R{i:02}' for i in range(1, 27)})

    def test_registry_rejects_missing_empty_duplicate_or_unknown(self):
        original = json.loads(driver.SPEC_PATH.read_text())
        for change in ('missing', 'empty', 'duplicate', 'requirement'):
            data = json.loads(json.dumps(original))
            if change == 'missing':
                del data['gates']['privacy']
            elif change == 'empty':
                data['gates']['privacy'] = []
            elif change == 'duplicate':
                data['gates']['privacy'].append(data['gates']['privacy'][0])
            else:
                data['gates']['privacy'][0]['requirements'] = ['R99']
            path = Path(self.tmp.name) / 'invalid.json'
            path.write_text(json.dumps(data))
            with self.subTest(change=change), self.assertRaises(ValueError):
                driver.registry(path)

    def test_real_nonempty_assertions_required(self):
        self.assertEqual(driver.execute('privacy', self.output, case_class=self.fake_class()), 0)
        report = json.loads(self.output.read_text())
        self.assertTrue(report['passed'])
        self.assertEqual(len(report['scenarios']), 2)
        self.assertTrue(all(r['assertions'] == 1 for r in report['scenarios']))

    def test_failed_skipped_zero_and_subtest_failure_never_pass(self):
        for outcome in ('fail', 'skip', 'zero', 'subtest'):
            with self.subTest(outcome=outcome):
                self.assertEqual(driver.execute('privacy', self.output, case_class=self.fake_class(outcome)), 1)
                report = json.loads(self.output.read_text())
                self.assertFalse(report['passed'])
                self.assertNotIn('secret@example.invalid', self.output.read_text())
                self.assertNotIn('sk-private-planted-value', self.output.read_text())

    def test_native_gate_blocks_before_executing_cases(self):
        with patch.object(driver, 'native_host', return_value=False), patch.object(driver, 'load_cases') as load:
            self.assertEqual(driver.execute('install', self.output), 1)
            load.assert_not_called()
        report = json.loads(self.output.read_text())
        self.assertTrue(report['native_required'])
        self.assertFalse(report['passed'])
        self.assertEqual(report['scenarios'], [])

    def test_missing_scenario_writes_failed_report(self):
        self.assertEqual(driver.execute('privacy', self.output, case_class=unittest.TestCase), 1)
        report = json.loads(self.output.read_text())
        self.assertFalse(report['passed'])
        self.assertEqual(report['dispatcher_error'], 'ValueError')

    def test_scope_excludes_later_native_scenarios_but_never_full_proof(self):
        def method(test):
            test.assertEqual(test.completion_milestone, '03-daemon-install')
            test.assertion_count = 1
        cls = type('PortableInstallFixture', (unittest.TestCase,), {
            'test_bundled_assets': method, 'test_daemon_lifecycle_plan': method})
        with patch.object(driver, 'native_host', return_value=False):
            self.assertEqual(driver.execute('install', self.output, '03-daemon-install', cls), 0)
        report = json.loads(self.output.read_text())
        self.assertEqual(report['milestone'], '03-daemon-install')
        self.assertFalse(report['native_required'])
        self.assertEqual({s['test'] for s in report['scenarios']}, set(cls.__dict__) &
                         {'test_bundled_assets', 'test_daemon_lifecycle_plan'})

    def test_unrelated_milestone_or_unknown_gate_rejected(self):
        with self.assertRaisesRegex(ValueError, 'unrelated'):
            driver.execute('privacy', self.output, '03-daemon-install')
        with self.assertRaisesRegex(ValueError, 'unknown'):
            driver.execute('unregistered', self.output)

    def test_generic_git_fixtures_and_nonmutation_fingerprint(self):
        # load_cases does not register its isolated module; read the same exact file.
        spec = importlib.util.spec_from_file_location('driver_fixture_test',
                    driver.ROOT / 'scripts/completion_scenarios/product.py')
        module = importlib.util.module_from_spec(spec)
        spec.loader.exec_module(module)
        root = Path(self.tmp.name)
        first = module.fixture(root, 'one', 500)
        second = module.fixture(root, 'two', 500)
        rev = lambda p: subprocess.check_output(['git', '-C', str(p), 'rev-parse', 'HEAD'], text=True)
        self.assertEqual(rev(first), rev(second))
        self.assertIn('work499', (first / 'src/Worker.java').read_text())
        self.assertTrue((first / 'src/client.ts').exists())
        self.assertTrue((first / 'unsupported.rs').exists())
        before = module.source_fingerprint(first)
        (first / 'src/New.java').write_text('class New {}\n')
        self.assertNotEqual(module.source_fingerprint(first), before)
        self.assertEqual(len(module.FAULTS), 6)

    def test_report_validation_rejects_false_native_and_empty_success(self):
        valid = {'schema_version': 1, 'passed': True, 'native_required': False,
                 'clean_product_tree': True, 'purpose': 'driver_fixture',
                 'scenarios': [{'test': 'test_fixture', 'result': 'pass', 'assertions': 1}]}
        driver.validate_report(valid)
        for fields in ({'scenarios': []}, {'blocked': 'hardware'}, {'dispatcher_error': 'missing'},
                       {'native_required': True, 'platform': 'Linux', 'architecture': 'x86_64'},
                       {'clean_product_tree': False}, {'purpose': 'product'},
                       {'scenarios': [{'result': 'pass', 'assertions': 0}]}):
            with self.subTest(fields=fields), self.assertRaises(ValueError):
                driver.validate_report(dict(valid, **fields))

    def test_dirty_source_is_blocked_without_running_scenarios(self):
        with patch.object(driver, 'product_dirty', return_value=['code.go']), patch.object(driver, 'load_cases') as load:
            self.assertEqual(driver.execute('privacy', self.output), 1)
            load.assert_not_called()
        report = json.loads(self.output.read_text())
        self.assertFalse(report['clean_product_tree'])
        self.assertFalse(report['passed'])

    def test_unbound_product_binary_cannot_certify_success(self):
        with patch.object(driver, 'binary_provenance', side_effect=ValueError('unrelated binary')), patch.object(driver, 'load_cases') as load:
            self.assertEqual(driver.execute('privacy', self.output), 1)
            load.assert_not_called()
        self.assertFalse(json.loads(self.output.read_text())['passed'])

    def test_partial_startup_line_obeys_deadline(self):
        spec = importlib.util.spec_from_file_location('startup_probe', driver.ROOT / 'scripts/completion_scenarios/product.py')
        module = importlib.util.module_from_spec(spec)
        spec.loader.exec_module(module)
        child = subprocess.Popen([sys.executable, '-c', 'import os,time; os.write(2,b"partial"); time.sleep(10)'], stderr=subprocess.PIPE)
        started = time.monotonic()
        try:
            with self.assertRaisesRegex(AssertionError, 'bounded'):
                module.startup_url(child, timeout=.2)
            self.assertLess(time.monotonic() - started, 2)
        finally:
            child.kill()
            child.wait(timeout=2)
            child.stderr.close()


if __name__ == '__main__':
    unittest.main()
