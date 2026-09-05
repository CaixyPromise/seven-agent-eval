import { useCallback, useEffect, useRef, useState } from "react";
import { safeErrorCode, type ErrorCode } from "./errors";
export function useResource<T>(
  load: (signal: AbortSignal) => Promise<T>,
  fallback: ErrorCode,
) {
  const [data, setData] = useState<T | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<ErrorCode | null>(null);
  const controller = useRef<AbortController | null>(null);
  const refresh = useCallback(async () => {
    controller.current?.abort();
    const current = new AbortController();
    controller.current = current;
    setLoading(true);
    setError(null);
    try {
      const value = await load(current.signal);
      if (!current.signal.aborted) {
        setData(value);
        return value;
      }
    } catch (cause) {
      if (!current.signal.aborted) setError(safeErrorCode(cause, fallback));
    } finally {
      if (!current.signal.aborted) setLoading(false);
    }
  }, [load, fallback]);
  useEffect(() => {
    setData(null);
    void refresh();
    return () => controller.current?.abort();
  }, [refresh]);
  return { data, loading, error, refresh, setData };
}
