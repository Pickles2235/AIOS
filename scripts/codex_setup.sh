#!/bin/sh
# Install repository dependencies only; system tools/auth remain user-owned.
set -eu
cd "$(dirname "$0")/.."
for tool in go git node npm python3 make java javac; do
    command -v "$tool" >/dev/null || { echo "Missing required tool: $tool" >&2; exit 2; }
done
# Fail early if a different executable named go is installed.
go version
python3 - <<'CHECK'
import re
import subprocess
import sys
if sys.version_info < (3, 10):
    raise SystemExit("Python 3.10+ is required")
go = subprocess.check_output(["go", "env", "GOVERSION"], text=True).strip()
match = re.match(r"go(\d+)\.(\d+)", go)
if not match or tuple(map(int, match.groups())) < (1, 25):
    raise SystemExit("Go 1.25+ is required")
node = subprocess.check_output(["node", "--version"], text=True).strip()
if int(node.lstrip("v").split(".")[0]) < 22:
    raise SystemExit("Node 22+ is required")
CHECK
java -version
javac -version
go env CGO_ENABLED CC
if [ "$(go env CGO_ENABLED)" != 1 ]; then
    echo "CGO must be enabled for tree-sitter; set CGO_ENABLED=1 with a C compiler" >&2
    exit 2
fi
command -v "$(go env CC)" >/dev/null || { echo "Missing C compiler" >&2; exit 2; }
git lfs version
node --version
npm --version
python3 --version
go mod download
npm ci --prefix web
# Repository-local filters/hooks; do not change global Git configuration.
git lfs install --local
git lfs pull
python3 scripts/codex_harness.py check
