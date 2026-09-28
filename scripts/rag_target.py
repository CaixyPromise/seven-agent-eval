"""Credential-free loopback RAG target; accepts inputs only, never reference data."""
import argparse
import json
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer

class Handler(BaseHTTPRequestHandler):
    def do_POST(self):
        body=json.loads(self.rfile.read(int(self.headers.get("Content-Length",0))))
        if "references" in body or "qrels" in body:
            self.send_error(400); return
        scenario=body.get("scenario","correct")
        docs=[{"id":"d1","text":"七号文档：示例知识。"},{"id":"d2","text":"相关补充文档。"}]
        if scenario=="wrong-rank": docs=[{"id":"noise","text":"无关文档。"}]+docs[::-1]
        citations=[{"document_id":"missing" if scenario=="bad-citation" else "d1","start":0,"end":5}]
        value={"answer":"" if scenario=="empty-answer" else "七号文档是示例知识。", "retrieval":docs,"citations":citations}
        payload=json.dumps(value,ensure_ascii=False).encode()
        self.send_response(200);self.send_header("Content-Type","application/json");self.end_headers();self.wfile.write(payload)
    def log_message(self,*args): pass

if __name__=="__main__":
    parser=argparse.ArgumentParser();parser.add_argument("--port",type=int,default=7453);args=parser.parse_args()
    server=ThreadingHTTPServer(("127.0.0.1",args.port),Handler)
    print(json.dumps({"port":server.server_port}),flush=True)
    server.serve_forever()
