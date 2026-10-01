"""Live product probes. Never included in ordinary core test discovery.

These scenarios intentionally fail against missing product behaviours. Each
implementation milestone expands its probes with adjacent fault/race tests;
final acceptance also requires installed native/manual evidence and review.
"""
from contextlib import contextmanager
import hashlib
import io
import http.cookiejar
import json
import os
from pathlib import Path
import platform
import plistlib
import selectors
import shutil
import subprocess
import tempfile
import time
import unittest
import urllib.error
import urllib.parse
import urllib.request
import zipfile

ROOT = Path(__file__).resolve().parents[2]
SOURCE_ROOT = ROOT
FAULTS = ('stage_binary', 'stage_state', 'migration', 'activation', 'health', 'commit')
PLANTED = ('alice.private@example.invalid', 'sk-test-PLANTED_SECRET_936',
           '/Users/planted-private/source', 'https://planted-private.invalid/repo',
           'PLANTED_QUERY_TEXT_936', 'PLANTED_SOURCE_TEXT_936')


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
    def server(self, data=None):
        data = data or self.root / 'data'
        child = subprocess.Popen([str(self.binary), 'ui', 'serve', '--data-dir', str(data)],
                                 stderr=subprocess.PIPE, stdout=subprocess.DEVNULL,
                                 stdin=subprocess.DEVNULL)
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

    def configure(self, api, source, mode='local'):
        body = {'mode': mode}
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

    def test_build_staging_activity(self):
        source = fixture(self.root, symbols=500)
        with self.server() as (api, data, origin):
            self.check(api('/api/v1/status')['repositories'] == [], 'empty knowledge before build')
            state = self.configure(api, source)
            events = api('/api/v1/activity')['events']
            self.check(any(e['stage'] == 'discovered' for e in events), 'actual discovered event')
            self.check(any(e['stage'] == 'staged' for e in events), 'actual staged event')
            self.check(any(e['stage'] == 'activated' for e in events), 'atomic promotion event')
            self.check(state['active_catalog']['sources'][0]['id'] == 'fixture', 'active catalog after promotion')

    def test_native_git_namespace(self):
        self.native()
        source = fixture(self.root)
        env = self.cli('daemon', 'credentials', '--data-dir', self.root / 'data', '--json')
        self.check(env['credential_helpers_enabled'] and env['login_context'], 'actual daemon Git auth environment')
        namespace = self.cli('namespace', 'verify', '--name', 'generic-fixture', '--json')
        self.check(namespace['resolved_loopback'] and namespace['http_origin_verified'], 'native addressing reaches secure origin')

    def test_dirty_untracked_capture(self):
        source = fixture(self.root)
        (source / 'src/Worker.java').write_text('class DirtyWorker {}\n')
        (source / 'src/New.java').write_text('class NewWorker {}\n')
        (source / 'ignored').mkdir()
        (source / 'ignored/Secret.java').write_text('class IgnoreWorker {}\n')
        before = source_fingerprint(source)
        with self.server() as (api, data, origin):
            self.configure(api, source)
            found = api('/api/v1/query', {'repository': 'fixture', 'text': 'DirtyWorker'})
            self.check(found['status'] == 'found', 'tracked dirty evidence captured')
            found = api('/api/v1/query', {'repository': 'fixture', 'text': 'NewWorker'})
            self.check(found['status'] == 'found', 'eligible untracked evidence captured')
            found = api('/api/v1/query', {'repository': 'fixture', 'text': 'IgnoreWorker'})
            self.check(found['status'] != 'found', 'ignored untracked excluded')
            self.check(api('/api/v1/repositories')['repositories'][0]['provenance'] == 'working_tree',
                       'working-tree provenance disclosed')
        self.check(source_fingerprint(source) == before, 'dirty capture never writes source')

    def test_durable_scheduler_recovery(self):
        source = fixture(self.root)
        with self.server() as (api, data, origin):
            self.configure(api, source)
            jobs = api('/api/v1/jobs')
            self.check(jobs['mirror_interval_seconds'] == 900, '15-minute default scheduling')
            self.check(jobs['durable'] and jobs['retry_max_attempts'] > 0, 'durable bounded retry policy')
            api('/api/v1/repositories/check-now', {'repository': 'fixture'}, expected=202)
            self.check(api('/api/v1/jobs')['coalescing'], 'jobs coalesce')
        with self.server(data) as (api, data, origin):
            self.check(api('/api/v1/jobs')['restored'], 'scheduler state restored after real restart')
        self.check(False, 'PENDING task05: observe changed generations, retry exhaustion, coalescing and per-repo isolation under injected faults')

    def test_remove_purge_rebuild(self):
        source = fixture(self.root)
        before = source_fingerprint(source)
        with self.server() as (api, data, origin):
            self.configure(api, source)
            identity = api('/api/v1/instance')['id']
            api('/api/v1/repositories/rebuild', {'repository': 'fixture'}, expected=202)
            self.check(api('/api/v1/instance')['id'] == identity, 'rebuild preserves identity')
            api('/api/v1/repositories/remove', {'repository': 'fixture'})
            self.check(api('/api/v1/status')['repositories'] == [], 'removed repository no longer active')
            self.check(api('/api/v1/repositories/purge-status?repository=fixture')['owned_records'] == 0,
                       'full owned knowledge/history/cache/mirror purge')
            api('/api/v1/repositories/retry', {'repository': 'fixture'}, expected=404)
        self.check(source_fingerprint(source) == before, 'purge preserves source bytes and Git state')

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
