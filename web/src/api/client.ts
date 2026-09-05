import type {
  Discovery,
  LivePlan,
  LiveReport,
  LiveWindow,
  Run,
  RunSummary,
} from "../domain/types";
import { ProductError, responseError, type ErrorCode } from "./errors";
async function request<T>(
  path: string,
  fallback: ErrorCode,
  options: RequestInit = {},
): Promise<T> {
  let response: Response;
  try {
    response = await fetch("/api" + path, options);
  } catch (error) {
    if (options.signal?.aborted) throw error;
    throw new ProductError(fallback);
  }
  const data: unknown = await response.json().catch(() => null);
  if (!response.ok) throw responseError(data, fallback);
  if (data === null) throw new ProductError(fallback);
  return data as T;
}
const runPath = (id: string) => "/runs/" + encodeURIComponent(id);
export const evaluationApi = {
  discovery: (signal?: AbortSignal) =>
    request<Discovery>("/assets", "LOAD_ASSETS", { signal }),
  runs: (signal?: AbortSignal) =>
    request<RunSummary[]>("/runs", "LOAD_RUNS", { signal }),
  run: (id: string, signal?: AbortSignal) =>
    request<Run>(runPath(id), "LOAD_RUN", { signal }),
  start: (plan: string) =>
    request<Run>("/runs", "START_UNKNOWN", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ plan }),
    }),
  cancel: (id: string) =>
    request<{ cancelled: boolean }>(runPath(id) + "/cancel", "CANCEL_UNKNOWN", {
      method: "POST",
    }),
  import: (file: File) =>
    request<Run>("/import", "IMPORT_UNKNOWN", { method: "POST", body: file }),
  async export(id: string) {
    let response: Response;
    try {
      response = await fetch("/api" + runPath(id) + "/export");
    } catch {
      throw new ProductError("EXPORT_UNKNOWN");
    }
    if (!response.ok)
      throw responseError(
        await response.json().catch(() => null),
        "EXPORT_UNKNOWN",
      );
    const blob = await response.blob();
    const url = URL.createObjectURL(blob);
    const link = document.createElement("a");
    link.href = url;
    link.download = id + ".seven.eval";
    link.click();
    setTimeout(() => URL.revokeObjectURL(url), 1000);
  },
  livePlans: (signal?: AbortSignal) =>
    request<LivePlan[]>("/live/plans", "LOAD_LIVE_PLANS", { signal }),
  liveReport: (planID: string, range: string, signal?: AbortSignal) =>
    request<LiveReport>(
      "/live/plans/" + encodeURIComponent(planID) + "/report?range=" + range,
      "LOAD_LIVE_REPORT",
      { signal },
    ),
  pauseLive: (planID: string) =>
    request<LivePlan>(
      "/live/plans/" + encodeURIComponent(planID) + "/pause",
      "LIVE_CONTROL_UNKNOWN",
      { method: "POST" },
    ),
  resumeLive: (planID: string) =>
    request<LivePlan>(
      "/live/plans/" + encodeURIComponent(planID) + "/resume",
      "LIVE_CONTROL_UNKNOWN",
      { method: "POST" },
    ),
  liveWindow: (id: string, signal?: AbortSignal) =>
    request<LiveWindow>(
      "/live/windows/" + encodeURIComponent(id),
      "WINDOW_NOT_FOUND",
      { signal },
    ),
  async exportWindow(id: string) {
    let response: Response;
    try {
      response = await fetch("/api/live/windows/" + encodeURIComponent(id) + "/export");
    } catch {
      throw new ProductError("WINDOW_EXPORT_UNAVAILABLE");
    }
    if (!response.ok)
      throw responseError(
        await response.json().catch(() => null),
        "WINDOW_EXPORT_UNAVAILABLE",
      );
    const blob = await response.blob();
    const url = URL.createObjectURL(blob);
    const link = document.createElement("a");
    link.href = url;
    link.download = id + ".seven.eval";
    link.click();
    setTimeout(() => URL.revokeObjectURL(url), 1000);
  },
};
