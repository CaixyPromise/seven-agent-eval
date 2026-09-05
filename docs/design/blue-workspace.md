# Blue workspace screen contract

Users: engineers evaluating Agent/RAG answers. Current executable scope stays offline basic evaluation.
Entry → choose an existing plan → run → inspect exceptions/evidence → export. Import → open report directly.
No creation wizard or speculative production controls. Preserve existing HTTP/worker/processor contracts.

## Visual target

`blue-workspace-prototype.png` is the built-in image generator's ui-mockup, generated 2026-09-05.
Requested 1440×960; output is 1536×1024. Compare the implementation at the output's native size,
DPR 1, light theme, Chinese, system sans-serif. The reference does not define exact fonts or executable behavior.
Primary #2563eb; white sidebar and cards; pale blue-gray canvas; 10px radii; compact hierarchy.
Generated copy is subordinate to this reviewed copy contract and actual data.

## Copy and flow gate

| Fact | Role / primary surface | Lifetime | Next action |
| --- | --- | --- | --- |
| F1 Current page and workspace | Page title / breadcrumb | Navigation | Choose plan or record |
| F2 Available plans and assets | Summary counts, plan card and table | Scan result | Run / inspect / rescan |
| F3 Plan input mode, dataset and recipe | Plan card and on-demand details | Selected plan | Run the existing configuration |
| F4 Object availability and safe diagnostic | Status badge and asset drawer | Until corrected | Fix asset and rescan |
| F5 Run state and completed sample count | Progress region or run row | Live run | Inspect or cancel |
| F6 Quality facts and exceptions | Report summary and metric table | Saved result | Filter anomalies / open evidence |
| F7 Coverage is not business correctness | Beside coverage label | Report | Interpret facts correctly |
| F8 Portable export / import action | Header control and import dialog | User action | Select a result file / download |
| F9 Expected failure and recovery | Affected list / action dialog | Until recovered or dismissed | Retry read / fix plan / choose file |
| F10 Unknown failure without raw details | Same affected region | Until recovered | Refresh records before retrying a business call |
| F11 Exact full evidence, catalog, configuration | Detail drawer / report tabs | On demand | Read full content |
| F12 Empty or loading content | Local skeleton / empty panel | While unavailable | Clear filters / choose plan / import |

Repeated implementation slogans, PHASE/version labels and transport explanations are removed.
Cancel explains that completed samples remain; no new confirmation step for ordinary local execution.
Controls represent existing discovered data, not free text re-entry; no remote resource picker introduced.

## Table widths

| Table | Identity | Values | Status | Actions | Compact policy |
| --- | --- | --- | --- | --- | --- |
| Assets | fluid ≥240px, wraps | type 100, source 100 | 110 | 80 | horizontal scroll ≥680px with complete detail |
| Runs | fluid ≥280px, full ID in detail | progress 120, date 170 | 100 | 90 | horizontal scroll ≥760px |
| Samples | fluid ≥180px | answer ≥220, count90 | 100 | 80 | horizontal scroll ≥700px, full evidence drawer |
| Metrics | fluid ≥220px | value140, coverage130 | parameter details | none | horizontal scroll ≥600px |

## React boundaries

`app/` owns routing and workspace coordination; `api/` owns safe errors and I/O;
`domain/` owns exchange types and pure derived selectors; `features/` owns discovery/runs/report/import;
`components/` owns thin domain adapters around Ant Design; `styles/` owns tokens and layout.
Pages do not call fetch directly. Independent reads begin in parallel; one bounded/coalesced SSE subscription
for the selected live run; abort obsolete read requests; never auto-retry a Subject-creating POST.
Filters and statistics are derived, not effect-synchronized state. Expensive report page is lazy loaded.

Ant Design 6.6.2 supplies buttons, tables, pagination, upload, forms, tabs, alerts, drawers and modals.
This supersedes the initial prototype's custom-control appearance following the user's explicit preference.
ConfigProvider owns component tokens; application CSS is primarily layout. Ant Design supplies overlay
focus containment and default motion; the dialog adapter restores the trigger on conditional unmount.
Reduced motion disables transitions and animations. No animated table rows are introduced.
