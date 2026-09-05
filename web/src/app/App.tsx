import {
  lazy,
  Suspense,
  useCallback,
  useRef,
  useState,
  useEffect,
} from "react";
import { evaluationApi } from "../api/client";
import { useResource } from "../api/useResource";
import { safeErrorCode, type ErrorCode } from "../api/errors";
import type { Asset } from "../domain/types";
import { isLive } from "../domain/presentation";
import { useRoute, openReport } from "./routes";
import { WorkspaceShell } from "./WorkspaceShell";
import { ErrorNotice, LoadingState } from "../components/Feedback";
import DiscoveryPage from "../features/discovery/DiscoveryPage";
import RunsPage from "../features/runs/RunsPage";
const ImportDialog = lazy(() =>
  import("../features/import/ImportDialog").then((module) => ({
    default: module.ImportDialog,
  })),
);
const ReportPage = lazy(() => import("../features/reports/ReportPage"));
const loadDiscovery = (signal: AbortSignal) => evaluationApi.discovery(signal);
const loadRuns = (signal: AbortSignal) => evaluationApi.runs(signal);
export default function App() {
  const route = useRoute();
  const discovery = useResource(loadDiscovery, "LOAD_ASSETS");
  const runs = useResource(loadRuns, "LOAD_RUNS");
  const [importOpen, setImportOpen] = useState(false),
    [starting, setStarting] = useState(false);
  const [startError, setStartError] = useState<ErrorCode | null>(null);
  const startLock = useRef(false),
    viewerOpened = useRef(false);
  const refreshRuns = runs.refresh;
  const onFinished = useCallback(() => {
    void refreshRuns();
  }, [refreshRuns]);
  const listVisible =
    route.page === "runs" || (route.page === "report" && !route.runId);
  const hasLiveRun = runs.data?.some((run) => isLive(run.status)) ?? false;
  useEffect(() => {
    if (listVisible) void refreshRuns();
  }, [listVisible, refreshRuns]);
  useEffect(() => {
    if (!listVisible || !hasLiveRun) return;
    let disposed = false;
    let timer: ReturnType<typeof setTimeout>;
    const poll = async () => {
      await refreshRuns();
      if (!disposed) timer = setTimeout(poll, 3000);
    };
    timer = setTimeout(poll, 3000);
    return () => {
      disposed = true;
      clearTimeout(timer);
    };
  }, [listVisible, hasLiveRun, refreshRuns]);
  useEffect(() => {
    if (!viewerOpened.current && discovery.data && runs.data) {
      viewerOpened.current = true;
      if (
        !window.location.hash &&
        discovery.data.package.id === "viewer" &&
        runs.data.length
      )
        openReport(runs.data[0].id);
    }
  }, [discovery.data, runs.data]);
  const start = async (asset: Asset) => {
    if (startLock.current) return;
    startLock.current = true;
    setStarting(true);
    setStartError(null);
    try {
      const run = await evaluationApi.start(asset.id);
      openReport(run.id);
      void refreshRuns();
    } catch (cause) {
      setStartError(safeErrorCode(cause, "START_UNKNOWN"));
    } finally {
      startLock.current = false;
      setStarting(false);
    }
  };
  return (
    <WorkspaceShell
      route={route}
      workspace={discovery.data?.package.name ?? "评测工作区"}
      onImport={() => setImportOpen(true)}
    >
      {route.page === "plans" ? (
        <>
          {startError ? (
            <ErrorNotice
              code={startError}
              onDismiss={() => setStartError(null)}
            />
          ) : null}
          <DiscoveryPage
            discovery={discovery.data}
            loading={discovery.loading}
            error={discovery.error}
            onRefresh={() => void discovery.refresh()}
            onRun={(asset) => void start(asset)}
            busy={starting}
          />
        </>
      ) : route.page === "report" && route.runId ? (
        <Suspense fallback={<LoadingState />}>
          <ReportPage
            key={route.runId}
            id={route.runId}
            onFinished={onFinished}
          />
        </Suspense>
      ) : (
        <RunsPage
          key={route.page}
          reports={route.page === "report"}
          runs={runs.data}
          loading={runs.loading}
          error={runs.error}
          onRefresh={() => void refreshRuns()}
        />
      )}
      {importOpen ? (
        <Suspense fallback={<LoadingState label="正在打开导入框" />}>
          <ImportDialog
            onBackgroundImported={onFinished}
            onClose={() => setImportOpen(false)}
            onImported={(id) => {
              setImportOpen(false);
              openReport(id);
              void refreshRuns();
            }}
          />
        </Suspense>
      ) : null}
    </WorkspaceShell>
  );
}
