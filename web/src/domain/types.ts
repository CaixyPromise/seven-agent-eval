export type Asset = {
  kind: string;
  id: string;
  version: string;
  source: string;
  path: string;
  status: string;
  diagnostic_code?: string;
  descriptor?: Record<string, unknown>;
};
export type EvaluationPackage = { id: string; name: string; version: string };
export type Discovery = { package: EvaluationPackage; assets: Asset[] };
export type Metric = {
  metric_id: string;
  status: string;
  value: unknown;
  reason?: string;
  evaluator: string;
  parameters?: Record<string, unknown>;
};
export type CaseResult = {
  redaction_applied?: boolean;
  case: { id: string; inputs: unknown; references?: unknown; metadata?: {episode_id?: string; turn_id?: string} };
  status: string;
  reason?: string;
  result: { response: unknown; contexts?: unknown; retrieval?: RetrievalDocument[]; tools?: unknown[]; citations?: unknown[] };
  metrics: Metric[];
};
export type RunSummary = {
  id: string;
  package: EvaluationPackage;
  status: string;
  created_at: string;
  completed: number;
  total: number;
};
export type Run = RunSummary & {
  dataset_revision?: unknown;
  episodes?: EpisodeResult[];
  scope?: WindowScope;
  cases: CaseResult[];
  outputs: { processor: string; data: Record<string, unknown> }[];
  conclusions: { code: string; level: string; summary: string }[];
  catalogs: unknown;
  effective: unknown;
};
export type EpisodeResult = { id: string; status: string; case_ids: string[]; completed: number };
export type RetrievalDocument = {id: string; text: string; score?: number};
export type Series = {
  metric_id: string;
  observed: number;
  total: number;
  rate?: number;
  mean?: number;
  p95?: number;
  evaluator?: string;
  parameters?: unknown;
};
export type BasicSummary = {
  rag_quality?: RagQuality[];
  scored?: number;
  coverage?: number;
  failure_ids?: string[];
  series?: Series[];
};
export type RagQuality = {
  id: string;
  evaluator: string;
  parameters: Record<string, unknown>;
  points: {
    k: number;
    precision: number | null;
    recall: number | null;
    f1: number | null;
    counts: Partial<Record<"precision" | "recall" | "f1", {observed: number; total: number}>>;
  }[];
};

export type LivePlan = {
  id: string;
  name: string;
  unit: "trace" | "turn";
  paused: boolean;
  pending: number;
  storage_bytes: number;
};

export type WindowScope = {
  statistics_available?: boolean;
  coverage?: number;
  reasons?: Record<string, number>;
  anomalies?: number;
  skipped?: number;
  budget_exhausted?: number;
  unscored?: number;
  kind: "window";
  plan_id: string;
  start: string;
  end: string;
  watermark: string;
  revision: number;
  state: "provisional" | "final";
  received: number;
  eligible: number;
  sampled: number;
  scored: number;
  not_computable: number;
  failed: number;
  pending: number;
  p50_latency_ms?: number;
  p95_latency_ms?: number;
  judge_sample_rate?: number;
  judge_per_window_budget?: number;
};

export type LiveWindow = Run & { scope: WindowScope };

export type LivePoint = {
  unscored?: number;
  time: string;
  coverage?: number;
  received: number;
  scored: number;
  not_computable: number;
  failed: number;
  pending: number;
  p50_latency_ms?: number;
  p95_latency_ms?: number;
};

export type LiveReport = {
  summary?: WindowScope;
  backlog_series?: { time: string; pending: number }[];
  plan: LivePlan;
  from: string;
  to: string;
  current?: LiveWindow;
  windows: LiveWindow[];
  series: LivePoint[];
};
