# Seven Agent Eval

Independent Go + Python + TypeScript + React monorepo for portable, evidence-based Agent/RAG evaluation.
It provides offline invocation/history evaluation, continuous event scoring, explicit candidate
comparison, replaceable evaluations/statistics/conclusions, a local UI and portable `.seven.eval` reports.

**Status: local Alpha release candidate; not published.** Project licensing awaits the owner's
decision; see [license recommendation](docs/licensing.md). This checkpoint does not grant an
open-source license. Real production acceptance (T14) remains pending.

## Start here

- **Use the tool:** [directory installation](docs/installation.md) and its credential-free example.
  Public release downloads are not available yet.
- **Build from source:** follow the commands below; fixtures use controlled local targets.
- **Evaluate RAG:** [sources, mappings, retrieval metrics and judge](docs/phase3-usage.md).
- **Score events:** [continuous scoring](docs/continuous-scoring.md).
- **Compare candidates:** [parameter comparison](docs/parameter-comparison.md).
- **Extend:** [Python](docs/python-extensions.md), [TS/JS](docs/typescript-extensions.md).
- **Reference:** [current documentation](docs/README.md), [release notes](CHANGELOG.md),
  [security boundaries](SECURITY.md).

The core is platform-independent; Seven Agent is an adapter. Missing evidence and execution
errors remain distinct from zero scores. Default conclusions do not infer business correctness.

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
and UI build. It does not need model credentials. For directory distributions, offline installation,
runtime diagnostics and a Python/Node-free viewer, see [Installation](docs/installation.md).
Actual native/container platform validation is recorded in [Phase 5 progress](docs/phase5-progress.md).

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
├── sources/                  # JSONL/CSV/TREC/BEIR source descriptors
├── mappings/                 # restricted field mappings
├── adapters/                 # HTTP/Seven YAML or Python/TS/JS subjects
├── evaluators/               # Python/TS/JS evaluators
├── processors/               # Python/TS/JS batched statistics
├── conclusions/              # Python/TS/JS conclusions
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

@processor(id="team-summary", version="2.0.0")
def summarize(context):
    if context["phase"] == "initialize":
        return {"sample_count": 0}
    if context["phase"] == "update":
        return {"sample_count": context["state"]["sample_count"] + len(context["cases"])}
    return {
        "processor": "team-summary@2.0.0",
        "input_digest": context["input_digest"],
        "data": context["state"],
    }
```

Place this in processors/team_summary.py. The custom example in examples/history includes a working plan.
Replacing statistics may make the default conclusion's dependency unavailable; the report explains that
instead of reusing stale statistics. Custom data renders as a structured table/JSON view.

Subject decorators receive `{"inputs": ...}`; evaluators receive case, result, subject status/reason;
processors receive initialize/update/finalize phases, bounded case batches and serialized state;
conclusions receive outputs, total and completed (not all cases).
Async callables are supported. One public decorated asset per file. Helpers such as _helper.py are
not registered but are included in the execution snapshot. Extensions are trusted local Python code.
A run snapshots Package and SDK source and records their digests. Dependencies use the prepared Python
environment; use SEVEN_EVAL_PYTHON to select an explicitly prepared interpreter.

## Portable results

```sh
./bin/seven-eval verify artifacts/phase1/demo.seven.eval
./bin/seven-eval --package /tmp --state /tmp/seven-viewer serve artifacts/phase1/demo.seven.eval
```

A version-2 result file is a finite ZIP snapshot containing manifest, shared schema, effective plan,
catalogs, partitioned input/result JSONL and a self-contained summary HTML. The viewer pages through
sample summaries and loads evidence on demand. Version-1 packages and old state are rejected;
regenerate them rather than migrate. The server renders verified data, not imported scripts.
Viewing a saved result does not launch Python or call the original target. Export refuses to overwrite
an existing file. Use a new destination filename.

The acceptance script deletes its original source Package before verifying the copied report.
Source snapshots remain private runtime files and are not included as executable content in the export.

## Limits and operation semantics

- Offline execution is sequential: at most 10,000 expanded cases, 1 GiB per source file,
  2 MiB per normalized input record. All inputs are validated before any Subject call.
- HTTP output is capped at 2 MiB, worker output at 8 MiB, result records at 4 MiB and
  metadata at 8 MiB. ZIP parts are at most 16 MiB, with at most 4,096 entries and 4 GiB
  total uncompressed content. These are separate safety limits, not a throughput SLA.
- Builtin exact aggregation uses SQLite; Python/TS custom processors use the same reducer
  contract. See [bounded execution contract](docs/bounded-evaluation.md).
- Per-operation timeout defaults to 120 seconds, configurable up to 300. No Subject retries.
- Cancelling stops further calls and preserves a partial report; graceful finishing after cancellation
  has a separate 30-second cap. Restart marks an unfinished run partial and never replays side effects.
- SQLite is a disposable read model reconstructed from the event journal. A filesystem lock prevents
  two coordinators from using the same workspace simultaneously.
- Scoring uses original in-memory values. Stored evidence is redacted by known credential keys and
  configured auth environment values, with redaction_applied recorded; redacted exports may not support
  exact re-evaluation. This is not a general PII detector.
- Local server only: no remote bind, authentication/multi-tenancy or untrusted code sandbox.
- Controlled offline, live EEP/OTLP JSON, Agent/RAG, Episode, Python/Node extensions and explicit
  candidate comparison have local acceptance records in `docs/phase2-progress.md`,
  `docs/phase3-progress.md` and `docs/phase4-progress.md`. These do not claim real production
  acceptance, arbitrary OTLP graphs or automatic parameter search. The separately validated
  installation scope is macOS arm64 and Debian arm64 containers; see the installation matrix.

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

See `docs/phase5-progress.md` for measured workloads, fault tests and maintenance evidence.
Run metadata is kept separate from partitioned case results; use CLI/server queries to page through
evidence. The journal is authoritative, SQLite is rebuildable, and portable 2.0 exports contain
metadata plus partitioned input/result files. See the bounded execution contract for current limits.

See [current contracts](docs/README.md) and [release preparation evidence](docs/open-source-readiness.md).
Phase-numbered records retain historical evidence; their old limits and future-task statements
do not override the current contract.
