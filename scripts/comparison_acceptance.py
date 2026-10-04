"""Actual loopback HTTP comparison and portable bundle; no paid external calls."""
from bundle_fixture import read_fixture, assert_viewer_run
import json
import os
import pathlib
import shutil
import socket
import subprocess
import tempfile
import threading
import time
import urllib.error
import urllib.request
import zipfile
from http.server import ThreadingHTTPServer
from compare_target import Handler

ROOT=pathlib.Path(__file__).resolve().parents[1]

class CountingHandler(Handler):
    count=0
    def do_POST(self):
        type(self).count+=1
        super().do_POST()


def reopen_without_python(bundle, expected, directory):
    with socket.socket() as listener:
        listener.bind(("127.0.0.1", 0))
        port = listener.getsockname()[1]
    address = f"http://127.0.0.1:{port}"
    viewer = subprocess.Popen(
        [str(ROOT/"bin/seven-eval"), "--home", str(ROOT), "--package", str(directory),
         "--addr", f"127.0.0.1:{port}", "serve", str(bundle)],
        env={**os.environ, "SEVEN_EVAL_PYTHON": "/does-not-exist"},
        stdout=subprocess.DEVNULL, stderr=subprocess.PIPE,
    )
    try:
        deadline = time.monotonic() + 10
        while True:
            if viewer.poll() is not None:
                raise AssertionError("independent viewer exited before becoming ready")
            try:
                with urllib.request.urlopen(address + "/api/health", timeout=1) as response:
                    assert response.status == 200
                break
            except (urllib.error.URLError, TimeoutError):
                if time.monotonic() >= deadline:
                    raise AssertionError("independent viewer did not become ready")
                time.sleep(.05)
        with urllib.request.urlopen(address + "/api/runs/" + expected["id"], timeout=5) as response:
            assert json.load(response) == {key:value for key,value in expected.items() if key!="cases"}
        assert_viewer_run(address,expected)
        with urllib.request.urlopen(address + "/api/runs/" + expected["id"] + "/report", timeout=5) as response:
            report = response.read().decode()
            assert "Candidate comparison" in report and "Paired statistics" in report
        with urllib.request.urlopen(address, timeout=5) as response:
            assert b"<html" in response.read()
    finally:
        if viewer.poll() is None:
            viewer.terminate()
        try:
            viewer.communicate(timeout=10)
        except subprocess.TimeoutExpired:
            viewer.kill()
            viewer.communicate()


def main():
    CountingHandler.count=0
    target=ThreadingHTTPServer(("127.0.0.1",0),CountingHandler)
    thread=threading.Thread(target=target.serve_forever);thread.start()
    artifacts=ROOT/"artifacts/phase4";artifacts.mkdir(parents=True,exist_ok=True)
    try:
        with tempfile.TemporaryDirectory(prefix="seven-compare-") as temp:
            package=pathlib.Path(temp)/"package"
            shutil.copytree(ROOT/"examples/compare-rag",package,ignore=shutil.ignore_patterns(".seven-evaluate"))
            adapter=package/"adapters/rag.yaml"
            adapter.write_text(adapter.read_text().replace("127.0.0.1:7455",f"127.0.0.1:{target.server_port}"))
            command=[str(ROOT/"bin/seven-eval"),"--home",str(ROOT),"--package",str(package)]
            discovered=json.loads(subprocess.check_output(command+["discover","--json"]))
            assert all(a["status"]=="valid" for a in discovered["assets"])
            result=subprocess.run(command+["run","compare"],text=True,capture_output=True)
            assert result.returncode==1, result.stderr  # Intentional target failures → partial.
            reply=json.loads(result.stdout)
            with zipfile.ZipFile(reply["bundle"]) as archive:
                run=read_fixture(archive);html=archive.read("report.html")
            assert run["status"]=="partial" and run["completed"]==15 and run["total"]==15
            assert CountingHandler.count==15
            assert run["comparison"]["baseline_id"]=="top-1"
            summary=run["outputs"][0]["data"]["comparison"]
            assert len(summary["candidates"])==3
            assert any(p["receipts"].get("mismatch") for p in summary["candidates"])
            assert any(p["receipts"].get("unconfirmed") for p in summary["candidates"])
            assert any(p["receipts"].get("target_confirmed") for p in summary["candidates"])
            assert any(row["metric_id"]=="usage.cost" for row in summary["rows"])
            assert any(row["metric_id"]=="latency.ms" for row in summary["rows"])
            baseline=next(c for c in run["cases"] if c["case"]["id"]=="top-1/normal")
            expanded=next(c for c in run["cases"] if c["case"]["id"]=="top-3/normal")
            assert len(baseline["result"]["retrieval"])==1 and len(expanded["result"]["retrieval"])==3
            assert baseline["case"]["inputs"]==expanded["case"]["inputs"]
            missing=next(c for c in run["cases"] if c["case"]["id"]=="top-3/missing-reference")
            assert all(m["value"] is None for m in missing["metrics"] if m["metric_id"].startswith("rag."))
            destination=artifacts/(run["id"]+".seven.eval");shutil.copy2(reply["bundle"],destination)
            (artifacts/"report.html").write_bytes(html)
            shutil.rmtree(package)
            subprocess.run(command+["verify",str(destination)],env={**os.environ,"SEVEN_EVAL_PYTHON":"/does-not-exist"},check=True,capture_output=True)
            reopen_without_python(destination, run, pathlib.Path(temp)/"clean-viewer")
            record={"status":"passed","run_id":run["id"],"bundle":str(destination),"calls":15,"candidates":3,
                    "target_parameters_actually_vary_retrieval":True,"receipts":"confirmed/mismatch/unconfirmed",
                    "missing_not_zero":True,"portable_without_python":True,
                    "clean_viewer_full_run_equal":True,"clean_viewer_html_available":True,
                    "scope":"controlled loopback HTTP; synthetic cost, not actual model billing or statistical significance"}
            (artifacts/"acceptance.json").write_text(json.dumps(record,ensure_ascii=False,indent=2));print(json.dumps(record,ensure_ascii=False,indent=2))
    finally:target.shutdown();target.server_close();thread.join()


if __name__=="__main__":main()
