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
import { formatDate, isAnomaly, pretty } from "../../domain/presentation";
import type { LiveWindow } from "../../domain/types";

export function WindowDrawer({
  open,
  window,
  loading,
  error,
  exporting,
  onClose,
  onExport,
}: {
  open: boolean;
  window?: LiveWindow;
  loading: boolean;
  error?: ErrorCode | null;
  exporting: boolean;
  onClose: () => void;
  onExport: () => void;
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
      {error ? <ErrorNotice code={error} /> : null}
      {window && scope ? (
        <div className="drawer-body live-drawer">
          <Alert
            type={scope.state === "final" ? "success" : "info"}
            showIcon
            title={scope.state === "final" ? "窗口已封板" : "窗口仍可能因迟到证据修订"}
            description={`水位线：${formatDate(scope.watermark)} · 修订 ${scope.revision}`}
          />
          <Descriptions column={2} size="small" title="计数漏斗">
            <Descriptions.Item label="接收">{scope.received}</Descriptions.Item>
            <Descriptions.Item label="合格">{scope.eligible}</Descriptions.Item>
            <Descriptions.Item label="选中">{scope.sampled}</Descriptions.Item>
            <Descriptions.Item label="已评分">{scope.scored}</Descriptions.Item>
            <Descriptions.Item label="证据不足">{scope.not_computable}</Descriptions.Item>
            <Descriptions.Item label="执行失败">{scope.failed}</Descriptions.Item>
            <Descriptions.Item label="待处理">{scope.pending}</Descriptions.Item>
            <Descriptions.Item label="p95 评分延迟">
              {scope.p95_latency_ms == null ? "—" : `${scope.p95_latency_ms.toFixed(0)} ms`}
            </Descriptions.Item>
          </Descriptions>
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
            <pre>{pretty({ scope, effective: window.effective })}</pre>
          </section>
        </div>
      ) : null}
    </Drawer>
  );
}
