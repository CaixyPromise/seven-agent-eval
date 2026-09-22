import {
  useCallback,
  useEffect,
  useMemo,
  useRef,
  useState,
} from "react";
import Alert from "antd/es/alert";
import Button from "antd/es/button";
import Card from "antd/es/card";
import Segmented from "antd/es/segmented";
import Select from "antd/es/select";
import Statistic from "antd/es/statistic";
import Table from "antd/es/table";
import Tag from "antd/es/tag";
import Tooltip from "antd/es/tooltip";
import type { ColumnsType } from "antd/es/table";
import {
  Activity,
  Download,
  Info,
  Pause,
  Play,
  RefreshCw,
} from "lucide-react";
import { evaluationApi } from "../../api/client";
import { safeErrorCode, type ErrorCode } from "../../api/errors";
import { ErrorNotice, EmptyState, LoadingState } from "../../components/Feedback";
import { StatusBadge } from "../../components/StatusBadge";
import { formatDate } from "../../domain/presentation";
import type { LivePlan, LiveReport, LiveWindow } from "../../domain/types";
import { LiveChart } from "./LiveChart";
import { WindowDrawer } from "./WindowDrawer";

const ranges = ["15m", "1h", "6h", "24h"] as const;
type Range = (typeof ranges)[number];
const chartBase = {
  grid: { left: 44, right: 18, top: 38, bottom: 38 },
  tooltip: { trigger: "axis" as const },
  textStyle: { color: "#4e5d73", fontFamily: "Inter, system-ui, sans-serif" },
};
const percent = (value?: number) => (value == null ? "—" : `${(value * 100).toFixed(1)}%`);
const latency = (value?: number) => (value == null ? "—" : `${Math.round(value)} ms`);

export default function LivePage() {
  const [plans, setPlans] = useState<LivePlan[]>();
  const [planID, setPlanID] = useState<string>();
  const [range, setRange] = useState<Range>("1h");
  const [report, setReport] = useState<LiveReport>();
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<ErrorCode | null>(null);
  const [controlling, setControlling] = useState(false);
  const [drawerID, setDrawerID] = useState<string>();
  const [drawerWindow, setDrawerWindow] = useState<LiveWindow>();
  const [drawerLoading, setDrawerLoading] = useState(false);
  const [drawerError, setDrawerError] = useState<ErrorCode | null>(null);
  const [exporting, setExporting] = useState(false);
  const requestVersion = useRef(0);
  const drawerVersion = useRef(0);

  const reloadPlans = useCallback(async () => {
    try {
      const next = await evaluationApi.livePlans();
      setError(null);
      setPlans(next);
      setPlanID((current) =>
        current && next.some((plan) => plan.id === current) ? current : next[0]?.id,
      );
    } catch (cause) {
      setError(safeErrorCode(cause, "LOAD_LIVE_PLANS"));
    }
  }, []);
  const reloadReport = useCallback(async () => {
    if (!planID) {
      setReport(undefined);
      setLoading(false);
      return;
    }
    const version = ++requestVersion.current;
    try {
      const next = await evaluationApi.liveReport(planID, range);
      if (version === requestVersion.current) {
        setReport(next);
        setError(null);
      }
    } catch (cause) {
      if (version === requestVersion.current)
        setError(safeErrorCode(cause, "LOAD_LIVE_REPORT"));
    } finally {
      if (version === requestVersion.current) setLoading(false);
    }
  }, [planID, range]);

  useEffect(() => {
    void reloadPlans().finally(() => setLoading(false));
  }, [reloadPlans]);
  useEffect(() => {
    setReport(undefined);
    setLoading(true);
    void reloadReport();
    return () => { requestVersion.current++; };
  }, [reloadReport]);
  useEffect(() => {
    if (!planID) return;
    let cancelled = false;
    const stream = new EventSource(
      "/api/live/plans/" + encodeURIComponent(planID) + "/events?range=" + range,
    );
    stream.addEventListener("snapshot", (event) => {
      if (cancelled) return;
      try {
        requestVersion.current++;
        setReport(JSON.parse((event as MessageEvent<string>).data) as LiveReport);
        setError(null);
        setLoading(false);
      } catch {
        setError("STREAM_LOST");
      }
    });
    stream.onerror = () => {
      if (!cancelled) setError("STREAM_LOST");
    };
    return () => {
      cancelled = true;
      stream.close();
    };
  }, [planID, range]);

  const selectedPlan = report && report.plan.id === planID ? report.plan : plans?.find((plan) => plan.id === planID);
  const current = report && report.plan.id === planID && report.summary ? { scope: report.summary } : undefined;
  const coverage = current?.scope.coverage;
  const openWindow = useCallback(async (id: string) => {
    const version = ++drawerVersion.current;
    setDrawerID(id);
    setDrawerWindow(undefined);
    setDrawerLoading(true);
    setDrawerError(null);
    try {
      const next = await evaluationApi.liveWindow(id);
      if (version === drawerVersion.current) setDrawerWindow(next);
    } catch (cause) {
      if (version === drawerVersion.current) setDrawerError(safeErrorCode(cause, "WINDOW_NOT_FOUND"));
    } finally {
      if (version === drawerVersion.current) setDrawerLoading(false);
    }
  }, []);
  const control = async () => {
    if (!selectedPlan) return;
    setControlling(true);
    try {
      await (selectedPlan.paused
        ? evaluationApi.resumeLive(selectedPlan.id)
        : evaluationApi.pauseLive(selectedPlan.id));
      await reloadPlans();
      await reloadReport();
    } catch (cause) {
      setError(safeErrorCode(cause, "LIVE_CONTROL_UNKNOWN"));
    } finally {
      setControlling(false);
    }
  };
  const exportWindow = async () => {
    if (!drawerWindow) return;
    setExporting(true);
    try {
      await evaluationApi.exportWindow(drawerWindow.id);
    } catch (cause) {
      setDrawerError(safeErrorCode(cause, "WINDOW_EXPORT_UNAVAILABLE"));
    } finally {
      setExporting(false);
    }
  };
  const openPoint = (index: number) => {
    const point = report?.series[index];
    const window = report?.windows.find((window) => window.scope.start === point?.time);
    if (window) void openWindow(window.id);
  };

  const timeAxis = useMemo(
    () => report?.series.map((point) => formatDate(point.time)) ?? [],
    [report],
  );
  const coverageOption = useMemo(
    () => ({
      ...chartBase,
      xAxis: { type: "category", data: timeAxis, boundaryGap: false },
      yAxis: { type: "value", min: 0, max: 100, axisLabel: { formatter: "{value}%" } },
      series: [{ type: "line", smooth: true, name: "评分覆盖率", data: report?.series.map((point) => point.coverage == null ? null : point.coverage * 100) ?? [], itemStyle: { color: "#1677ff" }, areaStyle: { color: "rgba(22,119,255,.10)" } }],
    }),
    [report, timeAxis],
  );
  const distributionOption = useMemo(
    () => ({
      ...chartBase,
      legend: { top: 4 },
      xAxis: { type: "category", data: timeAxis },
      yAxis: { type: "value" },
      series: [
        { type: "bar", stack: "count", name: "已评分", data: report?.series.map((point) => point.scored) ?? [], itemStyle: { color: "#1677ff" } },
        { type: "bar", stack: "count", name: "无有效评分", data: report?.series.map((point) => point.unscored ?? 0) ?? [], itemStyle: { color: "#bfbfbf" } },
        { type: "bar", stack: "count", name: "证据不足", data: report?.series.map((point) => point.not_computable) ?? [], itemStyle: { color: "#faad14" } },
        { type: "bar", stack: "count", name: "失败", data: report?.series.map((point) => point.failed) ?? [], itemStyle: { color: "#ff4d4f" } },
        { type: "bar", stack: "count", name: "待处理", data: report?.series.map((point) => point.pending) ?? [], itemStyle: { color: "#91a5c7" } },
      ],
    }),
    [report, timeAxis],
  );
  const latencyOption = useMemo(
    () => ({
      ...chartBase,
      legend: { top: 4 },
      xAxis: { type: "category", data: timeAxis, boundaryGap: false },
      yAxis: { type: "value", axisLabel: { formatter: "{value} ms" } },
      series: [
        { type: "line", smooth: true, name: "p50", data: report?.series.map((point) => point.p50_latency_ms) ?? [], itemStyle: { color: "#13a8a8" } },
        { type: "line", smooth: true, name: "p95", data: report?.series.map((point) => point.p95_latency_ms) ?? [], itemStyle: { color: "#fa8c16" } },
      ],
    }),
    [report, timeAxis],
  );
  const backlogOption = useMemo(
    () => ({
      ...chartBase,
      xAxis: { type: "category", data: report?.backlog_series?.map((point) => formatDate(point.time)) ?? [], boundaryGap: false },
      yAxis: { type: "value", minInterval: 1 },
      series: [{ type: "line", step: "end", name: "待处理", data: report?.backlog_series?.map((point) => point.pending) ?? [], itemStyle: { color: "#7b61ff" }, areaStyle: { color: "rgba(123,97,255,.10)" } }],
    }),
    [report, timeAxis],
  );
  const windowColumns: ColumnsType<LiveWindow> = [
    { title: "时间窗口", dataIndex: ["scope", "start"], render: (value: string, row) => <Button type="link" className="identity-button" onClick={() => void openWindow(row.id)}>{formatDate(value)}<span className="secondary-id">修订 {row.scope.revision}</span></Button> },
    { title: "状态", dataIndex: ["scope", "state"], render: (value: string) => <StatusBadge status={value} /> },
    { title: "接收 / 评分", render: (_, row) => `${row.scope.received} / ${row.scope.statistics_available ? row.scope.scored : "—"}` },
    { title: "覆盖率", render: (_, row) => percent(row.scope.coverage) },
    { title: "p95 延迟", dataIndex: ["scope", "p95_latency_ms"], render: latency },
    { title: "异常", render: (_, row) => row.scope.anomalies ? <Tag color="gold">{row.scope.anomalies}</Tag> : "—" },
    { title: "操作", render: (_, row) => <Button type="link" icon={<Download size={15} />} onClick={() => void openWindow(row.id)}>查看 / 导出</Button> },
  ];

  if (loading && !plans) return <LoadingState label="正在读取持续评分计划" />;
  return (
    <>
      <div className="page-heading live-heading">
        <div>
          <span className="eyebrow"><Activity size={15} /> CONTINUOUS EVALUATION</span>
          <h1>持续评分</h1>
          <p>按请求发生时间查看评分结果；迟到证据会生成新的窗口修订。</p>
        </div>
        <div className="heading-actions live-actions">
          <Select aria-label="持续评分计划" value={planID} onChange={setPlanID} loading={!plans} options={plans?.map((plan) => ({ value: plan.id, label: `${plan.name} · ${plan.unit}` }))} />
          <Segmented aria-label="报告时间范围" value={range} options={[...ranges]} onChange={(value) => setRange(value as Range)} />
          <Button icon={<RefreshCw size={16} />} onClick={() => { void reloadPlans(); void reloadReport(); }}>刷新</Button>
          <Button type="primary" icon={selectedPlan?.paused ? <Play size={16} /> : <Pause size={16} />} loading={controlling} disabled={!selectedPlan} onClick={() => void control()}>{selectedPlan?.paused ? "恢复评分" : "暂停评分"}</Button>
        </div>
      </div>
      {error ? <ErrorNotice code={error} onRetry={() => { void reloadPlans(); void reloadReport(); }} onDismiss={() => setError(null)} /> : null}
      {!plans?.length ? <EmptyState title="还没有可用的持续评分计划" description="在计划中设置 mode: stream 和明确的 trace 或 turn 单位，重新扫描后会显示在这里。" /> : <>
        {selectedPlan?.paused ? <Alert className="live-paused" type="warning" showIcon title="评分已暂停，接收与持久化仍在继续" description="恢复后会按照接收顺序消化当前积压。" /> : null}
        <div className="live-kpis">
          <Card className="summary-card"><Statistic title="接收量" value={current?.scope.received ?? 0} /></Card>
          <Card className="summary-card"><Statistic title="已评分量" value={current?.scope.statistics_available ? current.scope.scored : "—"} /></Card>
          <Card className="summary-card"><Statistic title={<span className="stat-label">评分覆盖率 <Tooltip title="覆盖率表示有有效评分且未执行失败的请求比例，不是业务正确率。"><Info size={14} /></Tooltip></span>} value={coverage == null ? "—" : coverage * 100} precision={coverage == null ? undefined : 1} suffix={coverage == null ? undefined : "%"} /></Card>
          <Card className="summary-card warning-value"><Statistic title="待处理量" value={selectedPlan?.pending ?? 0} /></Card>
          <Card className="summary-card"><Statistic title={<Tooltip title="从本机接收证据到完成评分计算的耗时，包含排队等待，不等于业务调用耗时。">p95 评分延迟</Tooltip>} value={current?.scope.p95_latency_ms == null ? "—" : Math.round(current.scope.p95_latency_ms)} suffix={current?.scope.p95_latency_ms == null ? undefined : "ms"} /></Card>
        </div>
        {!report?.windows.length ? <EmptyState title={loading ? "正在读取窗口" : "所选时间内还没有事件"} description="可以调整时间范围，或等待已完成的请求上报。" /> : <>
          {!report.series.length ? <Alert type="info" title="当前处理器未提供窗口统计" description="可以查看已接收样本并导出窗口，评分统计暂不可用。" /> :
          <div className="live-chart-grid">
            <Card title="评分覆盖率趋势" extra={<span className="muted">按事件时间</span>}><LiveChart ariaLabel="评分覆盖率趋势图" option={coverageOption} onPoint={openPoint} /></Card>
            <Card title="窗口分布" extra={<span className="muted">状态计数</span>}><LiveChart ariaLabel="已评分、证据不足、失败、无有效评分和待处理的窗口分布图" option={distributionOption} onPoint={openPoint} /></Card>
            <Card title="评分延迟" extra={<span className="muted">p50 / p95</span>}><LiveChart ariaLabel="评分延迟趋势图" option={latencyOption} onPoint={openPoint} /></Card>
            <Card title="积压趋势" extra={<span className="muted">所选请求 · 按接收时间</span>}><LiveChart ariaLabel="所选请求的待处理量随接收和评分完成时间的变化" option={backlogOption} /></Card>
          </div>}
          <section className="panel live-window-panel">
            <div className="panel-toolbar"><h2>窗口明细 <span className="count">{report.windows.length}</span></h2></div>
            <Table className="domain-table" rowKey="id" columns={windowColumns} dataSource={report.windows} pagination={false} scroll={{ x: 840 }} />
          </section>
        </>}
      </>}
      <WindowDrawer open={!!drawerID} window={drawerWindow} latest={report?.windows.find((w) => w.scope.start === drawerWindow?.scope.start)} loading={drawerLoading} error={drawerError} exporting={exporting} onRetry={() => { if (drawerID) void openWindow(drawerID); }} onRefresh={(id) => void openWindow(id)} onClose={() => { drawerVersion.current++; setDrawerID(undefined); setDrawerWindow(undefined); }} onExport={() => void exportWindow()} />
    </>
  );
}
