"""Run actual Linux arm64 acceptance with payload/harness only, no source mount."""

import argparse
import json
import subprocess
import time
import uuid
import zipfile
from pathlib import Path
from urllib.parse import quote

ROOT = Path(__file__).resolve().parents[1]


def runtime_free_viewer(archive, bundle):
    image = "debian:bookworm-slim@sha256:3783cc01769c7b2b1b83a5c5ad96c815348e28ed7da68e2e3687004faa906251"
    cached = subprocess.run(
        ["docker", "image", "inspect", image],
        stdout=subprocess.DEVNULL,
        stderr=subprocess.DEVNULL,
        check=False,
        timeout=15,
    )
    if cached.returncode != 0:
        subprocess.run(["docker", "pull", image], check=True, timeout=180)
    container = "seven-eval-viewer-" + uuid.uuid4().hex[:12]
    created = False
    script = """set -eu
for runtime in python python3 node; do
  if command -v "$runtime"; then echo 'unexpected language runtime' >&2; exit 1; fi
done
mkdir /release /state
tar -xzf /release.tar.gz --strip-components=1 -C /release
/release/bin/seven-eval install --viewer-only /viewer
/viewer/bin/seven-eval doctor --viewer-only
exec /viewer/bin/seven-eval --package /state serve /window.seven.eval
"""
    try:
        subprocess.run(
            [
                "docker",
                "create",
                "--name",
                container,
                "--network",
                "none",
                image,
                "bash",
                "-c",
                script,
            ],
            check=True,
        )
        created = True
        subprocess.run(
            ["docker", "cp", str(archive.resolve()), container + ":/release.tar.gz"],
            check=True,
        )
        subprocess.run(
            ["docker", "cp", str(bundle.resolve()), container + ":/window.seven.eval"],
            check=True,
        )
        subprocess.run(["docker", "start", container], check=True)

        def get(path):
            raw = subprocess.check_output(
                [
                    "docker",
                    "exec",
                    container,
                    "bash",
                    "-c",
                    'exec 3<>/dev/tcp/127.0.0.1/7431; printf "GET %s HTTP/1.0\\r\\nHost: localhost\\r\\n\\r\\n" "$1" >&3; cat <&3',
                    "--",
                    path,
                ],
                stderr=subprocess.DEVNULL,
                timeout=15,
            )
            headers, body = raw.split(b"\r\n\r\n", 1)
            assert headers.startswith((b"HTTP/1.0 200", b"HTTP/1.1 200")), headers
            return body

        deadline = time.monotonic() + 30
        while True:
            try:
                get("/api/health")
                break
            except (subprocess.CalledProcessError, ValueError):
                if time.monotonic() > deadline:
                    subprocess.run(["docker", "logs", container], check=False)
                    raise
                time.sleep(0.2)
        with zipfile.ZipFile(bundle) as result:
            expected = json.loads(result.read("run.json"))
            cases = []
            for name in sorted(result.namelist()):
                if name.startswith("results/"):
                    cases.extend(json.loads(line) for line in result.open(name))
        base = "/api/runs/" + expected["id"]
        assert json.loads(get(base)) == expected
        for case in cases:
            assert (
                json.loads(
                    get(
                        base
                        + "/cases/detail?case_id="
                        + quote(case["case"]["id"], safe="")
                    )
                )
                == case
            )
        html = get(base + "/report")
        assert b"team-summary@2.0.0" in html and b"TEAM_REVIEW" in html
        assert b"<html" in get("/")
        return {
            "status": "passed",
            "image_id": subprocess.check_output(
                ["docker", "image", "inspect", "--format", "{{.Id}}", image], text=True
            ).strip(),
            "python_node_installed": False,
            "source_or_package_mounted": False,
            "external_network": "disabled",
            "full_run_equal": True,
            "bundle_origin": str(bundle),
            "run_id": expected["id"],
        }
    finally:
        if created:
            subprocess.run(["docker", "rm", "-f", container], check=True)


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument(
        "--archive",
        type=Path,
        default=ROOT / "artifacts/t13/releases/seven-agent-eval-linux-arm64.tar.gz",
    )
    parser.add_argument(
        "--output", type=Path, default=ROOT / "artifacts/t13/linux-arm64"
    )
    parser.add_argument(
        "--viewer-bundle",
        type=Path,
        help="optional bundle from another OS, e.g. the macOS installation",
    )
    args = parser.parse_args()
    output = args.output.resolve()
    output.mkdir(parents=True, exist_ok=True)
    image = "seven-eval-t13-acceptance:local"
    subprocess.run(
        [
            "docker",
            "build",
            "-f",
            "scripts/installation-linux.Dockerfile",
            "-t",
            image,
            "scripts",
        ],
        cwd=ROOT,
        check=True,
    )
    container = "seven-eval-t13-" + uuid.uuid4().hex[:12]
    created = False
    try:
        subprocess.run(
            [
                "docker",
                "create",
                "--name",
                container,
                "--network",
                "none",
                image,
                "python",
                "/installation_acceptance.py",
                "--archive",
                "/release.tar.gz",
                "--python",
                "/usr/local/bin/python3.11",
                "--output",
                "/results",
            ],
            check=True,
        )
        created = True
        subprocess.run(
            [
                "docker",
                "cp",
                str(args.archive.resolve()),
                container + ":/release.tar.gz",
            ],
            check=True,
        )
        subprocess.run(
            [
                "docker",
                "cp",
                str(ROOT / "scripts/installation_acceptance.py"),
                container + ":/installation_acceptance.py",
            ],
            check=True,
        )
        subprocess.run(["docker", "start", "--attach", container], check=True)
        code = subprocess.check_output(
            ["docker", "inspect", "--format", "{{.State.ExitCode}}", container],
            text=True,
        ).strip()
        if code != "0":
            raise RuntimeError(
                "Linux installation acceptance failed, container exit=" + code
            )
        subprocess.run(
            ["docker", "cp", container + ":/results/.", str(output)], check=True
        )
        metadata = {
            "container": True,
            "architecture": subprocess.check_output(
                [
                    "docker",
                    "image",
                    "inspect",
                    "--format",
                    "{{.Os}}/{{.Architecture}}",
                    image,
                ],
                text=True,
            ).strip(),
            "image_id": subprocess.check_output(
                ["docker", "image", "inspect", "--format", "{{.Id}}", image], text=True
            ).strip(),
            "source_mounted": False,
            "external_network": "disabled",
            "physical_cross_machine": "NOT_RUN",
        }
        (output / "environment.json").write_text(json.dumps(metadata, indent=2) + "\n")
    finally:
        if created:
            subprocess.run(["docker", "rm", "-f", container], check=True)
    viewer = runtime_free_viewer(
        args.archive, args.viewer_bundle or output / "typescript.seven.eval"
    )
    (output / "runtime-free-viewer.json").write_text(
        json.dumps(viewer, indent=2) + "\n"
    )


if __name__ == "__main__":
    main()
