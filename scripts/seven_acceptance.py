"""Controlled Seven SSE/Episode acceptance; real Seven credentials not used."""
import json
import os
import pathlib
import shutil
import subprocess
import tempfile
import threading
import zipfile
from http.server import ThreadingHTTPServer
from seven_target import Handler

ROOT = pathlib.Path(__file__).resolve().parents[1]

def main():
    Handler.sessions, Handler.calls = 0, []
    server = ThreadingHTTPServer(("127.0.0.1", 0), Handler)
    thread = threading.Thread(target=server.serve_forever)
    thread.start()
    artifacts = ROOT / "artifacts/phase3"
    artifacts.mkdir(parents=True, exist_ok=True)
    try:
        with tempfile.TemporaryDirectory(prefix="seven-episode-") as temporary:
            package = pathlib.Path(temporary) / "package"
            shutil.copytree(ROOT / "examples/seven-fixture", package, ignore=shutil.ignore_patterns(".seven-evaluate"))
            adapter = package / "adapters/seven.yaml"
            adapter.write_text(adapter.read_text().replace("127.0.0.1:7454", f"127.0.0.1:{server.server_port}"))
            result = subprocess.run([str(ROOT / "bin/seven-eval"), "--home", str(ROOT), "--package", str(package), "run", "episode"], capture_output=True, text=True)
            run_info = json.loads(result.stdout)
            assert result.returncode == 1 and run_info["status"] == "partial", result.stderr
            with zipfile.ZipFile(run_info["bundle"]) as archive:
                run = json.loads(archive.read("run.json"))
            assert run["total"] == 4 and run["completed"] == 3
            assert Handler.sessions == 2 and len(Handler.calls) == 3
            assert Handler.calls[0]["path"] == Handler.calls[1]["path"] != Handler.calls[2]["path"]
            assert all("references" not in call["body"] for call in Handler.calls)
            assert all(call["body"]["text"] != "不得调用" for call in Handler.calls)
            assert [episode["status"] for episode in run["episodes"]] == ["completed", "partial"]
            assert run["cases"][2]["result"]["response"] == "已收到：fail"
            assert run["cases"][0]["result"]["tools"][0]["status"] == "completed"
            assert run["cases"][0]["case"]["metadata"]["turn_id"] == "first"
            destination = artifacts / "episode.seven.eval"
            shutil.copy2(run_info["bundle"], destination)
        verify = subprocess.run([str(ROOT / "bin/seven-eval"), "verify", str(destination)], capture_output=True, text=True,
                                env={**os.environ, "SEVEN_EVAL_PYTHON": "/does-not-exist"}, check=True)
        assert json.loads(verify.stdout)["valid"]
        summary = {"status": "passed", "scope": "controlled Seven Workspace SSE; real development environment NOT RUN",
                   "sessions": 2, "calls": 3, "turns_planned": 4, "turns_retained": 3,
                   "session_isolation_and_order": "passed", "failure_stops_later_turns": "passed",
                   "partial_answer_and_tools": "passed", "verify_without_package_or_python": "passed",
                   "bundle": str(destination)}
        (artifacts / "seven-acceptance.json").write_text(json.dumps(summary, ensure_ascii=False, indent=2))
        print(json.dumps(summary, ensure_ascii=False, indent=2))
    finally:
        server.shutdown(); server.server_close(); thread.join()

if __name__ == "__main__": main()
