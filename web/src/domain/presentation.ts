import type { Asset, BasicSummary, CaseResult, Run } from "./types";
export const assetKinds: Record<string, string> = {
  plan: "计划",
  dataset: "数据集",
  adapter: "适配器",
  evaluator: "评测器",
  processor: "处理器",
  conclusion: "结论",
  catalog: "码表",
  preset: "预设",
};
export const metricNames: Record<string, string> = {
  "output.present": "回答存在性",
  "output.exact": "参考答案匹配",
  "output.schema": "输出格式",
  "latency.ms": "响应耗时",
  "usage.tokens": "Token 用量",
  "usage.cost": "调用费用",
  "evaluation.error": "评测执行",
};
export const statusNames: Record<string, string> = {
  valid: "可用",
  invalid: "需处理",
  completed: "已完成",
  running: "运行中",
  processing: "统计中",
  partial: "部分结果",
  cancelled: "已取消",
  failed: "未完成",
  error: "执行错误",
  observed: "已评分",
  skipped: "已跳过",
  not_applicable: "不适用",
  not_computable: "证据不足",
  provisional: "统计中",
  final: "已结束",
};
export const reasonNames: Record<string, string> = {
  evidence_missing: "缺少可计算的证据",
  reference_missing: "未声明相应约束",
  usage_missing: "未提供可用用量",
  evaluator_error: "评测执行失败",
  insufficient_evidence: "不足以判断任务质量",
  dependency_missing: "所需统计输出不可用",
  not_sampled: "未被采样选中",
  judge_budget_exhausted: "窗口评测预算已用完",
  evaluation_interrupted: "评分中断，未重试不确定的调用",
  missing_response: "缺少回答",
  missing_reference: "未提供参考答案",
  missing_schema: "未配置格式规则",
  missing_latency: "没有耗时记录",
  missing_usage: "没有用量记录",
  subject_error: "目标调用未完成",
  worker_error: "评测脚本未完成",
  cancelled: "运行已取消",
};
export const isLive = (status: string) =>
  status === "running" || status === "processing";
export const pretty = (value: unknown): string =>
  value == null
    ? "—"
    : typeof value === "string"
      ? value
      : JSON.stringify(value, null, 2);
export function basicSummary(run: Run): BasicSummary | undefined {
  return run.outputs.find((o) => o.processor === "builtin.basic-summary@1")
    ?.data as BasicSummary | undefined;
}
export const isAnomaly = (sample: CaseResult) =>
  sample.status !== "completed" ||
  sample.metrics.some(
    (m) =>
      m.status === "error" || (m.status === "observed" && m.value === false),
  );
export function recipeName(asset: Asset): string {
  const descriptor = asset.descriptor;
  if (
    Array.isArray(descriptor?.evaluators) &&
    descriptor.evaluators.length === 0 &&
    Array.isArray(descriptor.processors) &&
    descriptor.processors.length === 0
  )
    return "仅保留结果";
  return Array.isArray(descriptor?.processors) ? "自定义统计" : "默认评测";
}
export const formatDate = (value: string) =>
  new Intl.DateTimeFormat("zh-CN", {
    month: "2-digit",
    day: "2-digit",
    hour: "2-digit",
    minute: "2-digit",
    hour12: false,
  }).format(new Date(value));
