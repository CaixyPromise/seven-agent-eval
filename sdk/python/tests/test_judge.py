import json
import os
import threading
import time
import unittest
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from unittest.mock import patch
from seven_evaluate import judge

class JudgeTest(unittest.TestCase):
    def test_controlled_http_and_invalid(self):
        state={"score":.75,"calls":0}
        class Handler(BaseHTTPRequestHandler):
            def do_POST(self):
                state["calls"]+=1
                body=json.loads(self.rfile.read(int(self.headers["Content-Length"])))
                assert "qrels" not in json.dumps(body)
                payload={"choices":[{"message":{"content":json.dumps({"score":state["score"]})}}],"usage":{"total_tokens":20}}
                self.send_response(200);self.end_headers();self.wfile.write(json.dumps(payload).encode())
            def log_message(self,*args): pass
        server=ThreadingHTTPServer(("127.0.0.1",0),Handler)
        thread=threading.Thread(target=server.serve_forever);thread.start()
        ctx={"case":{"id":"q","references":{"qrels":{"a":1}}},"result":{"response":"ok","retrieval":[{"id":"a","text":"ok"}]},"status":"completed"}
        try:
            with patch.dict(os.environ,{"SEVEN_EVAL_JUDGE_ENDPOINT":f"http://127.0.0.1:{server.server_port}","SEVEN_EVAL_JUDGE_MODEL":"controlled"}):
                out=judge.evaluate(ctx);self.assertEqual(out[0]["value"],.75);self.assertEqual(out[0]["parameters"]["model"],"controlled")
                state["score"]=2;self.assertEqual(judge.evaluate(ctx)[0]["reason"],"judge_response_invalid")
            with patch.dict(os.environ,{},clear=True):
                self.assertEqual(judge.evaluate(ctx)[0]["reason"],"judge_unconfigured")
            self.assertEqual(state["calls"],2)
        finally: server.shutdown();server.server_close();thread.join()

    def test_http_error_timeout_and_malformed_are_distinct(self):
        class Handler(BaseHTTPRequestHandler):
            def do_POST(self):
                self.rfile.read(int(self.headers["Content-Length"]))
                if self.path=="/slow":
                    time.sleep(.15)
                self.send_response(503 if self.path=="/error" else 200)
                self.end_headers()
                try: self.wfile.write(b"not-json")
                except BrokenPipeError: pass
            def log_message(self,*args): pass
        server=ThreadingHTTPServer(("127.0.0.1",0),Handler)
        thread=threading.Thread(target=server.serve_forever);thread.start()
        ctx={"case":{"id":"q"},"result":{"response":"ok","retrieval":[{"id":"a","text":"ok"}]},"status":"completed","parameters":{"judge":{"timeout_seconds":.05}}}
        try:
            for path,reason in [("error","judge_http_error"),("slow","judge_timeout"),("invalid","judge_response_invalid")]:
                with patch.dict(os.environ,{"SEVEN_EVAL_JUDGE_ENDPOINT":f"http://127.0.0.1:{server.server_port}/{path}","SEVEN_EVAL_JUDGE_MODEL":"controlled"}):
                    observation=judge.evaluate(ctx)[0]
                    self.assertEqual(observation["reason"],reason)
                    self.assertIsNone(observation["value"])
        finally: server.shutdown();server.server_close();thread.join()

    def test_response_read_reset_is_transport_error_without_retry(self):
        ctx={"case":{"id":"q"},"result":{"response":"ok","retrieval":[{"id":"a","text":"ok"}]},"status":"completed"}
        with patch.dict(os.environ,{"SEVEN_EVAL_JUDGE_ENDPOINT":"http://127.0.0.1:1","SEVEN_EVAL_JUDGE_MODEL":"controlled"}), patch("seven_evaluate.judge.urllib.request.build_opener") as opener:
            opener.return_value.open.return_value.__enter__.return_value.read.side_effect=ConnectionResetError("private upstream diagnostic")
            observation=judge.evaluate(ctx)[0]
            self.assertEqual(observation["status"],"error")
            self.assertEqual(observation["reason"],"judge_transport_error")
            self.assertIsNone(observation["value"])
            self.assertNotIn("private upstream",json.dumps(observation))
            opener.return_value.open.assert_called_once()
