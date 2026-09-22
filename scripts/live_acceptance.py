"""Repeatable phase-2 checks using real daemons, SIGKILL, HTTP and portable ZIP.

Run after make build: uv run --project sdk/python python scripts/live_acceptance.py
Only temporary packages and loopback services are used. No business Subject exists.
"""
from datetime import datetime, timedelta, timezone
from pathlib import Path
import json
import os
import socket
import subprocess
import tempfile
import time
import urllib.request
import zipfile

from seven_evaluate.basic import process, conclude
from seven_evaluate.exporter import AsyncEEPExporter

ROOT = Path(__file__).resolve().parents[1]
BINARY = ROOT / 'bin/seven-eval'
OUT = ROOT / 'artifacts/phase2-closeout'


def free_port():
    with socket.socket() as sock:
        sock.bind(('127.0.0.1', 0))
        return sock.getsockname()[1]


def request(base, path, body=None):
    req = urllib.request.Request(base+path, data=None if body is None else json.dumps(body).encode(),
                                 headers={'Content-Type': 'application/json'})
    with urllib.request.urlopen(req, timeout=15) as res:
        return json.load(res)


def eventually(fn, timeout=40):
    deadline = time.monotonic()+timeout
    while time.monotonic() < deadline:
        try:
            result = fn()
            if result:
                return result
        except (OSError, ValueError):
            pass
        time.sleep(.1)
    raise AssertionError('condition timed out')


def main():
    OUT.mkdir(parents=True, exist_ok=True)
    proof = {}
    with tempfile.TemporaryDirectory(prefix='seven-live-accept-') as temp:
        package = Path(temp)/'package'
        subprocess.run([str(BINARY), 'init', str(package)], check=True, capture_output=True)
        (package/'evaluators').mkdir(exist_ok=True)
        (package/'evaluators/judge.py').write_text(
            "from seven_evaluate import evaluator\n"
            "@evaluator(id='judge', execution_class='judge', metrics=['output.present'])\n"
            "def evaluate(ctx):\n"
            " assert ctx.get('idempotency_key')\n"
            " return [{'id':ctx['case']['id']+':judge','case_id':ctx['case']['id'],"
            "'metric_id':'output.present','status':'observed','value':True,'evaluator':'judge@1'}]\n")
        (package/'plans/production.yaml').write_text(
            'schema_version: evaluate.plan/v1\nid: production\nmode: stream\n'
            'evaluators: [evaluator:basic, evaluator:judge]\nstream:\n  unit: trace\n'
            '  judge_sample_rate: 1\n  judge_per_window_budget: 1\n')
        port = free_port()
        base = f'http://127.0.0.1:{port}'
        log = (OUT/'daemon.log').open('w')
        daemon = None
        exporter = None

        def start():
            proc = subprocess.Popen([str(BINARY), '--home', str(ROOT), '--package', str(package),
                                     '--addr', f'127.0.0.1:{port}', 'serve'], stdout=log, stderr=log)
            eventually(lambda: request(base, '/api/live/plans'))
            return proc

        def report():
            return request(base, '/api/live/plans/production/report?range=1h')

        event_time = (datetime.now(timezone.utc)-timedelta(minutes=5)).replace(second=10, microsecond=0).isoformat()

        def event(index, revision=1):
            return {'schema_version': 'evaluate.event/v1', 'source': 'acceptance', 'project': 'local-only',
                    'event_id': f'event-{index}-r{revision}', 'trace_id': f'trace-{index}',
                    'event_time': event_time, 'evidence_revision': revision,
                    'case': {'id': 'shared-source-case-id', 'inputs': 'refund?', 'references': {'exact': '3 days'}},
                    'result': {'response': '3 days' if index % 2 else '5 days', 'contexts': ['3 days'], 'latency_ms': 999999}}

        try:
            daemon = start()
            request(base, '/api/live/plans/production/pause', {})
            events = [event(i) for i in range(3)]
            ack = request(base, '/api/ingest/events', {'plan_id': 'production', 'events': events})
            assert ack['accepted'] == 3
            assert report()['current']['scope']['pending'] == 3
            daemon.kill()  # ACK is durable; no evaluator has started while paused.
            daemon.wait(timeout=10)
            daemon = start()
            assert request(base, '/api/live/plans')[0]['paused']
            ack = request(base, '/api/ingest/events', {'plan_id': 'production', 'events': events})
            assert ack['duplicate'] == 3 and ack['accepted'] == 0
            proof['ack_sigkill_restart_duplicate'] = 'passed'
            request(base, '/api/live/plans/production/resume', {})
            eventually(lambda: report()['current']['scope']['pending'] == 0)
            before = report()['current']
            assert before['scope']['received'] == 3 and before['scope']['budget_exhausted'] == 2
            assert len({c['case']['id'] for c in before['cases']}) == 3
            assert before['scope']['p95_latency_ms'] < 999999
            proof['ordered_budget_and_latency'] = 'passed'
            revised = event(0, 2)
            revised['result']['response'] = '3 days'
            ack = request(base, '/api/ingest/events', {'plan_id': 'production', 'events': [revised]})
            assert ack['accepted'] == 1
            eventually(lambda: report()['current']['scope']['pending'] == 0)
            after = report()['current']
            assert after['scope']['received'] == 3 and after['scope']['revision'] > before['scope']['revision']
            assert request(base, '/api/live/windows/'+before['id']) == before
            proof['revision_replaces_contribution_and_frozen_snapshot'] = 'passed'

            daemon.kill()
            daemon.wait(timeout=10)
            exporter = AsyncEEPExporter(base+'/api/ingest/events', 'production', max_queue=4, timeout=.15)
            started = time.monotonic()
            assert exporter.emit(event(3)) and exporter.emit(event(4))
            elapsed = time.monotonic()-started
            assert elapsed < .1
            eventually(lambda: exporter.retried > 0)
            daemon = start()
            eventually(lambda: exporter.sent == 2)
            eventually(lambda: report()['current']['scope']['pending'] == 0)
            proof['exporter_outage_recovery'] = {'emit_ms': round(elapsed*1000, 3), 'sent': exporter.sent,
                                                'retried': exporter.retried, 'dropped': exporter.dropped}
            final = report()['current']
            assert final['scope']['received'] == 5 and final['scope']['state'] == 'final'
            assert final['scope']['scored'] == 5
            summary = process({'cases': final['cases'], 'total': final['total'], 'window': final['scope'],
                               'input_digest': final['outputs'][0]['input_digest']})
            assert summary == final['outputs'][0]
            assert conclude({'outputs': [summary], 'cases': final['cases']}) == final['conclusions']
            proof['closed_window_online_offline_exact'] = 'passed'
            with urllib.request.urlopen(base+'/api/live/windows/'+final['id']+'/export', timeout=20) as res:
                (OUT/'window.seven.eval').write_bytes(res.read())
            with zipfile.ZipFile(OUT/'window.seven.eval') as bundle:
                portable = json.loads(bundle.read('run.json'))
                assert portable['outputs'] == final['outputs'] and portable['scope'] == final['scope']
                (OUT/'report.html').write_bytes(bundle.read('report.html'))
            subprocess.run([str(BINARY), 'verify', str(OUT/'window.seven.eval')], check=True, capture_output=True)
            proof['export_integrity'] = 'passed'
        finally:
            if exporter:
                exporter.close()
            if daemon and daemon.poll() is None:
                daemon.terminate()
                daemon.wait(timeout=15)
            log.close()

        viewer_dir = Path(temp)/'clean-viewer'
        viewer_dir.mkdir()
        viewer_port = free_port()
        viewer_log = (OUT/'viewer.log').open('w')
        viewer = subprocess.Popen([str(BINARY), '--home', str(ROOT), '--package', str(viewer_dir),
                                   '--addr', f'127.0.0.1:{viewer_port}', 'serve', str(OUT/'window.seven.eval')],
                                  env={**os.environ, 'SEVEN_EVAL_PYTHON': '/nonexistent/seven-eval-python'},
                                  cwd=viewer_dir, stdout=viewer_log, stderr=viewer_log)
        try:
            opened = eventually(lambda: request(f'http://127.0.0.1:{viewer_port}', '/api/runs/'+final['id']))
            assert opened['scope'] == final['scope'] and opened['catalogs'] == final['catalogs']
            assert opened['outputs'] == final['outputs'] and opened['conclusions'] == final['conclusions']
            proof['clean_viewer_invalid_python_path'] = 'passed'
        finally:
            viewer.terminate()
            viewer.wait(timeout=15)
            viewer_log.close()
    proof['status'] = 'passed'
    proof['scope'] = 'loopback controlled events; not real Seven Agent production or browser acceptance'
    (OUT/'acceptance.json').write_text(json.dumps(proof, indent=2, ensure_ascii=False)+'\n')
    print(json.dumps(proof, indent=2, ensure_ascii=False))


if __name__ == '__main__':
    main()
