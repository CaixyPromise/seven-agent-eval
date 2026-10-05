"""Small disposable actual stream for paginated window UI acceptance.

Starts only its own daemon, prints fixture.json, and stays until Ctrl-C/SIGTERM.
Its entire temporary Package is removed on normal termination.
"""
import argparse
from datetime import datetime,timedelta,timezone
import json
from pathlib import Path
import signal
import subprocess
import tempfile
import time

from live_acceptance import free_port,request,eventually

ROOT=Path(__file__).resolve().parents[1]

def main():
    p=argparse.ArgumentParser();p.add_argument('--event-time');p.add_argument('--count',type=int,default=66);args=p.parse_args()
    assert 1<=args.count<=100
    out=ROOT/'artifacts/t12-online/browser';out.mkdir(parents=True,exist_ok=True)
    def interrupted(signum,frame):raise KeyboardInterrupt()
    signal.signal(signal.SIGTERM,interrupted)
    with tempfile.TemporaryDirectory(prefix='seven-online-browser-') as temp:
        package=Path(temp)/'package';binary=ROOT/'bin/seven-eval'
        subprocess.run([str(binary),'init',str(package)],check=True,capture_output=True)
        (package/'plans/production.yaml').write_text('schema_version: evaluate.plan/v1\nid: production\nmode: stream\nstream:\n  unit: trace\n')
        port=free_port();base=f'http://127.0.0.1:{port}'
        stamp=args.event_time or (datetime.now(timezone.utc)-timedelta(minutes=5)).replace(second=10,microsecond=0).isoformat()
        log=(out/'daemon.log').open('w')
        proc=subprocess.Popen([str(binary),'--home',str(ROOT),'--package',str(package),'--addr',f'127.0.0.1:{port}','serve'],stdout=log,stderr=log)
        try:
            eventually(lambda:request(base,'/api/health'))
            events=[{'schema_version':'evaluate.event/v1','source':'t12-browser','project':'controlled','event_id':'before-cleanup' if i==0 else f'e{i}',
                     'trace_id':'t12-trace' if i==0 else 'long-trace-'+str(i)+'x'*180,'event_time':stamp,'evidence_revision':1,
                     'case':{'id':'t12-case' if i==0 else 'long-case-'+str(i)+'x'*180,'inputs':'退款需要多久？','references':{'exact':'三个工作日'}},
                     'result':{'response':'' if i and i%10==0 else '三个工作日'}} for i in range(args.count)]
            ack=request(base,'/api/ingest/events',{'plan_id':'production','events':events});assert ack['accepted']==args.count
            def finished():
                report=request(base,'/api/live/plans/production/report?range=1h')
                return report if report.get('current') and report['current']['scope']['pending']==0 else None
            report=eventually(finished,timeout=120)
            state={'package':str(package),'pid':proc.pid,'port':port,'url':base+'/#/live','window_id':report['current']['id'],'event_time':stamp,'events':events}
            (out/'fixture.json').write_text(json.dumps(state,ensure_ascii=False,indent=2));print(json.dumps({k:v for k,v in state.items() if k!='events'},ensure_ascii=False),flush=True)
            while True:time.sleep(1)
        except KeyboardInterrupt:pass
        finally:
            proc.terminate()
            try:proc.wait(timeout=15)
            except subprocess.TimeoutExpired:proc.kill();proc.wait(timeout=5)
            log.close()
    print(json.dumps({'fixture_removed':not Path(temp).exists()}),flush=True)

if __name__=='__main__':main()
