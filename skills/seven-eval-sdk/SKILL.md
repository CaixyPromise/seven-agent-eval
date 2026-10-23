---
name: seven-eval-sdk
description: Develop and validate Seven Agent Eval Python or TS/JS extensions. Use for Subjects, Evaluators, Processors, Conclusions, KPI mapping and portable evaluation packages.
license: Apache-2.0
---

# Seven Eval SDK

Build an extension that is discoverable, produces truthful metrics/statistics, runs against a
small fixture, and exports a verifiable `.seven.eval` report. Use the user's existing language
and Package unless a different target is requested.

## Locate the installation and task

Find the tool home: it contains `sdk/`, `protocol/schemas/exchange.json`, `examples/` and `docs/`.
It may be a source checkout or an extracted/installed directory. Find the user's Package through
`evaluation.yaml`; keep writable Packages outside a managed installation.
If this Skill was copied independently, ask for or locate the tool home; never assume a host-specific path.
Resolve this Skill's `references/` and `assets/` relative to its own directory.

Use the installed `bin/seven-eval`, or run `make setup && make build` in a source checkout if needed.
Use Python 3.11; TS/JS needs Node >=20.19. `SEVEN_EVAL_PYTHON` and `SEVEN_EVAL_NODE` select prepared runtimes.
Do not suggest `pip install seven-evaluate` or `npm install @seven-agent/eval`: registry publication
is not part of the current release. Do not modify global AI/tool configuration to activate this Skill.

Select the extension by responsibility:

| Kind / folder | Responsibility |
|---|---|
| Subject / `adapters/` | Call the target with inputs and explicit parameters; return response and evidence |
| Evaluator / `evaluators/` | Return observations with status, metric/evaluator identity and parameters |
| Processor / `processors/` | Aggregate bounded batches; produce statistics, optional homepage KPIs |
| Conclusion / `conclusions/` | Interpret processor outputs and explain evidence limitations |

Read [the exchange and SDK contract](references/sdk-contract.md) before implementing.
For detailed fields, use the tool home's schema and the matching `docs/python-extensions.md`
or `docs/typescript-extensions.md`. Inspect an existing example for the chosen kind rather than
inventing a second protocol. Source/Mapping changes use `docs/phase3-usage.md`.

## Implement in the Package

Each entry exports exactly one registered function; `_` helpers are not discovered as assets.
Python uses `seven_evaluate`; TS/JS uses ESM imports from `@seven-agent/eval`.
TS/JS supports SDK, `node:` built-ins and Package-local relative modules; third-party bare imports
and CommonJS are unsupported. Discovery loads trusted module top-level code, so keep target calls
inside the registered function. Process isolation is not a sandbox.

For a summary Processor, adapt either [Python template](assets/team_summary.py) or
[TS template](assets/team_summary.ts) into `processors/`. Both use `team-summary@2.0.0`,
count units with at least one observed metric as scored, and count explicit false rule results,
metric errors or non-completed cases as anomalies. Numeric zero is not itself an anomaly.
Do not install both templates with the same ID in one Package. Different algorithms need distinct
identities/versions; metric aggregation must keep evaluator and all parameters, including K/judge identity.

Set `processors: [processor:team-summary]` in a plan to replace default statistics.
Omitting a stage inherits the preset; `[]` disables it; null is invalid. For a custom Processor,
use `conclusions: []` when no matching Conclusion is requested, or implement the appropriate
Conclusion rather than expecting the default to consume unknown outputs.

Processor phases are initialize/update/finalize. State must remain JSON-serializable and bounded;
use `ctx.workspace` for needed disk-backed aggregation, not an ever-growing list of all cases.
Each operation uses a fresh process. Subject session state is serialized through Go, not module globals.

## Validate a complete small flow

Use a fresh temporary Package or the user's authorized test Package. Start with replay data to
avoid calling a business target just to test aggregation. Fixtures should include an observed false,
numeric zero, missing/null evidence and an explicit evaluator error when relevant.

```sh
seven-eval --package PACKAGE discover --json
seven-eval --package PACKAGE plan inspect PLAN --json
seven-eval --package PACKAGE run PLAN
seven-eval --package PACKAGE export RUN_ID ./unique-result.seven.eval
seven-eval verify ./unique-result.seven.eval
```

Use the explicit binary path if it is not on PATH. Create unique export destinations; export refuses
to overwrite. Inspect descriptor readiness, actual metrics, Processor output and KPI pointers.
The templates count observed false as scored AND anomalous: coverage is not correctness.
Fix invalid output; never substitute missing/error values with zero to make a report pass.

When UI behavior is requested, stop CLI runs before starting `serve` on the same Package, and inspect
the report/KPIs and evidence. For portability, reopen the exported file with a viewer-only installation
and unavailable Python/Node runtimes; compare outputs/Conclusions and do not execute imported scripts.
Source checkouts provide `make acceptance-typescript` for the maintained cross-language fixture;
do not claim it tested a newly authored extension unless that extension was actually exercised.

## Hand off

Report files and logical references, metric/KPI definitions and denominators, runtime requirements,
commands and exit codes, exported artifact path, and remaining unverified behavior.
Use bounded synthetic data, clean only owned temporary artifacts, and preserve the small result
needed for review. No automatic Subject retry, paid judge, production replay, publish or push is
implied by writing an extension.
