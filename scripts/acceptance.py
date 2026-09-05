"""Run phase-1 acceptance using real CLI subprocesses and a local HTTP server."""
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from pathlib import Path
import json
import shutil
import subprocess
import tempfile
import threading
import zipfile

ROOT = Path(__file__).resolve().parents[1]
BINARY = ROOT / "bin/seven-eval"

def command(*args):
    result = subprocess.run([str(BINARY), "--home", str(ROOT), *args],
                            cwd=ROOT, capture_output=True, text=True, timeout=60)
    if result.returncode:
        raise RuntimeError(result.stderr)
    return result.stdout

def main():
    output = ROOT / "artifacts" / "phase1"
    output.mkdir(parents=True, exist_ok=True)
    calls = []
    class Target(BaseHTTPRequestHandler):
        def do_POST(self):
            data = json.loads(self.rfile.read(int(self.headers["Content-Length"])))
            calls.append(data)
            body = json.dumps({"answer": "Hello", "contexts": []}).encode()
            self.send_response(200)
            self.send_header("Content-Type", "application/json")
            self.send_header("Content-Length", str(len(body)))
            self.end_headers()
            self.wfile.write(body)
        def log_message(self, *args):
            pass
    target = ThreadingHTTPServer(("127.0.0.1", 0), Target)
    thread = threading.Thread(target=target.serve_forever, daemon=True)
    thread.start()
    try:
        with tempfile.TemporaryDirectory(prefix="seven-eval-accept-") as temporary:
            package = Path(temporary) / "package"
            command("init", str(package))
            assets = json.loads(command("--package", str(package), "discover", "--json"))
            assert any(a["kind"] == "plan" and a["status"] == "valid" for a in assets["assets"])
            replay = json.loads(command("--package", str(package), "run", "smoke"))
            assert replay["status"] == "completed" and not calls
            (package / "adapters").mkdir()
            (package / "adapters/http.yaml").write_text(
                "id: local-http\ntype: http\nendpoint: http://127.0.0.1:"
                + str(target.server_port) + "/respond\nresponse_path: $.answer\ncontexts_path: $.contexts\n")
            (package / "plans/invoke.yaml").write_text(
                "schema_version: evaluate.plan/v1\nid: invoke\nmode: invoke\n"
                "dataset: dataset:sample\nsubject: adapter:local-http\n")
            active = json.loads(command("--package", str(package), "run", "invoke"))
            assert active["status"] == "completed"
            assert calls == [{"question": "Say hello"}], calls
            for name, run in [("demo", replay), ("http", active)]:
                shutil.copyfile(run["bundle"], output / (name + ".seven.eval"))
            # Original evaluation package and its SQLite/index are removed by TemporaryDirectory.
        verified = json.loads(command("verify", str(output / "demo.seven.eval")))
        assert verified["valid"]

        with zipfile.ZipFile(output / "demo.seven.eval") as bundle:
            (output / "report.html").write_bytes(bundle.read("report.html"))
        report = {"status": "passed", "replay": replay["status"], "http": active["status"],
                  "http_calls": len(calls), "reference_leaked": False,
                  "verified_after_source_package_deleted": True,
                  "discovered_assets": len(assets["assets"]),
                  "bundles": ["demo.seven.eval", "http.seven.eval"]}
        (output / "acceptance.json").write_text(json.dumps(report, indent=2) + "\n")
        print(json.dumps(report, indent=2))
    finally:
        target.shutdown()
        target.server_close()
        thread.join()

if __name__ == "__main__":
    main()
