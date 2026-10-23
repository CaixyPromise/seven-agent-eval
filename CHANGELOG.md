# Changelog

## Unreleased — proposed 0.1.0-alpha.1

Local candidate; no tag, public release or registry publication exists. Licensed under
Apache-2.0, copyright 2026 CaixyPromise. SDK package versions remain 0.1.0 development metadata.
See [candidate release notes](docs/release-alpha.md) for scope and verification.

### Available

- Offline invocation/replay, discoverable Packages, replaceable assessment stages.
- JSONL/CSV/TREC/BEIR; retrieval Precision/Recall/F1/MRR/NDCG/MAP and citation checks.
- Controlled Seven Workspace SSE, history import and sequential Episode sessions.
- Explicit candidate comparisons with frozen inputs and paired samples.
- Python/TS/JS workers, batched Processor state and declared report KPI mapping.
- EEP/OTLP JSON scoring, deduplication/revisions, sampling/budgets and window exports.
- Bounded input/results, recovery and opt-in stopped-service retention maintenance.
- Ant Design/ECharts reports and portable 2.0 viewer without Python/Node.
- Directory install/doctor/uninstall; macOS arm64 and Debian arm64 container evidence.

### Breaking development changes

Release preparation also updates Vite within major 6, source-map-js and the TS SDK build-time
esbuild to address dependency audit findings, and adds CI, public-content checks and bundled notices.

Format 2.0 and batched Processors replace earlier development packages/state and whole-Run
Processor APIs. Old 1.0 bundles are rejected. Regenerate fixtures; no migration layer.

### Known limits

Real Seven/RAG/model quality and production T14 remain pending. No Windows/x86_64 install claim,
signing/notarization, auto-updater, third-party npm imports, OTLP protobuf, remote authentication,
production SLA or automatic parameter search. Vite large chunks remain a performance risk.
See [bounds](docs/bounded-evaluation.md) and [installation matrix](docs/installation.md).
