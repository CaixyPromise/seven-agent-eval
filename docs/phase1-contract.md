# Phase 1 contract

The project is an independent repository. CLI: seven-eval; Python: seven_evaluate.
Only independent cases execute. Conversation inputs remain opaque; shared session/environment lifecycle and production ingestion are later tasks.

## Effective plan
schema_version: evaluate.plan/v1; modes: invoke or replay. Dataset references use dataset:name.
Omitted stage arrays inherit builtin.basic@1; explicit arrays replace; [] disables. null is invalid.
Built-in references: evaluator:basic, processor:basic-summary, conclusion:basic-conclusion.
Python files declare id/version/kind and supported metrics in their decorator descriptor.
Assets are resolved within one Package and snapshotted with source digests. Modifications require a new run.

## Exchange
protocol/schemas/exchange.json is the JSON Schema 2020-12 contract. IDs are package-scoped.
Each JSON-RPC subprocess receives initialize and one operation (describe, subject/invoke,
evaluator/evaluate, processor/process, conclusion/conclude), then EOF. Logs go to stderr.
Operations are deadline-bound; errors are bounded messages, never scores.
Trusted extensions are process isolated, not sandboxed.

## Limits and defaults
Sequential execution; no subject retries. 1000 cases, 32 MiB dataset, 1 MiB per line,
2 MiB HTTP output, 8 MiB worker output, 120 second per-operation default (maximum 300).
Run summaries and indexed results have a 64 MiB ceiling. Package files and imports reject symlinks.
Replay needs a response field (null is a missing response); invoke passes inputs only.
Local server defaults to loopback; remote serving is not supported in phase 1.
A cancelled run keeps completed cases and produces a partial report; uncertain invocations are never replayed.
Source journal is authoritative; SQLite is rebuildable. Startup seals unfinished runs as partial.

## Evidence and report
Missing/error metrics have null values; boolean false and numeric zero are observed values.
Metric group identity includes metric_id, evaluator, parameters. Built-in conclusions summarize
rules only, never claim business correctness from HTTP success or output presence.
Export is a finite ZIP snapshot (.seven.eval). Import validates paths/digests/size and renders data;
it does not run extension code or untrusted report scripts.
