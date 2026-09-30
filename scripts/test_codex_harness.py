"""Offline regression coverage: never invokes a model or real authentication."""
import importlib.util
import json
import os
from pathlib import Path
import shutil
import signal
import subprocess
import tempfile
import time
import unittest

ROOT = Path(__file__).resolve().parents[1]
spec = importlib.util.spec_from_file_location("codex_harness", ROOT / "scripts/codex_harness.py")
harness = importlib.util.module_from_spec(spec)
spec.loader.exec_module(harness)


class RepositoryQueueTests(unittest.TestCase):
    def test_current_queue_is_valid(self):
        # Real task statuses evolve; validation must not force them back to pending.
        self.assertGreaterEqual(len(harness.load_queue(ROOT)), 7)


class QueueTests(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory()
        self.addCleanup(self.tmp.cleanup)
        self.root = Path(self.tmp.name)
        shutil.copytree(ROOT / ".codex", self.root / ".codex")
        # Tests need stable pending fixtures even after real tasks land on main.
        tasks = harness.load_queue(self.root)
        for task in tasks:
            task.update(status="pending", completion_commit=None)
        self.save(tasks)

    def save(self, tasks):
        (self.root / ".codex/tasks.json").write_text(json.dumps({"schema_version": 1, "tasks": tasks}))

    def test_seed_is_ordered_pending_and_resumable(self):
        tasks = harness.load_queue(self.root)
        self.assertGreaterEqual(len(tasks), 7)
        for i, task in enumerate(tasks):
            self.assertEqual(task["status"], "pending")
            self.assertTrue(set(task["dependencies"]).issubset({t["id"] for t in tasks[:i]}))
            plan = (self.root / task["plan"]).read_text()
            for heading in ("Decisions", "Progress", "Actual validation", "Blockers", "Handoff"):
                self.assertIn(heading, plan)
        self.assertIn("Complete exactly this one task", harness.prompt(tasks[0]))
        self.assertIn("Do not report unrun checks as passing", harness.prompt(tasks[0]))
        self.assertIn("git push origin HEAD:main", harness.prompt(tasks[0]))
        self.assertIn("completed only after verifying", harness.prompt(tasks[0]))
        self.assertIn(".codex/HANDOFF_TEMPLATE.md", harness.prompt(tasks[0]))
        self.assertNotIn("open a draft PR", harness.prompt(tasks[0]))

    def test_rejects_missing_fields_cycles_bad_status_and_completion_records(self):
        originals = harness.load_queue(self.root)
        mutations = [lambda t: t[0].pop("acceptance"),
                     lambda t: t[0].update(dependencies=[t[1]["id"]]),
                     lambda t: t[0].update(status="done"),
                     lambda t: t[0].update(status="completed"),
                     lambda t: t[0].update(completion_commit="a" * 40),
                     lambda t: t[0].update(plan="../../outside"),
                     lambda t: t[0].update(checks=[])]
        for mutate in mutations:
            tasks = json.loads(json.dumps(originals))
            mutate(tasks)
            self.save(tasks)
            with self.subTest(tasks=tasks[0]), self.assertRaises(ValueError):
                harness.load_queue(self.root)


@unittest.skipUnless(os.name == "posix", "launcher subprocess fixtures require POSIX; native CI covers Linux/macOS")
class LauncherTests(QueueTests):
    def setUp(self):
        super().setUp()
        (self.root / "scripts").mkdir()
        shutil.copy(ROOT / "scripts/codex_harness.py", self.root / "scripts/codex_harness.py")
        (self.root / "tracked.txt").write_text("baseline\n")
        self.git("init", "-b", "main")
        self.git("config", "user.name", "Harness Test")
        self.git("config", "user.email", "harness@example.invalid")
        self.git("add", ".")
        self.git("commit", "-m", "fixture")
        self.bin = self.root.parent / (self.root.name + "-bin")
        self.bin.mkdir()
        self.addCleanup(shutil.rmtree, self.bin)
        stub = self.bin / "codex"
        stub.write_text('''#!/usr/bin/env python3
import json, os, pathlib, sys, time
pathlib.Path(".git/fake-args.json").write_text(json.dumps(sys.argv[1:]))
print("fake stdout", flush=True)
print("fake stderr", file=sys.stderr, flush=True)
text = sys.stdin.read()
print(text, flush=True)
if os.environ.get("FAKE_WAIT"):
    pathlib.Path(".git/fake-started").touch()
    time.sleep(30)
sys.exit(int(os.environ.get("FAKE_EXIT", "0")))
''')
        stub.chmod(0o700)
        self.env = dict(os.environ, PATH=str(self.bin) + os.pathsep + os.environ["PATH"])
        self.task = harness.load_queue(self.root)[0]["id"]

    def git(self, *args):
        return subprocess.check_output(["git", "-C", str(self.root), *args], text=True, stderr=subprocess.DEVNULL).strip()

    def cli(self, *args, **extra):
        return subprocess.run(["python3", str(self.root / "scripts/codex_harness.py"), *args], env=dict(self.env, **extra), text=True, capture_output=True)

    def test_success_uses_configured_cli_and_dedicated_branch(self):
        result = self.cli("run", self.task)
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertTrue(self.git("branch", "--show-current").startswith("codex/" + self.task))
        args = json.loads((self.root / ".git/fake-args.json").read_text())
        self.assertEqual(args, ["exec", "--sandbox", "workspace-write", "-"])
        self.assertEqual(self.git("status", "--porcelain"), "")
        self.assertFalse((self.root / ".git/aios-codex.lock").exists())

    def test_dirty_tracked_staged_and_untracked_are_preserved(self):
        for mode in ("tracked", "staged", "untracked"):
            path = self.root / ("new.txt" if mode == "untracked" else "tracked.txt")
            path.write_text("preserve me\n")
            if mode == "staged":
                self.git("add", "tracked.txt")
            before = self.git("status", "--porcelain")
            result = self.cli("run", self.task)
            self.assertEqual(result.returncode, 2)
            self.assertIn("dirty checkout", result.stderr)
            self.assertEqual(self.git("status", "--porcelain"), before)
            self.assertEqual(path.read_text(), "preserve me\n")
            self.assertEqual(self.git("branch", "--show-current"), "main")
            if mode == "untracked":
                path.unlink()
            else:
                self.git("restore", "--staged", "--worktree", "tracked.txt")

    def test_failed_cli_retains_logs_and_branch(self):
        result = self.cli("run", self.task, FAKE_EXIT="7")
        self.assertEqual(result.returncode, 7, result.stderr)
        logs = list((self.root / ".git/aios-codex-runs").iterdir())
        self.assertEqual(len(logs), 1)
        self.assertEqual((logs[0] / "exit-code.txt").read_text(), "7\n")
        self.assertIn("fake stderr", (logs[0] / "codex.log").read_text())
        self.assertIn(self.task, (logs[0] / "prompt.txt").read_text())
        self.assertTrue(self.git("branch", "--show-current").startswith("codex/"))
        self.assertFalse((self.root / ".git/aios-codex.lock").exists())

    def test_dependencies_need_completed_status_and_reachable_commits(self):
        tasks = harness.load_queue(self.root)
        task = tasks[1]
        self.assertFalse(harness.eligible(self.root, task, tasks))
        tasks[0].update(status="completed", completion_commit=self.git("rev-parse", "HEAD"))
        self.assertFalse(harness.eligible(self.root, task, tasks))
        self.git("update-ref", "refs/remotes/origin/main", tasks[0]["completion_commit"])
        self.assertTrue(harness.eligible(self.root, task, tasks))
        (self.root / "tracked.txt").write_text("unpublished\n")
        self.git("commit", "-am", "local predecessor")
        tasks[0]["completion_commit"] = self.git("rev-parse", "HEAD")
        self.assertFalse(harness.eligible(self.root, task, tasks))
        self.git("update-ref", "refs/remotes/origin/main", tasks[0]["completion_commit"])
        self.git("checkout", "HEAD~1")
        self.assertFalse(harness.eligible(self.root, task, tasks))

    def test_ready_task_cannot_launch(self):
        tasks = harness.load_queue(self.root)
        tasks[0]["status"] = "ready"
        self.save(tasks)
        self.git("commit", "-am", "ready status")
        result = self.cli("run", self.task)
        self.assertEqual(result.returncode, 2)
        self.assertIn("not pending", result.stderr)
        self.assertEqual(self.git("branch", "--show-current"), "main")

    def test_live_run_blocks_linked_worktree_and_interrupt_cleans_lock(self):
        other = self.root.parent / (self.root.name + "-worktree")
        self.git("worktree", "add", "-b", "other", str(other))
        self.addCleanup(shutil.rmtree, other)
        proc = subprocess.Popen(["python3", str(self.root / "scripts/codex_harness.py"), "run", self.task], env=dict(self.env, FAKE_WAIT="1"), stdout=subprocess.PIPE, stderr=subprocess.PIPE, text=True)
        self.addCleanup(lambda: proc.kill() if proc.poll() is None else None)
        deadline = time.monotonic() + 10
        while not (self.root / ".git/fake-started").exists() and time.monotonic() < deadline:
            time.sleep(.05)
        self.assertTrue((self.root / ".git/fake-started").exists())
        rejected = subprocess.run(["python3", str(other / "scripts/codex_harness.py"), "run", self.task], env=self.env, capture_output=True, text=True)
        self.assertEqual(rejected.returncode, 2)
        self.assertIn("another run or stale lock", rejected.stderr)
        proc.send_signal(signal.SIGTERM)
        stdout, stderr = proc.communicate(timeout=15)
        self.assertEqual(proc.returncode, 130, stdout + stderr)
        self.assertFalse((self.root / ".git/aios-codex.lock").exists())
        self.assertTrue(list((self.root / ".git/aios-codex-runs").glob("*/exit-code.txt")))


if __name__ == "__main__":
    unittest.main()
