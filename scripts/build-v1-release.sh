#!/bin/sh
set -eu

usage() { echo "usage: $0 --version VERSION --output-dir DIRECTORY" >&2; exit 2; }

version=
output_dir=
while [ "$#" -gt 0 ]; do
  case "$1" in
    --version) [ "$#" -ge 2 ] || usage; version=$2; shift 2 ;;
    --output-dir) [ "$#" -ge 2 ] || usage; output_dir=$2; shift 2 ;;
    *) usage ;;
  esac
done
[ -n "$version" ] && [ -n "$output_dir" ] || usage
case "$version" in *[!A-Za-z0-9._-]*|'') echo "invalid version" >&2; exit 2;; esac
case "$version" in 1|1.*) ;; *) echo "V1 release version must be 1 or start with 1." >&2; exit 2;; esac

root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd -P)
case "$output_dir" in /*) ;; *) output_dir="$PWD/$output_dir";; esac
mkdir -p "$output_dir"
chmod 700 "$output_dir"
stage=$(mktemp -d "${TMPDIR:-/tmp}/aios-release.XXXXXX")
trap 'rm -rf "$stage"' EXIT HUP INT TERM
os=$(uname -s | tr '[:upper:]' '[:lower:]')
case "$os" in darwin) os=macos;; linux) :;; *) echo "unsupported release host: $os" >&2; exit 2;; esac
arch=$(uname -m)
case "$arch" in x86_64|amd64) arch=amd64;; arm64|aarch64) arch=arm64;; *) echo "unsupported release architecture: $arch" >&2; exit 2;; esac
package="aios-$version-$os-$arch"
mkdir -p "$stage/$package/bin" "$stage/$package/docs"

cd "$root"
npm --prefix "$root/web" run build
CGO_ENABLED=1 go build -trimpath -ldflags='-s -w' -o "$stage/$package/bin/aios" "$root/cmd/aios"
cp "$root/README.md" "$root/config.example.json" "$stage/$package/"
cp -R "$root/docs/." "$stage/$package/docs/"
cp -R "$root/acceptance" "$stage/$package/acceptance"
cp "$root/cmd/aios/notices/Apache-2.0.txt" "$stage/$package/NOTICE-Apache-2.0.txt"
find "$stage/$package/docs" -depth -type d -empty -delete
chmod 755 "$stage/$package/bin/aios"
find "$stage/$package" -type f ! -path '*/bin/aios' -exec chmod 644 {} \;

archive="$output_dir/$package.tar.gz"
COPYFILE_DISABLE=1 tar -C "$stage" -czf "$archive" "$package"
checksum="$archive.sha256"
(cd "$output_dir" && shasum -a 256 "$(basename "$archive")" > "$(basename "$checksum")")
chmod 600 "$archive" "$checksum"
printf '%s\n' "$archive"
