"""Normalize Windows fnpack Unix modes and refresh its verified app.tgz MD5.

Only touches the supplied build artifact; does not extract to the filesystem.
Fails closed if the current fnpack checksum format changes.
"""
import argparse
import copy
import gzip
import hashlib
import io
import os
import re
import tarfile
from pathlib import Path

def archive(entries):
    output = io.BytesIO()
    with gzip.GzipFile(fileobj=output, mode="wb", mtime=0) as gz:
        with tarfile.open(fileobj=gz, mode="w", format=tarfile.USTAR_FORMAT) as tf:
            for info, content in entries:
                tf.addfile(info, io.BytesIO(content) if info.isfile() else None)
    return output.getvalue()

def read(data):
    entries = []
    with tarfile.open(fileobj=io.BytesIO(data), mode="r:gz") as tf:
        for member in tf.getmembers():
            # fnpack templates have no symlinks; reject unexpected archive forms.
            if member.name.startswith("/") or ".." in Path(member.name).parts:
                raise ValueError("unsafe package path")
            if not (member.isfile() or member.isdir()):
                raise ValueError("unexpected package member type")
            content = tf.extractfile(member).read() if member.isfile() else b""
            entries.append((copy.copy(member), content))
    return entries

def normalize(path):
    outer = read(path.read_bytes())
    files = {info.name: data for info, data in outer if info.isfile()}
    app = files["app.tgz"]
    manifest = files["manifest"].decode("utf-8")
    match = re.search(r"(?m)^checksum\s*=\s*([0-9a-f]{32})\s*$", manifest)
    if not match or match[1] != hashlib.md5(app).hexdigest():
        raise ValueError("fnpack app.tgz checksum contract changed or package corrupted")
    inner = read(app)
    for info, _ in inner:
        info.mode = 0o755 if info.isdir() or info.name in ("bin/netlite", "bin/caddy") else 0o644
    app = archive(inner)
    manifest = manifest[:match.start(1)] + hashlib.md5(app).hexdigest() + manifest[match.end(1):]
    for info, content in outer:
        if info.name == "app.tgz": content = app
        if info.name == "manifest": content = manifest.encode("utf-8")
        info.mode = 0o755 if info.isdir() or info.name.startswith("cmd/") else 0o644
        info.size = len(content) if info.isfile() else 0
        info.uid = info.gid = 0
        info.uname = info.gname = ""
        # Update the corresponding entry, keeping the original order.
        files[info.name] = content
    result = archive([(info, files[info.name]) for info, _ in outer])
    checked = read(result)
    checked_files = {info.name: data for info, data in checked if info.isfile()}
    checksum = re.search(r"(?m)^checksum\s*=\s*([0-9a-f]{32})", checked_files["manifest"].decode())[1]
    assert checksum == hashlib.md5(checked_files["app.tgz"]).hexdigest()
    for info, _ in checked:
        if info.name.startswith("cmd/") and info.isfile(): assert info.mode == 0o755
    for info, _ in read(checked_files["app.tgz"]):
        if info.name in ("bin/netlite", "bin/caddy"): assert info.mode == 0o755
    temp = path.with_suffix(path.suffix + ".tmp")
    temp.write_bytes(result)
    os.replace(temp, path)
    print("Verified FPK: executable modes 0755; app.tgz checksum matches manifest.")

if __name__ == "__main__":
    parser = argparse.ArgumentParser()
    parser.add_argument("package", type=Path)
    normalize(parser.parse_args().package.resolve())

