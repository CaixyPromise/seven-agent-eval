import Badge from "antd/es/badge";
import { statusNames } from "../domain/presentation";
const statusMap: Record<
  string,
  "success" | "processing" | "warning" | "error" | "default"
> = {
  valid: "success",
  completed: "success",
  observed: "success",
  running: "processing",
  processing: "processing",
  invalid: "warning",
  partial: "warning",
  cancelled: "warning",
  error: "error",
  failed: "error",
};
export function StatusBadge({ status }: { status: string }) {
  return (
    <Badge
      status={statusMap[status] ?? "default"}
      text={statusNames[status] ?? "未知状态"}
    />
  );
}
