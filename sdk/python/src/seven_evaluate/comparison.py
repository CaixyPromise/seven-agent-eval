"""Paired candidate comparison; statistics are never calculated in the UI."""
from collections import Counter
import json
import statistics


def identity(metric):
    return json.dumps([metric["metric_id"], metric["evaluator"], metric.get("parameters", {})], sort_keys=True)


def process(ctx):
    from .basic import process as basic_process
    candidates = ctx["candidates"]
    baseline = ctx["comparison"]["baseline_id"]
    expected = ctx["comparison"]["case_ids"]
    values, profiles, identities = {}, [], {}
    for candidate in candidates:
        cid = candidate["id"]
        cases = [c for c in ctx["cases"] if c["case"].get("metadata", {}).get("seven_eval_candidate") == cid]
        summary = basic_process({"cases": cases, "total": len(expected), "input_digest": ctx["input_digest"]})["data"]
        receipts = Counter((c["result"].get("parameter_receipt") or {}).get("state", "unconfirmed") for c in cases)
        profiles.append({"id": cid, "target_parameters": candidate["target_parameters"],
                         "processed": len(cases), "total": len(expected),
                         "failed": sum(c["status"] != "completed" for c in cases),
                         "receipts": dict(receipts), "series": summary["series"]})
        current = {}
        for case in cases:
            original_id = case["case"]["metadata"]["seven_eval_case_id"]
            for metric in case["metrics"]:
                key = identity(metric)
                identities[key] = {"metric_id": metric["metric_id"], "evaluator": metric["evaluator"], "parameters": metric.get("parameters", {})}
                if metric["status"] == "observed" and type(metric["value"]) in (int, float, bool):
                    if original_id in current.get(key, {}):
                        raise ValueError("duplicate metric identity in comparison case")
                    current.setdefault(key, {})[original_id] = float(metric["value"])
        values[cid] = current
    rows, suggestions = [], []
    for key in sorted(identities):
        base_values = values[baseline].get(key, {})
        item = identities[key]
        for profile in profiles:
            cid = profile["id"]
            observed = values[cid].get(key, {})
            paired = [case_id for case_id in expected if case_id in base_values and case_id in observed]
            bmean = statistics.mean(base_values[c] for c in paired) if paired else None
            cmean = statistics.mean(observed[c] for c in paired) if paired else None
            delta = cmean - bmean if paired else None
            row = {**item, "id": key + ":" + cid, "candidate_id": cid,
                   "mean": statistics.mean(observed.values()) if observed else None,
                   "observed": len(observed), "total": len(expected),
                   "paired": len(paired), "baseline_mean": bmean, "candidate_mean": cmean,
                   "delta": delta, "case_ids": paired}
            rows.append(row)
            if cid != baseline and delta is not None and item["metric_id"] in ("rag.f1", "latency.ms", "usage.cost"):
                suggestions.append({"candidate_id": cid, "metric_id": item["metric_id"],
                                    "parameters": item["parameters"], "evaluator":item["evaluator"],
                                    "paired": len(paired), "delta": delta, "case_ids": paired,
                                    "direction": "higher" if item["metric_id"] == "rag.f1" else "lower"})
    quality_groups = {}
    for row in rows:
        if row["metric_id"] not in ("rag.precision", "rag.recall", "rag.f1"):
            continue
        key = json.dumps([row["evaluator"], row["parameters"]], sort_keys=True)
        group = quality_groups.setdefault(key, {"id":key, "evaluator":row["evaluator"], "parameters":row["parameters"],
                                               "points":[{"id":p["id"],"precision":None,"recall":None,"f1":None} for p in profiles]})
        point = next(p for p in group["points"] if p["id"]==row["candidate_id"])
        point[row["metric_id"].split(".")[1]] = row["mean"]
    scored = sum(any(m["status"] == "observed" for m in c["metrics"]) for c in ctx["cases"])
    return {"processor": "builtin.basic-summary@1", "input_digest": ctx["input_digest"],
            "data": {"total":ctx["total"], "processed":len(ctx["cases"]), "scored":scored,
                     "coverage":scored/ctx["total"] if ctx["total"] else None,
                     "series":[], "rag_quality":[],
                     "failure_ids":[c["case"]["id"] for c in ctx["cases"] if c["status"] != "completed" or any(m["value"] is False and m["status"] == "observed" for m in c["metrics"])],
                     "comparison":{"baseline_id":baseline, "input_digest":ctx["comparison"]["input_digest"],
                                   "candidates":profiles, "rows":rows, "suggestions":suggestions, "quality_series":list(quality_groups.values()),
                                   "note":"配对差值只使用双方均可计算的同一案例；未设置综合权重，不自动推荐唯一最优配置。目标确认是目标自报，不是独立验证。"}}}


def conclude(summary):
    profiles=summary["comparison"]["candidates"]
    unconfirmed=sum(p["receipts"].get("unconfirmed",0) for p in profiles)
    mismatched=sum(p["receipts"].get("mismatch",0) for p in profiles)
    failures=sum(p["failed"] for p in profiles)
    return [{"handler":"builtin.comparison-conclusion@1","code":"comparison_checked","level":"warning" if failures or unconfirmed or mismatched or summary["processed"]<summary["total"] else "info",
             "summary":f"已处理 {summary['processed']}/{summary['total']} 次候选调用；{failures} 次执行异常，{unconfirmed} 次目标未确认参数，{mismatched} 次返回参数不一致。配对差值仅覆盖同一案例的有效观测，未设综合权重或唯一最优配置。"}]
