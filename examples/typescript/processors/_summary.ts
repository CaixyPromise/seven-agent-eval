import type { ProcessorContext } from "@seven-agent/eval";
export function summarize(ctx: ProcessorContext) {
  const scored_count = ctx.cases.filter((c) =>
    c.metrics.some((m) => m.status === "observed"),
  ).length;
  const anomaly_count = ctx.cases.filter(
    (c) =>
      c.status !== "completed" ||
      c.metrics.some(
        (m) =>
          m.status === "error" ||
          (m.status === "observed" && m.value === false),
      ),
  ).length;
  return {
    scored_count,
    coverage_ratio: ctx.total ? scored_count / ctx.total : null,
    anomaly_count,
  };
}
