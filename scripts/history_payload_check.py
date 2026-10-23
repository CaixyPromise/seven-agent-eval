"""Inspect historical ZIP/result payloads omitted by text-diff secret scanners.

Never execute archived content or print evidence values. No history rewriting.
Requires gitleaks on PATH; extracted members use generated names in an owned temp dir.
"""

import io
import json
import re
import subprocess
import tempfile
import zipfile
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]


def main():
    objects = subprocess.check_output(
        ["git", "rev-list", "--objects", "--all"], cwd=ROOT, text=True
    )
    records = []
    failed = False
    with tempfile.TemporaryDirectory(prefix="seven-history-review-") as directory:
        folder = Path(directory)
        for line in objects.splitlines():
            oid, _, name = line.partition(" ")
            if not name.endswith((".zip", ".seven.eval")):
                continue
            size = int(
                subprocess.check_output(["git", "cat-file", "-s", oid], cwd=ROOT)
            )
            if size > 16 * 1024 * 1024:
                raise RuntimeError("historical archive exceeds review limit: " + oid)
            data = subprocess.check_output(["git", "cat-file", "blob", oid], cwd=ROOT)
            record = {
                "object": oid,
                "path": name,
                "bytes": size,
                "members": [],
                "personal_path_members": [],
            }
            with zipfile.ZipFile(io.BytesIO(data)) as archive:
                if (
                    len(archive.infolist()) > 4096
                    or sum(i.file_size for i in archive.infolist()) > 64 * 1024 * 1024
                ):
                    raise RuntimeError(
                        "historical archive exceeds expanded review limit"
                    )
                for index, item in enumerate(archive.infolist()):
                    if item.is_dir():
                        continue
                    body = archive.read(item)
                    record["members"].append(item.filename)
                    if re.search(rb"/(?:Users|home)/[A-Za-z0-9_.-]+/", body):
                        record["personal_path_members"].append(item.filename)
                        failed = True
                    (folder / (oid + "-" + str(index) + ".txt")).write_bytes(body)
            records.append(record)
        scan = subprocess.run(
            ["gitleaks", "dir", "--redact", "--no-banner", str(folder)], check=False
        )
        failed = failed or scan.returncode != 0
    print(
        json.dumps(
            {
                "archives": records,
                "gitleaks_exit_code": scan.returncode,
                "temporary_data_removed": not folder.exists(),
            },
            indent=2,
        )
    )
    return bool(failed)


if __name__ == "__main__":
    raise SystemExit(main())
