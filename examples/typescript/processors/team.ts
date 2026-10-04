import { processor } from "@seven-agent/eval";
import { summarize } from "./_summary";
export default processor({ id: "team-summary", version: "2.0.0" }, async (ctx) => {
  if (ctx.phase === "initialize") return {scored_count:0, anomaly_count:0};
  if (ctx.phase === "update") {
    const batch=summarize(ctx);
    return {scored_count:ctx.state.scored_count+batch.scored_count,anomaly_count:ctx.state.anomaly_count+batch.anomaly_count};
  }
  return {processor:"team-summary@2.0.0",input_digest:ctx.input_digest,
    data:{...ctx.state,coverage_ratio:ctx.total?ctx.state.scored_count/ctx.total:null},
    report:{version:"1",kpis:{scored:"/scored_count",coverage:"/coverage_ratio",anomalies:"/anomaly_count"}}};
});
