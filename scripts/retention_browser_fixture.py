"""Prepare one controlled window for before/after retention browser checks.

Creates a fresh ignored directory every time; leaves its package for inspection.
No existing package is reused or removed. Stops only its own preparation daemon.
"""
from datetime import datetime, timedelta, timezone
import json
from pathlib import Path
import subprocess
import tempfile

from live_acceptance import free_port, request, eventually

ROOT=Path(__file__).resolve().parents[1]
BIN=ROOT/'bin/seven-eval'

def main():
    base_dir=ROOT/'artifacts/t12';base_dir.mkdir(parents=True,exist_ok=True)
    directory=Path(tempfile.mkdtemp(prefix='browser-',dir=base_dir))
    package=directory/'package'
    subprocess.run([str(BIN),'init',str(package)],check=True,capture_output=True)
    (package/'plans/production.yaml').write_text('schema_version: evaluate.plan/v1\nid: production\nmode: stream\nstream:\n  unit: trace\n')
    port=free_port();base=f'http://127.0.0.1:{port}'
    command=[str(BIN),'--home',str(ROOT),'--package',str(package),'--addr',f'127.0.0.1:{port}','serve']
    with (directory/'prepare.log').open('w') as log:
        proc=subprocess.Popen(command,stdout=log,stderr=log)
        try:
            eventually(lambda:request(base,'/api/health'))
            event={'schema_version':'evaluate.event/v1','source':'t12-browser','project':'controlled','event_id':'before-cleanup','trace_id':'t12-trace','event_time':(datetime.now(timezone.utc)-timedelta(minutes=5)).isoformat(),'evidence_revision':1,'case':{'id':'t12-case','inputs':'退款需要多久？','references':{'exact':'三个工作日'}},'result':{'response':'三个工作日'}}
            ack=request(base,'/api/ingest/events',{'plan_id':'production','events':[event]});assert ack['accepted']==1
            def finished():
                report=request(base,'/api/live/plans/production/report?range=1h')
                return report if report.get('current') and report['current']['scope']['pending']==0 else None
            report=eventually(finished)
            state={'package':str(package),'port':port,'url':base+'/#/live','window_id':report['current']['id'],'serve_command':command}
            (directory/'fixture.json').write_text(json.dumps(state,ensure_ascii=False,indent=2));print(json.dumps(state,ensure_ascii=False,indent=2))
        finally:
            proc.terminate()
            try:proc.wait(timeout=10)
            except subprocess.TimeoutExpired:proc.kill();proc.wait(timeout=5)

if __name__=='__main__':main()
