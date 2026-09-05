"""Credential-free local target for the integration demo."""
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
import json
import time

class Target(BaseHTTPRequestHandler):
    def do_POST(self):
        body = json.loads(self.rfile.read(int(self.headers.get("Content-Length", "0"))))
        if set(body) != {"question"}:
            self.send_error(400, "Only question belongs in the target request")
            return
        time.sleep(.3)
        answers = {"退款需要多久？": "3 个工作日", "配送需要多久？": "2 天"}
        response = {"answer": answers.get(body["question"], "我不知道"), "contexts": [],
                    "usage": {"total_tokens": 20}}
        encoded = json.dumps(response, ensure_ascii=False).encode()
        self.send_response(200)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(encoded)))
        self.end_headers()
        self.wfile.write(encoded)

if __name__ == "__main__":
    print("HTTP fixture: http://127.0.0.1:7432/respond", flush=True)
    ThreadingHTTPServer(("127.0.0.1", 7432), Target).serve_forever()
