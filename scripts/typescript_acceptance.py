"""Real TS/Python execution and portable viewer; no external services."""
from bundle_fixture import read_fixture, assert_viewer_run
import json
import os
import pathlib
import shutil
import socket
import subprocess
import tempfile
import time
import urllib.error
import urllib.request
import zipfile

ROOT = pathlib.Path(__file__).resolve().parents[1]

def reopen(bundle, expected, directory):
    with socket.socket() as listener:
        listener.bind(("127.0.0.1", 0))
        port = listener.getsockname()[1]
    address = f"http://127.0.0.1:{port}"
    viewer = subprocess.Popen([str(ROOT/"bin/seven-eval"), "--home", str(ROOT), "--package", str(directory), "--addr", f"127.0.0.1:{port}", "serve", str(bundle)],
        env={**os.environ, "SEVEN_EVAL_NODE":"/does-not-exist", "SEVEN_EVAL_PYTHON":"/does-not-exist"}, stdout=subprocess.DEVNULL, stderr=subprocess.PIPE)
    try:
        deadline = time.monotonic()+15
        while True:
            assert viewer.poll() is None, "viewer exited"
            try:
                with urllib.request.urlopen(address+"/api/health", timeout=1) as response:
                    assert response.status == 200
                break
            except (urllib.error.URLError, TimeoutError):
                assert time.monotonic()<deadline, "viewer startup timeout"
                time.sleep(.05)
        with urllib.request.urlopen(address+"/api/runs/"+expected["id"], timeout=5) as response:
            assert json.load(response)=={key:value for key,value in expected.items() if key!="cases"}
        assert_viewer_run(address,expected)
        with urllib.request.urlopen(address+"/api/runs/"+expected["id"]+"/report", timeout=5) as response:
            html=response.read().decode()
            assert "team-summary@2.0.0" in html and "TEAM_REVIEW" in html
        return html
    finally:
        if viewer.poll() is None: viewer.terminate()
        try: viewer.communicate(timeout=10)
        except subprocess.TimeoutExpired:
            viewer.kill();viewer.communicate()

def main():
    artifacts=ROOT/"artifacts/t11";artifacts.mkdir(parents=True,exist_ok=True)
    with tempfile.TemporaryDirectory(prefix="seven-typescript-") as temp:
        package=pathlib.Path(temp)/"package"
        shutil.copytree(ROOT/"examples/typescript",package,ignore=shutil.ignore_patterns(".seven-evaluate"))
        command=[str(ROOT/"bin/seven-eval"),"--home",str(ROOT),"--package",str(package)]
        discovered=json.loads(subprocess.check_output(command+["discover","--json"]))
        assert all(a["status"]=="valid" for a in discovered["assets"]), discovered
        assert not any(a["path"].endswith("_summary.ts") for a in discovered["assets"])
        runs={}
        for plan in ["typescript","python","missing","conflict"]:
            result=subprocess.run(command+["run",plan],text=True,capture_output=True)
            assert result.returncode==0,(plan,result.stdout,result.stderr)
            reply=json.loads(result.stdout)
            with zipfile.ZipFile(reply["bundle"]) as archive:
                run=read_fixture(archive)
            runs[plan]=run
            shutil.copy2(reply["bundle"],artifacts/(plan+".seven.eval"))
        ts=runs["typescript"];py=runs["python"]
        assert ts["outputs"][0]["data"]==py["outputs"][0]["data"]
        assert ts["outputs"][0]["report"]==py["outputs"][0]["report"]
        assert ts["conclusions"]==py["conclusions"]
        assert ts["outputs"][0]["processor"]=="team-summary@2.0.0"
        assert not runs["missing"]["outputs"] and len(runs["conflict"]["outputs"])==2
        assert ts["effective"]["node_runtime"]["version"]
        for a,b in zip(ts["cases"],py["cases"]):
            assert a==b, "equivalent replay changed cross-language evidence"
        shutil.rmtree(package)
        bundle=artifacts/"typescript.seven.eval"
        subprocess.run(command+["verify",str(bundle)],check=True,capture_output=True,env={**os.environ,"SEVEN_EVAL_NODE":"/does-not-exist","SEVEN_EVAL_PYTHON":"/does-not-exist"})
        html=reopen(bundle,ts,pathlib.Path(temp)/"clean-viewer")
        (artifacts/"report.html").write_text(html)
        record={"status":"passed","run_id":ts["id"],"bundle":str(bundle),"kpis":ts["outputs"][0]["data"],"python_ts_equivalent":True,"package_removed":True,"full_run_equal_without_node_python":True,"html_available":True,"scope":"local SDK and controlled replay; browser evidence recorded separately"}
        (artifacts/"acceptance.json").write_text(json.dumps(record,ensure_ascii=False,indent=2));print(json.dumps(record,ensure_ascii=False,indent=2))

if __name__=="__main__": main()
