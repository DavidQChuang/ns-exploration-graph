#!/usr/bin/env python3
"""Emit a reproducible checksum inventory of Git-tracked release-tree files."""

from __future__ import annotations

import argparse
import hashlib
import json
import re
import subprocess
import sys
from pathlib import Path


ROOT = Path(__file__).resolve().parent.parent
PROHIBITED_SUFFIXES = {
    ".a", ".class", ".db", ".dylib", ".dll", ".exe", ".jar", ".key",
    ".log", ".o", ".obj", ".p12", ".pem", ".pfx", ".pyc", ".so",
    ".sqlite", ".sqlite3",
}
SECRET_PATTERNS = [
    ("private_key_pem", re.compile(rb"-----BEGIN (?:RSA |EC |OPENSSH )?PRIVATE KEY-----")),
    ("aws_access_key_id", re.compile(rb"\b(?:AKIA|ASIA)[A-Z0-9]{16}\b")),
    ("github_token", re.compile(rb"\bgh[pousr]_[A-Za-z0-9]{30,}\b")),
    ("github_fine_grained_token", re.compile(rb"\bgithub_pat_[A-Za-z0-9_]{40,}\b")),
]
SENSITIVE_NAME = re.compile(r"(?:^|[/\\])(?:\.env(?:\.|$)|credentials?(?:\.|$)|id_rsa(?:\.|$)|secrets?\.ya?ml$)", re.I)


def tracked_paths() -> list[str]:
    result = subprocess.run(
        ["git", "ls-files", "-z"], cwd=ROOT, check=True, stdout=subprocess.PIPE
    )
    return [p.decode("utf-8", errors="strict") for p in result.stdout.split(b"\0") if p]


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--output", type=Path, help="write JSON to this path instead of stdout")
    args = parser.parse_args()

    files = []
    findings = []
    for rel in tracked_paths():
        path = Path(rel)
        # The inventory is deliberately source-tree only, even if local files
        # were accidentally added to the Git index.
        if ".local" in path.parts:
            findings.append({"path": rel, "kind": "excluded_local_runtime_path"})
            continue
        raw = (ROOT / path).read_bytes()
        files.append({
            "path": path.as_posix(),
            "size_bytes": len(raw),
            "sha256": hashlib.sha256(raw).hexdigest(),
        })
        suffix = path.suffix.lower()
        if suffix in PROHIBITED_SUFFIXES:
            findings.append({"path": path.as_posix(), "kind": "prohibited_file_type", "suffix": suffix})
        if SENSITIVE_NAME.search(path.as_posix()):
            findings.append({"path": path.as_posix(), "kind": "sensitive_filename"})
        for kind, pattern in SECRET_PATTERNS:
            if pattern.search(raw):
                findings.append({"path": path.as_posix(), "kind": kind})

    payload = {
        "format": "tracked-source-inventory-v1",
        "scope": "Git-tracked files in the working tree; excludes any .local path",
        "file_count": len(files),
        "total_size_bytes": sum(f["size_bytes"] for f in files),
        "files": files,
        "findings": findings,
    }
    output = json.dumps(payload, indent=2, sort_keys=True) + "\n"
    if args.output:
        dest = args.output if args.output.is_absolute() else ROOT / args.output
        dest.parent.mkdir(parents=True, exist_ok=True)
        dest.write_text(output, encoding="utf-8", newline="\n")
    else:
        sys.stdout.write(output)
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
