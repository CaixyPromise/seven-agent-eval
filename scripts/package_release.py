"""Build a platform-specific directory distribution; never copy a development venv.

Requires already built UI/TS assets. Python wheels are downloaded with uv.lock
hashes at build time, so the installer needs no network, uv, npm, pnpm or Go.
"""

import argparse
import hashlib
import json
import os
import platform
import shutil
import subprocess
import tarfile
import tempfile
from pathlib import Path

from dependency_notices import collect

ROOT = Path(__file__).resolve().parents[1]


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument(
        "--os",
        choices=["darwin", "linux"],
        default="darwin" if platform.system() == "Darwin" else "linux",
    )
    parser.add_argument(
        "--arch",
        choices=["arm64", "amd64"],
        default="arm64" if platform.machine() in ["arm64", "aarch64"] else "amd64",
    )
    parser.add_argument(
        "--python",
        required=True,
        help="Python 3.11 with pip, used to download locked wheels",
    )
    parser.add_argument("--output", type=Path, default=ROOT / "artifacts/t13/releases")
    args = parser.parse_args()
    subprocess.run(
        [
            args.python,
            "-c",
            "import sys; assert sys.version_info[:2]==(3,11); import pip",
        ],
        check=True,
    )
    output = args.output.resolve()
    output.mkdir(parents=True, exist_ok=True)
    name = f"seven-agent-eval-{args.os}-{args.arch}"
    archive = output / (name + ".tar.gz")
    # Temporary build staging is exclusively owned by this invocation.
    with tempfile.TemporaryDirectory(prefix="seven-release-") as temporary:
        package = Path(temporary) / name
        package.mkdir()

        def copy(relative):
            source = ROOT / relative
            destination = package / relative
            if source.is_dir():
                shutil.copytree(
                    source,
                    destination,
                    ignore=shutil.ignore_patterns(
                        ".venv",
                        "node_modules",
                        "__pycache__",
                        "*.pyc",
                        ".seven-evaluate",
                        ".DS_Store",
                    ),
                )
            else:
                destination.parent.mkdir(parents=True, exist_ok=True)
                shutil.copy2(source, destination)

        for relative in [
            "sdk/python/src",
            "sdk/python/pyproject.toml",
            "sdk/python/LICENSE",
            "sdk/python/uv.lock",
            "sdk/typescript/src",
            "sdk/typescript/package.json",
            "sdk/typescript/LICENSE",
            "sdk/typescript/dist",
            "sdk/typescript/pnpm-lock.yaml",
            "protocol/schemas",
            "protocol/examples",
            "protocol/conformance",
            "builtin",
            "web/dist",
            "examples",
            "README.md",
            "docs",
            "CONTRIBUTING.md",
            "SECURITY.md",
            "CHANGELOG.md",
            "THIRD_PARTY_NOTICES.md",
        ]:
            copy(relative)
        copy("scripts/http_fixture.py")
        copy("scripts/live_fixture.py")
        copy("scripts/rag_target.py")
        for optional in ["LICENSE", "NOTICE"]:
            if (ROOT / optional).is_file():
                copy(optional)
        (package / "bin").mkdir()
        subprocess.run(
            [
                "go",
                "build",
                "-trimpath",
                "-o",
                str(package / "bin/seven-eval"),
                "./cmd/seven-eval",
            ],
            cwd=ROOT,
            check=True,
            env={
                **os.environ,
                "GOOS": args.os,
                "GOARCH": args.arch,
                "CGO_ENABLED": "0",
            },
        )
        requirements = subprocess.check_output(
            [
                "uv",
                "export",
                "--locked",
                "--project",
                "sdk/python",
                "--no-dev",
                "--no-emit-project",
                "--no-header",
                "--no-annotate",
                "--format",
                "requirements.txt",
            ],
            cwd=ROOT,
        )
        (package / "requirements.txt").write_bytes(requirements)
        wheels = package / "wheelhouse"
        wheels.mkdir()
        command = [
            args.python,
            "-m",
            "pip",
            "--isolated",
            "download",
            "--only-binary=:all:",
            "--require-hashes",
            "-r",
            str(package / "requirements.txt"),
            "-d",
            str(wheels),
            "--python-version",
            "3.11",
            "--implementation",
            "cp",
            "--abi",
            "cp311",
        ]
        if args.os == "linux":
            cpu = "aarch64" if args.arch == "arm64" else "x86_64"
            command += [
                "--platform",
                f"manylinux_2_17_{cpu}",
                "--platform",
                f"manylinux2014_{cpu}",
                "--platform",
                f"manylinux_2_28_{cpu}",
            ]
        else:
            command += [
                "--platform",
                "macosx_11_0_arm64" if args.arch == "arm64" else "macosx_10_9_x86_64",
            ]
        subprocess.run(command, check=True)
        collect(package / "third-party", wheels)
        files = []
        for path in sorted(package.rglob("*")):
            if path.is_symlink():
                raise RuntimeError(f"symlink in release: {path.relative_to(package)}")
            if not path.is_file():
                continue
            mode = (
                0o755
                if path.relative_to(package).as_posix() == "bin/seven-eval"
                else 0o644
            )
            path.chmod(mode)
            with path.open("rb") as content:
                checksum = hashlib.file_digest(content, "sha256").hexdigest()
            files.append(
                {
                    "path": path.relative_to(package).as_posix(),
                    "sha256": checksum,
                    "mode": mode,
                }
            )
        diff = subprocess.check_output(
            [
                "git",
                "diff",
                "HEAD",
                "--",
                "cmd",
                "internal",
                "sdk",
                "protocol",
                "web",
                "scripts",
                "Makefile",
            ],
            cwd=ROOT,
        )
        manifest = {
            "version": "1",
            "product": "seven-agent-eval",
            "protocol": "2.0",
            "os": args.os,
            "arch": args.arch,
            "commit": subprocess.check_output(
                ["git", "rev-parse", "HEAD"], cwd=ROOT, text=True
            ).strip(),
            "source_tree": subprocess.check_output(
                ["git", "rev-parse", "HEAD^{tree}"], cwd=ROOT, text=True
            ).strip(),
            "tracked_diff_sha256": hashlib.sha256(diff).hexdigest(),
            "files": files,
        }
        (package / "release.json").write_text(json.dumps(manifest, indent=2) + "\n")
        with tarfile.open(archive, "w:gz") as target:
            target.add(package, arcname=name)
    checksum = hashlib.file_digest(archive.open("rb"), "sha256").hexdigest()
    archive.with_suffix(archive.suffix + ".sha256").write_text(
        checksum + "  " + archive.name + "\n"
    )
    print(
        json.dumps(
            {
                "archive": str(archive),
                "sha256": checksum,
                "os": args.os,
                "arch": args.arch,
                "files": len(files),
            },
            indent=2,
        )
    )


if __name__ == "__main__":
    main()
