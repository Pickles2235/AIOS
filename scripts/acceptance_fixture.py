#!/usr/bin/env python3
"""Create the hermetic 25-repository AIOS V1 acceptance estate."""

from __future__ import annotations

import argparse
import hashlib
import json
import os
import shutil
import subprocess
from pathlib import Path


FIXTURE_VERSION = "aios-v1-estate-1"
FEATURE_REPOSITORIES = ("shared-contracts", "orders-service", "gateway-service", "customer-portal")
REPOSITORY_IDS = FEATURE_REPOSITORIES + tuple(f"fixture-{index:02d}" for index in range(1, 22))
GIT_ENV = {
    "GIT_AUTHOR_NAME": "AIOS Fixture",
    "GIT_AUTHOR_EMAIL": "fixture@invalid.local",
    "GIT_COMMITTER_NAME": "AIOS Fixture",
    "GIT_COMMITTER_EMAIL": "fixture@invalid.local",
    "GIT_AUTHOR_DATE": "2024-01-01T00:00:00+0000",
    "GIT_COMMITTER_DATE": "2024-01-01T00:00:00+0000",
    "TZ": "UTC",
    "LC_ALL": "C",
}


BASELINE: dict[str, dict[str, str]] = {
    "shared-contracts": {
        "contracts/order-created.json": '{"event":"orders.created.v1","owner":"commerce-platform"}\n',
        "src/events.ts": 'export const ORDER_CREATED = "orders.created.v1";\n',
        "README.md": "# Shared contracts\nBoundary: commerce-platform.\n",
    },
    "orders-service": {
        "src/main/java/example/OrderController.java": """package example;
import org.springframework.web.bind.annotation.*;
@RestController @RequestMapping("/api/orders")
class OrderController {
  @PostMapping public void createOrder() { publisher.publish("orders.created.v1"); }
  private final OrderPublisher publisher = new OrderPublisher();
}
""",
        "src/main/java/example/OrderPublisher.java": """package example;
class OrderPublisher {
  void publish(String topic) { kafkaTemplate.send(topic, "created"); }
  private final KafkaTemplate kafkaTemplate = new KafkaTemplate();
}
class KafkaTemplate { void send(String topic, String body) {} }
""",
        "src/test/java/example/OrderControllerTest.java": "package example; class OrderControllerTest { void createsOrder() {} }\n",
        "build.gradle": "plugins { id 'java' }\n",
        "CODEOWNERS": "src/main/java/ @orders-team\n",
    },
    "gateway-service": {
        "src/main/java/example/OrderCreatedListener.java": """package example;
import org.springframework.kafka.annotation.KafkaListener;
class OrderCreatedListener {
  @KafkaListener(topics = "orders.created.v1")
  void consume(String event) { client.notifyPortal(event); }
  private final PortalClient client = new PortalClient();
}
class PortalClient { void notifyPortal(String event) {} }
""",
        "src/main/resources/application.yml": "orders-topic: orders.created.v1\nportal-url: /api/portal/orders\n",
        "src/test/java/example/OrderCreatedListenerTest.java": "package example; class OrderCreatedListenerTest {}\n",
        "README.md": "# Gateway service\nConsumes the shared order contract.\n",
    },
    "customer-portal": {
        "src/api/order-client.ts": """export async function loadOrders() {
  return fetch('/api/orders');
}
export const orderEvent = 'orders.created.v1';
""",
        "src/routes/orders.tsx": """import { loadOrders } from '../api/order-client';
export function OrdersRoute() { void loadOrders(); return <section>Orders</section>; }
""",
        "src/routes/orders.test.tsx": "import { OrdersRoute } from './orders'; void OrdersRoute;\n",
        "package.json": '{"name":"customer-portal","private":true,"scripts":{"test":"vitest"}}\n',
        "README.md": "# Customer portal\nOwned by digital-experience.\n",
    },
}

DELTA: dict[str, dict[str, str | None]] = {
    "shared-contracts": {
        "contracts/order-created.json": None,
        "contracts/order-submitted.json": '{"event":"orders.submitted.v2","owner":"commerce-platform"}\n',
        "src/events.ts": 'export const ORDER_SUBMITTED = "orders.submitted.v2";\n',
    },
    "orders-service": {
        "src/main/java/example/OrderPublisher.java": """package example;
class OrderPublisher {
  void publish(String topic) { kafkaTemplate.send("orders.submitted.v2", "submitted"); }
  private final KafkaTemplate kafkaTemplate = new KafkaTemplate();
}
class KafkaTemplate { void send(String topic, String body) {} }
""",
        "src/test/java/example/OrderControllerTest.java": None,
        "src/test/java/example/OrderSubmissionTest.java": "package example; class OrderSubmissionTest { void submitsOrder() {} }\n",
    },
    "gateway-service": {
        "src/main/java/example/OrderCreatedListener.java": None,
        "src/main/java/example/OrderSubmittedListener.java": """package example;
import org.springframework.kafka.annotation.KafkaListener;
class OrderSubmittedListener {
  @KafkaListener(topics = "orders.submitted.v2")
  void consume(String event) { client.notifyPortal(event); }
  private final PortalClient client = new PortalClient();
}
class PortalClient { void notifyPortal(String event) {} }
""",
        "src/main/resources/application.yml": "orders-topic: orders.submitted.v2\nportal-url: /api/portal/orders\n",
    },
    "customer-portal": {
        "src/api/order-client.ts": """export async function loadOrders() {
  return fetch('/api/orders?contract=orders.submitted.v2');
}
export const orderEvent = 'orders.submitted.v2';
""",
        "src/routes/orders.test.tsx": None,
    },
}


def run(command: list[str], cwd: Path | None = None, env: dict[str, str] | None = None) -> str:
    effective = os.environ.copy()
    effective.update(GIT_ENV)
    if env:
        effective.update(env)
    return subprocess.check_output(command, cwd=cwd, env=effective, text=True, stderr=subprocess.STDOUT).strip()


def write_tree(root: Path, files: dict[str, str | None]) -> None:
    for relative, content in sorted(files.items()):
        path = root / relative
        if content is None:
            path.unlink(missing_ok=True)
            continue
        path.parent.mkdir(parents=True, exist_ok=True)
        path.write_text(content, encoding="utf-8")


def commit(root: Path, message: str, date: str) -> str:
    run(["git", "add", "-A"], root)
    run(["git", "commit", "-q", "-m", message], root, {"GIT_AUTHOR_DATE": date, "GIT_COMMITTER_DATE": date})
    return run(["git", "rev-parse", "HEAD"], root)


def create_repository(base: Path, repository_id: str) -> dict[str, str]:
    work = base / "work" / repository_id
    remote = base / "remotes" / f"{repository_id}.git"
    work.mkdir(parents=True)
    run(["git", "init", "-q", "-b", "main"], work)
    files = BASELINE.get(repository_id, {
        "README.md": f"# {repository_id}\nDeterministic catalog member.\n",
        "OWNERS": f"repository: fixture-team-{repository_id}\n",
    })
    write_tree(work, files)
    baseline = commit(work, "fixture baseline", "2024-01-01T00:00:00+0000")
    if repository_id in DELTA:
        write_tree(work, DELTA[repository_id])
    else:
        write_tree(work, {"README.md": files["README.md"] + "Revision two.\n"})
    delta = commit(work, "fixture delta", "2024-01-02T00:00:00+0000")
    run(["git", "init", "-q", "--bare", str(remote)])
    run(["git", "remote", "add", "origin", str(remote)], work)
    run(["git", "push", "-q", "origin", f"{baseline}:refs/heads/main", f"{delta}:refs/heads/delta"], work)
    run(["git", "symbolic-ref", "HEAD", "refs/heads/main"], remote)
    return {"baseline": baseline, "delta": delta, "remote": str(remote.resolve())}


def create(root: Path) -> dict[str, object]:
    root = root.resolve()
    if root.exists():
        shutil.rmtree(root)
    root.mkdir(mode=0o700, parents=True)
    revisions = {repository_id: create_repository(root, repository_id) for repository_id in REPOSITORY_IDS}
    sources = []
    registry = []
    for repository_id in REPOSITORY_IDS:
        sources.append({
            "kind": "repository",
            "id": repository_id,
            "include": ["**/*"],
            "exclude": [".git/**"],
            "ownership": [{"coordinate": "repository", "owner": "fixture-team"}],
        })
        registry.append({"id": repository_id, "url": revisions[repository_id]["remote"], "ref": "refs/heads/main"})
    catalog = {
        "version": 1,
        "sources": sources,
        "limits": {"max_file_bytes": 1048576, "max_files_per_repo": 100, "max_total_bytes_per_repo": 10485760, "max_results": 100},
        "retention_generations": 3,
        "vector": {"enabled": True, "dimensions": 768},
    }
    manifest = {"fixture_version": FIXTURE_VERSION, "repositories": revisions}
    for name, value in (("catalog.json", catalog), ("registry.json", {"version": 1, "repositories": registry}), ("fixture-manifest.json", manifest)):
        (root / name).write_text(json.dumps(value, indent=2, sort_keys=True) + "\n", encoding="utf-8")
    return manifest


def advance(root: Path) -> None:
    manifest = json.loads((root / "fixture-manifest.json").read_text(encoding="utf-8"))
    for repository_id in FEATURE_REPOSITORIES:
        values = manifest["repositories"][repository_id]
        run(["git", "update-ref", "refs/heads/main", values["delta"]], Path(values["remote"]))


def semantic_digest(root: Path) -> str:
    manifest = json.loads((root / "fixture-manifest.json").read_text(encoding="utf-8"))
    stable = {key: {"baseline": value["baseline"], "delta": value["delta"]} for key, value in manifest["repositories"].items()}
    return hashlib.sha256(json.dumps(stable, sort_keys=True, separators=(",", ":")).encode()).hexdigest()


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("command", choices=("create", "advance", "digest"))
    parser.add_argument("--root", required=True, type=Path)
    args = parser.parse_args()
    if args.command == "create":
        manifest = create(args.root)
        print(json.dumps({"fixture_version": manifest["fixture_version"], "repositories": len(manifest["repositories"]), "digest": semantic_digest(args.root)}))
    elif args.command == "advance":
        advance(args.root.resolve(strict=True))
        print(json.dumps({"advanced": True, "repositories": len(REPOSITORY_IDS)}))
    else:
        print(semantic_digest(args.root.resolve(strict=True)))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
