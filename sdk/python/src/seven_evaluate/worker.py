"""Bounded one-operation JSON-RPC worker; ordinary stdout is redirected to stderr."""
import asyncio
import contextlib
import importlib.util
import inspect
import json
import pathlib
import sys
from . import basic, rag, judge, agent, reducer

def load(path):
    path = pathlib.Path(path).resolve()
    sys.path.insert(0, str(path.parent))
    spec = importlib.util.spec_from_file_location("seven_extension", path)
    module = importlib.util.module_from_spec(spec)
    with contextlib.redirect_stdout(sys.stderr):
        spec.loader.exec_module(module)
    entries = [v for v in vars(module).values() if callable(v) and hasattr(v, "__seven_descriptor__")]
    if len(entries) != 1:
        raise ValueError("one decorated public asset required")
    return entries[0]

def call(fn, ctx):
    with contextlib.redirect_stdout(sys.stderr):
        value = fn(ctx)
        if inspect.isawaitable(value):
            value = asyncio.run(value)
    return value

def dispatch(method, params):
    if method == "initialize":
        return {"protocol_version": "2.0", "capabilities": ["describe", "subject/create", "subject/cleanup", "subject/invoke", "evaluator/evaluate",
                                                          "processor/reduce", "conclusion/conclude"]}
    if method == "catalogs":
        return {**basic.CATALOG, "metrics":basic.CATALOG["metrics"] + [{"id":mid,"name":mid,"type":"number" if mid.startswith(("rag.","judge.")) or mid=="tool.repeated" else "boolean"} for mid in rag.METRICS+agent.METRICS+["rag.faithfulness","judge.latency.ms","judge.usage.tokens","judge.usage.cost"]],
                "conclusions":basic.CATALOG["conclusions"] + [{"id":"comparison_checked","description":"同数据候选配置的配对比较；不代表通用业务通过或唯一最优"}],
                "reasons":basic.CATALOG["reasons"] + [{"id":code,"description":text} for code,text in [("no_positive_reference","没有正相关参考文档"),("no_citations","没有声明引用"),("judge_unconfigured","未配置模型评分"),("judge_timeout","模型评分超时"),("judge_http_error","模型评分服务未完成请求"),("judge_transport_error","模型评分连接未完成"),("judge_response_invalid","模型评分结果格式无效"),("judge_configuration_invalid","模型评分配置无效")]]}
    path = params.get("path")
    fn = load(path) if path else None
    if method == "describe":
        return fn.__seven_descriptor__
    ctx = params.get("context", {})
    if not path and params.get("asset_id") == "rag" and method == "evaluator/evaluate":
        return rag.evaluate(ctx)
    if not path and params.get("asset_id") == "rag-judge" and method == "evaluator/evaluate":
        return judge.evaluate(ctx)
    if not path and params.get("asset_id") == "rag-conclusion" and method == "conclusion/conclude":
        return rag.conclude(ctx)
    if not path and params.get("asset_id") == "agent" and method == "evaluator/evaluate":
        return agent.evaluate(ctx)
    if not path and params.get("asset_id") == "agent-conclusion" and method == "conclusion/conclude":
        return agent.conclude(ctx)
    expected_kind = method.split("/")[0]
    if fn:
        if fn.__seven_descriptor__["kind"] != expected_kind:
            raise ValueError("extension kind mismatch")
        if method in ("subject/create","subject/cleanup"):
            hook=getattr(fn,"__seven_session_create__" if method=="subject/create" else "__seven_session_cleanup__",None)
            if hook is None: raise ValueError("session lifecycle unavailable")
            return call(hook,ctx)
        return call(fn, ctx)
    functions = {"evaluator/evaluate": basic.evaluate, "processor/reduce": reducer.reduce,
                 "conclusion/conclude": basic.conclude}
    if method not in functions:
        raise ValueError("unsupported method")
    return functions[method](ctx)

def main():
    for line in sys.stdin:
        request = None
        try:
            if len(line.encode()) > 64 * 1024 * 1024:
                raise ValueError("request too large")
            request = json.loads(line)
            if request.get("jsonrpc") != "2.0":
                raise ValueError("JSON-RPC 2.0 required")
            result = dispatch(request["method"], request.get("params", {}))
            response = {"jsonrpc": "2.0", "id": request["id"], "result": result}
            encoded = json.dumps(response, ensure_ascii=False, allow_nan=False)
        except Exception as exc:
            # User/provider strings may contain secrets. Return a classified bounded message.
            encoded = json.dumps({"jsonrpc": "2.0", "id": request.get("id") if request else None,
                                  "error": {"code": -32000, "message": type(exc).__name__ + ": worker operation failed"}})
        sys.stdout.write(encoded + "\n")
        sys.stdout.flush()

if __name__ == "__main__":
    main()
