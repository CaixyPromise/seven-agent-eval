# Seven Agent Eval

Independent Go + Python + React monorepo for portable, evidence-based Agent/RAG evaluation.
Phase 1 delivers offline HTTP invocation, historical-result evaluation, package discovery,
replaceable default evaluations/statistics/conclusions, a local UI and `.seven.eval` reports.

## Prepare and build

Development baseline: Go 1.25, uv with Python 3.11, Node 20.19+ and pnpm 10.
From this repository:

```sh
make setup
make build
make test
python3 scripts/acceptance.py
```

The developer setup installs the Python worker (including default evaluators), frontend dependencies
and UI build. It does not need model credentials. Full cross-platform installer packaging is T13.

## Try the report

```sh
./bin/seven-eval --package examples/history discover
./bin/seven-eval --package examples/history plan inspect smoke
./bin/seven-eval --package examples/history run smoke
./bin/seven-eval --package examples/history serve
```

Open http://127.0.0.1:7431. Choose a plan in Discovery, run it, inspect samples and export its report.
There is one active run per workspace. While the server owns a workspace, start runs from the browser;
stop that server before using a local CLI run on the same workspace.

For a real local HTTP call, start the included target in another terminal:

```sh
python3 scripts/http_fixture.py
./bin/seven-eval --package examples/http run smoke
```

The fixture expects only the question, never the reference. It binds 127.0.0.1:7432 and needs no secrets.

## Create your own package

```sh
./bin/seven-eval init /tmp/my-evaluation
./bin/seven-eval --package /tmp/my-evaluation discover --json
./bin/seven-eval --package /tmp/my-evaluation run smoke
```

```text
my-evaluation/
├── evaluation.yaml
├── plans/smoke.yaml
├── datasets/sample/cases.jsonl
├── adapters/                 # optional HTTP YAML or Python subjects
├── evaluators/               # optional Python evaluators
├── processors/               # optional Python statistics
├── conclusions/              # optional Python conclusions
├── catalogs/                 # custom metric definitions
└── tests/                    # not discovered as runtime assets
```

Package-local IDs use `dataset:sample`, `adapter:local-http`, `processor:team-summary`.
A dataset directory can contain `cases.jsonl` or `dataset.yaml` with a relative `source`.
File mappings support `$` and `$.field.nested`, JSON objects/arrays and literals only.

```yaml
schema_version: evaluate.plan/v1
id: smoke
mode: replay                # invoke calls the configured subject
dataset: dataset:sample
# preset defaults to builtin.basic@1
# subject: adapter:local-http
# processors: [processor:team-summary]   # replaces this stage
# conclusions: []                        # explicitly disables this stage
```

```json
{"id":"refund","inputs":{"question":"How long?"},"response":"3 days","contexts":["Refunds take 3 days."],"references":{"exact":"3 days"}}
```

Replay requires a response field; null represents missing output. References are visible to evaluators
but never automatically passed to a Subject. Free-form language is not automatically evaluated by exact
match: provide `references.exact` only when exact equality is meaningful, or `references.schema` for JSON output.
Calling a target uses the same case shape without needing response.

## Default and custom extensions

Omit a stage to inherit the default recipe. Specify a list to replace it. Use `[]` to disable it.
`null` is invalid. Defaults produce output/rule checks, per-metric coverage and statistics, and a
conclusion that explains verified rules and evidence limitations. HTTP success is not task success.
No paid judge is called automatically.

```python
from seven_evaluate import processor

@processor(id="team-summary", version="1.0.0")
def summarize(context):
    return {
        "processor": "team-summary@1.0.0",
        "input_digest": context["input_digest"],
        "data": {"sample_count": len(context["cases"])},
    }
```

Place this in processors/team_summary.py. The custom example in examples/history includes a working plan.
Replacing statistics may make the default conclusion's dependency unavailable; the report explains that
instead of reusing stale statistics. Custom data renders as a structured table/JSON view.

Subject decorators receive `{"inputs": ...}`; evaluators receive case, result, subject status/reason;
processors receive cases, total and input_digest; conclusions receive outputs and cases.
Async callables are supported. One public decorated asset per file. Helpers such as _helper.py are
not registered but are included in the execution snapshot. Extensions are trusted local Python code.
A run snapshots Package and SDK source and records their digests. Dependencies use the prepared Python
environment; use SEVEN_EVAL_PYTHON to select an explicitly prepared interpreter.

## Portable results

```sh
./bin/seven-eval verify artifacts/phase1/demo.seven.eval
./bin/seven-eval --package /tmp --state /tmp/seven-viewer serve artifacts/phase1/demo.seven.eval
```

A result file is a finite ZIP snapshot containing manifest, shared schema, effective plan,
catalogs, evidence/results and self-contained HTML. The server renders verified data, not imported scripts.
Viewing a saved result does not launch Python or call the original target. Export refuses to overwrite
an existing file. Use a new destination filename.

The acceptance script deletes its original source Package before verifying the copied report.
Source snapshots remain private runtime files and are not included as executable content in the export.

## Limits and operation semantics

- Offline execution is sequential, at most 1000 cases / 32 MiB input, 1 MiB per JSONL record.
- HTTP output is capped at 2 MiB, worker output at 8 MiB, export at 64 MiB uncompressed.
- Per-operation timeout defaults to 120 seconds, configurable up to 300. No Subject retries.
- Cancelling stops further calls and preserves a partial report; graceful finishing after cancellation
  has a separate 30-second cap. Restart marks an unfinished run partial and never replays side effects.
- SQLite is a disposable read model reconstructed from the event journal. A filesystem lock prevents
  two coordinators from using the same workspace simultaneously.
- Scoring uses original in-memory values. Stored evidence is redacted by known credential keys and
  configured auth environment values, with redaction_applied recorded; redacted exports may not support
  exact re-evaluation. This is not a general PII detector.
- Local server only: no remote bind, authentication/multi-tenancy or untrusted code sandbox in phase 1.
- Controlled offline, live EEP/OTLP JSON, Agent/RAG, Episode, Python/Node extensions and explicit
  candidate comparison have local acceptance records in `docs/phase2-progress.md`,
  `docs/phase3-progress.md` and `docs/phase4-progress.md`. These do not claim real production
  acceptance, arbitrary OTLP graphs, automatic parameter search or cross-platform installation.

## Explicit retention and resilience

Retention is opt-in maintenance, with the server stopped. Preview first, then apply the exact
digest returned by the preview; changes to authoritative files invalidate it:

```sh
./bin/seven-eval --package /path/to/package retention preview 2026-10-01T00:00:00Z
./bin/seven-eval --package /path/to/package retention apply PREVIEW_DIGEST
make acceptance-resilience
```

Export required evidence before applying: cleanup deletes evidence bodies, not just the query index.
Pending windows, referenced runtime snapshots, partial runs and unknown orphan snapshots are protected.
Deduplication receipts and expired-window ranges remain, so old traffic cannot silently be rescored.
Exported `.seven.eval` files remain independent. This first maintenance implementation does not promise
constant memory for unlimited historical receipts or zero-downtime cleanup.

See `docs/phase5-progress.md` for measured workloads, fault tests, current capacity gaps and exact
maintenance semantics. During a running offline evaluation, `result.json` is a lightweight checkpoint;
use CLI/server queries for complete live results. The journal is authoritative, SQLite is rebuildable,
and terminal checkpoints and portable exports still contain the complete Run.

See docs/phase1-contract.md for the public contract and docs/verification.md for current acceptance evidence.
