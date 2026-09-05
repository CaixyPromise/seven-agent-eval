import type { Discovery, Run, RunSummary } from "../domain/types";
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
};
