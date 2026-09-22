import Alert from "antd/es/alert";
import Button from "antd/es/button";
import Descriptions from "antd/es/descriptions";
import Drawer from "antd/es/drawer";
import Empty from "antd/es/empty";
import List from "antd/es/list";
import Tag from "antd/es/tag";
import type { ErrorCode } from "../../api/errors";
import { ErrorNotice, LoadingState } from "../../components/Feedback";
import { StatusBadge } from "../../components/StatusBadge";
import { formatDate, isAnomaly, pretty, reasonNames } from "../../domain/presentation";
import type { LiveWindow } from "../../domain/types";

export function WindowDrawer({
  open,
  window,
  loading,
  error,
  exporting,
  onClose,
  onExport,
  onRetry,
  onRefresh,
  latest,
}: {
  open: boolean;
  window?: LiveWindow;
  loading: boolean;
  error?: ErrorCode | null;
  exporting: boolean;
  onClose: () => void;
  onExport: () => void;
  onRetry: () => void;
  onRefresh: (id: string) => void;
  latest?: LiveWindow;
}) {
  const scope = window?.scope;
  const anomalies = window?.cases.filter(isAnomaly) ?? [];
  return (
    <Drawer
      width={560}
      open={open}
      onClose={onClose}
      title={scope ? `窗口详情 · ${formatDate(scope.start)}` : "窗口详情"}
      extra={
        window ? (
          <Button type="primary" loading={exporting} onClick={onExport}>
            导出窗口
          </Button>
        ) : null
      }
    >
      {loading ? <LoadingState label="正在读取窗口详情" /> : null}
      {error ? <ErrorNotice code={error} onRetry={error === "WINDOW_EXPORT_UNAVAILABLE" ? onExport : onRetry} /> : null}
      {window && scope ? (
        <div className="drawer-body live-drawer">
          {latest && latest.id !== window.id ? <Alert type="info" title={`已有更新：修订 ${latest.scope.revision}`} action={<Button onClick={() => onRefresh(latest.id)}>查看最新</Button>} /> : null}
          <Alert
            type={scope.state === "final" ? "success" : "info"}
            showIcon
            title={scope.state === "final" ? "窗口已结束，迟到证据仍可修订" : "窗口统计中"}
            description={`水位线：${formatDate(scope.watermark)} · 修订 ${scope.revision}`}
          />
          <Descriptions column={2} size="small" title="计数漏斗">
            <Descriptions.Item label="接收">{scope.received}</Descriptions.Item>
            <Descriptions.Item label="合格">{scope.eligible}</Descriptions.Item>
            <Descriptions.Item label="已执行评测">{scope.statistics_available ? scope.sampled : "—"}</Descriptions.Item>
            <Descriptions.Item label="已评分">{scope.statistics_available ? scope.scored : "—"}</Descriptions.Item>
            <Descriptions.Item label="证据不足">{scope.statistics_available ? scope.not_computable : "—"}</Descriptions.Item>
            <Descriptions.Item label="执行失败">{scope.statistics_available ? scope.failed : "—"}</Descriptions.Item>
            <Descriptions.Item label="涉及未采样">{scope.statistics_available ? scope.skipped : "—"}</Descriptions.Item>
            <Descriptions.Item label="涉及预算耗尽">{scope.statistics_available ? scope.budget_exhausted : "—"}</Descriptions.Item>
            <Descriptions.Item label="待处理">{scope.pending}</Descriptions.Item>
            <Descriptions.Item label="p95 评分延迟">
              {scope.p95_latency_ms == null ? "—" : `${scope.p95_latency_ms.toFixed(0)} ms`}
            </Descriptions.Item>
          </Descriptions>
          <section>
            <h3>指标原因分布</h3>
            <List size="small" locale={{ emptyText: scope.statistics_available ? "没有已记录的指标原因" : "原因统计不可用" }} dataSource={Object.entries(scope.reasons ?? {})} renderItem={([code, count]) => <List.Item><span>{reasonNames[code] ?? "未分类原因"}</span><Tag>{count}</Tag></List.Item>} />
          </section>
          <section>
            <h3>结论</h3>
            {window.conclusions.length ? (
              <List
                size="small"
                dataSource={window.conclusions}
                renderItem={(conclusion) => (
                  <List.Item>
                    <Tag>{conclusion.code}</Tag>
                    {conclusion.summary}
                  </List.Item>
                )}
              />
            ) : (
              <span className="muted">当前配方未输出结论。</span>
            )}
          </section>
          <section>
            <h3>异常样本</h3>
            {anomalies.length ? (
              <List
                size="small"
                dataSource={anomalies.slice(0, 20)}
                renderItem={(sample) => (
                  <List.Item>
                    <div className="window-sample">
                      <strong>{sample.case.id}</strong>
                      <StatusBadge status={sample.status} />
                      <span>{pretty(sample.result.response)}</span>
                    </div>
                  </List.Item>
                )}
              />
            ) : (
              <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="没有异常样本" />
            )}
          </section>
          <section>
            <h3>版本与范围</h3>
            <Descriptions column={1} size="small">
              <Descriptions.Item label="计划">{scope.plan_id}</Descriptions.Item>
              <Descriptions.Item label="窗口结束">{formatDate(scope.end)}</Descriptions.Item>
              <Descriptions.Item label="快照版本">{scope.revision}</Descriptions.Item>
            </Descriptions>
          </section>
        </div>
      ) : null}
    </Drawer>
  );
}
