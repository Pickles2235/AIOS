#!/usr/bin/env python3
"""Compile an explicitly approved V1 inventory into catalog and mirror registry."""

from __future__ import annotations

import argparse
import json
import os
import re
import stat
import subprocess
import sys
from pathlib import Path
from typing import Any

MAX_REPOSITORIES = 100
ID_PATTERN = re.compile(r"^[a-z0-9][a-z0-9._-]{0,62}$")
# No estate/language allowlist: secure discovery reports unsupported coverage.
INCLUDE = []
EXCLUDE = [
    "**/*.min.js", "**/*.map", "**/package-lock.json",
    "**/npm-shrinkwrap.json", "**/generated/**",
    "**/generated-sources/**", "**/coverage/**",
]
LIMITS = {
    "max_file_bytes": 1048576,
    "max_files_per_repo": 20000,
    "max_total_bytes_per_repo": 268435456,
    "max_results": 100,
}


def _load_json(path: Path) -> dict[str, Any]:
    value = json.loads(path.read_text(encoding="utf-8"))
    if not isinstance(value, dict):
        raise ValueError("inventory must be a JSON object")
    return value


def _patterns(repository: dict[str, Any], name: str, defaults: list[str]) -> list[str]:
    values = repository.get(name, defaults)
    if not isinstance(values, list) or not all(isinstance(value, str) and value for value in values):
        raise ValueError(f"repository {repository.get('id', '<unknown>')} {name} must be a list of non-empty strings")
    if len(values) > 64 or any(len(value) > 256 or value.startswith("/") or ".." in value for value in values):
        raise ValueError(f"repository {repository.get('id', '<unknown>')} has unsafe {name} patterns")
    return values


def _ownership(repository: dict[str, Any]) -> list[dict[str, str]]:
    values = repository.get("ownership", [])
    if not isinstance(values, list):
        raise ValueError(f"repository {repository.get('id', '<unknown>')} ownership must be a list")
    result: list[dict[str, str]] = []
    coordinates: set[str] = set()
    for value in values:
        if not isinstance(value, dict) or set(value) != {"coordinate", "owner"}:
            raise ValueError(f"repository {repository.get('id', '<unknown>')} has invalid ownership")
        coordinate, owner = value["coordinate"], value["owner"]
        if not isinstance(coordinate, str) or not coordinate or not isinstance(owner, str) or not owner:
            raise ValueError(f"repository {repository.get('id', '<unknown>')} has invalid ownership")
        if coordinate in coordinates:
            raise ValueError(f"repository {repository.get('id', '<unknown>')} duplicates ownership coordinate {coordinate}")
        coordinates.add(coordinate)
        result.append({"coordinate": coordinate, "owner": owner})
    return result


def build_outputs(inventory: dict[str, Any]) -> tuple[dict[str, Any], dict[str, Any]]:
    if set(inventory) - {"version", "repositories", "compiler_runtime", "vector"}:
        raise ValueError("inventory contains unsupported fields")
    if inventory.get("version") != 1:
        raise ValueError("inventory version must be 1")
    repositories = inventory.get("repositories")
    if not isinstance(repositories, list) or not 1 <= len(repositories) <= MAX_REPOSITORIES:
        found = len(repositories) if isinstance(repositories, list) else 0
        raise ValueError(f"expected 1–{MAX_REPOSITORIES} approved repositories, found {found}")
    seen: set[str] = set()
    sources, mirrors = [], []
    for index, repository in enumerate(repositories):
        if not isinstance(repository, dict):
            raise ValueError(f"repositories[{index}] must be an object")
        allowed = {"id", "url", "ref", "include", "exclude", "ownership"}
        if set(repository) - allowed:
            raise ValueError(f"repositories[{index}] contains unsupported fields")
        identifier, url, ref = repository.get("id"), repository.get("url"), repository.get("ref")
        if not isinstance(identifier, str) or not ID_PATTERN.fullmatch(identifier) or identifier in seen:
            raise ValueError(f"repositories[{index}].id is invalid or duplicated")
        if not isinstance(url, str) or not url.strip() or any(character in url for character in "\r\n"):
            raise ValueError(f"repository {identifier} has an invalid URL")
        if not isinstance(ref, str) or not ref.startswith("refs/") or any(character.isspace() for character in ref):
            raise ValueError(f"repository {identifier} ref must be a full refs/ name")
        seen.add(identifier)
        source = {
            "kind": "repository", "id": identifier,
            "include": _patterns(repository, "include", INCLUDE),
            "exclude": _patterns(repository, "exclude", EXCLUDE),
        }
        ownership = _ownership(repository)
        if ownership:
            source["ownership"] = ownership
        sources.append(source)
        mirrors.append({"id": identifier, "url": url, "ref": ref})
    sources.sort(key=lambda value: value["id"])
    mirrors.sort(key=lambda value: value["id"])
    catalog: dict[str, Any] = {
        "version": 1, "sources": sources, "limits": LIMITS,
        "retention_generations": 3,
    }
    for optional in ("compiler_runtime", "vector"):
        if optional in inventory:
            catalog[optional] = inventory[optional]
    return catalog, {"version": 1, "repositories": mirrors}


def verify_remotes(registry: dict[str, Any]) -> None:
    environment = {
        "PATH": os.environ.get("PATH", ""), "LC_ALL": "C",
        "GIT_TERMINAL_PROMPT": "0", "GIT_CONFIG_GLOBAL": os.devnull,
        "GIT_CONFIG_NOSYSTEM": "1", "GIT_ASKPASS": "/usr/bin/false",
    }
    for repository in registry["repositories"]:
        completed = subprocess.run(
            ["git", "--no-pager", "-c", "credential.helper=", "ls-remote", "--exit-code", repository["url"], repository["ref"]],
            text=True, capture_output=True, timeout=60, env=environment,
        )
        if completed.returncode != 0:
            detail = completed.stderr.strip() or "configured ref was not found"
            raise ValueError(f"repository {repository['id']} remote/ref unavailable: {detail}")


def _write_private(path: Path, value: dict[str, Any]) -> None:
    temporary = path.with_name(path.name + ".tmp")
    temporary.write_text(json.dumps(value, indent=2) + "\n", encoding="utf-8")
    os.chmod(temporary, stat.S_IRUSR | stat.S_IWUSR)
    temporary.replace(path)


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--inventory", required=True, type=Path)
    parser.add_argument("--data-dir", required=True, type=Path)
    parser.add_argument("--skip-remote-check", action="store_true", help="Only for offline validation; production bootstrap must verify remotes")
    args = parser.parse_args()
    try:
        inventory = _load_json(args.inventory.expanduser().resolve(strict=True))
        catalog, registry = build_outputs(inventory)
        if not args.skip_remote_check:
            verify_remotes(registry)
        data_dir = args.data_dir.expanduser().absolute()
        data_dir.mkdir(mode=0o700, parents=True, exist_ok=True)
        os.chmod(data_dir, stat.S_IRWXU)
        _write_private(data_dir / "catalog.json", catalog)
        _write_private(data_dir / "mirrors.json", registry)
    except (OSError, ValueError, json.JSONDecodeError, subprocess.SubprocessError) as error:
        print(f"error: {error}", file=sys.stderr)
        return 1
    print(json.dumps({"catalog": str(data_dir / "catalog.json"), "registry": str(data_dir / "mirrors.json")}))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
