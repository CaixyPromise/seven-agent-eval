import type { Run } from "./types";
export type ReportKPIs = {
  scored?: number | null;
  coverage?: number | null;
  anomalies?: number | null;
  conflict?: boolean;
  invalid?: boolean;
};
function pointer(data: unknown, path: string): unknown {
  if (path === "") return data;
  if (!path.startsWith("/")) throw new Error("invalid pointer");
  let value = data;
  for (const token of path.slice(1).split("/")) {
    if (/~(?![01])/u.test(token)) throw new Error("invalid escape");
    const key = token.replace(/~1/g, "/").replace(/~0/g, "~");
    if (
      value === null ||
      typeof value !== "object" ||
      !Object.hasOwn(value, key)
    )
      throw new Error("missing field");
    if (Array.isArray(value) && !/^(0|[1-9]\d*)$/.test(key))
      throw new Error("invalid index");
    value = (value as Record<string, unknown>)[key];
  }
  return value;
}
export function reportKpis(outputs: Run["outputs"]): ReportKPIs {
  const providers = outputs.filter((o) => o.report != null);
  if (providers.length > 1) return { conflict: true };
  if (providers.length === 1) {
    const output = providers[0],
      result: ReportKPIs = {};
    try {
      if (output.report!.version !== "1") throw new Error("unsupported report");
      for (const [key, path] of Object.entries(output.report!.kpis)) {
        if (
          !["scored", "coverage", "anomalies"].includes(key) ||
          typeof path !== "string"
        )
          throw new Error("invalid key");
        const value = pointer(output.data, path);
        if (
          value !== null &&
          (typeof value !== "number" ||
            !Number.isFinite(value) ||
            value < 0 ||
            (key === "coverage" ? value > 1 : !Number.isSafeInteger(value)))
        )
          throw new Error("invalid value");
        result[key as "scored" | "coverage" | "anomalies"] = value as
          | number
          | null;
      }
      return result;
    } catch {
      return { invalid: true };
    }
  }
  return {};
}
