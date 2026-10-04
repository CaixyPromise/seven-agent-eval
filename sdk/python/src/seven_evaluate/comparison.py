"""Comparison conclusions consume Processor output only."""
def process(ctx):
    from .basic import process as fixture_process
    return fixture_process(ctx)

def conclude(summary):
    profiles=summary["comparison"]["candidates"]
    unconfirmed=sum(p["receipts"].get("unconfirmed",0) for p in profiles)
    mismatched=sum(p["receipts"].get("mismatch",0) for p in profiles)
    failures=sum(p["failed"] for p in profiles)
    return [{"handler":"builtin.comparison-conclusion@1","code":"comparison_checked","level":"warning" if failures or unconfirmed or mismatched or summary["processed"]<summary["total"] else "info",
             "summary":f"已处理 {summary['processed']}/{summary['total']} 次候选调用；{failures} 次执行异常，{unconfirmed} 次目标未确认参数，{mismatched} 次返回参数不一致。配对差值仅覆盖同一案例的有效观测，未设综合权重或唯一最优配置。"}]
