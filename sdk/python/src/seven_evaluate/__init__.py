"""Seven Evaluate: ordinary callables behind a small, versioned descriptor."""
def _define(kind, **descriptor):
    descriptor = {"kind": kind, "version": "1.0.0", **descriptor}
    def register(fn):
        fn.__seven_descriptor__ = descriptor
        return fn
    return register

def subject(**kwargs):
    create=kwargs.pop("session_create",None)
    cleanup=kwargs.pop("session_cleanup",None)
    def register(fn):
        fn=_define("subject",**kwargs,**({"session_lifecycle":True} if create else {}))(fn)
        fn.__seven_session_create__=create
        fn.__seven_session_cleanup__=cleanup
        if cleanup:fn.__seven_descriptor__["session_cleanup"]=True
        return fn
    return register

def evaluator(**kwargs):
    return _define("evaluator", **kwargs)

def processor(**kwargs):
    return _define("processor", **kwargs)

def conclusion(**kwargs):
    return _define("conclusion", **kwargs)

from .exporter import AsyncEEPExporter

__all__ = ["subject", "evaluator", "processor", "conclusion", "AsyncEEPExporter"]
