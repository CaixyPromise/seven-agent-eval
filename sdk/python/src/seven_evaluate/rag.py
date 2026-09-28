"""Deterministic ranked-retrieval metrics; missing reference is never zero."""
import math
from .basic import metric

RETRIEVAL_METRICS = ["rag.recall", "rag.precision", "rag.f1", "rag.mrr", "rag.ndcg", "rag.map"]
CITATION_METRICS = ["citation.resolvable", "citation.retrieved", "citation.range"]
METRICS = RETRIEVAL_METRICS + CITATION_METRICS

def evaluate(ctx):
    case, result = ctx["case"], ctx["result"]
    cid = case["id"]
    params = ctx.get("parameters") or {}
    ks = params.get("k", [5])
    if type(ks) is int:
        ks = [ks]
    if not isinstance(ks, list) or not ks or any(type(k) is not int or k <= 0 for k in ks):
        raise ValueError("k must contain positive integers")
    refs = case.get("references") or {}
    qrels = refs.get("qrels")
    retrieval_available = "retrieval" in result and result["retrieval"] is not None
    docs = result.get("retrieval") or []
    ids = [d["id"] for d in docs]
    if len(ids) != len(set(ids)):
        raise ValueError("duplicate retrieval document")
    out = []
    def emit(mid, value=None, status="observed", reason="", identity=None):
        m = metric(cid, mid, status, value, reason, identity)
        m["evaluator"] = "builtin.rag-basic@1"
        m["id"] += ":" + str((identity or {}).get("k", "citation"))
        m["evidence_refs"] = [cid + ":retrieval"]
        out.append(m)
    positive = {d for d, grade in (qrels or {}).items() if grade > 0}
    for k in sorted(set(ks)):
        identity = {"k": k, "gain":"linear", "ap_denominator":"all_positive_qrels"}
        reason = "subject_error" if ctx.get("status") != "completed" else "reference_missing" if not qrels else "no_positive_reference" if not positive else "evidence_missing" if not retrieval_available else ""
        if reason:
            for mid in RETRIEVAL_METRICS:
                emit(mid, status="not_computable", reason=reason, identity=identity)
            continue
        ranked = ids[:k]
        hits = [int(d in positive) for d in ranked]
        gains = [qrels.get(d, 0) for d in ranked]
        dcg = sum(g/math.log2(i+2) for i, g in enumerate(gains))
        ideal = sorted(qrels.values(), reverse=True)[:k]
        idcg = sum(g/math.log2(i+2) for i, g in enumerate(ideal))
        ap = sum(sum(hits[:i+1])/(i+1) for i, hit in enumerate(hits) if hit)/len(positive)
        recall, precision = sum(hits)/len(positive), sum(hits)/k
        f1 = 2 * precision * recall / (precision + recall) if precision + recall else 0
        values = [recall, precision, f1,
                  next((1/(i+1) for i, h in enumerate(hits) if h), 0), dcg/idcg, ap]
        for mid, value in zip(RETRIEVAL_METRICS, values):
            emit(mid, value, identity=identity)
    catalog = {d["id"]: d for d in refs.get("documents", [])}
    catalog.update({d["id"]: d for d in docs})
    citations = result.get("citations")
    for mid in CITATION_METRICS:
        if citations is None:
            emit(mid, status="not_computable", reason="evidence_missing")
            continue
        checks = []
        missing = False
        for c in citations:
            doc = catalog.get(c["document_id"])
            if mid == "citation.resolvable":
                checks.append(doc is not None)
            elif mid == "citation.retrieved":
                if not retrieval_available:
                    missing = True
                checks.append(c["document_id"] in ids)
            else:
                start, end = c.get("start"), c.get("end")
                if start is None or end is None or doc is None:
                    missing = True
                checks.append(doc is not None and type(start) is int and type(end) is int
                              and 0 <= start <= end <= len(doc["text"]))
        if missing:
            emit(mid, status="not_computable", reason="evidence_missing")
        elif not checks:
            emit(mid, status="not_applicable", reason="no_citations")
        else:
            emit(mid, all(checks))
    return out

def conclude(ctx):
    summary=next((o["data"] for o in ctx["outputs"] if o["processor"]=="builtin.basic-summary@1"),None)
    if summary is None:
        return [{"handler":"builtin.rag-conclusion@1","code":"dependency_missing","level":"warning","summary":"当前统计没有提供 RAG 汇总，结论不可计算。"}]
    rules=[s for s in summary["series"] if s["metric_id"].startswith("citation.") or s["metric_id"] in ("output.schema","output.exact")]
    failures=sum(s.get("failed",0) for s in rules)
    unavailable=sum(s["total"]-s["observed"] for s in summary["series"] if s["metric_id"].startswith("rag."))
    return [{"handler":"builtin.rag-conclusion@1","code":"rules_failed" if failures else "rules_checked","level":"warning" if failures or unavailable else "info",
             "summary":f"检索指标按各自 K 和有效参考汇总；确定性引用与回答规则有 {failures} 项失败，检索指标有 {unavailable} 项缺少可用证据。没有设置通用业务及格线。"}]
