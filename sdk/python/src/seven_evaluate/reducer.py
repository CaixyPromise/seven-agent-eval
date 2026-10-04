"""Version-2 disk-backed basic Processor. Exact statistics, bounded batches."""
from collections import Counter
from decimal import Decimal
from functools import lru_cache
import json
import math
import pathlib
import sqlite3
from contextlib import contextmanager
import statistics

class OutputBudget:
    """Fail rather than truncate statistics when the bounded report cannot fit."""
    def __init__(self):
        self.remaining = 4 * 1024 * 1024

    def add(self, value):
        self.remaining -= len(json.dumps(value, allow_nan=False).encode())
        if self.remaining < 0:
            raise ValueError("processor report budget exceeded")
        return value


@lru_cache(maxsize=8192)
def numeric(value):
    return Decimal(value)


@contextmanager
def connect(ctx):
    db = sqlite3.connect(pathlib.Path(ctx["workspace"]) / "aggregate.sqlite")
    db.execute("PRAGMA cache_size=-2048")
    db.execute("PRAGMA temp_store=FILE")
    db.create_collation("numeric", lambda a, b: (numeric(a) > numeric(b)) - (numeric(a) < numeric(b)))
    try:
        yield db
        db.commit()
    finally:
        db.close()


def reduce(ctx):
    with connect(ctx) as db:
        phase = ctx["phase"]
        if phase == "initialize":
            db.executescript("""
                CREATE TABLE cases(id TEXT PRIMARY KEY,status TEXT,anomaly INTEGER,scored INTEGER,
                  failed INTEGER,insufficient INTEGER,sampled INTEGER,skipped INTEGER,budget INTEGER,
                  episode TEXT,candidate TEXT,original TEXT,receipt TEXT);
                CREATE TABLE metrics(identity TEXT,case_id TEXT,status TEXT,value TEXT,type TEXT,
                  reason TEXT,candidate TEXT,original TEXT);
                CREATE INDEX metric_identity ON metrics(identity,candidate);
                CREATE INDEX case_candidate ON cases(candidate);
            """)
            return {"processed": 0}
        if phase == "update":
            for c in ctx["cases"]:
                ms = c["metrics"]
                statuses = {m["status"] for m in ms}
                failed = c["status"] == "error" or "error" in statuses
                anomaly = c["status"] != "completed" or "error" in statuses or any(m["status"] == "observed" and m["value"] is False for m in ms)
                metadata = c["case"].get("metadata") or {}
                candidate, original = metadata.get("seven_eval_candidate", ""), metadata.get("seven_eval_case_id", "")
                db.execute("INSERT INTO cases VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?)", (
                    c["case"]["id"], c["status"], anomaly, "observed" in statuses, failed,
                    not failed and "observed" not in statuses and "not_computable" in statuses,
                    any(s not in ("skipped", "budget_exhausted") for s in statuses),
                    "skipped" in statuses, "budget_exhausted" in statuses,
                    metadata.get("episode_id", ""), candidate, original,
                    ((c.get("result") or {}).get("parameter_receipt") or {}).get("state", "unconfirmed")))
                db.executemany("INSERT INTO metrics VALUES(?,?,?,?,?,?,?,?)", (
                    (json.dumps([m["metric_id"], m["evaluator"], m.get("parameters", {})], sort_keys=True),
                     c["case"]["id"], m["status"], json.dumps(m["value"], allow_nan=False),
                     "boolean" if type(m["value"]) is bool else "number" if type(m["value"]) in (int, float) else "other",
                     m.get("reason", ""), candidate, original) for m in ms))
            return {"processed": ctx["state"]["processed"] + len(ctx["cases"])}
        if phase != "finalize":
            raise ValueError("unknown reducer phase")
        db.execute("CREATE INDEX ordered_values ON metrics(identity,candidate,value COLLATE numeric) WHERE status='observed' AND type='number'")
        budget = OutputBudget()
        data = summary(db, ctx["total"], budget=budget)
        if ctx.get("comparison"):
            data["comparison"] = comparison(db, ctx, budget)
            data["series"], data["rag_quality"] = [], []
        if ctx.get("episodes"):
            data["episodes"] = []
            for ep in ctx["episodes"]:
                item = {"id":ep["id"],"status":ep["status"],**summary(db,len(ep["case_ids"]),"episode",ep["id"],budget)}
                data["episodes"].append(budget.add(item))
        if ctx.get("window"):
            data["window"] = window(db, ctx["window"])
            data["scored"] = data["window"]["scored"]
            data["coverage"] = data["scored"]/ctx["total"] if ctx["total"] else None
        return {"processor": "builtin.basic-summary@1", "input_digest": ctx["input_digest"], "data": data,
                "report": {"version": "1", "kpis": {"scored": "/scored", "coverage": "/coverage", "anomalies": "/anomaly_count"}}}


def summary(db, total, field=None, value=None, budget=None):
    budget = budget or OutputBudget()
    from .basic import rag_quality
    where, args = ("", ()) if field is None else (f" WHERE {field}=?", (value,))
    count, scored, anomalies = db.execute("SELECT count(*),coalesce(sum(scored),0),coalesce(sum(anomaly),0) FROM cases"+where, args).fetchone()
    failures = [r[0] for r in db.execute("SELECT id FROM cases"+where+(" AND" if where else " WHERE")+" anomaly=1 ORDER BY rowid LIMIT 100", args)]
    metric_where = "" if field is None else (" WHERE candidate=?" if field == "candidate" else " WHERE case_id IN (SELECT id FROM cases WHERE episode=?)")
    series = []
    # At most the metadata budget's number of identities can be returned; row
    # values remain on disk even for exact median and nearest-rank p95.
    for (key,) in db.execute("SELECT DISTINCT identity FROM metrics"+metric_where+" ORDER BY identity", args):
        predicate = "identity=?" + (" AND candidate=?" if field == "candidate" else " AND case_id IN (SELECT id FROM cases WHERE episode=?)" if field else "")
        bindings = (key,) + args
        states = dict(db.execute("SELECT status,count(*) FROM metrics WHERE "+predicate+" GROUP BY status", bindings))
        observed = states.get("observed", 0)
        mid, evaluator, parameters = json.loads(key)
        stats = {"metric_id": mid, "evaluator": evaluator, "parameters": parameters, "total": sum(states.values()), "observed": observed, "statuses": states}
        types = {r[0] for r in db.execute("SELECT DISTINCT type FROM metrics WHERE "+predicate+" AND status='observed'", bindings)}
        if observed and types == {"boolean"}:
            passed = db.execute("SELECT count(*) FROM metrics WHERE "+predicate+" AND status='observed' AND value='true'", bindings).fetchone()[0]
            stats.update(passed=passed, failed=observed-passed, rate=passed/observed)
        elif observed and types == {"number"}:
            query = "SELECT value FROM metrics WHERE "+predicate+" AND status='observed' AND type='number'"
            mean = statistics.mean(json.loads(r[0]) for r in db.execute(query, bindings))
            def at(index):
                return json.loads(db.execute(query+" ORDER BY value COLLATE numeric LIMIT 1 OFFSET ?", bindings+(index,)).fetchone()[0])
            median = at(observed//2) if observed % 2 else statistics.mean([at(observed//2-1), at(observed//2)])
            stats.update(mean=mean, median=median, p95=at(math.ceil(.95*observed)-1), min=at(0), max=at(observed-1))
        series.append(budget.add(stats))
    return {"total": total, "processed": count, "scored": scored, "pending": max(0, total-count),
            "coverage": scored/total if total else None, "anomaly_count": anomalies,
            "case_statuses": dict(db.execute("SELECT status,count(*) FROM cases"+where+" GROUP BY status", args)),
            "series": series, "rag_quality": rag_quality(series), "failure_ids": failures,
            "failure_ids_truncated": anomalies > len(failures),
            "note": "覆盖率表示产生了有效指标的样本比例，不代表业务正确率。异常样本明细按页查询。"}


def window(db, scope):
    from datetime import datetime
    from collections import defaultdict
    def seconds(s): return datetime.fromisoformat(s.replace("Z", "+00:00")).timestamp()
    facts = scope.get("facts", [])
    count, failed, scored, insufficient, sampled, skipped, budget, anomalies = db.execute("""SELECT count(*),coalesce(sum(failed),0),coalesce(sum(scored AND NOT failed),0),
      coalesce(sum(insufficient),0),coalesce(sum(sampled),0),coalesce(sum(skipped),0),coalesce(sum(budget),0),coalesce(sum(anomaly),0) FROM cases""").fetchone()
    reasons = dict(db.execute("SELECT reason,count(*) FROM metrics WHERE reason!='' GROUP BY reason"))
    stats = dict(received=len(facts), eligible=len(facts), sampled=sampled, scored=scored,
                 unscored=count-scored-failed-insufficient, skipped=skipped, budget_exhausted=budget,
                 failed=failed, not_computable=insufficient, pending=len(facts)-count,
                 coverage=scored/len(facts) if facts else None, reasons=reasons, anomalies=anomalies)
    delays = sorted(max(0, (seconds(f["scored_at"])-seconds(f["received_at"]))*1000) for f in facts if f.get("scored_at"))
    if delays: stats.update(p50_latency_ms=delays[math.ceil(.5*len(delays))-1], p95_latency_ms=delays[math.ceil(.95*len(delays))-1])
    changes = defaultdict(int)
    for f in scope.get("backlog_facts", facts):
        changes[f["received_at"]] += 1
        if f.get("scored_at"): changes[f["scored_at"]] -= 1
    pending, backlog = 0, []
    for stamp in sorted(changes, key=seconds):
        pending += changes[stamp]; backlog.append({"time": stamp, "pending": pending})
    stats["backlog_series"] = backlog
    stats["series"] = {"time": scope["start"], **{k:v for k,v in stats.items() if k not in ("reasons", "anomalies", "backlog_series")}}
    return stats


def comparison(db, ctx, budget=None):
    budget = budget or OutputBudget()
    baseline, expected = ctx["comparison"]["baseline_id"], ctx["comparison"]["case_ids"]
    # Duplicate identities are invalid even if they occur in different batches.
    if db.execute("SELECT 1 FROM metrics WHERE status='observed' AND type IN ('number','boolean') GROUP BY identity,candidate,original HAVING count(*)>1 LIMIT 1").fetchone():
        raise ValueError("duplicate metric identity")
    db.execute("CREATE INDEX paired_values ON metrics(identity,candidate,original,status)")
    profiles, rows, suggestions = [], [], []
    for c in ctx["candidates"]:
        cid = c["id"]; data = summary(db, len(expected), "candidate", cid, budget)
        profiles.append({"id": cid, "target_parameters": c["target_parameters"], "processed": data["processed"], "total": len(expected),
                         "failed": db.execute("SELECT count(*) FROM cases WHERE candidate=? AND status!='completed'", (cid,)).fetchone()[0],
                         "receipts": dict(db.execute("SELECT receipt,count(*) FROM cases WHERE candidate=? GROUP BY receipt", (cid,))), "series": data["series"]})
    for (key,) in db.execute("SELECT DISTINCT identity FROM metrics ORDER BY identity"):
        mid, evaluator, parameters = json.loads(key)
        for profile in profiles:
            cid = profile["id"]
            observed = db.execute("SELECT count(*) FROM metrics WHERE identity=? AND candidate=? AND status='observed' AND type IN ('number','boolean')", (key,cid)).fetchone()[0]
            mean = statistics.mean(float(json.loads(r[0])) for r in db.execute("SELECT value FROM metrics WHERE identity=? AND candidate=? AND status='observed' AND type IN ('number','boolean')", (key,cid))) if observed else None
            query = """FROM metrics a JOIN metrics b ON a.identity=b.identity AND a.original=b.original
              WHERE a.identity=? AND a.candidate=? AND b.candidate=? AND a.status='observed' AND b.status='observed'
              AND a.type IN ('number','boolean') AND b.type IN ('number','boolean')"""
            args = (key, baseline, cid)
            paired = db.execute("SELECT count(*) "+query, args).fetchone()[0]
            bmean = statistics.mean(float(json.loads(r[0])) for r in db.execute("SELECT a.value "+query, args)) if paired else None
            cmean = statistics.mean(float(json.loads(r[0])) for r in db.execute("SELECT b.value "+query, args)) if paired else None
            ids = [r[0] for r in db.execute("SELECT a.original "+query+" ORDER BY a.rowid LIMIT 100", args)]
            delta = cmean-bmean if paired else None
            row = {"metric_id":mid,"evaluator":evaluator,"parameters":parameters,"id":key+":"+cid,"candidate_id":cid,
                   "mean":mean,"observed":observed,"total":len(expected),"paired":paired,"baseline_mean":bmean,"candidate_mean":cmean,
                   "delta":delta,"case_ids":ids,"case_ids_truncated":paired>len(ids)}
            rows.append(budget.add(row))
            if cid != baseline and delta is not None and mid in ("rag.f1","latency.ms","usage.cost"):
                suggestions.append({"candidate_id":cid,"metric_id":mid,"evaluator":evaluator,"parameters":parameters,"paired":paired,"delta":delta,
                                    "case_ids":ids,"case_ids_truncated":paired>len(ids),"direction":"higher" if mid=="rag.f1" else "lower"})
    groups = {}
    for row in rows:
        if row["metric_id"] not in ("rag.precision","rag.recall","rag.f1"): continue
        key = json.dumps([row["evaluator"],row["parameters"]],sort_keys=True)
        group = groups.setdefault(key,{"id":key,"evaluator":row["evaluator"],"parameters":row["parameters"],"points":[{"id":p["id"],"precision":None,"recall":None,"f1":None} for p in profiles]})
        next(p for p in group["points"] if p["id"]==row["candidate_id"])[row["metric_id"].split(".")[1]]=row["mean"]
    return {"baseline_id":baseline,"input_digest":ctx["comparison"]["input_digest"],"candidates":profiles,"rows":rows,"suggestions":suggestions,"quality_series":list(groups.values()),
            "note":"配对统计使用双方均有效的全部案例；样本 ID 最多展示前 100 个，其余可在样本页查询。未设置综合权重，不自动推荐唯一最优配置。"}
