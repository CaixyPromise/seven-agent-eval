import { useSyncExternalStore } from "react";
export type Route = { page: "plans" | "runs" | "report" | "live"; runId?: string };
const subscribe = (callback: () => void) => {
  window.addEventListener("hashchange", callback);
  return () => window.removeEventListener("hashchange", callback);
};
const snapshot = () => window.location.hash;
export function parseRoute(hash: string): Route {
  const [page, id] = hash.replace(/^#\/?/, "").split("/");
  if (page === "runs") return { page: "runs" };
  if (page === "live") return { page: "live" };
  if (page === "reports") {
    try {
      return { page: "report", runId: id ? decodeURIComponent(id) : undefined };
    } catch {
      return { page: "report" };
    }
  }
  return { page: "plans" };
}
export function useRoute(): Route {
  return parseRoute(useSyncExternalStore(subscribe, snapshot));
}
export const reportHref = (id: string) => "#/reports/" + encodeURIComponent(id);
export const openReport = (id: string) => {
  window.location.hash = reportHref(id);
};
