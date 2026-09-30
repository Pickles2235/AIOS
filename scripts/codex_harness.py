#!/usr/bin/env python3
"""One-task development tooling. No third-party Python dependencies."""
import argparse
import json
import os
from pathlib import Path
import re
import shutil
import signal
import subprocess
import sys
import uuid

ROOT = Path(__file__).resolve().parents[1]
STATUSES = {"pending", "in_progress", "blocked", "ready", "completed"}


def git(root, *args):
    return subprocess.check_output(["git", "-C", str(root), *args], text=True).strip()


def load_queue(root):
    queue = json.loads((root / ".codex/tasks.json").read_text())
    if not isinstance(queue, dict) or queue.get("schema_version") != 1 or not isinstance(queue.get("tasks"), list):
        raise ValueError("invalid queue schema")
    seen = set()
    for task in queue["tasks"]:
        if not isinstance(task, dict):
            raise ValueError("task must be an object")
        for key in ("id", "title", "scope", "non_goals", "acceptance", "checks", "dependencies", "status", "plan", "completion_commit"):
            if key not in task:
                raise ValueError(f"missing task field: {key}")
        if not isinstance(task["title"], str) or not task["title"].strip():
            raise ValueError("task title must be nonempty text")
        tid = task["id"]
        if not isinstance(tid, str) or not re.fullmatch(r"[0-9]{2}-[a-z0-9-]+", tid) or tid in seen:
            raise ValueError("invalid or duplicate task ID")
        if not isinstance(task["status"], str) or task["status"] not in STATUSES:
            raise ValueError(f"invalid status for {tid}")
        for key in ("scope", "non_goals", "acceptance", "checks"):
            if not isinstance(task[key], list) or not task[key] or not all(isinstance(x, str) and x.strip() for x in task[key]):
                raise ValueError(f"invalid {key} for {tid}")
        deps = task["dependencies"]
        if not isinstance(deps, list) or not all(isinstance(x, str) for x in deps) or len(set(deps)) != len(deps) or any(dep not in seen for dep in deps):
            raise ValueError(f"dependencies must precede {tid} (unknown, duplicate, or cyclic)")
        if not isinstance(task["plan"], str):
            raise ValueError(f"invalid plan for {tid}")
        plan = Path(task["plan"])
        if plan.is_absolute() or ".." in plan.parts or plan.parts[:2] != (".codex", "plans") or not (root / plan).is_file():
            raise ValueError(f"missing or unsafe plan for {tid}")
        commit = task["completion_commit"]
        if task["status"] == "completed":
            if not isinstance(commit, str) or not re.fullmatch(r"[0-9a-f]{40}", commit):
                raise ValueError(f"completed task {tid} needs a full implementation commit")
        elif commit is not None:
            raise ValueError(f"unpublished task {tid} cannot have a completion commit")
        seen.add(tid)
    if not seen:
        raise ValueError("empty task queue")
    return queue["tasks"]


def ancestor(root, commit, ref):
    return subprocess.run(["git", "-C", str(root), "merge-base", "--is-ancestor", commit, ref], stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL).returncode == 0


def eligible(root, task, tasks):
    if task["status"] != "pending":
        return False
    by_id = {t["id"]: t for t in tasks}
    return all(by_id[d]["status"] == "completed" and
               ancestor(root, by_id[d]["completion_commit"], "refs/remotes/origin/main") and
               ancestor(root, by_id[d]["completion_commit"], "HEAD") for d in task["dependencies"])


def prompt(task):
    return f"""Work independently on AIOS task {task['id']}: {task['title']}.
Read AGENTS.md, .codex/README.md, .codex/tasks.json, and {task['plan']}.
Inspect current implementation, tests, documentation, branch and worktree first.
Verify eligibility with scripts/codex_harness.py; predecessors must be completed on origin/main,
not merely on local branches. Preserve existing work and use a dedicated branch.
Complete exactly this one task. Scope: {json.dumps(task['scope'])}
Non-goals: {json.dumps(task['non_goals'])}
Acceptance: {json.dumps(task['acceptance'])}
Required checks: {json.dumps(task['checks'])}
Mark in_progress before implementation. Persist decisions, progress, actual check
commands/results, blockers and handoff in the plan. Make routine decisions yourself;
ask only for an unresolved scope or public-contract decision. Preserve all AGENTS.md
invariants. Do not report unrun checks as passing. Inspect the final diff, commit
scoped work and publish directly with a normal fast-forward push to main
(git push origin HEAD:main). Fetch origin/main first; reconcile concurrent trunk
changes and rerun affected checks before publishing. Never force-push. Set ready
when validated, then completed only after verifying the implementation commit is
on origin/main; record its full SHA as completion_commit in a follow-up bookkeeping
commit. If publishing fails, retain ready status and record the precise blocker.
Use .codex/HANDOFF_TEMPLATE.md and stop after this task's reviewable handoff.
Do not auto-merge, schedule calls, or continue to another task.
"""


def run_task(root, task, tasks):
    common = Path(git(root, "rev-parse", "--path-format=absolute", "--git-common-dir"))
    lock = common / "aios-codex.lock"
    try:
        lock.mkdir(mode=0o700)
    except FileExistsError:
        raise ValueError(f"another run or stale lock exists: {lock}; inspect before manual recovery")
    child = None
    old_handlers = {}
    try:
        (lock / "owner.json").write_text(json.dumps({"pid": os.getpid(), "checkout": str(root)}))
        if git(root, "status", "--porcelain", "--untracked-files=all", "--ignore-submodules=none"):
            raise ValueError("dirty checkout; preserve and resolve existing work before launching")
        if not eligible(root, task, tasks):
            raise ValueError("task is not pending or dependencies are not completed in origin/main and HEAD")
        if not shutil.which("codex"):
            raise ValueError("Codex CLI not installed; use the prompt command")
        run_id = task["id"] + "-" + uuid.uuid4().hex[:12]
        logs = common / "aios-codex-runs" / run_id
        logs.mkdir(parents=True, mode=0o700)
        text = prompt(task)
        (logs / "prompt.txt").write_text(text)
        branch = "codex/" + run_id
        subprocess.run(["git", "-C", str(root), "switch", "-c", branch], check=True)
        (logs / "run.json").write_text(json.dumps({"task": task["id"], "branch": branch, "start_commit": git(root, "rev-parse", "HEAD")}))
        print(f"Branch: {branch}\nLogs: {logs}", flush=True)

        def interrupt(signum, _frame):
            if child is not None and child.poll() is None:
                if os.name == "posix":
                    os.killpg(child.pid, signal.SIGTERM)
                else:
                    child.terminate()
            raise KeyboardInterrupt

        for sig in (signal.SIGINT, signal.SIGTERM):
            old_handlers[sig] = signal.signal(sig, interrupt)
        code = 1
        with (logs / "codex.log").open("w") as output:
            try:
                child = subprocess.Popen(["codex", "exec", "--sandbox", "workspace-write", "-"], cwd=root, stdin=subprocess.PIPE,
                                         stdout=output, stderr=subprocess.STDOUT, text=True,
                                         start_new_session=os.name == "posix")
                child.communicate(text)
                code = child.returncode
            finally:
                if child is not None and child.poll() is None:
                    if os.name == "posix":
                        os.killpg(child.pid, signal.SIGTERM)
                    else:
                        child.terminate()
                    try:
                        child.wait(timeout=10)
                    except subprocess.TimeoutExpired:
                        if os.name == "posix":
                            os.killpg(child.pid, signal.SIGKILL)
                        else:
                            child.kill()
                        child.wait()
                (logs / "exit-code.txt").write_text(str(child.returncode if child is not None else code) + "\n")
        return code
    finally:
        for sig, handler in old_handlers.items():
            signal.signal(sig, handler)
        shutil.rmtree(lock)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("command", choices=["check", "next", "prompt", "run"])
    parser.add_argument("task", nargs="?")
    args = parser.parse_args()
    tasks = load_queue(ROOT)
    if args.command == "check":
        print(f"Queue valid: {len(tasks)} dependency-ordered tasks")
        return 0
    if args.command == "next":
        task = next((t for t in tasks if eligible(ROOT, t, tasks)), None)
        print(task["id"] if task else "No eligible pending task; inspect plans and completed dependency records")
        return 0
    task = next((t for t in tasks if t["id"] == args.task), None)
    if task is None:
        raise ValueError("provide a known task ID")
    if args.command == "prompt":
        if not eligible(ROOT, task, tasks):
            raise ValueError("task is not eligible; inspect status and completed dependencies")
        print(prompt(task))
        return 0
    return run_task(ROOT, task, tasks)


if __name__ == "__main__":
    try:
        sys.exit(main())
    except (ValueError, OSError, subprocess.CalledProcessError) as error:
        print(f"harness: {error}", file=sys.stderr)
        sys.exit(2)
    except KeyboardInterrupt:
        print("harness: interrupted; branch and logs preserved", file=sys.stderr)
        sys.exit(130)
