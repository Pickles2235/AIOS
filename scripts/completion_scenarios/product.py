"""Live product probes. Never included in ordinary core test discovery.

These scenarios intentionally fail against missing product behaviours. Each
implementation milestone expands its probes with adjacent fault/race tests;
final acceptance also requires installed native/manual evidence and review.
"""
from contextlib import contextmanager
from concurrent.futures import ThreadPoolExecutor
import hashlib
from datetime import datetime
import io
import http.cookiejar
import json
import os
from pathlib import Path
import platform
import plistlib
import selectors
import shutil
import sqlite3
import subprocess
import tempfile
import time
import unittest
import urllib.error
import urllib.parse
import urllib.request
import zipfile
from completion_scenarios.auth_fixture import credential_remote

ROOT = Path(__file__).resolve().parents[2]
SOURCE_ROOT = ROOT
FAULTS = ('stage_binary', 'stage_state', 'migration', 'activation', 'health', 'commit')
PLANTED = ('alice.private@example.invalid', 'sk-test-PLANTED_SECRET_936',
           '/Users/planted-private/source', 'https://planted-private.invalid/repo',
           'PLANTED_QUERY_TEXT_936', 'PLANTED_SOURCE_TEXT_936')

def time_from_json(value):
    return datetime.fromisoformat(value.replace('Z', '+00:00')).timestamp()


def source_fingerprint(root):
    rows = []
    for path in sorted(Path(root).rglob('*')):
        if path.is_symlink():
            rows.append((str(path.relative_to(root)), 'link', os.readlink(path)))
        elif path.is_file():
            rows.append((str(path.relative_to(root)), path.stat().st_mode & 0o777,
                         hashlib.sha256(path.read_bytes()).hexdigest()))
    return hashlib.sha256(json.dumps(rows).encode()).hexdigest()


def fixture(root, name='fixture', symbols=10):
    source = root / name
    source.mkdir()
    (source / 'src').mkdir()
    (source / '.gitignore').write_text('ignored/\n')
    (source / 'src/Worker.java').write_text('class Worker {\n' + ''.join(
        f'  void work{i}() {{}}\n' for i in range(symbols)) + '}\n')
    (source / 'src/client.ts').write_text('export function requestOrder() { return fetch("/api/orders"); }\n')
    (source / 'unsupported.rs').write_text('fn unsupported() {}\n')
    (source / 'README.md').write_text('# Generic fixture\n')
    env = dict(os.environ, GIT_CONFIG_NOSYSTEM='1', GIT_CONFIG_GLOBAL=os.devnull,
               GIT_AUTHOR_NAME='Fixture', GIT_AUTHOR_EMAIL='fixture@example.invalid',
               GIT_COMMITTER_NAME='Fixture', GIT_COMMITTER_EMAIL='fixture@example.invalid',
               GIT_AUTHOR_DATE='2024-01-01T00:00:00+0000',
               GIT_COMMITTER_DATE='2024-01-01T00:00:00+0000')
    for args in (('init', '-q', '-b', 'main', str(source)),
                 ('-C', str(source), 'add', '.'),
                 ('-C', str(source), 'commit', '-qm', 'generic fixture')):
        subprocess.run(['git', *args], env=env, check=True, capture_output=True)
    return source


def startup_url(child, timeout=15):
    """Bounded byte reads: a partial stderr line must not defeat the deadline."""
    output = b''
    with selectors.DefaultSelector() as selector:
        selector.register(child.stderr, selectors.EVENT_READ)
        deadline = time.monotonic() + timeout
        while time.monotonic() < deadline and child.poll() is None:
            if not selector.select(timeout=min(.1, max(0, deadline - time.monotonic()))):
                continue
            chunk = os.read(child.stderr.fileno(), 4096)
            if not chunk:
                break
            output += chunk
            if len(output) > 65536:
                break
            for line in output.split(b'\n')[:-1]:
                if line.startswith(b'Open local UI: '):
                    return line.split(b'Open local UI: ', 1)[1].decode().strip()
    raise AssertionError('bounded loopback backend startup')


class ProductScenarios(unittest.TestCase):
    def setUp(self):
        self.assertion_count = 0
        self.measurements = []
        self.temporary = tempfile.TemporaryDirectory(prefix='aios-product-case-')
        self.addCleanup(self.temporary.cleanup)
        self.root = Path(self.temporary.name).resolve()
        self.binary = Path(os.environ.get('AIOS_COMPLETION_BINARY', SOURCE_ROOT / 'bin/aios')).resolve()

    def check(self, value, code):
        self.assertion_count += 1
        if not value:
            raise AssertionError(code)

    def cli(self, *args, expected=0, binary=None, env=None, timeout=90):
        run = subprocess.run([str(binary or self.binary), *map(str, args)],
                             env=env, capture_output=True, text=True, timeout=timeout)
        self.check(run.returncode == expected, 'CLI exit contract: ' + str(args[0]))
        try:
            return json.loads(run.stdout)
        except json.JSONDecodeError:
            self.check(False, 'CLI must return structured JSON')

    @contextmanager
    def server(self, data=None, env=None):
        data = data or self.root / 'data'
        child = subprocess.Popen([str(self.binary), 'ui', 'serve', '--data-dir', str(data)],
                                 stderr=subprocess.PIPE, stdout=subprocess.DEVNULL,
                                 stdin=subprocess.DEVNULL, env=env)
        try:
            url = startup_url(child)
            self.check(url is not None, 'loopback backend startup')
            parsed = urllib.parse.urlparse(url)
            self.check(parsed.hostname in ('127.0.0.1', 'localhost'), 'loopback URL')
            origin = f'{parsed.scheme}://{parsed.netloc}'
            jar = http.cookiejar.CookieJar()
            opener = urllib.request.build_opener(urllib.request.ProxyHandler({}),
                                                urllib.request.HTTPCookieProcessor(jar))
            csrf = ''

            def request(path, body=None, expected=200, authenticated=True):
                headers = {'Origin': origin}
                if authenticated and csrf:
                    headers['X-CSRF-Token'] = csrf
                payload = None
                if body is not None:
                    payload = json.dumps(body).encode()
                    headers['Content-Type'] = 'application/json'
                try:
                    response = opener.open(urllib.request.Request(origin + path, data=payload,
                                                                  headers=headers), timeout=10)
                except urllib.error.HTTPError as error:
                    response = error
                self.check(response.code == expected, 'HTTP status contract ' + path)
                raw = response.read(1 << 20)
                self.check(len(raw) < 1 << 20, 'bounded response')
                try:
                    return json.loads(raw)
                except json.JSONDecodeError:
                    return raw

            csrf = request('/api/v1/session', {'token': parsed.fragment.removeprefix('token=')},
                           authenticated=False)['csrf_token']
            self.check(all(c._rest.get('HttpOnly') is None and 'HttpOnly' in c._rest for c in jar),
                       'HttpOnly session cookie')
            yield request, data, origin
        finally:
            child.terminate()
            try:
                child.wait(timeout=10)
            except subprocess.TimeoutExpired:
                child.kill()
                child.wait()
            child.stderr.close()

    def native(self):
        self.check(platform.system() == 'Darwin' and platform.machine() == 'arm64',
                   'real Darwin arm64 required')

    def package(self):
        value = os.environ.get('AIOS_COMPLETION_PACKAGE')
        self.check(bool(value), 'checksummed candidate package required')
        path = Path(value).resolve()
        self.check(path.is_file(), 'candidate package exists')
        self.check(zipfile.is_zipfile(path), 'installable candidate is ZIP')
        return path

    def install(self, package, root):
        # Register before mutation so partial installation also gets removed.
        label = 'dev.aios.completion.' + root.name + '.' + hashlib.sha256(str(root).encode()).hexdigest()[:12]
        self.addCleanup(self.cli, 'uninstall', '--root', root, '--service-label', label,
                        '--delete-data', '--json')
        return self.cli('install', '--package', package, '--root', root,
                        '--service-label', label, '--json')

    def configure(self, api, source, mode='local', rules=None):
        body = {'mode': mode}
        if rules is not None:
            body['rules'] = rules
        if mode == 'local':
            body['local_repositories'] = [{'id': 'fixture', 'path': str(source)}]
        else:
            body['repositories'] = [{'id': 'fixture', 'url': str(source), 'ref': 'refs/heads/main'}]
        api('/api/v1/onboarding/configure', body)
        api('/api/v1/onboarding/start', {}, expected=202)
        deadline = time.monotonic() + 30
        while time.monotonic() < deadline:
            state = api('/api/v1/onboarding')
            if state['state'] in ('ready', 'failed', 'interrupted'):
                self.check(state['state'] == 'ready', 'fixture indexing completes')
                return state
            time.sleep(.1)
        self.check(False, 'bounded indexing completion')

    def test_bundled_assets(self):
        with self.server() as (api, data, origin):
            page = api('/')
            self.check(isinstance(page, bytes) and b'<script' in page, 'embedded production UI')
            import re
            asset = re.search(rb'src="([^"]+\.js)"', page)
            self.check(asset is not None, 'production JS asset referenced')
            self.check(len(api(asset[1].decode())) > 1000, 'embedded production JS served')
            runtime = api('/api/v1/runtime')
            self.check('compiler_coverage' in runtime and 'bundled_assets' in runtime,
                       'honest optional compiler and bundled runtime status')

    def test_daemon_lifecycle_plan(self):
        plan = self.cli('daemon', 'plan', '--data-dir', self.root / 'data', '--json')
        self.check(plan['scope'] == 'user' and plan['run_at_load'], 'per-user login lifecycle')
        plist = plistlib.loads(plan['plist'].encode())
        self.check(plist['RunAtLoad'] and plist['KeepAlive'], 'headless restart/login plist')
        self.check(plist['ProgramArguments'][0] == str(self.binary), 'actual binary path')
        self.check('UserName' not in plist, 'no root LaunchDaemon requirement')

    def test_native_install_headless(self):
        self.native()
        package = self.package()
        installed = self.install(package, self.root / 'install')
        self.check(installed['scope'] == 'user', 'native per-user installation')
        candidate = Path(installed['binary'])
        self.check(candidate.is_file(), 'installed binary exists')
        state = self.cli('daemon', 'start', '--root', self.root / 'install', '--json', binary=candidate)
        try:
            self.check(state['running'] and state['browser_required'] is False, 'actual headless service')
            old = state['instance_id']
            self.cli('daemon', 'stop', '--root', self.root / 'install', '--json', binary=candidate)
            state = self.cli('daemon', 'start', '--root', self.root / 'install', '--json', binary=candidate)
            self.check(state['instance_id'] == old, 'native restart stable identity')
        finally:
            self.cli('uninstall', '--root', self.root / 'install', '--delete-data', '--json', binary=candidate)

    def test_onboarding_exclusive_mode(self):
        source = fixture(self.root)
        before = source_fingerprint(source)
        with self.server() as (api, data, origin):
            api('/api/v1/onboarding/configure', {'mode': 'local',
                'local_repositories': [{'id': 'fixture', 'path': str(source)}],
                'repositories': [{'id': 'other', 'url': str(source), 'ref': 'refs/heads/main'}]}, expected=400)
            self.check(api('/api/v1/onboarding')['state'] == 'unconfigured', 'mixed modes reject without mutation')
        self.check(source_fingerprint(source) == before, 'onboarding source nonmutation')

    def test_branding_namespace_scope(self):
        source = fixture(self.root)
        with self.server() as (api, data, origin):
            first = api('/api/v1/instance')
            brand = api('/api/v1/instance', {'name': 'Generic fixture', 'seed_colour': '#168aad'})
            self.check(brand['id'] == first['id'], 'branding preserves identity')
            logo = api('/api/v1/instance/logo/generate', {'seed': 'generic'})
            self.check(logo['mime'] == 'image/png', 'local logo generation')
            scope = api('/api/v1/onboarding/preview', {'mode': 'local', 'paths': [str(source)]})
            self.check(scope['repositories'][0]['languages'] and scope['exclusions'] is not None,
                       'batch language/exclusion scope preview')
            result = api('/api/v1/namespace', {'namespace': 'generic-fixture'})
            self.check(result['namespace'] == 'generic-fixture' and result['persisted'], 'chosen namespace persists')
            self.check(result['local_only'] and result['port'], 'honest namespace port contract')
            with self.server(self.root / 'second-data') as (other, _, _):
                collision = other('/api/v1/namespace', {'namespace':'generic-fixture'}, expected=409)
                self.check(collision['requires_selection'] and collision['suggestions'], 'human collision alternatives')
                self.check(other('/api/v1/namespace')['namespace'] == '', 'collision saved no automatic rename')
                choice = collision['suggestions'][0]
                self.check(other('/api/v1/namespace', {'namespace':choice})['namespace'] == choice,
                           'explicit alternative selected')
            invalid = api('/api/v1/onboarding/preview', {'mode':'local','paths':[str(source),str(source/'absent')]})
            self.check(not invalid['valid'] and invalid['repositories'][0]['valid'] and
                       not invalid['repositories'][1]['valid'], 'actual partial batch preview')
            self.check(api('/api/v1/onboarding')['state'] == 'unconfigured', 'preview never saves approval')
            scoped = api('/api/v1/onboarding/preview', {'mode':'local','local_repositories':[{'id':'fixture','path':str(source)}],
                         'rules':{'fixture':{'exclude':['src/client.ts']}}})
            self.check(scoped['valid'] and scoped['repositories'][0]['exclusions']['catalog_pattern'] == 1,
                       'actual include/exclude scope')

    def test_build_staging_activity(self):
        source = fixture(self.root, symbols=500)
        with self.server() as (api, data, origin):
            self.check(api('/api/v1/status')['repositories'] == [], 'empty knowledge before build')
            state = self.configure(api, source)
            events = api('/api/v1/activity')['events']
            self.check(any(e['stage'] == 'discovered' for e in events), 'actual discovered event')
            self.check(any(e['stage'] == 'staged' for e in events), 'actual staged event')
            self.check(any(e['stage'] == 'activated' for e in events), 'atomic promotion event')
            self.check(all(e['queryable'] == (e['stage'] == 'activated') for e in events), 'staged events never claim queryability')
            status = api('/api/v1/status')
            self.check(status['active_catalog_revision'] and status['repositories'][0]['id'] == 'fixture', 'active catalog after promotion')

    def test_git_helper_failure_keeps_mode_and_last_good(self):
        source = fixture(self.root)
        before = source_fingerprint(source)
        with credential_remote(self.root/'external-machine-profile', source) as remote:
            env = dict(os.environ, **remote['environment'])
            with self.server(env=env) as (api, _, _):
                body = {'mode':'mirror','repositories':[{'id':'fixture','url':remote['url'],'ref':'refs/heads/main'}]}
                self.check(api('/api/v1/onboarding/preview',body)['valid'], 'actual TLS authenticated preview')
                self.check(remote['marker'].is_file() and remote['successful'].is_set(), 'external helper actually supplied auth')
                api('/api/v1/onboarding/configure',body)
                api('/api/v1/onboarding/start',{},expected=202)
                def wait(expected):
                    deadline = time.monotonic()+60
                    while time.monotonic()<deadline:
                        state=api('/api/v1/onboarding')
                        if state['state'] == expected: return state
                        if state['state'] in ('failed','ready'): break
                        time.sleep(.05)
                    self.check(False,'bounded authenticated build outcome')
                wait('ready')
                active = api('/api/v1/status')['active_catalog_revision']
                remote['accept'].clear()
                api('/api/v1/onboarding/start',{},expected=202)
                failed = wait('failed')
                self.check(failed['mode'] == 'mirror' and 'Direct' in failed['error'], 'auth remediation preserves chosen mode')
                self.check(remote['url'] not in failed['error'] and 'fixture-only-password' not in json.dumps(failed), 'sanitized auth diagnostics')
                self.check(api('/api/v1/status')['active_catalog_revision'] == active, 'auth failure retains last good catalog')
                self.check(api('/api/v1/query',{'repository':'fixture','text':'Worker'})['status'] == 'found', 'last good evidence remains queryable')
        self.check(source_fingerprint(source) == before, 'authentication never modifies source')

    def test_native_git_namespace(self):
        self.native()
        from verify_onboarding_native import run
        report = run(self.root/'native-onboarding.json',binary=self.binary,package=self.package())
        self.check(report['passed'] and len(report['checks']) >= 9, 'actual installed login helper and named-origin outcomes')

    def test_dirty_untracked_capture(self):
        source = fixture(self.root)
        (source / 'src/Worker.java').write_text('class DirtyWorker {}\n')
        (source / 'src/New.java').write_text('class NewWorker {}\n')
        (source / 'ignored').mkdir()
        (source / 'ignored/Secret.java').write_text('class IgnoreWorker {}\n')
        (source / 'excluded.dat').write_bytes(bytes(2 << 20))
        (source / 'excluded.link').symlink_to(self.root / 'never-open')
        before = source_fingerprint(source)
        with self.server() as (api, data, origin):
            self.configure(api, source, rules={'fixture':{'exclude':['excluded.*']}})
            found = api('/api/v1/query', {'repository': 'fixture', 'text': 'DirtyWorker'})
            self.check(found['status'] == 'found', 'tracked dirty evidence captured')
            excerpt=api('/api/v1/evidence',{'evidence':found['entities'][0]['evidence'],'before':0,'after':0,'max_lines':10})
            self.check(excerpt['working_tree'] and 'DirtyWorker' in '\n'.join(excerpt['lines']),
                       'canonical dirty evidence discloses working-tree provenance')
            found = api('/api/v1/query', {'repository': 'fixture', 'text': 'NewWorker'})
            self.check(found['status'] == 'found', 'eligible untracked evidence captured')
            found = api('/api/v1/query', {'repository': 'fixture', 'text': 'IgnoreWorker'})
            self.check(found['status'] != 'found', 'ignored untracked excluded')
            self.wait_job(api, 'fixture', lambda j: bool(j['active_generation']))
            self.check(api('/api/v1/repositories')['repositories'][0]['provenance'] == 'working_tree',
                       'working-tree provenance disclosed')
        self.check(source_fingerprint(source) == before, 'dirty capture never writes source')

    def wait_job(self, api, repository, predicate, timeout=30):
        deadline = time.monotonic() + timeout
        while time.monotonic() < deadline:
            for job in api('/api/v1/jobs')['jobs']:
                if job['repository'] == repository and predicate(job):
                    return job
            time.sleep(.1)
        self.check(False, 'bounded actual maintained repository outcome')

    def test_durable_scheduler_recovery(self):
        source = fixture(self.root)
        with self.server() as (api, data, origin):
            self.configure(api, source)
            first = self.wait_job(api, 'fixture', lambda j: j['state'] == 'idle' and bool(j['active_generation']))
            identity = api('/api/v1/instance')['id']
            jobs = api('/api/v1/jobs')
            self.check(jobs['mirror_interval_seconds'] == 900, '15-minute default scheduling')
            self.check(jobs['durable'] and jobs['retry_max_attempts'] > 0, 'durable bounded retry policy')
            api('/api/v1/repositories/check-now', {'repository': 'fixture'}, expected=202)
            self.wait_job(api, 'fixture', lambda j: j['state'] == 'idle' and j['runs'] > first['runs'])
            self.check(api('/api/v1/jobs')['coalescing'] and jobs['queue_capacity'] == 100
                       and jobs['worker_capacity'] == 1, 'bounded coalescing policy')
            initial_catalog = api('/api/v1/status')['active_catalog_revision']
            (source / 'src/Worker.java').write_text('class WatchedWorker {}\n')
            before = source_fingerprint(source)
            watched = self.wait_job(api, 'fixture', lambda j: j['active_generation'] != first['active_generation'] and j['state'] == 'idle')
            self.check(watched['reason'] == 'workspace_edit' and watched['changed_files'] == 1,
                       'actual OS watch activates one-file delta')
            self.check(api('/api/v1/status')['active_catalog_revision'] != initial_catalog,
                       'watched canonical generation changes')
            self.check(api('/api/v1/query', {'repository': 'fixture', 'text': 'WatchedWorker'})['status'] == 'found',
                       'real changed evidence queryable')
            self.check(source_fingerprint(source) == before, 'watch indexing never writes source')
        # No watcher is running for this edit. Actual process restart must repair it.
        (source / 'src/Worker.java').write_text('class RestartWorker {}\n')
        before = source_fingerprint(source)
        with self.server(data) as (api, data, origin):
            self.check(api('/api/v1/jobs')['restored'], 'scheduler state restored after real restart')
            repaired = self.wait_job(api, 'fixture', lambda j: j['active_generation'] != watched['active_generation'] and j['state'] == 'idle')
            self.check(repaired['reason'] == 'restart_reconcile' and repaired['changed_files'] == 1,
                       'real restart repairs missed edits with actual delta')
            self.check(api('/api/v1/instance')['id'] == identity, 'restart preserves identity')
            self.check(api('/api/v1/query', {'repository': 'fixture', 'text': 'RestartWorker'})['status'] == 'found',
                       'restart repair produces actual searchable evidence')
        self.check(source_fingerprint(source) == before, 'restart indexing never writes source')

    def test_direct_continuous_edits_and_projection_repair(self):
        source=fixture(self.root)
        with self.server() as (api,data,origin):
            self.configure(api,source)
            first=self.wait_job(api,'fixture',lambda j:j['state']=='idle' and bool(j['active_generation']))
            api('/api/v1/jobs/configure',expected=405)
            api('/api/v1/repositories/check-now',{'repository':'fixture'},expected=403,authenticated=False)
            api('/api/v1/repositories/check-now',{'repository':'unapproved'},expected=409)
            wall_begin=time.time();begin=time.monotonic();observed=False;iteration=0
            # Continuously edit faster than the production one-second quiet period.
            while time.monotonic()-begin<6.5:
                (source/'src/Worker.java').write_text(f'class ContinuousWorker{iteration} {{}}\n')
                iteration+=1
                job=next(j for j in api('/api/v1/jobs')['jobs'] if j['repository']=='fixture')
                if job['runs']>first['runs']:
                    if not observed:
                        self.check(time_from_json(job['last_attempt'])-wall_begin<=6,
                                   'actual continuous edits have bounded five-second maximum debounce')
                    observed=True
                time.sleep(.15)
            self.check(observed,'actual capture starts under continuous edits')
            (source/'src/Worker.java').write_text('class StableWatchedWorker {}\n')
            before=source_fingerprint(source)
            deadline=time.monotonic()+30
            while time.monotonic()<deadline:
                found=api('/api/v1/query',{'repository':'fixture','text':'StableWatchedWorker'})
                if found['status']=='found':break
                time.sleep(.1)
            self.check(found['status']=='found','continuous edits settle into actual stable searchable evidence')
            stable=self.wait_job(api,'fixture',lambda j:j['state']=='idle')
            self.check(source_fingerprint(source)==before,'continuous capture does not modify source')
            catalog=api('/api/v1/status')['active_catalog_revision']
            # Inject a real owned projection transaction failure, not a response mock.
            with sqlite3.connect(data/'index.db') as db:
                db.execute("CREATE TRIGGER fixture_projection_fault BEFORE INSERT ON projection_builds WHEN NEW.projection_kind='graph' BEGIN SELECT RAISE(ABORT,'fixture projection fault'); END")
            try:
                (source/'src/Worker.java').write_text('class RepairedProjectionWorker {}\n')
                before=source_fingerprint(source)
                failed=self.wait_job(api,'fixture',lambda j:j['state']=='retry_wait')
                self.check(failed['active_generation']==stable['active_generation'] and failed['stale'],
                           'failed staged projection retains last-good maintained generation')
                self.check(api('/api/v1/status')['active_catalog_revision']==catalog,
                           'projection fault rolls back complete catalog activation')
                self.check(api('/api/v1/query',{'repository':'fixture','text':'StableWatchedWorker'})['status']=='found',
                           'concurrent reads serve prior knowledge during failed repair')
                self.check(api('/api/v1/query',{'repository':'fixture','text':'RepairedProjectionWorker'})['status']!='found',
                           'staged projection never leaks into queries')
            finally:
                with sqlite3.connect(data/'index.db') as db:db.execute('DROP TRIGGER fixture_projection_fault')
            api('/api/v1/repositories/check-now',{'repository':'fixture'},expected=202)
            repaired=self.wait_job(api,'fixture',lambda j:j['state']=='idle' and j['active_generation']!=stable['active_generation'])
            self.check(not repaired.get('error') and repaired['changed_files']==1,
                       'actual failed revision repairs with one-file delta')
            self.check(api('/api/v1/query',{'repository':'fixture','text':'RepairedProjectionWorker'})['status']=='found',
                       'repair promotes actual new evidence')
            self.check(source_fingerprint(source)==before,'projection failure and repair never write source')

    def test_mirror_retry_isolation_coalescing(self):
        bad, good = fixture(self.root, 'offline'), fixture(self.root, 'healthy')
        # Enough actual input to observe requests arriving during an update.
        for i in range(600):
            (good / f'facts-{i:04}.txt').write_text(f'fixture knowledge {i}\n')
        git_env = dict(os.environ, GIT_CONFIG_GLOBAL=os.devnull, GIT_CONFIG_NOSYSTEM='1',
                       GIT_AUTHOR_NAME='Fixture', GIT_AUTHOR_EMAIL='fixture@example.invalid',
                       GIT_COMMITTER_NAME='Fixture', GIT_COMMITTER_EMAIL='fixture@example.invalid')
        def commit(source):
            for args in (('add', '.'), ('commit', '-qm', 'updated fixture')):
                subprocess.run(['git', '-C', str(source), *args], env=git_env, check=True,
                               capture_output=True, timeout=30)
        commit(good)
        before_bad = source_fingerprint(bad)
        with self.server(env=git_env) as (api, data, origin):
            api('/api/v1/onboarding/configure', {'mode':'mirror', 'repositories':[
                {'id':'offline','url':str(bad),'ref':'refs/heads/main'},
                {'id':'healthy','url':str(good),'ref':'refs/heads/main'}]})
            api('/api/v1/onboarding/start', {}, expected=202)
            baseline = self.wait_job(api, 'offline', lambda j: j['state'] == 'idle' and bool(j['active_generation']))
            healthy = self.wait_job(api, 'healthy', lambda j: j['state'] == 'idle' and bool(j['active_generation']))
            moved = bad.with_name('offline-unavailable')
            bad.rename(moved)
            try:
                api('/api/v1/repositories/check-now', {'repository':'offline'}, expected=202)
                failed = self.wait_job(api, 'offline', lambda j: j['state'] == 'retry_wait')
                delay = (time_from_json(failed['next_attempt']) - time_from_json(failed['last_attempt']))
                self.check(failed['attempts'] == 1 and delay >= 30 and failed['stale']
                           and failed['active_generation'] == baseline['active_generation'],
                           'offline retry retains last-good revision with actual backoff')
                (good / 'src/Worker.java').write_text('class HealthyUpdatedWorker {}\n')
                commit(good)
                before_good = source_fingerprint(good)
                with ThreadPoolExecutor(max_workers=16) as pool:
                    list(pool.map(lambda _: api('/api/v1/repositories/check-now', {'repository':'healthy'}, expected=202), range(100)))
                updated = self.wait_job(api, 'healthy', lambda j: j['state'] == 'idle' and j['active_generation'] != healthy['active_generation'])
                self.check(updated['coalesced'] > healthy['coalesced'] and updated['requests'] >= healthy['requests']+100
                           and updated['runs'] < healthy['runs']+100, 'actual request burst executes coalesced bounded work')
                self.check(updated['changed_files'] in (0,1), 'unchanged repeated captures do not recompile source')
                self.check(api('/api/v1/query', {'repository':'healthy','text':'HealthyUpdatedWorker'})['status'] == 'found',
                           'offline member does not disable healthy changed evidence')
                self.check(api('/api/v1/query', {'repository':'offline','text':'Worker'})['status'] == 'found',
                           'failed member retains actual queryable last-good evidence')
                second = self.wait_job(api, 'offline', lambda j: j['attempts'] >= 2, timeout=45)
                self.check(second['attempts'] == 2 and second['state'] == 'retry_wait'
                           and time_from_json(second['next_attempt'])-time_from_json(second['last_attempt']) >= 60,
                           'actual second retry doubles backoff')
                exhausted = self.wait_job(api, 'offline', lambda j: j['state'] == 'exhausted', timeout=440)
                self.check(exhausted['attempts'] == 5 and exhausted['stale']
                           and exhausted['active_generation'] == baseline['active_generation'],
                           'actual product retry budget exhausts without losing known-good knowledge')
                self.check(source_fingerprint(good) == before_good, 'mirror updates never modify healthy source')
            finally:
                moved.rename(bad)
        self.check(source_fingerprint(bad) == before_bad, 'offline failures never modify source')
        with self.server(data, env=git_env) as (api, data, origin):
            restored = api('/api/v1/jobs')
            failed = next(j for j in restored['jobs'] if j['repository'] == 'offline')
            self.check(restored['restored'] and failed['state'] == 'exhausted' and failed['attempts'] == 5,
                       'real restart preserves exhausted retry budget')
            api('/api/v1/repositories/check-now', {'repository':'offline'}, expected=202)
            self.wait_job(api, 'offline', lambda j: j['state'] == 'idle' and not j.get('error'))
            self.check(api('/api/v1/query', {'repository':'offline','text':'Worker'})['status'] == 'found',
                       'explicit retry repairs restored source')

    def test_healthy_bootstrap_with_unavailable_member(self):
        bad,good=fixture(self.root,'bad-bootstrap'),fixture(self.root,'good-bootstrap')
        before_good=source_fingerprint(good);before_bad=source_fingerprint(bad)
        with self.server() as (api,data,origin):
            api('/api/v1/onboarding/configure',{'mode':'mirror','repositories':[
                {'id':'bad','url':str(bad),'ref':'refs/heads/main'},
                {'id':'good','url':str(good),'ref':'refs/heads/main'}]})
            moved=bad.with_name('unavailable-bootstrap');bad.rename(moved)
            try:
                api('/api/v1/onboarding/start',{},expected=202)
                healthy=self.wait_job(api,'good',lambda j:j['state']=='idle' and bool(j['active_generation']))
                failed=self.wait_job(api,'bad',lambda j:j['state']=='retry_wait')
                self.check(healthy['last_success'] and not failed['active_generation'],
                           'initial source failure does not invent evidence or disable healthy bootstrap')
                self.check(api('/api/v1/query',{'repository':'good','text':'Worker'})['status']=='found',
                           'actual healthy partial bootstrap is searchable')
                self.check(api('/api/v1/query',{'repository':'bad','text':'Worker'})['status']=='unknown',
                           'never indexed unavailable member remains unknown')
                self.check(api('/api/v1/onboarding')['state']=='maintaining',
                           'live entry flow exposes healthy knowledge while failed sources retry')
            finally:moved.rename(bad)
        self.check(source_fingerprint(good)==before_good and source_fingerprint(bad)==before_bad,
                   'partial bootstrap and failed source operations do not mutate sources')

    def test_remove_purge_rebuild(self):
        for mode in ('local', 'mirror'):
            source = fixture(self.root, 'removed-' + mode)
            survivor = fixture(self.root, 'survivor-' + mode)
            added = fixture(self.root, 'added-' + mode)
            before = [source_fingerprint(p) for p in (source, survivor, added)]
            data = self.root / ('management-' + mode)
            def member(id, path):
                return {'id': id, 'path': str(path)} if mode == 'local' else {
                    'id': id, 'url': str(path), 'ref': 'refs/heads/main'}
            def wait_build(api):
                deadline = time.monotonic() + 30
                while time.monotonic() < deadline:
                    state = api('/api/v1/onboarding')
                    if state['state'] in ('ready', 'failed', 'interrupted'):
                        self.check(state['state'] == 'ready', 'actual rebuild validates before promotion')
                        return
                    time.sleep(.05)
                self.check(False, 'bounded rebuild completion')
            with self.server(data) as (api, _, origin):
                key = 'local_repositories' if mode == 'local' else 'repositories'
                api('/api/v1/onboarding/configure', {'mode': mode, key: [
                    member('removed', source), member('survivor', survivor)]})
                api('/api/v1/onboarding/start', {}, expected=202)
                wait_build(api)
                self.wait_job(api, 'survivor', lambda j: j['state'] == 'idle')
                identity = api('/api/v1/instance')['id']
                first = {r['id']: r['generation'] for r in api('/api/v1/status')['repositories']}
                old_handle = api('/api/v1/query', {'repository': 'removed', 'text': 'Worker'})['entities'][0]['handle']
                api('/api/v1/repositories/rebuild', {'repository': 'removed'}, expected=202)
                wait_build(api)
                second = {r['id']: r['generation'] for r in api('/api/v1/status')['repositories']}
                self.check(first['removed'] != second['removed'] and first['survivor'] == second['survivor'],
                           'force recompiles unchanged selected bytes; survivor generation stays intact')
                api('/api/v1/repositories/rebuild', {}, expected=202)
                # Queries during replacement must find complete last-good or new evidence.
                with ThreadPoolExecutor(max_workers=4) as pool:
                    reads = list(pool.map(lambda _: api('/api/v1/query', {'repository': 'survivor', 'text': 'Worker'}), range(12)))
                self.check(all(r['status'] == 'found' for r in reads), 'concurrent reads remain coherent under whole KB rebuild')
                wait_build(api)
                third = {r['id']: r['generation'] for r in api('/api/v1/status')['repositories']}
                self.check(all(third[id] != second[id] for id in second), 'whole KB freshly compiles every approved input')
                self.check(api('/api/v1/instance')['id'] == identity, 'rebuild preserves identity')
                api('/api/v1/repositories/add', {
                    'local_repository' if mode == 'local' else 'mirror_repository': member('added', added),
                    'rules': {'exclude': ['src/client.ts']}}, expected=202)
                self.wait_job(api, 'added', lambda j: j['state'] == 'idle' and bool(j['active_generation']))
                self.check(api('/api/v1/query', {'repository': 'added', 'text': 'Worker'})['status'] == 'found',
                           'actual add compiles independent member')
                self.check(api('/api/v1/query', {'repository': 'added', 'text': 'requestOrder'})['status'] != 'found',
                           'add obeys approved excludes')
                old_added = next(r['generation'] for r in api('/api/v1/status')['repositories'] if r['id'] == 'added')
                api('/api/v1/repositories/rules', {'repository': 'added', 'rules': {'exclude': ['src/Worker.java']}}, expected=202)
                self.wait_job(api, 'added', lambda j: j['state'] == 'idle' and j['active_generation'] != old_added)
                self.check(api('/api/v1/query', {'repository': 'added', 'text': 'requestOrder'})['status'] == 'found',
                           'scope edits compile newly included files')
                api('/api/v1/repositories/retry', {'repository': 'added'}, expected=202)
                api('/api/v1/onboarding/configure', {'mode': mode, key: [member('survivor', survivor)]}, expected=409)
                # A real projection fault must roll back the entire replacement.
                self.wait_job(api, 'added', lambda j: j['state'] == 'idle')
                db = sqlite3.connect(data / 'index.db')
                try:
                    db.execute("CREATE TRIGGER management_projection_fault BEFORE INSERT ON projection_lookup_records BEGIN SELECT RAISE(ABORT,'injected projection failure'); END")
                    db.commit()
                    before_fault = api('/api/v1/status')['active_catalog_revision']
                    api('/api/v1/repositories/rebuild', {}, expected=202)
                    deadline = time.monotonic() + 30
                    while time.monotonic() < deadline and api('/api/v1/onboarding')['state'] == 'ingesting':
                        time.sleep(.05)
                    self.check(api('/api/v1/onboarding')['state'] in ('failed', 'maintaining'), 'actual projection failure reported')
                    self.check(api('/api/v1/status')['active_catalog_revision'] == before_fault,
                               'failed replacement never changes active catalog')
                    self.check(api('/api/v1/query', {'repository': 'survivor', 'text': 'Worker'})['status'] == 'found',
                               'failed rebuild keeps last-good evidence')
                finally:
                    db.execute('DROP TRIGGER management_projection_fault'); db.commit(); db.close()
                # Plant a valid owned shared backup to verify history bytes are discarded.
                backup = data / 'index.ir-9-backup-management.db'
                shutil.copyfile(data / 'index.db', backup); backup.chmod(0o600)
                api('/api/v1/repositories/rebuild', {'repository': 'removed'}, expected=202)
                api('/api/v1/repositories/remove', {'repository': 'removed'})
                self.check(not backup.exists(), 'shared source-bearing migration backup purged')
                self.check(all(r['id'] != 'removed' for r in api('/api/v1/status')['repositories']),
                           'remove withdraws repository while replacement is in flight')
                api('/api/v1/entity', {'handle': old_handle}, expected=409)
                self.check(api('/api/v1/repositories/purge-status?repository=removed')['owned_records'] == 0,
                           'full owned canonical/staged/history/snapshot/mirror purge verified')
                api('/api/v1/repositories/retry', {'repository': 'removed'}, expected=404)
                self.check(api('/api/v1/query', {'repository': 'survivor', 'text': 'Worker'})['status'] == 'found',
                           'survivor still searchable after purge')
            with self.server(data) as (api, _, origin):
                self.check(api('/api/v1/instance')['id'] == identity, 'management restart retains identity')
                self.check(all(j['repository'] != 'removed' for j in api('/api/v1/jobs')['jobs']),
                           'durable restart cannot resurrect removed job')
                self.check(api('/api/v1/repositories/purge-status?repository=removed')['owned_records'] == 0,
                           'restart retains completed purge')
                # Unsafe owned link leaves a durable withdrawal that actual next-start recovers.
                link = data / 'snapshots' / 'survivor' / 'foreign-link'
                link.symlink_to(survivor, target_is_directory=True)
                api('/api/v1/repositories/remove', {'repository': 'survivor'}, expected=503)
                self.check(api('/api/v1/onboarding')['pending_removal'] == 'survivor', 'failed physical purge keeps durable intent')
                api('/api/v1/query', {'repository': 'survivor', 'text': 'Worker'}, expected=503)
                link.unlink()
            with self.server(data) as (api, _, origin):
                self.check(not api('/api/v1/onboarding').get('pending_removal'), 'next process startup finishes actual removal recovery')
                self.check(api('/api/v1/repositories/purge-status?repository=survivor')['owned_records'] == 0,
                           'recovery purges retired history and owned assets')
                api('/api/v1/repositories/remove', {'repository': 'added'})
                self.check(api('/api/v1/status')['repositories'] == [], 'last repository removal publishes truthful empty knowledge')
                self.check(api('/api/v1/jobs')['jobs'] == [], 'last repository removal purges persisted job history')
            self.check([source_fingerprint(p) for p in (source, survivor, added)] == before,
                       'add, scope, retry, rebuild, failure, in-flight removal and crash recovery never write sources')

    def test_native_wake_reconciliation(self):
        self.native()
        result = self.cli('daemon', 'wake-smoke', '--data-dir', self.root / 'data', '--json')
        self.check(result['native_wake_observed'] and result['reconciled'], 'actual native wake observation')

    def test_upgrade_compatibility(self):
        package = self.package()
        plan = self.cli('upgrade', 'inspect', '--package', package, '--json')
        self.check(plan['disk_schema'] and plan['compatible_from'], 'declared compatible upgrade path')
        self.check(plan['downloads'] is False and plan['preserve_identity'], 'explicit local update preserves state')

    def test_upgrade_fault_rollback(self):
        package = self.package()
        for boundary in FAULTS:
            with self.subTest(boundary=boundary):
                install = self.root / boundary
                self.install(package, install)
                old = source_fingerprint(install)
                # Only completion-test builds enable these injected crash points.
                env = dict(os.environ, AIOS_COMPLETION_FAULT=boundary)
                self.cli('upgrade', 'apply', '--package', package, '--root', install, '--json',
                         env=env, expected=1)
                self.cli('upgrade', 'recover', '--root', install, '--json')
                self.check(source_fingerprint(install) == old, 'matching binary/state rollback at ' + boundary)
        self.check(False, 'PENDING task07: distinct prior package, nonempty IR, killed-process crash recovery and installed binary/state parity')

    def test_uninstall_preserve_delete(self):
        package = self.package()
        for preserve in (True, False):
            install = self.root / str(preserve)
            result = self.install(package, install)
            data = Path(result['data_dir'])
            self.cli('uninstall', '--root', install, '--preserve-data' if preserve else '--delete-data', '--json')
            self.check(data.exists() == preserve, 'explicit owned-data uninstall choice')

    def test_native_supported_upgrade(self):
        self.native()
        prior = os.environ.get('AIOS_COMPLETION_PRIOR_PACKAGE')
        self.check(bool(prior) and Path(prior).is_file(), 'actual preserved compatible prior candidate required')
        self.check(hashlib.sha256(Path(prior).read_bytes()).digest() != hashlib.sha256(self.package().read_bytes()).digest(),
                   'distinct prior and current packages required')
        result = self.install(prior, self.root / 'install')
        old = self.cli('daemon', 'status', '--root', self.root / 'install', '--json')
        self.check(bool(old['active_generations']), 'preserved nonempty prior IR required')
        update = self.cli('upgrade', 'apply', '--package', self.package(), '--root', self.root / 'install', '--json')
        self.check(update['instance_id'] == old['instance_id'] and update['active_generations'] == old['active_generations'],
                   'real supported binary/state upgrade retains identity/IR')

    def test_redaction_before_persistence(self):
        source = fixture(self.root)
        (source / 'src/Worker.java').write_text('class Worker { /* ' + ' '.join(PLANTED) + ' */ }\n')
        with self.server() as (api, data, origin):
            self.configure(api, source)
            api('/api/v1/query', {'text': PLANTED[-2]})
            api('/api/v1/onboarding/preview', {'mode': 'mirror', 'urls': [PLANTED[3]]}, expected=400)
            status = api('/api/v1/diagnostics/status')
            self.check(status['otel'] and status['local_only'] and status['correlation'], 'local correlated OTEL')
            files = list((data / 'diagnostics').rglob('*'))
            self.check(any(p.is_file() for p in files), 'real persisted diagnostics exist')
            for path in files:
                if path.is_file():
                    self.check(not any(v.encode() in path.read_bytes() for v in PLANTED), 'redaction before persistence')
        self.check(False, 'PENDING task08: enumerate every telemetry sink and exercise job/error/upgrade/export channels with outbound deny probe')

    def test_diagnostic_archive_policy(self):
        source = fixture(self.root)
        (source / 'src/Worker.java').write_text('class Worker { /* ' + ' '.join(PLANTED) + ' */ }\n')
        with self.server() as (api, data, origin):
            self.configure(api, source)
            api('/api/v1/query', {'text': PLANTED[-2]})
            api('/api/v1/diagnostics/export', {'expanded': True}, expected=400)
            archive = api('/api/v1/diagnostics/export', {'expanded': False})
            for expanded, archive_bytes in ((False, archive), (True, api('/api/v1/diagnostics/export',
                    {'expanded': True, 'acknowledge_warning': True}))):
                self.check(isinstance(archive_bytes, bytes) and 0 < len(archive_bytes) <= 1 << 20,
                           'actual bounded diagnostic archive bytes')
                with zipfile.ZipFile(io.BytesIO(archive_bytes)) as bundle:
                    self.check(bool(bundle.namelist()), 'nonempty diagnostic archive')
                    self.check(sum(i.file_size for i in bundle.infolist()) <= 4 << 20, 'bounded uncompressed archive')
                    for info in bundle.infolist():
                        self.check(not any(v.encode() in bundle.read(info) for v in PLANTED),
                                   'default and warned archives redact actual planted bytes')
                        if not expanded:
                            self.check(not any(part in info.filename.lower() for part in ('source', 'query', 'snapshot')),
                                       'default archive omits source/query artifacts')

    def test_resource_policy_fairness(self):
        with self.server() as (api, data, origin):
            policy = api('/api/v1/resources')
            self.check(policy['state'] in ('normal', 'constrained', 'idle_opportunity'), 'observable resource policy')
            self.check(policy['max_workers'] > 0 and policy['max_queue'] > 0 and policy['retention_bytes'] > 0,
                       'finite worker/queue/disk budgets')
            self.check(policy['oldest_job_max_wait_seconds'] > 0 and policy['cancel_supported'],
                       'fair eventual progress and cancellation')
        self.check(False, 'PENDING task09: injected normal/constrained/idle transitions, queue bounds, starvation, cancel and disk-full outcomes')

    def test_native_power_load(self):
        self.native()
        policy = self.cli('resources', 'status', '--json')
        self.check(policy['signal_provider'] == 'darwin' and policy['power_source'] in ('battery', 'ac'),
                   'actual native power signals')
        self.check(policy['load'] >= 0 and policy['observed_at'], 'actual native load observation')

    def test_investigation_history_copy(self):
        source = fixture(self.root)
        with self.server() as (api, data, origin):
            self.configure(api, source)
            query = api('/api/v1/investigation', {'text': '@fixture Worker'})
            self.check(query['schema_version'] == 1 and query['findings'] and query['canonical_evidence'],
                       'versioned evidence-backed investigation')
            self.check(query['generations'] and query['coverage'] and 'budget' in query,
                       'copy payload generation/coverage/budget metadata')
            history = api('/api/v1/history')
            self.check(history['entries'] and len(history['entries']) <= history['max_entries'], 'bounded local history')
            api('/api/v1/history/clear', {})
            self.check(api('/api/v1/history')['entries'] == [], 'explicit clear history')

    def test_reviewed_gold_corpus(self):
        corpus = SOURCE_ROOT / 'acceptance/completion/gold.json'
        self.check(corpus.is_file(), 'independently reviewed 150-case gold corpus required')
        data = json.loads(corpus.read_text())
        self.check(len(data['cases']) >= 150 and data['independent_review'], 'reviewed gold corpus minimum')
        result = self.cli('benchmark', '--fixture', corpus, '--data-dir', self.root / 'gold')
        self.check(len(result['results']) == len(data['cases']), 'every reviewed gold question executed')
        self.check(all(r['state_correct'] and r['canonical']['provenance_correct'] for r in result['results']),
                   'reviewed evidence/state correctness')

    def browser(self, name):
        run = subprocess.run(['npm', 'exec', '--', 'playwright', 'test',
                              '--config', 'playwright.completion.config.ts', '--grep', name,
                              '--reporter=json'], cwd=SOURCE_ROOT / 'web',
                             text=True, capture_output=True, timeout=180)
        self.check(run.returncode == 0, 'live authenticated browser scenario ' + name)
        report = json.loads(run.stdout)
        self.check(report['stats']['expected'] > 0 and report['stats']['unexpected'] == 0 and
                   report['stats']['skipped'] == 0, 'browser executed nonempty unskipped scenario suite')

    def test_volumetric_browser(self):
        self.browser('volumetric truth and semantic zoom')

    def test_contextual_modules_browser(self):
        self.browser('contextual modules and reconnect')

    def test_native_gpu_evidence(self):
        self.native()
        self.browser('native GPU dense cloud evidence')

    def test_benchmark_isolation_parity(self):
        with self.server() as (api, data, origin):
            before = api('/api/v1/status')
            run = api('/api/v1/benchmarks/run', {'mode': 'demo'})
            self.check(run['isolated'] and run['source_snapshot_sha256'] == run['baseline_snapshot_sha256'],
                       'Demo isolation and immutable source parity')
            self.check(api('/api/v1/status') == before, 'Demo never contaminates user knowledge')
            self.check(run['baseline_invocation'] and run['indexing_cost'] and run['temperature'] in ('cold', 'warm'),
                       'transparent baseline/index/temperature costs')

    def test_benchmark_losses_unknown(self):
        with self.server() as (api, data, origin):
            run = api('/api/v1/benchmarks/run', {'mode': 'demo'})
            rows = run['results']
            self.check(any(r['grep_wins'] for r in rows), 'demonstrated literal baseline win retained')
            self.check(any(r['kb_state'] == 'unknown' and not r['kb_correct'] for r in rows),
                       'unknown not scored as correct')
            self.check(any(not r['kb_correct'] for r in rows), 'incorrect KB outcomes retained')
            self.check(all('source_bytes_read' in r and 'context_bytes' in r and 'estimated_tokens' in r for r in rows),
                       'per-query machine-readable source/context/token metrics')

    def test_mixed_dense_fixtures(self):
        # Actual indexing/query measurements, never native frame-rate proof.
        for count in (1, 25, 100):
            roots = [fixture(self.root, f'repo-{count}-{i}', symbols=500) for i in range(count)]
            catalog = self.root / f'catalog-{count}.json'
            catalog.write_text(json.dumps({'version': 1, 'sources': [
                {'kind': 'repository', 'id': p.name} for p in roots]}))
            result = self.cli('catalog', 'validate', '--config', catalog)
            self.check(result['valid'], 'actual bounded mixed fixture catalog validates')
            self.check(sum(len((p / 'src/Worker.java').read_text().splitlines()) for p in roots) >= count * 500,
                       'dense symbols, not empty repositories')
            before = {p.name: source_fingerprint(p) for p in roots}
            registry = self.root / f'local-{count}.json'
            registry.write_text(json.dumps({'version': 1, 'repositories': [
                {'id': p.name, 'path': str(p)} for p in roots]}))
            data = self.root / f'data-{count}'
            started = time.monotonic()
            self.cli('local', 'ingest', '--all', '--config', catalog, '--registry', registry,
                     '--data-dir', data, timeout=1800)
            elapsed = time.monotonic() - started
            status = self.cli('status', '--data-dir', data)
            self.check(len(status['repositories']) == count, 'all dense fixture repositories indexed')
            self.check(all(source_fingerprint(p) == before[p.name] for p in roots),
                       'scale ingestion never mutates sources')
            self.measurements.append({'repositories': count, 'declared_methods': count * 500,
                'index_seconds': elapsed, 'disk_bytes': sum(p.stat().st_size for p in data.rglob('*') if p.is_file())})

    def test_native_scale_soak(self):
        self.native()
        self.check(bool(os.environ.get('AIOS_COMPLETION_REFERENCE_HARDWARE')), 'declared reference hardware required')
        self.browser('native GPU dense cloud evidence')
        result = self.cli('benchmark', 'soak', '--repositories', '1,25,100', '--minutes', '30',
                          '--data-dir', self.root / 'soak', '--json', timeout=2400)
        self.check(result['minutes'] >= 30 and result['restart_cycles'] > 0 and result['maintenance_cycles'] > 0,
                   'real maintenance/query/restart soak')
        self.check(result['peak_memory_bytes'] > 0 and result['disk_bytes'] > 0 and 'target_misses' in result,
                   'measured native envelope and honest misses')

    def test_release_manifest_checksums(self):
        package = self.package()
        with zipfile.ZipFile(package) as archive:
            names = archive.namelist()
            self.check(all(any(n.endswith(suffix) for n in names) for suffix in
                           ('install.sh', 'uninstall.sh', 'update.sh', 'manifest.json', 'LICENSES.txt', 'USER-GUIDE.md')),
                       'complete candidate tooling/licenses/guide')
            manifest = json.loads(archive.read(next(n for n in names if n.endswith('manifest.json'))))
            self.check(all(not n.startswith('/') and '..' not in Path(n).parts for n in names), 'safe package members')
            files = {i.filename for i in archive.infolist() if not i.is_dir()}
            manifest_name = next(n for n in names if n.endswith('manifest.json'))
            self.check(set(manifest['sha256']) == files - {manifest_name}, 'every candidate file checksummed')
            commit = subprocess.check_output(['git', 'rev-parse', 'HEAD'], cwd=SOURCE_ROOT, text=True).strip()
            self.check(manifest['source_commit'] == commit, 'candidate built from tested commit')
            binaries = [n for n in files if n.endswith('/bin/aios')]
            self.check(len(binaries) == 1, 'one installed candidate binary')
            self.check(hashlib.sha256(archive.read(binaries[0])).hexdigest() == hashlib.sha256(self.binary.read_bytes()).hexdigest(),
                       'candidate contains exact gate-tested binary')
            for path, expected in manifest['sha256'].items():
                self.check(hashlib.sha256(archive.read(path)).hexdigest() == expected, 'packaged file checksum')

    def test_native_installed_candidate(self):
        self.native()
        self.test_native_install_headless()
        self.test_native_supported_upgrade()
        self.check(False, 'PENDING task15: open and query the installed artifact and independently check manifest/UI evidence')
