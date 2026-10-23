"""Verify an index tree in a temporary repository, with no ignored/source runtime files.

Uses existing toolchains/download caches; not a fresh OS or offline dependency acquisition test.
All large temporary build, install and runtime directories are owned and cleaned in finally.
"""

import argparse
import hashlib
import json
import os
import platform
import subprocess
import tarfile
import tempfile
import time
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--python", required=True, help="external Python 3.11 with pip")
    parser.add_argument("--output", type=Path, default=ROOT / "artifacts/open-source")
    args = parser.parse_args()
    output = args.output.resolve()
    output.mkdir(parents=True, exist_ok=True)
    tree = subprocess.check_output(["git", "write-tree"], cwd=ROOT, text=True).strip()
    result = {
        "tree": tree,
        "platform": platform.platform(),
        "steps": [],
        "status": "running",
        "scope": "clean index export; existing system toolchains and download caches",
        "browser": "NOT_RUN",
        "license": "Apache-2.0",
    }
    environment = {
        k: v
        for k, v in os.environ.items()
        if k
        not in {
            "SEVEN_EVAL_HOME",
            "SEVEN_EVAL_PYTHON",
            "SEVEN_EVAL_NODE",
            "PYTHONPATH",
            "VIRTUAL_ENV",
        }
    }
    environment["PYTHONDONTWRITEBYTECODE"] = "1"
    try:
        with tempfile.TemporaryDirectory(prefix="seven-public-clean-") as directory:
            checkout = Path(directory) / "source"
            checkout.mkdir()
            with tempfile.TemporaryFile() as stream:
                subprocess.run(
                    ["git", "archive", tree], cwd=ROOT, stdout=stream, check=True
                )
                stream.seek(0)
                with tarfile.open(fileobj=stream) as archive:
                    for item in archive.getmembers():
                        p = Path(item.name)
                        if (
                            p.is_absolute()
                            or ".." in p.parts
                            or not (item.isfile() or item.isdir())
                        ):
                            raise RuntimeError("unsafe index export")
                    archive.extractall(checkout)
            # Tooling records Git provenance. A fresh repository ensures no original .git/config is reused.
            for command in [
                ["git", "init", "-q"],
                ["git", "add", "."],
                [
                    "git",
                    "-c",
                    "user.name=Acceptance",
                    "-c",
                    "user.email=acceptance@example.invalid",
                    "commit",
                    "-qm",
                    "Clean index snapshot",
                ],
            ]:
                subprocess.run(command, cwd=checkout, env=environment, check=True)

            def run(label, command):
                start = time.monotonic()
                with (output / (label + ".log")).open("w") as log:
                    process = subprocess.run(
                        command,
                        cwd=checkout,
                        env=environment,
                        stdout=log,
                        stderr=subprocess.STDOUT,
                        timeout=1800,
                        check=False,
                    )
                step = {
                    "name": label,
                    "command": command,
                    "exit_code": process.returncode,
                    "seconds": round(time.monotonic() - start, 3),
                }
                result["steps"].append(step)
                print(json.dumps(step), flush=True)
                if process.returncode:
                    raise RuntimeError(f"{label} failed; inspect {label}.log")

            for label, command in [
                ("setup", ["make", "setup"]),
                ("test", ["make", "test"]),
                ("vet", ["go", "vet", "./..."]),
                ("build", ["make", "build"]),
                ("content", ["python3", "scripts/public_content_check.py"]),
                ("offline", ["python3", "scripts/acceptance.py"]),
                (
                    "rag",
                    [
                        "uv",
                        "run",
                        "--project",
                        "sdk/python",
                        "python",
                        "scripts/phase3_acceptance.py",
                    ],
                ),
                (
                    "typescript",
                    [
                        "uv",
                        "run",
                        "--project",
                        "sdk/python",
                        "python",
                        "scripts/typescript_acceptance.py",
                    ],
                ),
                (
                    "skill",
                    ["python3", "scripts/skill_acceptance.py"],
                ),
                (
                    "compare",
                    [
                        "uv",
                        "run",
                        "--project",
                        "sdk/python",
                        "python",
                        "scripts/comparison_acceptance.py",
                    ],
                ),
                (
                    "live",
                    [
                        "uv",
                        "run",
                        "--project",
                        "sdk/python",
                        "python",
                        "scripts/live_acceptance.py",
                    ],
                ),
                (
                    "seven",
                    [
                        "uv",
                        "run",
                        "--project",
                        "sdk/python",
                        "python",
                        "scripts/seven_acceptance.py",
                    ],
                ),
                (
                    "package",
                    [
                        "python3",
                        "scripts/package_release.py",
                        "--python",
                        args.python,
                        "--output",
                        str(output / "releases"),
                    ],
                ),
            ]:
                run(label, command)
            os_name = "darwin" if platform.system() == "Darwin" else "linux"
            arch = "arm64" if platform.machine() in {"arm64", "aarch64"} else "amd64"
            release = output / "releases" / f"seven-agent-eval-{os_name}-{arch}.tar.gz"
            run(
                "installed",
                [
                    "python3",
                    "scripts/installation_acceptance.py",
                    "--archive",
                    str(release),
                    "--python",
                    args.python,
                    "--output",
                    str(output / "installed"),
                ],
            )
            result["archive_sha256"] = hashlib.file_digest(
                release.open("rb"), "sha256"
            ).hexdigest()
            result["status"] = "passed"
    except Exception:
        result["status"] = "failed"
        raise
    finally:
        result["temporary_data_removed"] = (
            not Path(directory).exists() if "directory" in locals() else True
        )
        (output / "acceptance.json").write_text(json.dumps(result, indent=2) + "\n")


if __name__ == "__main__":
    main()
