#!/usr/bin/env bash
set -euo pipefail

appimage=${1:?usage: verify-linux-appimage.sh APPIMAGE ARCH}
arch=${2:?usage: verify-linux-appimage.sh APPIMAGE ARCH}

# Each check says what it wanted before it fails, so a red run names the missing
# or wrong thing instead of stopping on a bare test.
fail() {
  echo "AppImage check failed: $*" >&2
  exit 1
}
octets() { dd if="$1" bs=1 skip="$2" count="$3" 2>/dev/null | od -An -tx1 | tr -d ' \n'; }

case "$arch" in
  amd64) machine=3e00 ;;
  arm64) machine=b700 ;;
  *) fail "unsupported architecture '$arch' (want amd64 or arm64)" ;;
esac

if [ ! -s "$appimage" ]; then
  echo "AppImage check failed: wanted a non-empty file at '$appimage'; the directory holds:" >&2
  ls -la "$(dirname "$appimage")" >&2 || true
  exit 1
fi

# An AppImage is an ELF runtime with a type marker at offset 8: "AI" and the type
# byte 2, the squashfs flavour. Offset 18 is the ELF machine, little-endian.
[ "$(octets "$appimage" 0 4)" = 7f454c46 ] || fail "'$appimage' does not start with the ELF magic 7f454c46 (got $(octets "$appimage" 0 4))"
[ "$(octets "$appimage" 8 3)" = 414902 ] || fail "'$appimage' has no AppImage type-2 marker 414902 at offset 8 (got $(octets "$appimage" 8 3))"
[ "$(octets "$appimage" 18 2)" = "$machine" ] || fail "'$appimage' is built for ELF machine $(octets "$appimage" 18 2), want $machine for $arch"

stage=$(mktemp -d)
trap 'rm -rf -- "$stage"' EXIT
cp "$appimage" "$stage/app"
chmod +x "$stage/app"
# Extracting reads the image itself, so it needs neither FUSE nor a display.
(cd "$stage" && ./app --appimage-extract >/dev/null) || fail "'$appimage --appimage-extract' did not run"
root=$stage/squashfs-root
[ -d "$root" ] || fail "extracting '$appimage' produced no squashfs-root"
host=$(find "$root" -type f -path '*/bin/reasonix-studio-host' ! -path '*app.asar*' | head -1)
page=$(find "$root" -type f -path '*/frontend-next/dist/index.html' ! -path '*app.asar*' | head -1)
[ -n "$host" ] || fail "no bin/reasonix-studio-host in the image; its top level holds: $(ls "$root" | tr '\n' ' ')"
[ -x "$host" ] || fail "'${host#"$root"/}' is not executable"
[ -n "$page" ] || fail "no frontend-next/dist/index.html in the image"
[ "$(octets "$host" 18 2)" = "$machine" ] || fail "the kernel '${host#"$root"/}' is ELF machine $(octets "$host" 18 2), want $machine for $arch"
[ -e "$root/AppRun" ] || fail "no AppRun at the top of the image"
echo "AppImage ok: kernel ${host#"$root"/}, page ${page#"$root"/}"
