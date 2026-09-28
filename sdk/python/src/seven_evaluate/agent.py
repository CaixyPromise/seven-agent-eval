"""Explicit tool expectations and effect facts, separate from call completion."""
import json
from .basic import metric, json_equal

METRICS=["agent.call.completed","tool.expected","tool.arguments","tool.execution","tool.repeated","agent.effect"]

def evaluate(ctx):
    cid=ctx["case"]["id"];refs=ctx["case"].get("references") or {};tools=ctx["result"].get("tools")
    out=[]
    def emit(mid,status,value=None,reason=""):
        m=metric(cid,mid,status,value,reason);m["evaluator"]="builtin.agent-basic@1";out.append(m)
    emit("agent.call.completed","observed",ctx.get("status")=="completed")
    expected=refs.get("expected_tools")
    if expected is None:
        emit("tool.expected","not_applicable",reason="reference_missing")
        emit("tool.arguments","not_applicable",reason="reference_missing")
    elif tools is None:
        emit("tool.expected","not_computable",reason="evidence_missing")
        emit("tool.arguments","not_computable",reason="evidence_missing")
    else:
        emit("tool.expected","observed",all(any(t["name"]==e["name"] for t in tools) for e in expected))
        parameter_rules=[e for e in expected if "arguments" in e]
        if parameter_rules:
            emit("tool.arguments","observed",all(any(t["name"]==e["name"] and json_equal(t.get("arguments",{}),e["arguments"]) for t in tools) for e in parameter_rules))
        else:
            emit("tool.arguments","not_applicable",reason="reference_missing")
    if tools is None:
        for mid in ("tool.execution","tool.repeated"):emit(mid,"not_computable",reason="evidence_missing")
    else:
        completed=[t for t in tools if t["status"]!="pending"]
        emit("tool.execution","observed",all(t["status"]=="completed" for t in completed)) if completed else emit("tool.execution","not_computable",reason="evidence_missing")
        identities=[json.dumps([t["name"],t.get("arguments",{})],sort_keys=True) for t in tools]
        emit("tool.repeated","observed",len(identities)-len(set(identities)))
    effects=[t.get("result",{}).get("effectStatus") for t in tools or [] if isinstance(t.get("result"),dict) and "effectStatus" in t["result"]]
    if not effects or "effect_status" not in refs:emit("agent.effect","not_computable",reason="evidence_missing" if not effects else "reference_missing")
    else:emit("agent.effect","observed",all(json_equal(s,refs["effect_status"]) for s in effects))
    return out

def conclude(ctx):
    summary=next((o["data"] for o in ctx["outputs"] if o["processor"]=="builtin.basic-summary@1"),None)
    if summary is None:
        return [{"handler":"builtin.agent-conclusion@1","code":"dependency_missing","level":"warning","summary":"当前统计没有提供 Agent 汇总，结论不可计算。"}]
    rules=[s for s in summary["series"] if s["metric_id"] in ("tool.expected","tool.arguments","tool.execution","agent.effect","output.exact","output.schema")]
    failures=sum(s.get("failed",0) for s in rules)
    effects=sum(s["observed"] for s in rules if s["metric_id"]=="agent.effect")
    return [{"handler":"builtin.agent-conclusion@1","code":"rules_failed" if failures else "rules_checked","level":"warning" if failures or not effects else "info",
             "summary":f"工具与回答规则有 {failures} 项失败；{effects} 个样本提供了可检查的效果证据。调用完成不等于任务完成。"}]
