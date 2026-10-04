import { processor } from "@seven-agent/eval";
import { summarize } from "./_summary";
export default processor(
  { id: "team-summary", version: "1.0.0" },
  async (ctx) => ({
    processor: "team-summary@1.0.0",
    input_digest: ctx.input_digest,
    data: summarize(ctx),
    report: {
      version: "1",
      kpis: {
        scored: "/scored_count",
        coverage: "/coverage_ratio",
        anomalies: "/anomaly_count",
      },
    },
  }),
);
