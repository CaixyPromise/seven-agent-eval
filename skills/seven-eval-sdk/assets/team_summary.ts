import { processor } from "@seven-agent/eval";

export default processor({ id: "team-summary", version: "2.0.0" }, (ctx) => {
  if (ctx.phase === "initialize") return { scored_count: 0, anomaly_count: 0 };
  if (ctx.phase === "update") {
    let scored = 0;
    let anomalies = 0;
    for (const c of ctx.cases ?? []) {
      scored += Number(c.metrics.some((m) => m.status === "observed"));
      anomalies += Number(c.status !== "completed" || c.metrics.some((m) =>
        m.status === "error" || (m.status === "observed" && m.value === false),
      ));
    }
    return {
      scored_count: ctx.state.scored_count + scored,
      anomaly_count: ctx.state.anomaly_count + anomalies,
    };
  }
  return {
    processor: "team-summary@2.0.0",
    input_digest: ctx.input_digest,
    data: { ...ctx.state, coverage_ratio: ctx.total ? ctx.state.scored_count / ctx.total : null },
    report: { version: "1", kpis: {
      scored: "/scored_count", coverage: "/coverage_ratio", anomalies: "/anomaly_count",
    } },
  };
});
