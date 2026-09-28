import { lazy, Suspense, useRef, useState } from "react";
import Button from "antd/es/button";
import Card from "antd/es/card";
import Statistic from "antd/es/statistic";
import Tooltip from "antd/es/tooltip";
import Tabs from "antd/es/tabs";
import Progress from "antd/es/progress";
import Alert from "antd/es/alert";
import {
  ArrowDownToLine,
  ExternalLink,
  RefreshCw,
  Square,
  ArrowLeft,
  Info,
} from "lucide-react";
import { evaluationApi } from "../../api/client";
import { safeErrorCode, type ErrorCode } from "../../api/errors";
import { basicSummary, formatDate, isLive } from "../../domain/presentation";
import type { CaseResult } from "../../domain/types";
import { useRun } from "../runs/useRun";
import { StatusBadge } from "../../components/StatusBadge";
import { ErrorNotice, LoadingState } from "../../components/Feedback";
import { JsonDetails } from "../../components/JsonDetails";
import { MetricSummary } from "./MetricSummary";
import { EpisodeTable } from "./EpisodeTable";
import { SampleTable } from "./SampleTable";
import { SampleDrawer } from "./SampleDrawer";
const RagQualityChart = lazy(() => import("./RagQualityChart"));
export default function ReportPage({
  id,
  onFinished,
}: {
  id: string;
  onFinished: () => void;
}) {
  const {
    data: run,
    loading,
    error,
    refresh,
    disconnected,
  } = useRun(id, onFinished);
  const [sample, setSample] = useState<CaseResult | null>(null);
  const [actionError, setActionError] = useState<ErrorCode | null>(null),
    [action, setAction] = useState<"cancel" | "export" | null>(null);
  const actionLock = useRef(false);
  async function perform(kind: "cancel" | "export") {
    if (actionLock.current) return;
    actionLock.current = true;
    setAction(kind);
    setActionError(null);
    try {
      if (kind === "cancel") {
        await evaluationApi.cancel(id);
        await refresh();
      } else await evaluationApi.export(id);
    } catch (cause) {
      setActionError(
        safeErrorCode(
          cause,
          kind === "cancel" ? "CANCEL_UNKNOWN" : "EXPORT_UNKNOWN",
        ),
      );
    } finally {
      actionLock.current = false;
      setAction(null);
    }
  }
  const basic = run ? basicSummary(run) : undefined;
  return (
    <>
      <Button
        className="back-link"
        type="link"
        href="#/runs"
        icon={<ArrowLeft size={15} />}
      >
        运行记录
      </Button>
      {error ? (
        <ErrorNotice code={error} onRetry={() => void refresh()} />
      ) : null}
      {!run && loading ? (
        <LoadingState label="正在加载评测报告" />
      ) : run ? (
        <>
          <header className="page-heading">
            <div>
              <h1>{run.package.name}</h1>
              <p className="run-id">
                {formatDate(run.created_at)} · {run.id}
              </p>
            </div>
            <div className="heading-actions">
              {isLive(run.status) ? (
                <Tooltip title="取消后保留已完成的样本">
                  <Button
                    loading={action === "cancel"}
                    disabled={action === "export"}
                    onClick={() => void perform("cancel")}
                    icon={<Square size={15} />}
                  >
                    取消运行
                  </Button>
                </Tooltip>
              ) : (
                <Button
                  type="primary"
                  loading={action === "export"}
                  onClick={() => void perform("export")}
                  icon={<ArrowDownToLine size={16} />}
                >
                  导出结果
                </Button>
              )}
              <Button
                target="_blank"
                rel="noreferrer"
                href={"/api/runs/" + encodeURIComponent(run.id) + "/report"}
                icon={<ExternalLink size={15} />}
              >
                独立报告
              </Button>
            </div>
          </header>
          {run.scope ? <Alert type="info" showIcon title={`窗口快照 · 修订 ${run.scope.revision}`} description={`${run.scope.plan_id} · ${formatDate(run.scope.start)} — ${formatDate(run.scope.end)} · ${run.scope.state === "final" ? "已结束" : "统计中（部分结果）"}`} /> : null}
          {actionError ? (
            <ErrorNotice
              code={actionError}
              onDismiss={() => setActionError(null)}
            />
          ) : null}
          <div className="run-progress">
            <StatusBadge status={run.status} />
            <span>
              {run.completed} / {run.total} 条已处理
            </span>
            <Progress
              aria-label="样本处理进度"
              percent={run.total ? (run.completed / run.total) * 100 : 0}
              showInfo={false}
              status="normal"
              size="small"
            />
            <Button
              aria-label="刷新运行状态"
              loading={loading}
              onClick={() => void refresh()}
              icon={<RefreshCw size={16} />}
            />
          </div>
          {disconnected ? (
            <ErrorNotice code="STREAM_LOST" onRetry={() => void refresh()} />
          ) : null}
          {["cancelled", "partial", "failed"].includes(run.status) ? (
            <Alert
              className="feedback"
              type="warning"
              showIcon
              title="本次为部分结果，仅包含已保留的样本和可用统计。"
            />
          ) : null}
          <div className="summary-grid">
            <Card className="summary-card">
              <Statistic title="样本总数" value={run.total} />
            </Card>
            <Card className="summary-card">
              <Statistic
                title="有有效指标的样本"
                value={basic?.scored ?? "—"}
              />
            </Card>
            <Card className="summary-card">
              <Statistic
                title={
                  <span className="stat-label">
                    评分覆盖率{" "}
                    <Tooltip title="产生有效指标的样本比例，不代表业务正确率">
                      <Button
                        type="text"
                        size="small"
                        aria-label="评分覆盖率说明"
                        icon={<Info size={14} />}
                      />
                    </Tooltip>
                  </span>
                }
                value={
                  basic?.coverage != null
                    ? (basic.coverage * 100).toFixed(0) + "%"
                    : "—"
                }
              />
            </Card>
            <Card className="summary-card warning-value">
              <Statistic
                title="异常样本"
                value={basic?.failure_ids?.length ?? "—"}
              />
            </Card>
          </div>
          <Card className="conclusions" title="评测结论" size="small">
            {run.conclusions.length ? (
              run.conclusions.map((conclusion, index) => (
                <p key={index}>{conclusion.summary}</p>
              ))
            ) : (
              <p>
                {isLive(run.status)
                  ? "评测进行中，完成后生成结论。"
                  : "没有可用结论，可查看样本或处理器输出。"}
              </p>
            )}
          </Card>
          {basic?.rag_quality?.length ? (
            <Suspense fallback={<LoadingState label="正在加载检索质量图表" />}>
              <RagQualityChart groups={basic.rag_quality} />
            </Suspense>
          ) : null}
          {basic?.series?.length ? (
            <MetricSummary series={basic.series} />
          ) : null}
          <section className="panel report-details">
            <Tabs
              defaultActiveKey="samples"
              items={[
                ...(run.episodes?.length ? [{key:"episodes",label:"Episode 与 Turns",children:<EpisodeTable episodes={run.episodes} samples={run.cases} onOpen={setSample} />}] : []),
                {
                  key: "samples",
                  label: "样本与证据",
                  children: (
                    <SampleTable samples={run.cases} onOpen={setSample} />
                  ),
                },
                {
                  key: "outputs",
                  label: "处理器输出",
                  children: (
                    <div className="panel-body">
                      <JsonDetails value={run.outputs} />
                    </div>
                  ),
                },
                {
                  key: "catalogs",
                  label: "码表",
                  children: (
                    <div className="panel-body">
                      <JsonDetails value={run.catalogs} />
                    </div>
                  ),
                },
                {
                  key: "config",
                  label: "执行配置",
                  children: (
                    <div className="panel-body">
                      {run.dataset_revision ? <JsonDetails label="数据版本" value={run.dataset_revision} /> : null}
                      <JsonDetails value={run.effective} />
                    </div>
                  ),
                },
              ]}
            />
          </section>
          {sample ? (
            <SampleDrawer sample={sample} onClose={() => setSample(null)} />
          ) : null}
        </>
      ) : null}
    </>
  );
}
