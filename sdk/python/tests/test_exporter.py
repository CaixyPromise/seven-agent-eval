import json
import threading
import time
import unittest
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer

from seven_evaluate.exporter import AsyncEEPExporter


class ExporterTest(unittest.TestCase):
    def test_outage_fifo_queue_full_and_per_item_ack(self):
        received = []
        available = threading.Event()

        class Handler(BaseHTTPRequestHandler):
            def log_message(self, *_):
                pass

            def do_POST(self):
                event = json.loads(self.rfile.read(int(self.headers['Content-Length'])))['events'][0]
                if not available.is_set():
                    self.send_response(503)
                    self.end_headers()
                    return
                received.append(event['event_id'])
                self.send_response(202)
                self.end_headers()
                status = 'rejected' if event['event_id'] == 'invalid' else 'accepted'
                self.wfile.write(json.dumps({'items': [{'status': status, 'code': 'EVENT_INVALID' if status == 'rejected' else ''}]}).encode())

        server = ThreadingHTTPServer(('127.0.0.1', 0), Handler)
        thread = threading.Thread(target=server.serve_forever, daemon=True)
        thread.start()
        exporter = AsyncEEPExporter(f'http://127.0.0.1:{server.server_port}', 'production', max_queue=2, timeout=.1)
        try:
            started = time.monotonic()
            self.assertTrue(exporter.emit({'event_id': 'a'}))
            self.assertLess(time.monotonic()-started, .1)
            deadline = time.monotonic()+3
            while exporter.retried == 0 and time.monotonic() < deadline:
                time.sleep(.01)
            self.assertGreater(exporter.retried, 0)
            self.assertTrue(exporter.emit({'event_id': 'b'}))
            self.assertTrue(exporter.emit({'event_id': 'invalid'}))
            self.assertFalse(exporter.emit({'event_id': 'overflow'}))
            available.set()
            deadline = time.monotonic()+5
            while exporter.sent+exporter.rejected < 3 and time.monotonic() < deadline:
                time.sleep(.01)
            self.assertEqual(received, ['a', 'b', 'invalid'])
            self.assertEqual((exporter.sent, exporter.rejected, exporter.dropped), (2, 1, 1))
        finally:
            exporter.close()
            server.shutdown()
            server.server_close()
            thread.join()

    def test_positive_bounds_and_emit_after_close(self):
        with self.assertRaises(ValueError):
            AsyncEEPExporter('http://127.0.0.1:1', 'production', max_queue=0)
        exporter = AsyncEEPExporter('http://127.0.0.1:1', 'production')
        exporter.close()
        self.assertFalse(exporter.emit({'event_id': 'closed'}))
        self.assertEqual(exporter.dropped, 1)
