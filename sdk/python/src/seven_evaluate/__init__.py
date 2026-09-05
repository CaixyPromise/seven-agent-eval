"""Seven Evaluate: ordinary callables behind a small, versioned descriptor."""
def _define(kind, **descriptor):
    descriptor = {"kind": kind, "version": "1.0.0", **descriptor}
    def register(fn):
        fn.__seven_descriptor__ = descriptor
        return fn
    return register

def subject(**kwargs):
    return _define("subject", **kwargs)

def evaluator(**kwargs):
    return _define("evaluator", **kwargs)

def processor(**kwargs):
    return _define("processor", **kwargs)

def conclusion(**kwargs):
    return _define("conclusion", **kwargs)

from .exporter import AsyncEEPExporter

__all__ = ["subject", "evaluator", "processor", "conclusion", "AsyncEEPExporter"]
