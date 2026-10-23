"""T13: use ONLY extracted/installed resources, with no --home override.

Standalone stdlib harness: can be copied into a container without mounting the
repository. All Package/venv/state data is scoped to an owned temporary directory.
"""

import argparse
import contextlib
import hashlib
import json
import os
import platform
import shutil
import socket
import subprocess
import tarfile
import tempfile
import threading
import time
import urllib.request
import zipfile
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from pathlib import Path


def fixture(bundle):
    with zipfile.ZipFile(bundle) as archive:
        run = json.loads(archive.read("run.json"))
        run["cases"] = []
        for name in sorted(archive.namelist()):
            if name.startswith("results/"):
                run["cases"].extend(json.loads(line) for line in archive.open(name))
        return run


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--archive", type=Path, required=True)
    parser.add_argument("--python", required=True)
    parser.add_argument("--output", type=Path, required=True)
    parser.add_argument(
        "--browser-workspace",
        type=Path,
        help="keep explicit owned browser services; caller must terminate recorded PIDs and clean this directory",
    )
    args = parser.parse_args()
    archive = args.archive.resolve()
    output = args.output.resolve()
    output.mkdir(parents=True, exist_ok=True)
    environment = {
        k: v
        for k, v in os.environ.items()
        if k
        not in ["SEVEN_EVAL_HOME", "SEVEN_EVAL_PYTHON", "SEVEN_EVAL_NODE", "PYTHONPATH"]
    }
    environment["PYTHONDONTWRITEBYTECODE"] = "1"
    calls = []

    class Target(BaseHTTPRequestHandler):
        def do_POST(self):
            calls.append(
                json.loads(self.rfile.read(int(self.headers["Content-Length"])))
            )
            body = b'{"answer":"Hello","contexts":[]}'
            self.send_response(200)
            self.send_header("Content-Type", "application/json")
            self.send_header("Content-Length", str(len(body)))
            self.end_headers()
            self.wfile.write(body)

        def log_message(self, *args):
            pass

    target = ThreadingHTTPServer(("127.0.0.1", 0), Target)
    threading.Thread(target=target.serve_forever, daemon=True).start()
    processes = []
    keep = False
    record = None
    manager = None
    if args.browser_workspace:
        temporary = args.browser_workspace.resolve()
        temporary.mkdir(parents=True, exist_ok=False)
        scope = contextlib.nullcontext(str(temporary))
    else:
        manager = tempfile.TemporaryDirectory(prefix="seven-installed-")
        scope = contextlib.nullcontext(manager.name)
    try:
        with scope as directory:
            root = Path(directory)
            with tarfile.open(archive) as release:
                for member in release.getmembers():
                    p = Path(member.name)
                    assert (
                        not p.is_absolute()
                        and ".." not in p.parts
                        and (member.isfile() or member.isdir())
                    ), "unsafe release member"
                release.extractall(root / "extracted")
            source = next((root / "extracted").iterdir())
            source_binary = source / "bin/seven-eval"
            prefix = root / "installed with spaces"
            viewer_prefix = root / "viewer"

            def cli(binary, *arguments, env=None, ok=True):
                result = subprocess.run(
                    [str(binary), *map(str, arguments)],
                    cwd=root,
                    env=env or environment,
                    text=True,
                    capture_output=True,
                    timeout=180,
                    check=False,
                )
                assert (result.returncode == 0) == ok, (
                    arguments,
                    result.returncode,
                    result.stdout,
                    result.stderr,
                )
                return result

            def reply(binary, *arguments, **kwargs):
                return json.loads(cli(binary, *arguments, **kwargs).stdout)

            assert (
                "PYTHON_UNAVAILABLE"
                in cli(
                    source_binary,
                    "install",
                    "--python",
                    "/seven-missing",
                    root / "bad",
                    ok=False,
                ).stderr
            )
            assert not (root / "bad").exists()
            blocked = root / "not-a-directory"
            blocked.write_text("user data")
            assert (
                "INSTALL_PARENT_UNAVAILABLE"
                in cli(
                    source_binary,
                    "install",
                    "--viewer-only",
                    blocked / "child",
                    ok=False,
                ).stderr
            )
            assert blocked.read_text() == "user data"
            cli(source_binary, "install", "--python", args.python, prefix)
            binary = prefix / "bin/seven-eval"
            health = reply(binary, "doctor")
            assert health["resources"] == "verified" and health["mode"] == "execution"
            link = root / "linked-seven-eval"
            link.symlink_to(binary)
            assert reply(link, "doctor")["resources"] == "verified"
            # Losing release.json must fail, not fall back to development discovery.
            metadata = prefix / "release.json"
            saved = metadata.read_bytes()
            metadata.unlink()
            assert (
                "INSTALL_RESOURCES_MISSING"
                in cli(binary, "doctor", "--viewer-only", ok=False).stderr
            )
            metadata.write_bytes(saved)
            # Optional Node absence must not invalidate Python assets or execution.
            missing_node = {**environment, "SEVEN_EVAL_NODE": "/seven-missing"}
            assert (
                "INSTALL_EXISTS"
                in cli(
                    source_binary, "install", "--viewer-only", prefix, ok=False
                ).stderr
            )
            assert (
                "INSTALL_DATA_LOCATION"
                in cli(binary, "init", prefix / "data", ok=False).stderr
            )
            # External base interpreter only; venv was created at the FINAL prefix.
            cfg = (prefix / "sdk/python/.venv/pyvenv.cfg").read_text()
            assert str(root / "extracted") not in cfg
            package = root / "package"
            cli(binary, "init", package)
            assets = reply(binary, "--package", package, "discover", "--json")
            assert any(
                a["kind"] == "plan" and a["status"] == "valid" for a in assets["assets"]
            )
            basic = reply(binary, "--package", package, "run", "smoke")
            assert basic["status"] == "completed" and not calls
            assert (
                reply(binary, "--package", package, "run", "smoke", env=missing_node)[
                    "status"
                ]
                == "completed"
            )
            (package / "adapters").mkdir()
            (package / "adapters/http.yaml").write_text(
                f"id: local-http\ntype: http\nendpoint: http://127.0.0.1:{target.server_port}/respond\nresponse_path: $.answer\ncontexts_path: $.contexts\n"
            )
            (package / "plans/invoke.yaml").write_text(
                "schema_version: evaluate.plan/v1\nid: invoke\nmode: invoke\ndataset: dataset:sample\nsubject: adapter:local-http\n"
            )
            http = reply(binary, "--package", package, "run", "invoke")
            assert http["status"] == "completed" and calls == [
                {"question": "Say hello"}
            ], calls
            demo = root / "demo.seven.eval"
            cli(binary, "--package", package, "export", basic["run_id"], demo)
            shutil.copy2(demo, output / "demo.seven.eval")
            team = root / "team"
            shutil.copytree(prefix / "examples/typescript", team)
            moved = root / "moved-team"
            team.rename(moved)
            discovery = reply(binary, "--package", moved, "discover", "--json")
            assert all(a["status"] == "valid" for a in discovery["assets"]), discovery
            unavailable = reply(
                binary, "--package", moved, "discover", "--json", env=missing_node
            )
            assert any(
                a["path"].endswith("team.ts")
                and a["status"] == "invalid"
                and "NODE_UNAVAILABLE" in a["message"]
                for a in unavailable["assets"]
            )
            assert any(
                a["path"].endswith("python_summary.py") and a["status"] == "valid"
                for a in unavailable["assets"]
            )
            ts = reply(binary, "--package", moved, "run", "typescript")
            expected = fixture(ts["bundle"])
            assert expected["outputs"][0]["processor"] == "team-summary@2.0.0"
            assert (
                expected["outputs"][0]["report"]["kpis"]["coverage"]
                == "/coverage_ratio"
            )
            bundle = root / "typescript.seven.eval"
            cli(binary, "--package", moved, "export", ts["run_id"], bundle)
            shutil.copy2(bundle, output / "typescript.seven.eval")
            # A viewer-only install does not create a venv, even if runtimes don't exist.
            no_runtime = {
                **environment,
                "SEVEN_EVAL_NODE": "/seven-missing",
                "SEVEN_EVAL_PYTHON": "/seven-missing",
                "PATH": "/seven-no-tools",
            }
            cli(
                source_binary, "install", "--viewer-only", viewer_prefix, env=no_runtime
            )
            viewer_binary = viewer_prefix / "bin/seven-eval"
            assert (
                reply(viewer_binary, "doctor", "--viewer-only", env=no_runtime)["mode"]
                == "viewer"
            )
            assert not (viewer_prefix / "sdk/python/.venv").exists()
            shutil.rmtree(package)
            shutil.rmtree(moved)
            assert reply(viewer_binary, "verify", bundle, env=no_runtime)["valid"]

            def serve(program, package_path, bundle_path=None, env=environment):
                with socket.socket() as listener:
                    listener.bind(("127.0.0.1", 0))
                    port = listener.getsockname()[1]
                args = [
                    str(program),
                    "--package",
                    str(package_path),
                    "--addr",
                    f"127.0.0.1:{port}",
                    "serve",
                ]
                if bundle_path:
                    args.append(str(bundle_path))
                process = subprocess.Popen(
                    args,
                    cwd=root,
                    env=env,
                    stdout=subprocess.DEVNULL,
                    stderr=subprocess.DEVNULL,
                    start_new_session=True,
                )
                processes.append(process)
                address = f"http://127.0.0.1:{port}"
                deadline = time.monotonic() + 15
                while True:
                    assert process.poll() is None, "installed server exited"
                    try:
                        with urllib.request.urlopen(
                            address + "/api/health", timeout=1
                        ) as response:
                            assert response.status == 200
                        break
                    except (OSError, TimeoutError):
                        assert time.monotonic() < deadline
                        time.sleep(0.1)
                return process, address

            _, address = serve(viewer_binary, root / "viewer-state", bundle, no_runtime)
            installed_skill = viewer_prefix / "skills/seven-eval-sdk"
            for relative in ["SKILL.md", "references/sdk-contract.md", "assets/team_summary.py", "assets/team_summary.ts"]:
                assert (installed_skill / relative).read_bytes() == (source / "skills/seven-eval-sdk" / relative).read_bytes()
            base = address + "/api/runs/" + expected["id"]
            with urllib.request.urlopen(base) as response:
                assert json.load(response) == {
                    k: v for k, v in expected.items() if k != "cases"
                }
            with urllib.request.urlopen(base + "/cases?offset=0&limit=100") as response:
                assert len(json.load(response)["items"]) == expected["completed"]
            from urllib.parse import quote

            for case in expected["cases"]:
                with urllib.request.urlopen(
                    base + "/cases/detail?case_id=" + quote(case["case"]["id"], safe="")
                ) as response:
                    assert json.load(response) == case
            with urllib.request.urlopen(base + "/report") as response:
                html = response.read().decode()
                assert "team-summary@2.0.0" in html and "TEAM_REVIEW" in html
            (output / "report.html").write_text(html)
            with urllib.request.urlopen(address) as response:
                index_html = response.read()
                assert b"<html" in index_html and b"/favicon.svg" in index_html
            # Exercise assets from the installed viewer, without the source tree.
            assets = viewer_prefix / "web/dist/assets"
            symbols = list(assets.glob("seven-agent-symbol-*.svg"))
            assert len(symbols) == 1, "installed brand symbol missing or ambiguous"
            symbol = symbols[0]
            assert any(
                symbol.name.encode() in script.read_bytes()
                for script in assets.glob("*.js")
            ), "built UI does not reference the brand symbol"
            for path, expected_body in [
                ("/assets/" + symbol.name, symbol.read_bytes()),
                ("/favicon.svg", (viewer_prefix / "web/dist/favicon.svg").read_bytes()),
            ]:
                with urllib.request.urlopen(address + path) as response:
                    assert response.status == 200 and response.read() == expected_body
            # Modified/added user content stops uninstall instead of deleting it.
            user_file = prefix / "user-notes.txt"
            user_file.write_text("keep me")
            assert (
                "UNINSTALL_USER_CONTENT"
                in cli(source_binary, "uninstall", prefix, ok=False).stderr
                and user_file.read_text() == "keep me"
            )
            user_file.unlink()
            # No interpreter installed in the viewer, and no original source mount
            # exists at all when this harness is run inside the Linux container.
            if not args.browser_workspace:
                cli(source_binary, "uninstall", prefix)
                assert not prefix.exists() and bundle.exists()
                cli(source_binary, "install", "--python", args.python, prefix)
                cli(prefix / "bin/seven-eval", "doctor")
                cli(source_binary, "uninstall", prefix)
            record = {
                "status": "passed",
                "platform": platform.platform(),
                "machine": platform.machine(),
                "archive_sha256": hashlib.file_digest(
                    archive.open("rb"), "sha256"
                ).hexdigest(),
                "health": health,
                "installed_only_commands": True,
                "home_override": False,
                "http_calls": len(calls),
                "reference_leaked": False,
                "package_moved": True,
                "package_removed": True,
                "node_missing_python_works": True,
                "viewer_no_python_node": True,
                "full_run_equal": True,
                "kpis": expected["outputs"][0]["data"],
                "run_id": expected["id"],
                "user_content_preserved": True,
                "reinstall": not bool(args.browser_workspace),
                "browser": "NOT_RUN by this script",
                "brand_assets_served": True,
                "brand_symbol": symbol.name,
                "sdk_skill_installed": True,
            }
            if args.browser_workspace:
                browser_package = root / "browser-package"
                shutil.copytree(prefix / "examples/typescript", browser_package)
                _, execution_address = serve(binary, browser_package)
                record["browser_services"] = {
                    "execution": execution_address,
                    "viewer": address,
                    "pids": [p.pid for p in processes],
                    "workspace": str(root),
                }
                keep = True
            (output / "acceptance.json").write_text(
                json.dumps(record, ensure_ascii=False, indent=2) + "\n"
            )
            print(json.dumps(record, ensure_ascii=False, indent=2))
    finally:
        target.shutdown()
        target.server_close()
        if not keep:
            for process in processes:
                if process.poll() is None:
                    process.terminate()
                try:
                    process.wait(timeout=10)
                except subprocess.TimeoutExpired:
                    process.kill()
                    process.wait()
            if args.browser_workspace:
                shutil.rmtree(temporary)
            if manager:
                path = Path(manager.name)
                manager.cleanup()
                if record:
                    record["temporary_data_removed"] = not path.exists()
                    (output / "acceptance.json").write_text(
                        json.dumps(record, ensure_ascii=False, indent=2) + "\n"
                    )


if __name__ == "__main__":
    main()
