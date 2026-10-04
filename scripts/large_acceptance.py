"""Isolated v2 capacity acceptance. Large data is removed on success AND failure."""
from contextlib import ExitStack
from pathlib import Path
import argparse
import hashlib
import json
import os
import platform
import signal
import socket
import subprocess
import tempfile
import time
import urllib.error
import urllib.request
import zipfile

ROOT = Path(__file__).resolve().parents[1]


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--cases", type=int, default=10000)
    args = parser.parse_args()
    if not 1 <= args.cases <= 10000:
        parser.error("cases must be 1..10000")
    output = ROOT / "artifacts/phase5-large"
    output.mkdir(parents=True, exist_ok=True)
    record = {"status": "failed", "cases": args.cases, "observations": args.cases*100,
              "machine": platform.platform(), "cpu_count": os.cpu_count(),
              "scope": "controlled offline replay + Node evaluator + Python reducer; not a production SLA"}
    with (ROOT/"bin/seven-eval").open("rb") as binary:
        record["binary_sha256"]=hashlib.file_digest(binary,"sha256").hexdigest()
    started = time.monotonic()
    owned = None
    children = []
    def interrupted(signum, _frame):
        raise InterruptedError(f"acceptance interrupted: {signum}")
    signal.signal(signal.SIGTERM, interrupted)
    try:
        with ExitStack() as stack:
            owned = Path(stack.enter_context(tempfile.TemporaryDirectory(prefix="seven-eval-million-")))
            def stop_children():
                for child in children:
                    if child.poll() is None:
                        os.killpg(child.pid, signal.SIGTERM)
                    try:
                        child.communicate(timeout=20)
                    except subprocess.TimeoutExpired:
                        os.killpg(child.pid, signal.SIGKILL)
                        child.communicate()
            # LIFO: exact child PIDs are stopped before their temp files are removed.
            stack.callback(stop_children)
            package = owned/"package"
            for directory in ("datasets/large", "evaluators", "plans", "catalogs"):
                (package/directory).mkdir(parents=True, exist_ok=True)
            (package/"evaluation.yaml").write_text("schema_version: evaluate.package/v1\nid: large\nname: Capacity acceptance\nversion: '1'\n")
            (package/"catalogs/capacity.yaml").write_text(json.dumps({"id":"capacity","version":"1","metrics":[{"id":f"capacity.{j}","name":f"Controlled value {j}","type":"number"} for j in range(100)]}))
            with (package/"datasets/large/cases.jsonl").open("w") as data:
                for i in range(args.cases):
                    data.write(json.dumps({"id":f"case-{i:05d}", "inputs":{"index":i},
                                          "response":"answer-"+str(i)})+"\n")
            (package/"evaluators/hundred.ts").write_text("""
import {evaluator} from "@seven-agent/eval";
export default evaluator({id:"hundred",version:"1",metrics:Array.from({length:100},(_,j)=>"capacity."+j)},ctx=>
 Array.from({length:100},(_,j)=>({id:"metric-"+j,case_id:ctx.case.id,metric_id:"capacity."+j,status:"observed",
 value:(Number((ctx.case.inputs as {index:number}).index)+j)%100,evaluator:"hundred@1",parameters:{}})));
""")
            (package/"plans/large.yaml").write_text("schema_version: evaluate.plan/v1\nid: large\nmode: replay\ndataset: dataset:large\nevaluators: [evaluator:hundred]\ntimeout_seconds: 180\n")
            command = [str(ROOT/"bin/seven-eval"),"--home",str(ROOT),"--package",str(package)]
            with (owned/"run.stdout").open("w") as stdout, (owned/"run.stderr").open("w") as stderr:
                invocation = ["/usr/bin/time","-l",*command,"run","large"] if platform.system()=="Darwin" else command+["run","large"]
                child = subprocess.Popen(invocation, stdout=stdout, stderr=stderr, start_new_session=True)
                children.append(child)
                code = child.wait(timeout=3600)
            record["execute_seconds"] = round(time.monotonic()-started, 3)
            record["exit_code"] = code
            diagnostic = (owned/"run.stderr").read_text()
            if code:
                record["controlled_failure"] = diagnostic[:4000].replace(str(owned),"<temporary>")
            record["resource_measurement"] = "\n".join(line for line in diagnostic.splitlines() if any(key in line for key in ("maximum resident", "peak memory", " real ", "user ", "system ")))
            assert code == 0, "capacity execution failed (temporary diagnostics omitted)"
            reply = json.loads((owned/"run.stdout").read_text())
            assert reply["status"] == "completed", reply["status"]
            bundle = Path(reply["bundle"])
            assert bundle.is_relative_to(owned), "large artifact escaped owned directory"
            with bundle.open("rb") as file:
                record["bundle_sha256"] = hashlib.file_digest(file,"sha256").hexdigest()
            record["bundle_bytes"] = bundle.stat().st_size
            with zipfile.ZipFile(bundle) as archive:
                meta = json.loads(archive.read("run.json"))
                manifest = json.loads(archive.read("manifest.json"))
                assert meta["schema_version"]=="2.0" and "cases" not in meta
                assert meta["completed"]==args.cases==manifest["result_count"]
                assert manifest["input_count"]==args.cases
                record["expanded_bytes"] = sum(item.file_size for item in archive.infolist())
                if args.cases==10000:
                    assert record["expanded_bytes"]>64*1024*1024
                data = meta["outputs"][0]["data"]
                assert data["scored"]==args.cases and data["coverage"]==1
                assert len(data["series"])==100
                for series in data["series"]:
                    assert series["observed"]==args.cases
                    if args.cases%100==0:
                        assert series["mean"]==49.5 and series["median"]==49.5 and series["p95"]==94
                observations=0
                for name in sorted(archive.namelist()):
                    if name.startswith("results/"):
                        with archive.open(name) as lines:
                            for line in lines:
                                observations += len(json.loads(line)["metrics"])
                assert observations==args.cases*100
                record["exact_statistics_checked"] = args.cases%100==0
                html = archive.read("report.html")
                assert len(html)<8*1024*1024
                record["html_sha256"] = hashlib.sha256(html).hexdigest()
            subprocess.run(command+["verify",str(bundle)],check=True,capture_output=True,timeout=600)
            with socket.socket() as sock:
                sock.bind(("127.0.0.1",0))
                port=sock.getsockname()[1]
            address=f"http://127.0.0.1:{port}"
            with (owned/"viewer.log").open("w") as log:
                viewer=subprocess.Popen([str(ROOT/"bin/seven-eval"),"--home",str(ROOT),"--package",str(owned/"viewer"),"--addr",f"127.0.0.1:{port}","serve",str(bundle)],
                    env={**os.environ,"SEVEN_EVAL_NODE":"/not-installed","SEVEN_EVAL_PYTHON":"/not-installed"},stdout=log,stderr=log,start_new_session=True)
                children.append(viewer)
                deadline=time.monotonic()+600
                while True:
                    assert viewer.poll() is None, "independent viewer exited"
                    try:
                        with urllib.request.urlopen(address+"/api/health",timeout=1):
                            break
                    except (urllib.error.URLError,TimeoutError):
                        assert time.monotonic()<deadline, "viewer startup timeout"
                        time.sleep(.2)
                def read(suffix):
                    with urllib.request.urlopen(address+suffix,timeout=15) as response:
                        return response.read()
                base="/api/runs/"+meta["id"]
                assert json.loads(read(base))==meta
                first=json.loads(read(base+"/cases?offset=0&limit=25"))
                last=json.loads(read(base+f"/cases?offset={args.cases-1}&limit=1"))
                assert first["total"]==args.cases and len(first["items"])==min(25,args.cases)
                assert last["items"][0]["id"]==f"case-{args.cases-1:05d}"
                detail=json.loads(read(base+"/cases/detail?case_id="+last["items"][0]["id"]))
                assert len(detail["metrics"])==100
                assert hashlib.sha256(read(base+"/report")).hexdigest()==record["html_sha256"]
                record["independent_viewer_without_workers"]=True
                record["page_bytes"]=len(json.dumps(first).encode())
                record["run_metadata_bytes"]=len(json.dumps(meta).encode())
            record["status"]="passed"
    finally:
        record["seconds"]=round(time.monotonic()-started,3)
        record["large_data_removed"]=owned is not None and not owned.exists()
        (output/f"acceptance-{args.cases}.json").write_text(json.dumps(record,indent=2)+"\n")
        print(json.dumps(record,indent=2),flush=True)


if __name__=="__main__":
    main()
