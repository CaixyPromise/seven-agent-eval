"""Indexed online history + real mixed workers + recovery + explicit retention.

Run after make build. Only the owned TemporaryDirectory is removed. Keeps a
small portable window and measurements, never the historical journal/database.
RSS samples sum the daemon and its observed descendants; this is sampled RSS,
not a guarantee that short-lived worker peaks were observed.
"""
import argparse
from datetime import datetime, timedelta, timezone
import hashlib
import json
import os
from pathlib import Path
import platform
import shutil
import signal
import sqlite3
import subprocess
import tempfile
import threading
import time
import urllib.error
import urllib.request
from urllib.parse import quote
import zipfile

from live_acceptance import free_port, request, eventually
from seven_evaluate.exporter import AsyncEEPExporter

ROOT=Path(__file__).resolve().parents[1]
BIN=ROOT/'bin/seven-eval'

def main():
    parser=argparse.ArgumentParser()
    parser.add_argument('--history',type=int,default=10000)
    parser.add_argument('--seconds',type=int,default=60)
    parser.add_argument('--output',type=Path,default=ROOT/'artifacts/t12-online')
    args=parser.parse_args()
    assert 1<=args.history<=10000 and args.seconds>=1
    out=args.output.resolve();out.mkdir(parents=True,exist_ok=True)
    proof={'status':'running','started_at':datetime.now(timezone.utc).isoformat(),
           'profile':{'history_units':args.history,'sustained_seconds':args.seconds,'pending_limit':64,'input_response_bytes':1024},
           'binary_sha256':hashlib.sha256(BIN.read_bytes()).hexdigest(),
           'hardware':{'platform':platform.platform(),'cpu':subprocess.check_output(['sysctl','-n','machdep.cpu.brand_string'],text=True).strip(),'memory_bytes':int(subprocess.check_output(['sysctl','-n','hw.memsize']))},
           'gates':{'daemon_rss_bytes':1<<30,'sampled_combined_rss_bytes':2<<30,'history_rss_growth_bytes':128<<20,'warm_report_seconds':5},
           'scope':'controlled loopback; historical occupancy has no evaluators; recent scoring runs real Python + Node; not production SLA'}
    path=out/'acceptance.json'
    def save():path.write_text(json.dumps(proof,ensure_ascii=False,indent=2)+'\n')
    def interrupted(signum,frame):raise KeyboardInterrupt(f'signal {signum}')
    prior=signal.signal(signal.SIGTERM,interrupted)
    owned=None;daemon=None;viewer=None;exporter=None;log=None
    stop=threading.Event();root_pid=[None];resource=[]
    def monitor():
        while not stop.is_set():
            pid=root_pid[0]
            if pid:
                try:
                    rows=subprocess.check_output(['ps','-axo','pid=,ppid=,rss='],text=True).splitlines()
                    table={int(a):(int(b),int(c)*1024) for a,b,c in (line.split() for line in rows)}
                    descendants={pid}
                    for _ in range(8):
                        added={p for p,(parent,rss) in table.items() if parent in descendants}
                        if added<=descendants:break
                        descendants|=added
                    daemon_rss=table.get(pid,(0,0))[1]
                    worker_rss=sum(table.get(p,(0,0))[1] for p in descendants if p!=pid)
                    resource.append({'daemon':daemon_rss,'workers':worker_rss,'combined':daemon_rss+worker_rss,'worker_count':len(descendants)-1})
                except (OSError,ValueError):pass
            stop.wait(.1)
    thread=threading.Thread(target=monitor,daemon=True);thread.start()
    try:
        owned=Path(tempfile.mkdtemp(prefix='seven-online-capacity-'))
        binary=owned/'seven-eval'
        shutil.copy2(BIN,binary)
        pkg=owned/'package'
        subprocess.run([str(binary),'init',str(pkg)],check=True,capture_output=True)
        (pkg/'evaluators').mkdir(exist_ok=True)
        (pkg/'evaluators/loadpy.py').write_text("from seven_evaluate import evaluator\n@evaluator(id='loadpy',metrics=['output.present'])\ndef evaluate(ctx):\n return [{'id':ctx['case']['id']+':py','case_id':ctx['case']['id'],'metric_id':'output.present','status':'observed','value':bool(ctx['result'].get('response')),'evaluator':'loadpy@1'}]\n")
        (pkg/'evaluators/loadjs.ts').write_text("import {evaluator} from '@seven-agent/eval';\nexport default evaluator({id:'loadjs',version:'1',metrics:['output.present']},ctx=>[{id:ctx.case.id+':js',case_id:ctx.case.id,metric_id:'output.present',status:'observed',value:Boolean(ctx.result.response),evaluator:'loadjs@1'}]);\n")
        (pkg/'plans/history.yaml').write_text('schema_version: evaluate.plan/v1\nid: history\nmode: stream\nevaluators: []\nprocessors: []\nconclusions: []\nstream:\n  unit: trace\n')
        (pkg/'plans/production.yaml').write_text('schema_version: evaluate.plan/v1\nid: production\nmode: stream\nevaluators: [evaluator:loadpy, evaluator:loadjs]\nstream:\n  unit: trace\n  max_pending: 64\n')
        port=free_port();base=f'http://127.0.0.1:{port}'
        command=[str(binary),'--home',str(ROOT),'--package',str(pkg)]
        log=(out/'daemon.log').open('w')
        def start():
            proc=subprocess.Popen(command+['--addr',f'127.0.0.1:{port}','serve'],stdout=log,stderr=log)
            root_pid[0]=proc.pid
            try:eventually(lambda:request(base,'/api/health'),timeout=120)
            except BaseException:proc.kill();proc.wait();raise
            return proc
        def pending(plan='production'):
            return next(p['pending'] for p in request(base,'/api/live/plans') if p['id']==plan)
        stamp=(datetime.now(timezone.utc)-timedelta(minutes=10)).replace(second=10,microsecond=0).isoformat()
        old_stamp=(datetime.now(timezone.utc)-timedelta(hours=3)).replace(second=10,microsecond=0).isoformat()
        def event(i,history=False,revision=1):
            return {'schema_version':'evaluate.event/v1','source':'t12','project':'controlled','event_id':f'{"h" if history else "e"}{i}-r{revision}','trace_id':f'{"h" if history else "t"}{i}',
                    'event_time':old_stamp if history else stamp,'evidence_revision':revision,'case':{'id':f'c{i}','inputs':'q'},'result':{'response':'x'*1024}}
        def ingest(plan,events):return request(base,'/api/ingest/events',{'plan_id':plan,'events':events})
        def report(plan='production',range='1h'):return request(base,f'/api/live/plans/{plan}/report?range={range}',timeout=300 if plan=='history' else 30)
        def probe(stage):
            report();samples=[]
            for _ in range(3):
                tick=time.monotonic();r=report();samples.append(time.monotonic()-tick)
                assert 'cases' not in r['current'] and not r['current']['scope'].get('facts')
            rss=int(subprocess.check_output(['ps','-o','rss=','-p',str(daemon.pid)],text=True).strip())*1024
            assert max(samples)<proof['gates']['warm_report_seconds'] and rss<proof['gates']['daemon_rss_bytes']
            return {'stage':stage,'daemon_rss_bytes':rss,'warm_report_seconds':samples,'report_bytes':len(json.dumps(r).encode()),'journal_bytes':(pkg/'.seven-evaluate/live/events.jsonl').stat().st_size,'projection_bytes':(pkg/'.seven-evaluate/live/projection.sqlite').stat().st_size}
        daemon=start()
        assert ingest('production',[event(i) for i in range(5)])['accepted']==5
        eventually(lambda:pending()==0,timeout=120)
        probes=[probe('before_history')]
        begin=time.monotonic()
        split=min(1000,args.history)
        for low,high in [(0,split),(split,args.history)]:
            for offset in range(low,high,100):
                ack=ingest('history',[event(i,True) for i in range(offset,min(offset+100,high))]);assert ack['accepted']==min(100,high-offset),ack
            proof['history_stage']={'accepted':high,'pending_after_ingest':pending('history')};save()
            eventually(lambda:pending('history')==0,timeout=600)
            if high>low:probes.append(probe(f'history_{high}'))
        proof['history_seconds']=time.monotonic()-begin
        proof['history_probes']=probes
        assert probes[-1]['daemon_rss_bytes']-probes[0]['daemon_rss_bytes']<proof['gates']['history_rss_growth_bytes']
        # Materialize the old final window before maintenance; no statistical
        # output is invented for its explicitly disabled Processor.
        history=report('history','6h');assert history['current']['scope']['received']==args.history
        assert not history['current']['scope']['statistics_available']
        proof['history_window_id']=history['current']['id']
        request(base,'/api/live/plans/production/pause',{})
        initial=[event(i) for i in range(5,70)]
        ack=ingest('production',initial);assert ack['accepted']==64 and ack['rejected']==1 and ack['items'][-1]['code']=='QUEUE_FULL'
        daemon.kill();daemon.wait(timeout=10);daemon=start()
        ack=ingest('production',initial[:64]);assert ack['duplicate']==64 and pending()==64
        request(base,'/api/live/plans/production/resume',{});tick=time.monotonic()
        eventually(lambda:pending()==0,timeout=180)
        proof['ack_sigkill_ordered_recovery_seconds']=time.monotonic()-tick
        # Check accepted/started order, not merely eventual counts.
        journal=pkg/'.seven-evaluate/live/events.jsonl'
        accepted=[];started=[]
        with journal.open() as f:
            for line in f:
                item=json.loads(line)
                if item.get('plan_id')=='production' and item['type'] in ('event.accepted','event.started'):
                    (accepted if item['type']=='event.accepted' else started).append(item['record']['sequence'])
        assert accepted==started
        proof['ordered_by_durable_sequence']='PASS'
        begin=time.monotonic();acks=[]
        for i in range(args.seconds):
            tick=time.monotonic();ack=ingest('production',[event(100+i)]);assert ack['accepted']==1
            acks.append(time.monotonic()-tick);time.sleep(max(0,1-(time.monotonic()-tick)))
        eventually(lambda:pending()==0,timeout=120)
        proof['sustained']={'elapsed_seconds':time.monotonic()-begin,'nominal_events_per_second':1,'max_ack_seconds':max(acks),'units':args.seconds}
        before=report()['current'];before= request(base,'/api/live/windows/'+before['id'])
        ack=ingest('production',[event(0,revision=2)]);assert ack['accepted']==1
        eventually(lambda:pending()==0,timeout=120)
        after=report()['current'];assert after['scope']['received']==before['scope']['received'] and after['scope']['revision']>before['scope']['revision']
        assert request(base,'/api/live/windows/'+before['id'])==before
        # Exporter retries after daemon outage without blocking the caller.
        daemon.kill();daemon.wait(timeout=10)
        # A cold mixed-language descriptor preparation exceeds 150 ms.
        # This timeout is background transport time, never business latency.
        exporter=AsyncEEPExporter(base+'/api/ingest/events','production',max_queue=4,timeout=2)
        tick=time.monotonic();assert exporter.emit(event(10000));emit=time.monotonic()-tick;assert emit<.1
        eventually(lambda:exporter.retried>0);daemon=start()
        eventually(lambda:exporter.sent==1 or exporter.rejected>0)
        assert exporter.sent==1, {'rejected':exporter.rejected,'receiver':ingest('production',[event(10000)])}
        eventually(lambda:pending()==0,timeout=120)
        proof['exporter']={'emit_ms':emit*1000,'sent':exporter.sent,'retried':exporter.retried,'dropped':exporter.dropped}
        exporter.close();exporter=None
        current=report()['current'];window=request(base,'/api/live/windows/'+current['id'])
        assert window['scope']['received']==70+args.seconds and window['scope']['pending']==0
        assert window['scope']['scored']==70+args.seconds
        with sqlite3.connect(pkg/'.seven-evaluate/live/projection.sqlite') as db:
            assert db.execute('SELECT count(*) FROM live_records WHERE scored=0').fetchone()[0]==0
            proof['reconciled_records']=db.execute('SELECT count(*) FROM live_records').fetchone()[0]
            assert proof['reconciled_records']==args.history+71+args.seconds
        with urllib.request.urlopen(base+'/api/live/windows/'+window['id']+'/export',timeout=30) as res:(out/'window.seven.eval').write_bytes(res.read())
        with zipfile.ZipFile(out/'window.seven.eval') as z:(out/'report.html').write_bytes(z.read('report.html'))
        daemon.terminate();daemon.wait(timeout=15);daemon=None;root_pid[0]=None
        maintenance={**os.environ,'SEVEN_EVAL_NODE':'/missing-node','SEVEN_EVAL_PYTHON':'/missing-python'}
        preview=json.loads(subprocess.check_output(command+['retention','preview',datetime.now(timezone.utc).isoformat()],text=True,env=maintenance))
        assert len(preview['records'])==args.history+71+args.seconds
        before_bytes=journal.stat().st_size
        subprocess.run(command+['retention','apply',preview['digest']],check=True,capture_output=True,env=maintenance)
        proof['retention']={'expired_records':len(preview['records']),'expired_windows':len(preview['windows']),'journal_bytes_before':before_bytes,'journal_bytes_after':journal.stat().st_size}
        daemon=start();assert ingest('production',[event(0)])['duplicate']==1
        revision=event(0,revision=3);ack=ingest('production',[revision]);assert ack['items'][0]['code']=='DATA_EXPIRED'
        try:request(base,'/api/live/windows/'+window['id']);raise AssertionError('expired window survived')
        except urllib.error.HTTPError as e:assert e.code==410
        assert pending()==0
        daemon.terminate();daemon.wait(timeout=15);daemon=None;root_pid[0]=None
        shutil.rmtree(pkg)
        viewer_dir=owned/'viewer';viewer_dir.mkdir();viewer_port=free_port();viewer_base=f'http://127.0.0.1:{viewer_port}'
        subprocess.run([str(binary),'verify',str(out/'window.seven.eval')],check=True,capture_output=True,env=maintenance)
        viewer=subprocess.Popen([str(binary),'--package',str(viewer_dir),'--addr',f'127.0.0.1:{viewer_port}','serve',str(out/'window.seven.eval')],env=maintenance,stdout=log,stderr=log)
        opened=eventually(lambda:request(viewer_base,'/api/runs/'+window['id']),timeout=30)
        assert opened==window
        first=request(viewer_base,'/api/runs/'+window['id']+'/cases?limit=25');assert first['total']==70+args.seconds
        detail=request(viewer_base,'/api/runs/'+window['id']+'/cases/detail?case_id='+quote(first['items'][0]['id'],safe=''));assert len(detail['metrics'])==2
        viewer.terminate();viewer.wait(timeout=15);viewer=None
        proof['portable_reopen_no_package_node_python']='PASS'
    except BaseException:
        proof['status']='failed';raise
    finally:
        stop.set();thread.join(timeout=3)
        if exporter:exporter.close()
        for proc in (daemon,viewer):
            if proc and proc.poll() is None:
                proc.terminate()
                try:proc.wait(timeout=15)
                except subprocess.TimeoutExpired:proc.kill();proc.wait(timeout=5)
        if log:log.close()
        if owned and owned.exists():shutil.rmtree(owned)
        signal.signal(signal.SIGTERM,prior)
        proof['large_data_removed']=owned is not None and not owned.exists()
        proof['resources']={'sample_interval_ms':100,'sample_count':len(resource),
            'daemon_peak_rss_bytes':max((s['daemon'] for s in resource),default=0),
            'worker_peak_sum_rss_bytes':max((s['workers'] for s in resource),default=0),
            'combined_peak_rss_bytes':max((s['combined'] for s in resource),default=0),
            'samples_with_workers':sum(s['worker_count']>0 for s in resource)}
        save()
    assert proof['large_data_removed'] and proof['resources']['samples_with_workers']>0
    assert proof['resources']['daemon_peak_rss_bytes']<proof['gates']['daemon_rss_bytes']
    assert proof['resources']['combined_peak_rss_bytes']<proof['gates']['sampled_combined_rss_bytes']
    proof['status']='passed';proof['finished_at']=datetime.now(timezone.utc).isoformat();save();print(json.dumps(proof,ensure_ascii=False,indent=2))

if __name__=='__main__':main()
