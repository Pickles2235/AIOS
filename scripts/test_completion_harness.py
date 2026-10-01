"""Offline harness regression tests. No model calls or product-verification claims."""
import copy
import hashlib
import importlib.util
import json
from pathlib import Path
import shutil
import subprocess
import tempfile
import unittest
from unittest.mock import patch

ROOT = Path(__file__).resolve().parents[1]
spec = importlib.util.spec_from_file_location('completion', ROOT / 'scripts/completion_harness.py')
h = importlib.util.module_from_spec(spec)
spec.loader.exec_module(h)


class HarnessTests(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory()
        self.addCleanup(self.tmp.cleanup)
        self.root = Path(self.tmp.name)
        shutil.copytree(ROOT / '.codex/completion', self.root / '.codex/completion')
        self.git('init','-b','main')
        self.git('config','user.name','Harness Test')
        self.git('config','user.email','test@example.invalid')
        self.git('add','.')
        self.git('commit','-m','baseline')
        self.sha = self.git('rev-parse','HEAD')
        self.git('update-ref','refs/remotes/origin/main',self.sha)

    def git(self,*args):
        return subprocess.check_output(['git','-C',str(self.root),*args],text=True,stderr=subprocess.DEVNULL).strip()

    def save_queue(self,tasks):
        h.write(self.root / h.BASE / 'tasks.json',{'schema_version':1,'tasks':tasks})

    def report(self,path,commit=None):
        h.write(self.root/path,dict(schema_version=1,role='independent_reviewer',session_id='separate-test-session',
                 reviewed_commit=commit or self.sha,verdict='pass',blocking_findings=[],checks_performed=['fixture review']))

    def manifest(self):
        tasks,gates=h.queue(self.root)
        report='.codex/completion/evidence/review.json'
        self.report(report)
        for t in tasks:
            t.update(status='completed',completion_commit=self.sha,review=report)
        self.save_queue(tasks)
        paths=[]
        for name,g in gates.items():
            p=f'.codex/completion/evidence/gate-{name}.json';paths.append(p)
            h.write(self.root/p,dict(schema_version=1,gate=name,milestone=None,tested_commit=self.sha,passed=True,
                    clean_product_tree=True,platform='Darwin',architecture='arm64',
                    commands=[dict(argv=c,exit_code=0) for c in g['commands']]))
        req='.codex/completion/evidence/requirements.json'
        h.write(self.root/req,{f'R{i:02}':dict(result='pass',scenarios=['fixture'],evidence=['fixture']) for i in range(1,27)})
        artifact=self.root/'candidate.zip';artifact.write_bytes(b'fixture artifact')
        # Artifact is owned external output, not an uncommitted product file.
        artifact.rename(self.root/h.BASE/'evidence/candidate.zip')
        artifact=self.root/h.BASE/'evidence/candidate.zip'
        m=dict(schema_version=1,tested_commit=self.sha,artifact=dict(path=str(artifact),sha256=hashlib.sha256(artifact.read_bytes()).hexdigest()),
               receipts=paths,review=report,requirements_report=req,candidate_ready=True,stakeholder_release='pending')
        h.write(self.root/h.BASE/'evidence/release.json',m)
        return m

    def test_queue_maps_all_requirements(self):
        tasks,gates=h.queue(self.root)
        self.assertEqual(len(tasks),15)
        self.assertEqual(len(gates),13)
        self.assertTrue(h.eligible(self.root,tasks[0],tasks))
        self.assertFalse(h.eligible(self.root,tasks[1],tasks))

    def test_bad_dependency_or_missing_requirement_rejected(self):
        tasks,_=h.queue(self.root)
        for mutate in [lambda q:q[0].update(dependencies=[q[1]['id']]),lambda q:q[0].update(requirements=['R15']),
                       lambda q:q[0].update(product_gates=['fake']),lambda q:q[0].update(plan='../outside')]:
            q=copy.deepcopy(tasks);mutate(q);self.save_queue(q)
            with self.assertRaises(ValueError):h.queue(self.root)

    def test_completed_requires_review_and_commit(self):
        tasks,_=h.queue(self.root);tasks[0].update(status='completed')
        self.save_queue(tasks)
        with self.assertRaises(ValueError):h.queue(self.root)

    def test_dependency_requires_publication_and_review(self):
        tasks,_=h.queue(self.root)
        p='.codex/completion/evidence/review.json';self.report(p)
        tasks[0].update(status='completed',completion_commit=self.sha,review=p)
        self.assertTrue(h.eligible(self.root,tasks[1],tasks))
        self.git('update-ref','-d','refs/remotes/origin/main')
        self.assertFalse(h.eligible(self.root,tasks[1],tasks))

    def test_safe_paths_and_symlink_escape(self):
        for p in ['/tmp/a','../a','.codex/completion/../outside']:
            with self.assertRaises(ValueError):h.safe_file(self.root,p)
        (self.root/h.BASE/'escape').symlink_to(self.root.parent)
        with self.assertRaises(ValueError):h.safe_file(self.root,str(h.BASE/'escape/a'))

    def test_native_gate_refuses_linux(self):
        with patch.object(h.platform,'system',return_value='Linux'),self.assertRaisesRegex(ValueError,'actual macOS'):
            h.gate(self.root,'install',self.root/h.BASE/'evidence/install.json')

    def test_gate_failure_receipt_and_scoped_commands(self):
        output=self.root/h.BASE/'evidence/scoped.json'
        with patch.object(h,'run_child',return_value=7) as run:
            self.assertEqual(h.gate(self.root,'install',output,'03-daemon-install'),1)
            self.assertEqual(run.call_args.args[0],['make','completion-milestone','COMPLETION_MILESTONE=03-daemon-install','COMPLETION_GATE=install'])
        receipt=h.read(output)
        self.assertFalse(receipt['passed']);self.assertEqual(receipt['commands'][0]['exit_code'],7)

    def test_gate_refuses_dirty_product(self):
        (self.root/'code.go').write_text('changed')
        with self.assertRaisesRegex(ValueError,'commit product'):
            h.gate(self.root,'core',self.root/h.BASE/'evidence/core.json')

    def test_evidence_bookkeeping_is_allowed(self):
        self.manifest()
        self.assertEqual(h.product_dirty(self.root),[])
        self.assertEqual(h.accept(self.root,str(h.BASE/'evidence/release.json')),0)

    def test_bootstrap_scope_uses_existing_core_command(self):
        output=self.root/h.BASE/'evidence/scoped.json'
        with patch.object(h,'run_child',return_value=0) as run:
            self.assertEqual(h.gate(self.root,'core',output,'01-baseline-contract'),0)
            self.assertEqual(run.call_args.args[0],['make','harness-validate'])

    def test_accept_rejects_missing_failed_stale_and_scoped_receipts(self):
        m=self.manifest();p=self.root/m['receipts'][0];original=h.read(p)
        for field,value in [('passed',False),('tested_commit','a'*40),('milestone','01-baseline-contract'),('commands',[])]:
            r=dict(original);r[field]=value;h.write(p,r)
            with self.assertRaises(ValueError):h.accept(self.root,str(h.BASE/'evidence/release.json'))
        h.write(p,original);m['receipts'].pop();h.write(self.root/h.BASE/'evidence/release.json',m)
        with self.assertRaisesRegex(ValueError,'missing final gates'):h.accept(self.root,str(h.BASE/'evidence/release.json'))

    def test_accept_rejects_bad_review_native_artifact_and_requirements(self):
        m=self.manifest()
        for path,field,value in [(m['review'],'verdict','changes_required'),(m['receipts'][2],'platform','Linux')]:
            p=self.root/path;original=h.read(p);r=dict(original);r[field]=value;h.write(p,r)
            with self.assertRaises(ValueError):h.accept(self.root,str(h.BASE/'evidence/release.json'))
            h.write(p,original)
        Path(m['artifact']['path']).write_bytes(b'corrupt')
        with self.assertRaisesRegex(ValueError,'checksum'):h.accept(self.root,str(h.BASE/'evidence/release.json'))

    def test_accept_rejects_product_change_after_tested_commit(self):
        self.manifest();(self.root/'code.go').write_text('new code')
        self.git('add','.');self.git('commit','-m','changed product')
        with self.assertRaisesRegex(ValueError,'product changed'):h.accept(self.root,str(h.BASE/'evidence/release.json'))

    def test_prompt_has_full_mission_not_one_task(self):
        p=h.mission(self.root)
        self.assertIn('Continue through every eligible milestone',p)
        self.assertIn('SEPARATE reviewer',p)
        self.assertIn('No checks', (self.root/h.BASE/'plans/01-baseline-contract.md').read_text())


if __name__=='__main__':
    unittest.main()
