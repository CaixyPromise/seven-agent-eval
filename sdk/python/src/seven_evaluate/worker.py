"""Bounded one-operation JSON-RPC worker; ordinary stdout is redirected to stderr."""
import asyncio
import contextlib
import importlib.util
import inspect
import json
import pathlib
import sys
from . import basic

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
        return {"protocol_version": "1.0", "capabilities": ["describe", "subject/invoke", "evaluator/evaluate",
                                                          "processor/process", "conclusion/conclude"]}
    if method == "catalogs":
        return basic.CATALOG
    path = params.get("path")
    fn = load(path) if path else None
    if method == "describe":
        return fn.__seven_descriptor__
    ctx = params.get("context", {})
    expected_kind = method.split("/")[0]
    if fn:
        if fn.__seven_descriptor__["kind"] != expected_kind:
            raise ValueError("extension kind mismatch")
        return call(fn, ctx)
    functions = {"evaluator/evaluate": basic.evaluate, "processor/process": basic.process,
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
