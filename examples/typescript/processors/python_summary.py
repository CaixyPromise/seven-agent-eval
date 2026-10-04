from seven_evaluate import processor

@processor(id="python-summary", version="1.0.0")
def summarize(ctx):
    scored = sum(any(m["status"] == "observed" for m in c["metrics"]) for c in ctx["cases"])
    anomalies = sum(c["status"] != "completed" or any(m["status"] == "error" or (m["status"] == "observed" and m["value"] is False) for m in c["metrics"]) for c in ctx["cases"])
    return {"processor":"python-summary@1.0.0", "input_digest":ctx["input_digest"],
            "data":{"scored_count":scored,"coverage_ratio":scored / ctx["total"] if ctx["total"] else None,"anomaly_count":anomalies},
            "report":{"version":"1","kpis":{"scored":"/scored_count","coverage":"/coverage_ratio","anomalies":"/anomaly_count"}}}
