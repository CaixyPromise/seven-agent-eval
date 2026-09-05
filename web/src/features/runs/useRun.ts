import { useCallback, useEffect, useState } from "react";
import { evaluationApi } from "../../api/client";
import { useResource } from "../../api/useResource";
import { isLive } from "../../domain/presentation";
export function useRun(id: string, onFinished: () => void) {
  const loader = useCallback(
    (signal: AbortSignal) => evaluationApi.run(id, signal),
    [id],
  );
  const resource = useResource(loader, "LOAD_RUN");
  const [disconnected, setDisconnected] = useState(false);
  const live = resource.data ? isLive(resource.data.status) : false;
  const refresh = resource.refresh;
  useEffect(() => {
    if (!live) return;
    let disposed = false,
      fetching = false,
      pending = false,
      ended = false;
    let attempts = 0;
    let retryTimer: ReturnType<typeof setTimeout> | undefined;
    setDisconnected(false);
    const stream = new EventSource(
      "/api/runs/" + encodeURIComponent(id) + "/events",
    );
    const reload = async () => {
      if (fetching) {
        pending = true;
        return;
      }
      fetching = true;
      do {
        pending = false;
        const latest = await refresh();
        if (latest && !isLive(latest.status)) {
          stream.close();
          if (!disposed) onFinished();
          return;
        }
      } while (pending && !disposed);
      fetching = false;
      if (ended && !disposed && attempts < 3) {
        retryTimer = setTimeout(() => void reload(), 1000 * 2 ** attempts++);
      }
    };
    stream.onopen = () => setDisconnected(false);
    stream.onerror = () => {
      if (!ended) setDisconnected(true);
    };
    stream.addEventListener("snapshot", (event) => {
      if (disposed) return;
      try {
        const state = JSON.parse((event as MessageEvent).data) as {
          status: string;
        };
        ended = !isLive(state.status);
        if (ended) stream.close();
        void reload();
      } catch {
        setDisconnected(true);
      }
    });
    return () => {
      disposed = true;
      clearTimeout(retryTimer);
      stream.close();
    };
  }, [id, live, refresh, onFinished]);
  return { ...resource, disconnected: live && disconnected };
}
