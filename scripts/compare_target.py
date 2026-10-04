"""Controlled candidate target: actually varies retrieval by request parameters.

Cost is an explicit synthetic fixture, not real billing. References are rejected.
"""
import argparse
import json
import time
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer


class Handler(BaseHTTPRequestHandler):
    def do_POST(self):
        body=json.loads(self.rfile.read(int(self.headers.get("Content-Length",0))))
        if "references" in body or "qrels" in body:
            self.send_error(400); return
        params=body["parameters"]
        scenario=body.get("scenario")
        if scenario=="error" and params["top_k"]==3:
            self.send_error(503); return
        effective=dict(params)
        if scenario=="mismatch": effective["top_k"]=1
        docs=[{"id":"d1","text":"七号知识文档"},{"id":"noise","text":"无关文档"},{"id":"d2","text":"相关补充"}]
        if effective.get("strategy")=="reversed": docs=list(reversed(docs))
        docs=docs[:effective["top_k"]]
        time.sleep(.005*effective["top_k"])
        response={"answer":" / ".join(d["text"] for d in docs),"retrieval":docs,
                  "usage":{"total_tokens":10*len(docs),"cost":.001*len(docs),"currency":"USD","source":"controlled-synthetic"}}
        if scenario!="unconfirmed":response["effective_parameters"]=effective
        payload=json.dumps(response,ensure_ascii=False).encode()
        self.send_response(200);self.send_header("Content-Type","application/json");self.end_headers();self.wfile.write(payload)

    def log_message(self,*args):pass


if __name__=="__main__":
    parser=argparse.ArgumentParser();parser.add_argument("--port",type=int,default=7455);args=parser.parse_args()
    server=ThreadingHTTPServer(("127.0.0.1",args.port),Handler)
    print(json.dumps({"port":server.server_port}),flush=True)
    server.serve_forever()
