"""Small, disposable real-server fixture for bounded report browser acceptance."""
from pathlib import Path
import json
import signal
import socket
import subprocess
import tempfile
import threading
import urllib.request
import time

ROOT=Path(__file__).resolve().parents[1]

def main():
    stopped=threading.Event()
    signal.signal(signal.SIGTERM,lambda *_:stopped.set())
    signal.signal(signal.SIGINT,lambda *_:stopped.set())
    with tempfile.TemporaryDirectory(prefix="seven-bounded-browser-") as temp:
        package=Path(temp)/"package"
        command=[str(ROOT/"bin/seven-eval"),"--home",str(ROOT),"--package",str(package)]
        subprocess.run(command+["init",str(package)],check=True,capture_output=True)
        with (package/"datasets/sample/cases.jsonl").open("w") as rows:
            for i in range(65):
                rows.write(json.dumps({"id":f"case-{i:03d}-长标识-"+"trace-"*12,"inputs":{"question":f"第 {i} 条受控问题"},"response":"" if i%10==0 else f"第 {i} 条实际回答","references":{}},ensure_ascii=False)+"\n")
        result=subprocess.run(command+["run","smoke"],capture_output=True,text=True,check=True,timeout=180)
        run=json.loads(result.stdout)
        import zipfile
        with zipfile.ZipFile(run["bundle"]) as archive:
            run["id"]=json.loads(archive.read("run.json"))["id"]
        with socket.socket() as listener:
            listener.bind(("127.0.0.1",0));port=listener.getsockname()[1]
        with (Path(temp)/"server.log").open("w") as log:
            server=subprocess.Popen(command+["--addr",f"127.0.0.1:{port}","serve"],stdout=log,stderr=log)
            try:
                deadline=time.monotonic()+30
                while True:
                    if server.poll() is not None:raise RuntimeError("fixture server exited")
                    try:
                        with urllib.request.urlopen(f"http://127.0.0.1:{port}/api/health",timeout=1):break
                    except OSError:
                        if time.monotonic()>deadline:raise
                        time.sleep(.1)
                print(json.dumps({"pid":server.pid,"package":str(package),"url":f"http://127.0.0.1:{port}/#/reports/{run['id']}","run_id":run["id"],"bundle":run["bundle"]}),flush=True)
                stopped.wait()
            finally:
                if server.poll() is None:server.terminate()
                try:server.wait(timeout=20)
                except subprocess.TimeoutExpired:server.kill();server.wait()

if __name__=="__main__":main()
