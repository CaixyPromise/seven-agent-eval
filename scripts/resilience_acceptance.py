"""T12 local, isolated workload. No production state or external service is used.

Build first. Full defaults: 1,000 cases, 10 observations/case, 60 seconds live.
Overrides are smoke profiles, never evidence for the full default capacity.
"""
import argparse
from datetime import datetime, timedelta, timezone
import json
import hashlib
import os
from pathlib import Path
import platform
import re
import shutil
import subprocess
import tempfile
import threading
import time
import urllib.error
import zipfile

from live_acceptance import free_port, request, eventually

ROOT=Path(__file__).resolve().parents[1]
BIN=ROOT/'bin/seven-eval'
OUT=ROOT/'artifacts/t12'

PY_EVALUATOR='''from seven_evaluate import evaluator
@evaluator(id="loadpy", metrics=["output.present"])
def evaluate(ctx):
 return [{"id":ctx["case"]["id"]+":py:"+str(i),"case_id":ctx["case"]["id"],"metric_id":"output.present","status":"observed","value":True,"evaluator":"loadpy@1.0.0","parameters":{"slot":i}} for i in range(10)]
'''
JS_EVALUATOR='''import { evaluator } from "@seven-agent/eval";
export default evaluator({id:"loadjs",version:"1.0.0",metrics:["output.present"]},(ctx)=>Array.from({length:10},(_,i)=>({id:ctx.case.id+":js:"+i,case_id:ctx.case.id,metric_id:"output.present",status:"observed",value:true,evaluator:"loadjs@1.0.0",parameters:{slot:i}})));
'''

def timed(command, name, env=None):
    started=time.monotonic()
    proc=subprocess.run(['/usr/bin/time','-l',*map(str,command)],capture_output=True,text=True,env=env)
    (OUT/(name+'.stdout')).write_text(proc.stdout)
    (OUT/(name+'.stderr')).write_text(proc.stderr)
    assert proc.returncode==0,(name,proc.returncode,proc.stderr[-2000:],proc.stdout[-1000:])
    rss=re.search(r'(\d+)\s+maximum resident set size',proc.stderr)
    return json.loads(proc.stdout),{'seconds':time.monotonic()-started,'max_rss_bytes':int(rss[1]) if rss else None,'exit_code':proc.returncode}

def main():
    global OUT
    parser=argparse.ArgumentParser();parser.add_argument('--cases',type=int,default=1000);parser.add_argument('--seconds',type=int,default=60);parser.add_argument('--output',type=Path,default=OUT);args=parser.parse_args()
    OUT=args.output.resolve()
    assert OUT.is_relative_to(ROOT/'artifacts'), 'acceptance output must stay under artifacts/'
    assert 1<=args.cases<=1000 and args.seconds>=1
    OUT.mkdir(parents=True,exist_ok=True)
    # Write a failing/incomplete record first; an interrupted test cannot leave a stale PASS.
    record={'status':'running','started_at':datetime.now(timezone.utc).isoformat(),'profile':{'cases':args.cases,'seconds':args.seconds},'binary_sha256':hashlib.sha256(BIN.read_bytes()).hexdigest(),'hardware':{'platform':platform.platform(),'cpu':subprocess.check_output(['sysctl','-n','machdep.cpu.brand_string'],text=True).strip(),'memory_bytes':int(subprocess.check_output(['sysctl','-n','hw.memsize']))},'offline':{},'limits':{'cases':1000,'bundle_uncompressed_bytes':64<<20,'legacy_10000_cases_1000000_observations':'NOT_SUPPORTED'},'scope':'local replay and loopback stream; macOS command max RSS is not summed concurrent process RSS'}
    path=OUT/'acceptance.json';path.write_text(json.dumps(record,indent=2))
    with tempfile.TemporaryDirectory(prefix='seven-t12-') as temp:
        pkg=Path(temp)/'package'
        subprocess.run([str(BIN),'init',str(pkg)],check=True,capture_output=True)
        command=[str(BIN),'--home',str(ROOT),'--package',str(pkg)]
        (pkg/'evaluators').mkdir(exist_ok=True)
        (pkg/'evaluators/loadpy.py').write_text(PY_EVALUATOR)
        (pkg/'evaluators/loadjs.js').write_text(JS_EVALUATOR)
        dataset=pkg/'datasets/sample/cases.jsonl'
        with dataset.open('w') as f:
            for i in range(args.cases):f.write(json.dumps({'id':f'case-{i}','inputs':{'question':f'q{i}'},'response':'x'*1024})+'\n')
        saved=[]
        for name,evaluators in [('python','loadpy'),('node','loadjs'),('mixed','loadpy, evaluator:loadjs')]:
            (pkg/f'plans/{name}.yaml').write_text(f'schema_version: evaluate.plan/v1\nid: {name}\nmode: replay\ndataset: dataset:sample\nevaluators: [evaluator:{evaluators}]\n')
            reply,stats=timed(command+['run',name],'offline-'+name)
            with zipfile.ZipFile(reply['bundle']) as archive:run=json.loads(archive.read('run.json'))
            assert run['status']=='completed' and run['completed']==args.cases
            count=sum(len(c['metrics']) for c in run['cases'])
            assert count==args.cases*(20 if name=='mixed' else 10)
            assert all(m['status']=='observed' and m['value'] is True for c in run['cases'] for m in c['metrics'])
            assert stats['seconds']<(900 if name=='mixed' else 600)
            assert stats['max_rss_bytes'] is not None and stats['max_rss_bytes']<2*(1<<30)
            stats.update(cases=args.cases,observations=count,bundle_bytes=Path(reply['bundle']).stat().st_size)
            record['offline'][name]=stats;saved.append(run['id'])
            path.write_text(json.dumps(record,indent=2))
        # Hard-limit rejection happens before any Subject; no limit is silently enlarged.
        with dataset.open('w') as f:
            for i in range(1001):f.write(json.dumps({'id':str(i),'inputs':'q','response':'a'})+'\n')
        rejected=subprocess.run(command+['run','python'],text=True,capture_output=True)
        assert rejected.returncode!=0 and '1000' in rejected.stderr,rejected.stderr
        record['over_limit_preflight']='PASS'
        (pkg/'plans/production.yaml').write_text('schema_version: evaluate.plan/v1\nid: production\nmode: stream\nevaluators: [evaluator:loadpy, evaluator:loadjs]\nstream:\n  unit: trace\n  max_pending: 64\n')
        port=free_port();base=f'http://127.0.0.1:{port}';log=(OUT/'daemon.log').open('w');daemon=None
        def start():
            proc=subprocess.Popen(command+['--addr',f'127.0.0.1:{port}','serve'],stdout=log,stderr=log)
            eventually(lambda:request(base,'/api/health'),timeout=30);return proc
        event_time=(datetime.now(timezone.utc)-timedelta(minutes=10)).isoformat()
        def event(i):return {'schema_version':'evaluate.event/v1','source':'t12','project':'controlled','event_id':f'e{i}','trace_id':f't{i}','event_time':event_time,'evidence_revision':1,'case':{'id':f'c{i}','inputs':'q'},'result':{'response':'x'*1024}}
        def pending():return request(base,'/api/live/plans')[0]['pending']
        try:
            daemon=start();request(base,'/api/live/plans/production/pause',{})
            initial=[event(i) for i in range(65)]
            started=time.monotonic();ack=request(base,'/api/ingest/events',{'plan_id':'production','events':initial})
            assert ack['accepted']==64 and ack['rejected']==1 and ack['items'][-1]['code']=='QUEUE_FULL'
            record['batch_ack_seconds']=time.monotonic()-started
            daemon.kill();daemon.wait(timeout=10);daemon=start()
            duplicate=request(base,'/api/ingest/events',{'plan_id':'production','events':initial[:64]})
            assert duplicate['duplicate']==64 and pending()==64
            request(base,'/api/live/plans/production/resume',{});started=time.monotonic()
            eventually(lambda:pending()==0,timeout=120)
            record['backlog_recovery']={'cases':64,'seconds':time.monotonic()-started,'subject_calls':0}
            samples=[];next_id=65;started=time.monotonic();monitor_stop=threading.Event();queue_samples=[]
            def monitor():
                while not monitor_stop.is_set():
                    try:queue_samples.append({'seconds':time.monotonic()-started,'pending':pending()})
                    except (OSError,ValueError):queue_samples.append({'seconds':time.monotonic()-started,'query_failed':True})
                    monitor_stop.wait(1)
            monitor_thread=threading.Thread(target=monitor);monitor_thread.start()
            try:
                for _ in range(args.seconds):
                    tick=time.monotonic();ack=request(base,'/api/ingest/events',{'plan_id':'production','events':[event(next_id)]})
                    ack_seconds=time.monotonic()-tick
                    assert ack['accepted']==1;next_id+=1
                    rss=int(subprocess.check_output(['ps','-o','rss=','-p',str(daemon.pid)],text=True).strip())*1024
                    assert rss<2*(1<<30)
                    samples.append({'seconds':time.monotonic()-started,'ack_seconds':ack_seconds,'daemon_rss_bytes':rss,'journal_bytes':(pkg/'.seven-evaluate/live/events.jsonl').stat().st_size})
                    time.sleep(max(0,1-(time.monotonic()-tick)))
            finally:
                monitor_stop.set();monitor_thread.join(timeout=20);assert not monitor_thread.is_alive()
            eventually(lambda:pending()==0,timeout=120)
            report=request(base,'/api/live/plans/production/report?range=1h');window=report['current']
            assert window['scope']['received']==64+len(samples) and window['scope']['pending']==0
            record['sustained']={'nominal_events_per_second':1,'elapsed_seconds':time.monotonic()-started,'samples':samples,'queue_samples':queue_samples,'received':window['scope']['received'],'p95_scoring_latency_ms':window['scope'].get('p95_latency_ms')}
            import urllib.request
            with urllib.request.urlopen(base+'/api/live/windows/'+window['id']+'/export') as res: (OUT/'window.seven.eval').write_bytes(res.read())
            daemon.terminate();daemon.wait(timeout=15);daemon=None
            cutoff=datetime.now(timezone.utc).isoformat()
            maintenance_env={**os.environ,'SEVEN_EVAL_NODE':'/missing-node','SEVEN_EVAL_PYTHON':'/missing-python'}
            preview=json.loads(subprocess.check_output(command+['retention','preview',cutoff],text=True,env=maintenance_env))
            assert len(preview['records'])==64+len(samples) and len(preview['runs'])==3,preview
            journal=pkg/'.seven-evaluate/live/events.jsonl';before_bytes=journal.stat().st_size
            subprocess.run(command+['retention','apply',preview['digest']],check=True,capture_output=True,env=maintenance_env)
            record['retention']={'expired_records':len(preview['records']),'expired_runs':len(preview['runs']),'digest':preview['digest'],'journal_bytes_before':before_bytes,'journal_bytes_after':journal.stat().st_size}
            daemon=start()
            duplicate=request(base,'/api/ingest/events',{'plan_id':'production','events':initial[:1]});assert duplicate['duplicate']==1
            try:request(base,'/api/live/windows/'+window['id']);raise AssertionError('expired window remained queryable')
            except urllib.error.HTTPError as error:assert error.code==410
            assert pending()==0
        finally:
            if daemon and daemon.poll() is None:
                daemon.terminate()
                try:daemon.wait(timeout=15)
                except subprocess.TimeoutExpired:daemon.kill();daemon.wait(timeout=5)
            log.close()
        # The original Package is now gone, including all extension code and databases.
        shutil.rmtree(pkg)
        viewer=Path(temp)/'viewer';viewer.mkdir()
        env={**os.environ,'SEVEN_EVAL_NODE':'/missing-node','SEVEN_EVAL_PYTHON':'/missing-python'}
        subprocess.run([str(BIN),'verify',str(OUT/'window.seven.eval')],check=True,capture_output=True,env=env)
        port=free_port();base=f'http://127.0.0.1:{port}'
        proc=subprocess.Popen([str(BIN),'--package',str(viewer),'--addr',f'127.0.0.1:{port}','serve',str(OUT/'window.seven.eval')],env=env,stdout=subprocess.DEVNULL,stderr=subprocess.DEVNULL)
        try:
            eventually(lambda:request(base,'/api/health'),timeout=15)
            assert request(base,'/api/runs/'+window['id'])==window
            with urllib.request.urlopen(base+'/api/runs/'+window['id']+'/report') as res:(OUT/'report.html').write_bytes(res.read())
            record['portable_reopen_without_runtimes']='PASS'
        finally:
            proc.terminate();proc.wait(timeout=10)
    record['status']='passed';record['finished_at']=datetime.now(timezone.utc).isoformat();path.write_text(json.dumps(record,indent=2));print(json.dumps(record,indent=2))

if __name__=='__main__':main()
