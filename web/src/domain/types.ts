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
  case: { id: string; inputs: unknown; references?: unknown };
  status: string;
  reason?: string;
  result: { response: unknown; contexts?: unknown };
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
  cases: CaseResult[];
  outputs: { processor: string; data: Record<string, unknown> }[];
  conclusions: { code: string; level: string; summary: string }[];
  catalogs: unknown;
  effective: unknown;
};
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
  scored?: number;
  coverage?: number;
  failure_ids?: string[];
  series?: Series[];
};
