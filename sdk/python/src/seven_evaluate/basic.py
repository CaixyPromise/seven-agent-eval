"""Deterministic starter recipe; no model credentials are required."""
from collections import Counter, defaultdict
import json
import math
import statistics
from jsonschema import Draft202012Validator, exceptions

CATALOG = {
    "metrics": [
        {"id": "output.present", "name": "输出存在", "type": "boolean", "description": "响应非空；不代表业务正确"},
        {"id": "evaluation.error", "name": "评测异常", "type": "json", "description": "扩展执行或结果契约错误"},
        {"id": "output.schema", "name": "格式约束", "type": "boolean", "description": "用户声明的 JSON Schema"},
        {"id": "output.exact", "name": "精确匹配", "type": "boolean", "description": "仅在 references.exact 显式声明时启用，不做隐式规范化"},
        {"id": "latency.ms", "name": "调用耗时", "type": "number", "unit": "ms"},
        {"id": "usage.tokens", "name": "Token 用量", "type": "number", "unit": "token"},
        {"id": "usage.cost", "name": "调用成本", "type": "number", "description": "仅接受带 currency/source 的已提供成本"},
    ],
    "reasons": [
        {"id": code, "description": desc} for code, desc in [
            ("evidence_missing", "缺少响应或调用证据"), ("reference_missing", "没有声明相应约束"),
            ("usage_missing", "未提供可用用量"), ("subject_error", "目标调用失败"),
            ("evaluator_error", "评测过程失败"), ("insufficient_evidence", "证据不足以判定任务质量"),
            ("dependency_missing", "结论需要的统计输出不可用"), ("cancelled", "用户取消或运行中断"),
        ]
    ],
    "conclusions": [
        {"id": "rules_failed", "description": "已验证规则存在失败，不等同于所有任务失败"},
        {"id": "rules_checked", "description": "已声明规则检查完成"},
        {"id": "insufficient_evidence", "description": "未声明业务参考，无法判断整体质量"},
        {"id": "dependency_missing", "description": "统计被关闭或替换，默认结论依赖不可用"},
    ],
}

def metric(case_id, metric_id, status, value=None, reason="", parameters=None):
    return {"id": case_id + ":" + metric_id, "case_id": case_id, "metric_id": metric_id,
            "status": status, "value": value, "reason": reason, "evaluator": "builtin.basic@1",
            "parameters": parameters or {}, "evidence_refs": [case_id + ":response"]}

def json_equal(a, b):
    if isinstance(a, bool) or isinstance(b, bool):
        return type(a) is type(b) and a == b
    if isinstance(a, dict) and isinstance(b, dict):
        return a.keys() == b.keys() and all(json_equal(a[k], b[k]) for k in a)
    if isinstance(a, list) and isinstance(b, list):
        return len(a) == len(b) and all(json_equal(x, y) for x, y in zip(a, b))
    return a == b

def evaluate(ctx):
    case, result = ctx["case"], ctx["result"]
    cid, response = case["id"], result.get("response")
    refs = case.get("references") or {}
    if ctx.get("status") != "completed":
        return [metric(cid, "output.present", "not_computable", reason=ctx.get("reason") or "subject_error")]
    present = response is not None and response != "" and response != [] and response != {}
    out = [metric(cid, "output.present", "observed", present)]
    for key in ("schema", "exact"):
        mid = "output." + key
        if key not in refs:
            out.append(metric(cid, mid, "not_applicable", reason="reference_missing"))
            continue
        if not present:
            out.append(metric(cid, mid, "not_computable", reason="evidence_missing"))
            continue
        if key == "exact":
            good = json_equal(response, refs[key])
        else:
            try:
                Draft202012Validator.check_schema(refs[key])
                value = json.loads(response) if isinstance(response, str) else response
                # External references are unsupported: validation never fetches a network resource.
                def local_schema(node):
                    if isinstance(node, dict):
                        for ref_key in ("$ref", "$dynamicRef", "$recursiveRef"):
                            if ref_key in node and not node[ref_key].startswith("#"):
                                raise ValueError("external schema references unsupported")
                        for v in node.values(): local_schema(v)
                    elif isinstance(node, list):
                        for v in node: local_schema(v)
                local_schema(refs[key])
                good = Draft202012Validator(refs[key]).is_valid(value)
            except json.JSONDecodeError:
                good = False
            except (exceptions.SchemaError, ValueError):
                out.append(metric(cid, mid, "error", reason="input_invalid"))
                continue
        out.append(metric(cid, mid, "observed", good))
    latency = result.get("latency_ms")
    out.append(metric(cid, "latency.ms", "observed" if latency is not None else "not_computable",
                      latency, "" if latency is not None else "evidence_missing"))
    usage = result.get("usage") or {}
    for key, mid in [("total_tokens", "usage.tokens"), ("cost", "usage.cost")]:
        value = usage.get(key)
        valid = type(value) in (float, int) and math.isfinite(value) and value >= 0
        if key == "cost":
            valid = valid and bool(usage.get("currency")) and bool(usage.get("source"))
        params = {"currency": usage["currency"], "source": usage["source"]} if valid and key == "cost" else {}
        out.append(metric(cid, mid, "observed" if valid else "not_computable",
                          value if valid else None, "" if valid else "usage_missing", params))
    return out

def process(ctx):
    cases = ctx["cases"]
    grouped = defaultdict(list)
    for item in cases:
        for m in item["metrics"]:
            key = json.dumps([m["metric_id"], m["evaluator"], m.get("parameters", {})], sort_keys=True)
            grouped[key].append(m)
    series = []
    for key, items in sorted(grouped.items()):
        mid, evaluator, parameters = json.loads(key)
        observed = [m["value"] for m in items if m["status"] == "observed"]
        stats = {"metric_id": mid, "evaluator": evaluator, "parameters": parameters,
                 "total": len(items), "observed": len(observed),
                 "statuses": dict(Counter(m["status"] for m in items))}
        if observed and all(type(v) is bool for v in observed):
            stats.update(passed=sum(observed), failed=len(observed)-sum(observed),
                         rate=sum(observed)/len(observed))
        elif observed and all(type(v) in (int, float) for v in observed):
            ordered = sorted(observed)
            stats.update(mean=statistics.mean(ordered), median=statistics.median(ordered),
                         p95=ordered[max(0, math.ceil(.95*len(ordered))-1)], min=ordered[0], max=ordered[-1])
        series.append(stats)
    scored = sum(any(m["status"] == "observed" for m in c["metrics"]) for c in cases)
    failure_ids = [c["case"]["id"] for c in cases if c["status"] != "completed" or
                   any(m["status"] == "observed" and m["value"] is False for m in c["metrics"])]
    return {"processor": "builtin.basic-summary@1",
            "data": {"total": ctx["total"], "processed": len(cases), "scored": scored,
                     "pending": max(0, ctx["total"]-len(cases)),
                     "coverage": scored/ctx["total"] if ctx["total"] else None,
                     "case_statuses": dict(Counter(c["status"] for c in cases)),
                     "series": series, "failure_ids": failure_ids,
                     "note": "覆盖率表示产生了有效指标的样本比例，不代表业务正确率。"},
            "input_digest": ctx["input_digest"]}

def conclude(ctx):
    summary = next((o["data"] for o in ctx["outputs"] if o["processor"] == "builtin.basic-summary@1"), None)
    if summary is None:
        return [{"handler": "builtin.basic-conclusion@1", "code": "dependency_missing", "level": "warning",
                 "summary": "基础汇总已关闭或被替换，默认结论无法读取所需统计。"}]
    rules = [s for s in summary["series"] if s["metric_id"] in ("output.schema", "output.exact")]
    count = sum(s["observed"] for s in rules)
    failures = sum(s.get("failed", 0) for s in rules)
    code = "rules_failed" if failures else "rules_checked" if count else "insufficient_evidence"
    text = (f"已执行 {count} 项显式规则检查，其中 {failures} 项失败。" if count
            else "未提供可判定任务正确性的业务参考；当前仅报告基础事实。")
    text += f" 已处理 {summary['processed']}/{summary['total']} 条；异常样本 {len(summary['failure_ids'])} 条。"
    return [{"handler": "builtin.basic-conclusion@1", "code": code,
             "level": "warning" if failures or not count or summary["failure_ids"] else "info", "summary": text}]
