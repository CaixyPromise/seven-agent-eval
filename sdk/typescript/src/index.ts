import type {
  Case,
  CaseResult,
  SubjectResult,
  Metric,
  ProcessorOutput,
  Conclusion,
  Descriptor,
} from "../dist/exchange";
export type * from "../dist/exchange";

export type Parameters = Record<string, unknown>;
export type SubjectContext = {
  inputs: unknown;
  target_parameters?: Parameters;
  session?: unknown;
};
export type EvaluatorContext = {
  case: Case;
  result: SubjectResult;
  status: string;
  parameters: Parameters;
};
export type ProcessorContext = {
  cases: CaseResult[];
  total: number;
  input_digest: string;
  parameters: Parameters;
  window?: unknown;
  episodes?: unknown;
  comparison?: unknown;
  candidates?: unknown;
};
export type ConclusionContext = {
  outputs: ProcessorOutput[];
  cases: CaseResult[];
  parameters: Parameters;
};
type Callable<C, R> = (context: C) => R | Promise<R>;
type Registration = Pick<Descriptor, "id" | "metrics" | "execution_class"> & {
  version?: Descriptor["version"];
} & Record<string, unknown>;
type Hooks = {
  create?: Callable<Record<string, never>, unknown>;
  cleanup?: Callable<{ session: unknown }, unknown>;
};
function define<C, R>(
  kind: string,
  descriptor: Registration,
  fn: Callable<C, R>,
  hooks: Hooks = {},
) {
  return Object.assign(fn, {
    __seven_descriptor__: {
      ...descriptor,
      version: descriptor.version ?? "1.0.0",
      kind,
      ...(hooks.create ? { session_lifecycle: true } : {}),
      ...(hooks.cleanup ? { session_cleanup: true } : {}),
    },
    __seven_session_create__: hooks.create,
    __seven_session_cleanup__: hooks.cleanup,
  });
}
export const subject = (
  descriptor: Registration,
  fn: Callable<SubjectContext, SubjectResult>,
  hooks: Hooks = {},
) => define("subject", descriptor, fn, hooks);
export const evaluator = (
  descriptor: Registration,
  fn: Callable<EvaluatorContext, Metric[]>,
) => define("evaluator", descriptor, fn);
export const processor = (
  descriptor: Registration,
  fn: Callable<ProcessorContext, ProcessorOutput>,
) => define("processor", descriptor, fn);
export const conclusion = (
  descriptor: Registration,
  fn: Callable<ConclusionContext, Conclusion[]>,
) => define("conclusion", descriptor, fn);
