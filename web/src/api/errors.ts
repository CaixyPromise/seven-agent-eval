export const errorCopy = {
  START_UNAVAILABLE: "评测暂时无法启动。请检查本地评测服务后重试。",
  RUN_BUSY: "已有评测正在运行。请等待完成，或到运行记录中取消。",
  RUN_NOT_ACTIVE: "这次运行已结束。刷新报告可查看最新结果。",
  RUN_NOT_FOUND: "找不到这次运行。请返回运行记录选择其他结果。",
  PLAN_INVALID: "计划暂时不能运行。请检查计划及关联资产，然后重新扫描。",
  PACKAGE_INVALID: "无法读取评测目录。请检查 evaluation.yaml 后重新扫描。",
  BUNDLE_INVALID:
    "结果文件不完整或格式不受支持。请选择有效的 .seven.eval 文件。",
  BUNDLE_TOO_LARGE: "结果文件超过 64 MiB。请选择更小的结果包。",
  BUNDLE_EXISTS: "这份结果已在运行记录中。可关闭导入框并查看已有报告。",
  ACCESS_DENIED: "当前页面不能执行此操作。请从本地工作区地址重新打开。",
  EXPORT_UNAVAILABLE: "这次结果暂时无法导出。请刷新报告，确认运行状态。",
  FILE_TYPE: "请选择 .seven.eval 结果文件。",
  LOAD_ASSETS: "暂时无法加载评测资产。请重新扫描。",
  LOAD_RUNS: "暂时无法加载运行记录。请重试。",
  LOAD_RUN: "暂时无法加载这份报告。请重试。",
  START_UNKNOWN: "未能确认评测是否已启动。请先查看运行记录，避免重复调用。",
  CANCEL_UNKNOWN: "未能确认取消结果。请刷新运行状态。",
  IMPORT_UNKNOWN:
    "未能确认结果是否已导入。请先查看运行记录，再决定是否重新导入。",
  EXPORT_UNKNOWN: "暂时无法下载结果。请稍后再试。",
  STREAM_LOST: "实时更新已断开，正在重新连接。也可以手动刷新状态。",
  LOAD_LIVE_PLANS: "暂时无法读取持续评分计划。请重新扫描或稍后重试。",
  LOAD_LIVE_REPORT: "暂时无法刷新窗口报告。请重试。",
  LIVE_CONTROL_UNKNOWN: "未能确认暂停或恢复操作。请刷新计划状态后再试。",
  LIVE_PLAN_INVALID: "这个计划不能持续评分。请检查 stream 配置后重新扫描。",
  INGEST_BATCH_TOO_LARGE: "这批生产事件超过接收上限。请拆分后重新上报。",
  EVENT_CONFLICT: "事件标识与已接收内容不一致。请使用新的事件标识上报修订。",
  QUEUE_FULL: "评测待处理队列已满。请等待积压下降后继续上报。",
  OTLP_ENCODING_UNSUPPORTED: "当前仅支持 JSON 编码的 OTLP 追踪数据。",
  OTLP_MAPPING_INVALID: "追踪数据缺少持续评分所需字段，尚未接收。",
  WINDOW_NOT_FOUND: "找不到这个时间窗口。请返回窗口列表重新选择。",
  WINDOW_EXPORT_UNAVAILABLE: "这个窗口暂时无法导出。请刷新后重试。",
  LIVE_UNAVAILABLE: "持续评分服务暂不可用。请确认本地服务后重试。",
} as const;
export type ErrorCode = keyof typeof errorCopy;
export class ProductError extends Error {
  constructor(readonly code: ErrorCode) {
    super(code);
  }
}
export function safeErrorCode(value: unknown, fallback: ErrorCode): ErrorCode {
  return value instanceof ProductError ? value.code : fallback;
}
export function responseError(
  body: unknown,
  fallback: ErrorCode,
): ProductError {
  const code =
    typeof body === "object" && body !== null && "code" in body
      ? body.code
      : undefined;
  return new ProductError(
    typeof code === "string" && Object.hasOwn(errorCopy, code)
      ? (code as ErrorCode)
      : fallback,
  );
}
export const diagnosticCopy: Record<string, string> = {
  ASSET_DUPLICATE: "资产标识重复。修改标识及引用后重新扫描。",
  ASSET_REFERENCE: "关联资产不可用。检查计划引用后重新扫描。",
  ASSET_RUNTIME: "此脚本类型暂不支持执行。请使用 Python 扩展。",
  ASSET_INVALID: "资产暂不可用。请检查对应文件的格式和配置后重新扫描。",
};
