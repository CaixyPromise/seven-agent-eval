"""Controlled Workspace SSE subset, not a substitute for real Seven acceptance."""
import argparse
import json
import threading
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer

class Handler(BaseHTTPRequestHandler):
    sessions=0
    calls=[]
    lock=threading.Lock()
    def do_POST(self):
        body=json.loads(self.rfile.read(int(self.headers.get("Content-Length",0))))
        if self.path.endswith("/sessions"):
            with self.lock:type(self).sessions+=1;sid=type(self).sessions
            self.send_response(200);self.send_header("Content-Type","application/json");self.end_headers()
            self.wfile.write(json.dumps({"data":{"id":sid}}).encode());return
        with self.lock:type(self).calls.append({"path":self.path,"body":body})
        self.send_response(200);self.send_header("Content-Type","text/event-stream");self.end_headers()
        def send(event,data):
            self.wfile.write(("event: "+event+"\ndata: "+json.dumps(data,ensure_ascii=False)+"\n\n").encode());self.wfile.flush()
        send("conversation.started",{"ops":[{"op":"part.replace","part":{"id":"answer","type":"message","text":"已收到："}}]})
        send("conversation.patch",{"ops":[{"op":"part.append_text","partId":"answer","text":body.get("text","")},{"op":"part.replace","part":{"id":"tool","type":"tool","toolCallId":"search-1","toolName":"search","state":"completed","input":{"query":"demo"},"output":{"effectStatus":"verified"}}}]})
        send("controlled.unknown",{})
        send("conversation.failed" if body.get("text")=="fail" else "conversation.completed",{"terminalCommit":True})
    def log_message(self,*args):pass

if __name__=="__main__":
    parser=argparse.ArgumentParser();parser.add_argument("--port",type=int,default=7454);args=parser.parse_args()
    server=ThreadingHTTPServer(("127.0.0.1",args.port),Handler);print(json.dumps({"port":server.server_port}),flush=True);server.serve_forever()
