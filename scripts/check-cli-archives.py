#!/usr/bin/env python3
"""usage: check-cli-archives.py <dir>

The set a release publishes, and what is inside each archive. The remote install
and `reasonix upgrade` pick the executable out by its name, so an archive may
hold exactly one file of that name; the page must be present and must stay under
frontend-next/dist/. Standard library only.
"""
import os
import sys
import tarfile
import zipfile

WANT = [
    "reasonix-darwin-amd64.tar.gz", "reasonix-darwin-arm64.tar.gz",
    "reasonix-linux-amd64.tar.gz", "reasonix-linux-arm64.tar.gz",
    "reasonix-windows-amd64.zip", "reasonix-windows-arm64.zip", "SHA256SUMS",
]


def fail(message):
    print(f"::error::{message}", file=sys.stderr)
    sys.exit(1)


def entries(path):
    """(kind, name) for each entry; kind is 'file', 'dir' or 'other'."""
    out = []
    if path.endswith(".zip"):
        with zipfile.ZipFile(path) as z:
            for info in z.infolist():
                mode = info.external_attr >> 16
                if info.is_dir():
                    kind = "dir"
                elif mode == 0 or (mode & 0o170000) == 0o100000:
                    kind = "file"
                else:
                    kind = "other"
                out.append((kind, info.filename))
    else:
        with tarfile.open(path, "r:gz") as t:
            for member in t.getmembers():
                kind = "file" if member.isreg() else "dir" if member.isdir() else "other"
                out.append((kind, member.name))
    return out


def check(path, directory):
    base = os.path.basename(path)
    exe = "reasonix.exe" if base.endswith(".zip") else "reasonix"
    try:
        listed = entries(path)
    except (tarfile.TarError, zipfile.BadZipFile, OSError, EOFError) as err:
        fail(f"{base} cannot be read: {err}")
    if not listed:
        fail(f"{base} is empty")
    roots = 0
    has_index = False
    for kind, raw in listed:
        name = raw[2:] if raw.startswith("./") else raw
        if kind == "other":
            fail(f"{base}: {name} is not a plain file or directory")
        if name.startswith("/") or "\\" in name or "\x00" in name:
            fail(f"{base}: {name!r} is not a relative archive path")
        parts = name.rstrip("/").split("/")
        if ".." in parts or "." in parts or "" in parts:
            fail(f"{base}: {name} climbs out of or doubles back in the archive")
        if parts[-1] == exe:
            if name != exe or kind != "file":
                fail(f"{base}: {name} is named like the executable but is not the root binary")
            roots += 1
            continue
        if parts[0] != "frontend-next":
            fail(f"{base}: unexpected entry {name}")
        if len(parts) == 1:
            continue
        if parts[1] != "dist":
            fail(f"{base}: unexpected entry {name}")
        if parts == ["frontend-next", "dist", "index.html"] and kind == "file":
            has_index = True
    if roots != 1:
        fail(f"{base} holds {roots} copies of {exe}, want 1")
    if not has_index:
        fail(f"{base} has no frontend-next/dist/index.html")


def main():
    if len(sys.argv) != 2:
        print(__doc__, file=sys.stderr)
        sys.exit(2)
    directory = sys.argv[1]
    for want in WANT:
        if not os.path.isfile(os.path.join(directory, want)):
            fail(f"no {directory}/{want}")
    for want in WANT:
        if want != "SHA256SUMS":
            check(os.path.join(directory, want), directory)


main()
