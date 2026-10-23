# Exchange and extension contract

Tool-home `protocol/schemas/exchange.json` is authoritative. This Skill targets exchange 2.0;
old whole-Run Processor interfaces and 1.0 bundles are unsupported. Read only the relevant
SDK section and schema definition when adding fields.

## Registration and context

Python registers ordinary functions with `@subject`, `@evaluator`, `@processor`, `@conclusion`.
TS/JS uses `export default processor({id, version}, handler)` and equivalent functions for other kinds.
Both support async functions. Descriptors use kind, id, version and optional metrics/execution_class.
Package refs are `adapter:ID` for scripted Subjects and HTTP/Seven adapters, and
`processor:ID`, `evaluator:ID`, `conclusion:ID`. Confirm actual discover output before referencing.

| Operation | Context / result |
|---|---|
| Subject invoke | inputs, target_parameters, session → SubjectResult with response |
| Evaluator | case, result, status, parameters → Metric array |
| Processor initialize | phase, state=null, cases=null, total/completed, parameters → state |
| Processor update | same context plus bounded cases and previous state → new state |
| Processor finalize | cases=null plus final state/input_digest → ProcessorOutput |
| Conclusion | outputs, total, completed, parameters → Conclusion array |

Inspect SDK for precise supported fields. Do not infer a response from HTTP success or invent an effect
observation from an empty tool trace. Subject receives no references by default.

Session hooks: Python uses `session_create`/`session_cleanup` keyword handlers on `subject`;
TS uses a third argument `{create, cleanup}`. Creation returns serializable session data;
cleanup receives `{session}`. Episode shares the created session across ordered turns, with
isolation between Episodes. Optional cleanup requires `parameters.cleanup_sessions: true`.
No operation may depend on prior worker process memory.

## Metrics and missing values

Observed metrics have `status: observed`; false and numeric zero are valid observations.
Missing evidence, inapplicability, sampling/budget skips and execution error remain distinct
statuses/reasons according to schema. Preserve absent/null values, not fabricated zeros.
Use built-in metric constructors or a schema-conforming fixture for all required fields.
Metric identity includes evaluator, metric and parameters; never merge @1 with @5, or two judge configs.
Node preserves large JSON integers through bigint; do not coerce them to number first.
undefined, nonfinite values and non-JSON objects are not valid outputs.

## Processor and KPI

`update` receives up to 32 CaseResult values, bounded further by bytes. State is passed between
fresh processes; keep it bounded, or use workspace SQLite for exact grouped aggregation.
Finalize returns processor identity, input_digest, data, and optional report mapping.

```json
{
  "processor": "team-summary@2.0.0",
  "input_digest": "provided-by-context",
  "data": {"scored_count": 2, "coverage_ratio": 0.5, "anomaly_count": 1},
  "report": {"version": "1", "kpis": {
    "scored": "/scored_count", "coverage": "/coverage_ratio", "anomalies": "/anomaly_count"
  }}
}
```

Paths are JSON Pointers relative to data, not expressions. Count fields are nonnegative safe integers;
coverage is 0–1. Missing mappings/null render `—`; a declared missing path/type error invalidates output.
Multiple report providers cause a source conflict, not an implicit merge. Total comes from Run.
Scored is a Case with an observed metric; it can still fail a rule. Coverage is not accuracy.
No default output fallback when custom Processors are disabled or replaced.
KPI mapping does not generate conclusions or a generic chart system.

## Language and limits

Go owns dispatch, timeouts/cancellation, stream sampling and budgets, persistence and package validation.
Node executes TS/JS extensions, not the HTTP server. Console logs go to stderr; stdout is JSON-RPC.
SDK hides protocol details. Do not print protocol frames or catch errors and return success scores.
Selected code/helpers are frozen for execution. Runtime dependency preparation is explicit.
Per-operation deadlines, output and state size limits still apply; see tool-home
`docs/bounded-evaluation.md` for current byte/count limits.

## Portable validation

A 2.0 `.seven.eval` includes configuration, schema, catalogs, input/result partitions, metrics,
Processor/Conclusion outputs and self-contained HTML. It is a snapshot, not executable extension code.
Viewer-only installs do not need Node/Python. Missing custom runtime at viewing time is expected;
missing runtime during execution is an explicit unavailable/error state.
