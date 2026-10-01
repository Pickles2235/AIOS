#!/usr/bin/env python3
"""Finite, resumable AgentOS product-completion mission and evidence gates."""
import argparse
import hashlib
import json
import os
from pathlib import Path
import platform
import re
import signal
import subprocess
import sys
import time

ROOT = Path(__file__).resolve().parents[1]
BASE = Path('.codex/completion')
SHA = re.compile(r'[0-9a-f]{40}')
STATES = {'pending', 'in_progress', 'blocked', 'ready', 'completed'}


def read(path):
    return json.loads(Path(path).read_text())


def git(root, *args):
    return subprocess.check_output(['git', '-C', str(root), *args], text=True).strip()


def evidence_only(path):
    return (path.startswith(str(BASE / 'evidence') + '/') or
            path.startswith(str(BASE / 'plans') + '/') or path == str(BASE / 'tasks.json'))


def product_dirty(root):
    # NUL-separated names support whitespace and avoid porcelain path parsing.
    tracked = subprocess.check_output(['git', '-C', str(root), 'diff', 'HEAD', '--name-only', '-z'])
    untracked = subprocess.check_output(['git', '-C', str(root), 'ls-files', '--others', '--exclude-standard', '-z'])
    return [p.decode() for p in (tracked + untracked).split(b'\0') if p and not evidence_only(p.decode())]


def safe_file(root, value, prefix=BASE):
    if not isinstance(value, str):
        raise ValueError('path must be text')
    path = Path(value)
    if path.is_absolute() or '..' in path.parts or not path.is_relative_to(prefix):
        raise ValueError('unsafe evidence/plan path')
    resolved = (root / path).resolve()
    if not resolved.is_relative_to(root.resolve()):
        raise ValueError('evidence escapes repository')
    return resolved


def queue(root):
    data = read(root / BASE / 'tasks.json')
    if data.get('schema_version') != 1 or not isinstance(data.get('tasks'), list) or not data['tasks']:
        raise ValueError('invalid completion queue')
    gates = read(root / BASE / 'gates.json')
    if gates.get('schema_version') != 1 or not isinstance(gates.get('gates'), dict):
        raise ValueError('invalid gate registry')
    for name, gate in gates['gates'].items():
        if not isinstance(gate.get('native'), bool) or not gate.get('commands'):
            raise ValueError('invalid gate')
        if not all(isinstance(c, list) and c and all(isinstance(x, str) and x for x in c) for c in gate['commands']):
            raise ValueError('gate commands must be nonempty argv arrays')
    seen, requirements = set(), set()
    for task in data['tasks']:
        tid = task.get('id', '')
        if not re.fullmatch(r'\d{2}-[a-z0-9-]+', tid) or tid in seen or task.get('status') not in STATES:
            raise ValueError('invalid ID/status')
        deps = task.get('dependencies')
        if not isinstance(deps, list) or len(set(deps)) != len(deps) or not set(deps) <= seen:
            raise ValueError('dependencies must precede task')
        for key in ('title',):
            if not isinstance(task.get(key), str) or not task[key].strip():
                raise ValueError('missing title')
        for key in ('acceptance', 'requirements', 'gates', 'product_gates'):
            if not isinstance(task.get(key), list) or not task[key] or not all(isinstance(x, str) and x for x in task[key]):
                raise ValueError('missing acceptance/requirements/gates')
        if not set(task['gates'] + task['product_gates']) <= gates['gates'].keys():
            raise ValueError('unknown task gate')
        if not safe_file(root, task['plan'], BASE / 'plans').is_file():
            raise ValueError('missing plan')
        if task['status'] == 'completed':
            if not SHA.fullmatch(task.get('completion_commit') or '') or not task.get('review'):
                raise ValueError('completed milestone needs commit and independent review')
        elif task.get('completion_commit') is not None:
            raise ValueError('uncompleted milestone cannot claim completion commit')
        requirements.update(task['requirements'])
        seen.add(tid)
    if requirements != {f'R{i:02}' for i in range(1, 27)}:
        raise ValueError('requirements mapping incomplete')
    return data['tasks'], gates['gates']


def ancestor(root, commit, ref):
    return subprocess.run(['git', '-C', str(root), 'merge-base', '--is-ancestor', commit, ref],
                          stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL).returncode == 0


def review(root, path, commit):
    report = read(safe_file(root, path, BASE / 'evidence'))
    if (report.get('schema_version') != 1 or report.get('role') != 'independent_reviewer' or
            not isinstance(report.get('session_id'), str) or not report['session_id'].strip() or
            report.get('reviewed_commit') != commit or report.get('verdict') != 'pass' or
            report.get('blocking_findings') != [] or not report.get('checks_performed')):
        raise ValueError('missing, failing or stale independent review')
    return report


def eligible(root, task, tasks):
    if task['status'] not in {'pending', 'in_progress'}:
        return False
    by_id = {t['id']: t for t in tasks}
    for dep in task['dependencies']:
        prior = by_id[dep]
        commit = prior['completion_commit']
        if prior['status'] != 'completed' or not commit:
            return False
        if not ancestor(root, commit, 'HEAD') or not ancestor(root, commit, 'refs/remotes/origin/main'):
            return False
        review(root, prior['review'], commit)
    return True


def write(path, data):
    path = Path(path)
    path.parent.mkdir(parents=True, exist_ok=True)
    tmp = path.with_name(path.name + '.tmp')
    tmp.write_text(json.dumps(data, indent=2) + '\n')
    os.replace(tmp, path)


def run_child(argv, root, log, stdin=None):
    # Terminate process group on interruption; never orphan an agent/test process.
    child = subprocess.Popen(argv, cwd=root, stdin=subprocess.PIPE if stdin else None,
                             stdout=log, stderr=subprocess.STDOUT, text=True,
                             start_new_session=os.name == 'posix')
    old = {}
    def stop(signum, frame):
        raise KeyboardInterrupt
    try:
        for sig in (signal.SIGTERM, signal.SIGINT):
            old[sig] = signal.signal(sig, stop)
        child.communicate(stdin)
        return child.returncode
    finally:
        if child.poll() is None:
            if os.name == 'posix':
                os.killpg(child.pid, signal.SIGTERM)
            else:
                child.terminate()
            try:
                child.wait(timeout=10)
            except subprocess.TimeoutExpired:
                if os.name == 'posix':
                    os.killpg(child.pid, signal.SIGKILL)
                else:
                    child.kill()
                child.wait()
        for sig, handler in old.items():
            signal.signal(sig, handler)


def gate(root, name, output, milestone=None):
    tasks, gates = queue(root)
    if name not in gates:
        raise ValueError('unknown gate')
    spec = gates[name]
    if milestone:
        task = next((t for t in tasks if t['id'] == milestone), None)
        if not task or name not in task['product_gates']:
            raise ValueError('unknown milestone or unrelated gate')
        # Scoped implementation scenarios; full native proof remains a final gate.
        spec = (gates[name] if milestone == '01-baseline-contract' else
                {'native': False, 'commands': [['make','completion-milestone',
                 'COMPLETION_MILESTONE=' + milestone, 'COMPLETION_GATE=' + name]]})
    native = platform.system() == 'Darwin' and platform.machine().lower() in {'arm64', 'aarch64'}
    if spec['native'] and not native:
        raise ValueError('native gate requires actual macOS Apple Silicon')
    if product_dirty(root):
        raise ValueError('commit product changes before collecting gate evidence')
    before = git(root, 'rev-parse', 'HEAD')
    output = Path(output).resolve()
    results = []
    for i, command in enumerate(spec['commands']):
        log_path = output.with_name(output.stem + f'-{i}.log')
        log_path.parent.mkdir(parents=True, exist_ok=True)
        start = time.time()
        with log_path.open('w') as log:
            code = run_child(command, root, log)
        results.append({'argv': command, 'exit_code': code, 'seconds': time.time()-start,
                        'log_sha256': hashlib.sha256(log_path.read_bytes()).hexdigest()})
        if code:
            break
    passed = (all(x['exit_code'] == 0 for x in results) and len(results) == len(spec['commands']) and
              git(root, 'rev-parse', 'HEAD') == before and not product_dirty(root))
    write(output, {'schema_version': 1, 'gate': name, 'milestone': milestone, 'tested_commit': before,
                   'clean_product_tree': passed, 'platform': platform.system(),
                   'architecture': platform.machine(), 'timestamp': time.time(),
                   'passed': passed, 'commands': results})
    print(f'{name}: {"PASS" if passed else "FAIL"}; receipt {output}')
    return 0 if passed else 1


def accept(root, manifest_path):
    tasks, gates = queue(root)
    manifest = read(safe_file(root, manifest_path, BASE / 'evidence'))
    commit = manifest.get('tested_commit', '')
    if not SHA.fullmatch(commit) or not ancestor(root, commit, 'HEAD') or not ancestor(root, commit, 'refs/remotes/origin/main'):
        raise ValueError('tested commit must be on fetched main and HEAD')
    changes = git(root, 'diff', '--name-only', commit, 'HEAD').splitlines()
    if any(not evidence_only(x) for x in changes) or product_dirty(root):
        raise ValueError('product changed since final evidence')
    for task in tasks:
        sha = task['completion_commit']
        if task['status'] != 'completed' or not sha or not ancestor(root, sha, 'HEAD') or not ancestor(root, sha, 'refs/remotes/origin/main'):
            raise ValueError('unpublished/incomplete milestone: ' + task['id'])
        review(root, task['review'], sha)
    receipts = {}
    for path in manifest.get('receipts', []):
        receipt = read(safe_file(root, path, BASE / 'evidence'))
        name = receipt.get('gate')
        if name not in gates or name in receipts:
            raise ValueError('unknown/duplicate gate receipt')
        commands = receipt.get('commands', [])
        if (receipt.get('schema_version') != 1 or receipt.get('milestone') is not None or receipt.get('tested_commit') != commit or
                receipt.get('passed') is not True or receipt.get('clean_product_tree') is not True or
                [c.get('argv') for c in commands] != gates[name]['commands'] or
                any(c.get('exit_code') != 0 for c in commands)):
            raise ValueError('stale/failing gate: ' + name)
        if receipt.get('platform') != 'Darwin' or receipt.get('architecture', '').lower() not in {'arm64','aarch64'}:
            raise ValueError('missing native proof: ' + name)
        receipts[name] = receipt
    if set(receipts) != set(gates):
        raise ValueError('missing final gates')
    review(root, manifest['review'], commit)
    requirements = read(safe_file(root, manifest['requirements_report'], BASE / 'evidence'))
    for i in range(1, 27):
        item = requirements.get(f'R{i:02}', {})
        if item.get('result') != 'pass' or not item.get('scenarios') or not item.get('evidence'):
            raise ValueError('missing requirement acceptance')
    artifact = manifest['artifact']
    artifact_path = Path(artifact['path'])
    if not artifact_path.is_absolute():
        artifact_path = root / artifact_path
    if hashlib.sha256(artifact_path.read_bytes()).hexdigest() != artifact['sha256']:
        raise ValueError('artifact checksum mismatch')
    if manifest.get('schema_version') != 1 or manifest.get('candidate_ready') is not True or manifest.get('stakeholder_release') != 'pending':
        raise ValueError('invalid candidate/release authority')
    print('Candidate evidence accepted; stakeholder release pending.')
    return 0


def mission(root):
    return ((root / BASE / 'MISSION.md').read_text() + '\n\n'
            'Start with the next eligible milestone and continue until candidate acceptance.\n'
            'Read the entire PRODUCT.md and ACCEPTANCE.md before editing.\n')


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('command', choices=['check','status','next','prompt','run','gate','accept'])
    parser.add_argument('name', nargs='?')
    parser.add_argument('--output')
    parser.add_argument('--milestone', help='Scoped implementation scenarios; never final release proof')
    args = parser.parse_args()
    tasks, _ = queue(ROOT)
    if args.command == 'check':
        print(f'Valid completion harness: {len(tasks)} milestones, 26 requirements.')
        return 0
    if args.command == 'status':
        for t in tasks:
            print(t['id'], t['status'])
        return 0
    if args.command == 'next':
        print(next((t['id'] for t in tasks if eligible(ROOT, t, tasks)), 'No eligible milestone; inspect blockers/evidence.'))
        return 0
    if args.command == 'prompt':
        print(mission(ROOT))
        return 0
    if args.command == 'gate':
        if not args.output:
            raise ValueError('gate needs --output /owned/receipt.json')
        return gate(ROOT, args.name, args.output, args.milestone)
    if args.command == 'accept':
        return accept(ROOT, args.name or str(BASE / 'evidence/release.json'))
    common = Path(git(ROOT, 'rev-parse', '--path-format=absolute', '--git-common-dir'))
    # Share the old launcher's lock so the two workflows cannot mutate concurrently.
    lock = common / 'aios-codex.lock'
    try:
        lock.mkdir(mode=0o700)
    except FileExistsError:
        raise ValueError('another run or stale lock; inspect owner before recovery')
    try:
        write(lock / 'owner.json', {'pid': os.getpid(), 'checkout': str(ROOT), 'mission': 'completion'})
        if git(ROOT, 'status', '--porcelain', '--untracked-files=all', '--ignore-submodules=none'):
            raise ValueError('dirty checkout; preserve existing work before run')
        run_dir = common / 'aios-completion-runs' / str(time.time_ns())
        run_dir.mkdir(parents=True, mode=0o700)
        text = mission(ROOT)
        (run_dir / 'prompt.txt').write_text(text)
        print(f'Completion mission logs: {run_dir}', flush=True)
        with (run_dir / 'agent.log').open('w') as log:
            code = run_child(['codex','exec','--sandbox','workspace-write','-'], ROOT, log, text)
        (run_dir / 'exit-code.txt').write_text(str(code)+'\n')
        # CLI exit 0 is not product acceptance. Require actual final records.
        return code if code else accept(ROOT, str(BASE / 'evidence/release.json'))
    finally:
        for path in lock.iterdir():
            path.unlink()
        lock.rmdir()


if __name__ == '__main__':
    try:
        sys.exit(main())
    except KeyboardInterrupt:
        print('completion: interrupted; work and logs preserved', file=sys.stderr)
        sys.exit(130)
    except (ValueError, OSError, KeyError, TypeError, subprocess.CalledProcessError) as error:
        print(f'completion: {error}', file=sys.stderr)
        sys.exit(2)
