#!/bin/sh
# Automated native evidence only. The operator checklist still requires review.
set -eu
[ "$(uname -s)" = Darwin ] && [ "$(uname -m)" = arm64 ] || {
  echo "Native Apple Silicon macOS is required; Linux/cross builds are not verification." >&2
  exit 2
}
version=
native_output=
while [ "$#" -gt 0 ]; do
  case "$1" in
    --version) version=${2:?version is required}; shift 2;;
    --output-dir) native_output=${2:?owned output directory is required}; shift 2;;
    *) echo "Unknown argument: $1" >&2; exit 2;;
  esac
done
[ -n "$version" ] && [ -n "$native_output" ] || { echo "--version and --output-dir are required" >&2; exit 2; }
case "$native_output" in /*) ;; *) echo "Use an absolute owned output directory" >&2; exit 2;; esac
[ ! -e "$native_output" ] || { echo "Output directory must be fresh" >&2; exit 2; }
mkdir -m 700 "$native_output"
native_output=$(python3 -c 'import os,sys; print(os.path.realpath(sys.argv[1]))' "$native_output")
root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd -P)
cd "$root"
{
  sw_vers
  uname -sm
  sysctl hw.ncpu hw.memsize
  git rev-parse HEAD
  go version
  node --version
  python3 --version
  java -version 2>&1
  git lfs version
} > "$native_output/environment.txt"
make harness-validate
make release VERSION="$version" OUTPUT_DIR="$native_output/release"
archive="aios-$version-macos-arm64.tar.gz"
(cd "$native_output/release" && cat "$archive.sha256" && shasum -a 256 -c "$archive.sha256") > "$native_output/archive.sha256.txt"
mkdir -m 700 "$native_output/install"
tar -xzf "$native_output/release/$archive" -C "$native_output/install"
installed="$native_output/install/aios-$version-macos-arm64/bin/aios"
{
  file "$installed"
  shasum -a 256 "$installed"
  go version -m "$installed"
} > "$native_output/installed-binary.txt"
AIOS_UI_BINARY="$installed" make harness-validate-web
make acceptance-v1 ACCEPTANCE_OUTPUT="$native_output/acceptance-a"
make acceptance-v1 ACCEPTANCE_OUTPUT="$native_output/acceptance-b"
python3 - "$native_output" <<'PY'
import json,sys
from pathlib import Path
p=Path(sys.argv[1])
a,b=[json.loads((p/f'acceptance-{suffix}'/'acceptance-report.json').read_text()) for suffix in ('a','b')]
assert a['disposition']==b['disposition']=='accepted', 'Native acceptance was not accepted'
assert a['semantic_fingerprint']==b['semantic_fingerprint'], 'Native fingerprints differ'
(p/'fingerprints.txt').write_text(a['semantic_fingerprint']+'\n'+b['semantic_fingerprint']+'\n')
PY
# The installed archive must also carry the pinned model/helper, with no downloads.
# These reports are additional archive evidence, not the two make-gate reports.
sandbox-exec -p '(version 1) (allow default) (deny network*)' \
  python3 scripts/v1_acceptance.py --binary "$installed" --output-dir "$native_output/acceptance-installed"
python3 - "$native_output" <<'PY_CHECK'
import json,sys
from pathlib import Path
p=Path(sys.argv[1])
a=json.loads((p/'acceptance-a'/'acceptance-report.json').read_text())
i=json.loads((p/'acceptance-installed'/'acceptance-report.json').read_text())
assert i['disposition']=='accepted', 'Installed offline archive rejected'
assert i['semantic_fingerprint']==a['semantic_fingerprint'], 'Installed archive differs'
PY_CHECK
echo "Automated native checks passed. Record manual observations before completing task 07."
