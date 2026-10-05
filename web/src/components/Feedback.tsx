import Alert from "antd/es/alert";
import Button from "antd/es/button";
import Empty from "antd/es/empty";
import Skeleton from "antd/es/skeleton";
import Space from "antd/es/space";
import { errorCopy, type ErrorCode } from "../api/errors";
export function ErrorNotice({
  code,
  onRetry,
  onDismiss,
}: {
  code: ErrorCode;
  onRetry?: () => void;
  onDismiss?: () => void;
}) {
  return (
    <Alert
      className="feedback"
      type={code === "STREAM_LOST" ? "warning" : "error"}
      showIcon
      title={errorCopy[code]}
      closable={!!onDismiss}
      onClose={onDismiss}
      action={
        onRetry && code !== "DATA_EXPIRED" && code !== "WINDOW_CAPACITY_EXCEEDED" ? (
          <Button size="small" onClick={onRetry}>
            重试
          </Button>
        ) : undefined
      }
    />
  );
}
export function EmptyState({
  title,
  description,
  action,
}: {
  title: string;
  description?: string;
  action?: React.ReactNode;
}) {
  return (
    <Empty
      className="empty-state"
      image={Empty.PRESENTED_IMAGE_SIMPLE}
      description={
        <Space orientation="vertical" size={4}>
          <strong>{title}</strong>
          {description ? <span className="muted">{description}</span> : null}
        </Space>
      }
    >
      {action}
    </Empty>
  );
}
export function LoadingState({ label = "正在加载" }: { label?: string }) {
  return (
    <div className="loading-state" role="status" aria-label={label}>
      <Skeleton active paragraph={{ rows: 4 }} />
      <span className="sr-only">{label}</span>
    </div>
  );
}
