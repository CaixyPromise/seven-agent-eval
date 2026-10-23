from seven_evaluate import processor


@processor(id="team-summary", version="2.0.0")
def summarize(ctx):
    if ctx["phase"] == "initialize":
        return {"scored_count": 0, "anomaly_count": 0}
    if ctx["phase"] == "update":
        scored = 0
        anomalies = 0
        for case in ctx["cases"]:
            metrics = case["metrics"]
            scored += any(m["status"] == "observed" for m in metrics)
            anomalies += case["status"] != "completed" or any(
                m["status"] == "error"
                or (m["status"] == "observed" and m.get("value") is False)
                for m in metrics
            )
        return {
            "scored_count": ctx["state"]["scored_count"] + scored,
            "anomaly_count": ctx["state"]["anomaly_count"] + anomalies,
        }
    return {
        "processor": "team-summary@2.0.0",
        "input_digest": ctx["input_digest"],
        "data": {
            **ctx["state"],
            "coverage_ratio": ctx["state"]["scored_count"] / ctx["total"] if ctx["total"] else None,
        },
        "report": {
            "version": "1",
            "kpis": {"scored": "/scored_count", "coverage": "/coverage_ratio", "anomalies": "/anomaly_count"},
        },
    }
