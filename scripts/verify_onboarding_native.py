#!/usr/bin/env python3
"""Actual disposable launchd Git authentication and native namespace outcomes."""
import argparse
import json
import os
from pathlib import Path
import platform
import subprocess
import tempfile
import time

from completion_scenarios.auth_fixture import authenticated_api, credential_remote
from completion_scenarios.product import fixture, source_fingerprint
from verify_daemon_native import cli

ROOT = Path(__file__).resolve().parents[1]


def wait_build(api, expected):
    deadline = time.monotonic() + 60
    while time.monotonic() < deadline:
        state = api('/api/v1/onboarding')
        if state['state'] == expected: return state
        if state['state'] in ('ready', 'failed'): raise AssertionError('native build outcome')
        time.sleep(.05)
    raise AssertionError('bounded native build')


def run(output, binary=None, package=None):
    if platform.system() != 'Darwin' or platform.machine() != 'arm64':
        raise ValueError('actual Darwin arm64 required')
    source_sha = subprocess.check_output(['git','rev-parse','HEAD'], cwd=ROOT, text=True).strip()
    report = {'schema_version':1, 'tested_commit':source_sha, 'platform':'Darwin',
              'architecture':'arm64', 'purpose':'milestone_native_onboarding', 'passed':False}
    output = Path(output); output.parent.mkdir(parents=True, exist_ok=True)
    binary = Path(binary or ROOT/'bin/aios')
    try:
        with tempfile.TemporaryDirectory(prefix='aios-native-onboarding-') as temporary:
            root = Path(temporary).resolve()
            source = fixture(root)
            before = source_fingerprint(source)
            if package is None:
                subprocess.run(['python3','scripts/build_candidate.py','--version','1.0.0-onboarding-smoke',
                                '--output-dir',str(root/'package')], cwd=ROOT, check=True)
                package = next((root/'package').glob('*.zip'))
            install = root/'owned install'
            label = 'dev.aios.completion.auth.'+str(os.getpid())
            with credential_remote(root/'machine-profile',source) as remote:
                original = {k:os.environ.get(k) for k in remote['environment']}
                # The installer captures external static configuration pointers.
                # Start subsequently receives no terminal Git environment override.
                try:
                    os.environ.update(remote['environment'])
                    installed = Path(cli(binary,'install','--package',package,'--root',install,
                                         '--service-label',label)['binary'])
                    for key,value in original.items():
                        if value is None: os.environ.pop(key,None)
                        else: os.environ[key] = value
                    cli(installed,'daemon','start','--root',install)
                    credentials = cli(installed,'daemon','credentials','--root',install)
                    assert credentials['login_context'] and credentials['default_helper_configured']
                    assert not credentials['terminal_prompt_enabled'] and not credentials['product_token_store']
                    api = authenticated_api(cli(installed,'daemon','link','--root',install,'--recovery')['url'])
                    body = {'mode':'mirror','repositories':[{'id':'fixture','url':remote['url'],'ref':'refs/heads/main'}]}
                    preview = api('/api/v1/onboarding/preview',body)
                    assert preview['valid'] and remote['marker'].is_file() and remote['successful'].is_set()
                    api('/api/v1/onboarding/configure',body)
                    api('/api/v1/onboarding/start',{},202); wait_build(api,'ready')
                    active = api('/api/v1/status')['active_catalog_revision']
                    assert api('/api/v1/query',{'text':'Worker','repository':'fixture'})['status'] == 'found'
                    remote['accept'].clear()
                    api('/api/v1/onboarding/start',{},202)
                    failed = wait_build(api,'failed')
                    assert failed['mode'] == 'mirror' and 'Direct' in failed['error']
                    assert 'fixture-only-password' not in json.dumps(failed) and remote['url'] not in failed['error']
                    assert api('/api/v1/status')['active_catalog_revision'] == active
                    assert api('/api/v1/query',{'text':'Worker','repository':'fixture'})['status'] == 'found'
                    name = 'aios-auth-'+str(os.getpid())+'-'+str(time.time_ns())
                    selection = api('/api/v1/namespace',{'namespace':name})
                    assert selection['native_dns_sd'] and selection['local_only']
                    named = authenticated_api(selection['launch_url'])
                    assert named('/api/v1/status')['active_catalog_revision'] == active
                    replacement = named('/api/v1/namespace',{'namespace':name+'-next'})
                    renamed = authenticated_api(replacement['launch_url'])
                    assert renamed('/api/v1/namespace')['namespace'] == name+'-next'
                    renamed('/api/v1/status')
                    recovery = authenticated_api(cli(installed,'daemon','link','--root',install,'--recovery')['url'])
                    assert recovery('/api/v1/namespace')['namespace'] == name+'-next'
                    identity = cli(installed,'daemon','status','--root',install)['instance_id']
                    cli(installed,'daemon','stop','--root',install)
                    assert cli(installed,'daemon','start','--root',install)['instance_id'] == identity
                    restarted = authenticated_api(cli(installed,'daemon','link','--root',install)['url'])
                    assert restarted('/api/v1/namespace')['namespace'] == name+'-next'
                    assert source_fingerprint(source) == before
                    report['passed'] = True
                    report['checks'] = ['actual_launchd_external_helper_tls_auth','noninteractive_login_context',
                        'auth_failure_direct_remediation_mode_unchanged','auth_failure_retains_last_good',
                        'native_named_origin_authenticated','old_name_to_new_name_capability_migration',
                        'localhost_recovery','namespace_identity_restart','source_nonmutation']
                finally:
                    for key,value in original.items():
                        if value is None: os.environ.pop(key,None)
                        else: os.environ[key] = value
                    cli(binary,'uninstall','--root',install,'--service-label',label,'--delete-data')
                    assert not install.exists()
    finally:
        import sys
        if sys.exc_info()[0] is not None: report['passed'] = False
        output.write_text(json.dumps(report,indent=2)+'\n'); output.chmod(0o600)
    return report


if __name__ == '__main__':
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--output',type=Path,required=True)
    args = parser.parse_args()
    run(args.output)
