# Seven Eval Web

React 19 + TypeScript + Vite + Ant Design 6.6.2. Blue theme and Chinese locale are configured once;
the UI calls the existing Go API and never runs evaluators itself.

```sh
pnpm install --frozen-lockfile
pnpm dev     # Vite proxies /api to the Go server on 127.0.0.1:7431
pnpm test    # Node test runner + TypeScript compilation, no extra test framework
pnpm build  # type check and production assets
```

## Structure

```text
src/
  main.tsx                # root, locale, ConfigProvider
  app/                    # shell, hash routes, page boundary, theme, coordination
  api/                    # HTTP client, safe product errors, cancellable read hook
  domain/                 # exchange types, labels, pure display selectors
  features/
    discovery/            # plan cards, asset table and details
    runs/                 # run list and one selected-run SSE subscription
    reports/              # report page, metric table, samples and evidence
    import/               # Form + Upload + Modal import flow
  components/             # small shared domain adapters, not a parallel UI library
  styles/                 # product tokens, layout and responsive composition
tests/                    # pure display, error, and no-auto-retry tests
scripts/                  # test runner
```

Routes: `#/plans`, `#/runs`, `#/reports`, `#/reports/:runId`. Hash routing works when served by
the Go file server without extra rewrite rules and preserves browser back/forward and reload.

## Component conventions

- Use Ant Design first: Button, Table/pagination, Input, Segmented, Card, Statistic, Badge, Tooltip,
  Drawer, Modal, Form, Upload, Tabs, Collapse, Alert, Skeleton, Empty, Menu and Breadcrumb.
- `StatusBadge` maps evaluation statuses to Badge; `Dialog` keeps shared Drawer focus restoration;
  `Feedback` maps safe product codes to Alert. They do not reimplement library controls.
- Keep a page's composition in its feature. Reuse domain components only across genuine shared behavior.
- Import component implementations from `antd/es/<component>`; root imports are type-only.
- UI theme belongs in `app/theme.ts`. CSS owns layout and explicit width/overflow, not control behavior.
- Independent asset/run reads start together. Obsolete reads abort. SSE refreshes are coalesced and
  terminal report reads retry at most three times (1/2/4 seconds). Subject-creating POSTs never auto-retry.
- Run list polling waits for the previous read to settle, then schedules the next one.
- Report and import screens load lazily. Do not introduce cross-dependent manual vendor chunks merely
  to hide a size warning. The current AntD entry chunk is above Vite's advisory 500kB threshold.
- Errors render from `api/errors.ts` allowlisted codes. Never display arbitrary response error bodies.
- Package-local asset lists are bounded by the offline discovery protocol; current Table pagination is
  client-side. This is not a server-paginated production-scale resource selector.

Tests include false vs zero, recipe replacement/disable, no implicit correctness inference,
safe known/unknown error messages and no Subject retries. Browser acceptance and review findings
are recorded in `../../seven-agent-docs/seven-agent-eval/docs/design/blue-workspace-review.md`.
