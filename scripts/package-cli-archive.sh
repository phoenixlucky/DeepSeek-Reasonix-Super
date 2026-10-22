#!/usr/bin/env bash
# usage: package-cli-archive.sh <goos> <goarch> <binary> <page-dir> <out-dir>
# One CLI archive: the executable at the root and the built page under
# frontend-next/dist/, where serve.FindPage looks beside the executable. Nothing
# else goes in, and the modes are fixed here rather than inherited from the runner.
set -euo pipefail

[ $# -eq 5 ] || { echo "usage: $0 <goos> <goarch> <binary> <page-dir> <out-dir>" >&2; exit 2; }
os="$1"; arch="$2"; bin="$3"; page="$4"; out="$5"

[ -f "$bin" ] || { echo "::error::no binary at $bin" >&2; exit 1; }
[ -f "$page/index.html" ] || { echo "::error::no $page/index.html: the page is not built" >&2; exit 1; }
[ -d "$out" ] || { echo "::error::no output directory $out" >&2; exit 1; }
out="$(cd "$out" && pwd)"

name=reasonix
[ "$os" = windows ] && name=reasonix.exe

stage="$(mktemp -d)"
trap 'rm -rf "$stage"' EXIT
cp "$bin" "$stage/$name"
chmod 755 "$stage/$name"
mkdir -p "$stage/frontend-next"
cp -R "$page" "$stage/frontend-next/dist"
if [ -n "$(find "$stage/frontend-next" ! -type f ! -type d -print -quit)" ]; then
  echo "::error::the page holds something that is neither a file nor a directory" >&2
  exit 1
fi
find "$stage/frontend-next" -type d -exec chmod 755 {} +
find "$stage/frontend-next" -type f -exec chmod 644 {} +

if [ "$os" = windows ]; then
  (cd "$stage" && zip -qr "$out/reasonix-$os-$arch.zip" "$name" frontend-next)
else
  tar czf "$out/reasonix-$os-$arch.tar.gz" -C "$stage" "$name" frontend-next
fi
