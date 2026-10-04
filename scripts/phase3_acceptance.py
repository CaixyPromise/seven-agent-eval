"""Portable RAG acceptance against a real loopback HTTP server; no credentials."""
from bundle_fixture import read_fixture, assert_viewer_run
import json
import os
import pathlib
import shutil
import subprocess
import tempfile
import threading
import zipfile
from http.server import ThreadingHTTPServer
from rag_target import Handler

ROOT=pathlib.Path(__file__).resolve().parents[1]

def main():
    server=ThreadingHTTPServer(("127.0.0.1",0),Handler)
    thread=threading.Thread(target=server.serve_forever);thread.start()
    artifacts=ROOT/"artifacts/phase3";artifacts.mkdir(parents=True,exist_ok=True)
    try:
        with tempfile.TemporaryDirectory(prefix="seven-rag-") as temp:
            package=pathlib.Path(temp)/"moved-package"
            shutil.copytree(ROOT/"examples/rag-http",package,ignore=shutil.ignore_patterns(".seven-evaluate"))
            adapter=package/"adapters/rag.yaml"
            adapter.write_text(adapter.read_text().replace("127.0.0.1:7453",f"127.0.0.1:{server.server_port}"))
            def cli(*args,env=None):
                result=subprocess.run([str(ROOT/"bin/seven-eval"),"--home",str(ROOT),"--package",str(package),*args],capture_output=True,text=True,env=env,check=True)
                return json.loads(result.stdout)
            discovered=cli("discover","--json")
            assert all(a["status"]=="valid" for a in discovered["assets"])
            result=cli("run","rag")
            bundle=pathlib.Path(result["bundle"])
            with zipfile.ZipFile(bundle) as archive:
                run=read_fixture(archive);html=archive.read("report.html")
            assert run["completed"]==5 and run["status"]=="completed"
            assert run["dataset_revision"]["source_id"]=="questions"
            missing=next(c for c in run["cases"] if c["case"]["id"]=="missing-qrels")
            assert all(m["status"]=="not_computable" and m["value"] is None for m in missing["metrics"] if m["metric_id"].startswith("rag."))
            identities={m["parameters"]["k"] for m in run["cases"][0]["metrics"] if m["metric_id"]=="rag.precision"}
            assert identities=={1,5}
            quality=next(o for o in run["outputs"] if o["processor"]=="builtin.basic-summary@1")["data"]["rag_quality"]
            assert [p["k"] for p in quality[0]["points"]]==[1,5]
            for k in (1,5):
                f1=[m["value"] for c in run["cases"] for m in c["metrics"] if m["metric_id"]=="rag.f1" and m["parameters"]["k"]==k and m["status"]=="observed"]
                point=next(p for p in quality[0]["points"] if p["k"]==k)
                assert abs(point["f1"]-sum(f1)/len(f1))<1e-12
                assert point["counts"]["f1"]["observed"]==len(f1)
            assert any(m["id"]=="rag.f1" for m in run["catalogs"]["metrics"])
            dest=artifacts/(run["id"]+".seven.eval");shutil.copy2(bundle,dest)
            (artifacts/"report.html").write_bytes(html)
            shutil.rmtree(package)
            cli("verify",str(dest),env={**os.environ,"SEVEN_EVAL_PYTHON":"/does-not-exist"})
            summary={"status":"passed","scope":"controlled HTTP RAG; not real Seven or model quality","run_id":run["id"],"cases":5,"source_mapping_move":"passed","separate_k":"passed","missing_reference_not_zero":"passed","source_deleted_verify_no_python":"passed","bundle":str(dest)}
            (artifacts/"acceptance.json").write_text(json.dumps(summary,ensure_ascii=False,indent=2))
            print(json.dumps(summary,ensure_ascii=False,indent=2))
    finally:server.shutdown();server.server_close();thread.join()

if __name__=="__main__":main()
