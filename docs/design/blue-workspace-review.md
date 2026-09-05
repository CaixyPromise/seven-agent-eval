# Blue workspace implementation review

Date: 2026-09-05. Scope: offline evaluation frontend and safe server feedback.

## Before | After | Why

| Before | After | Why |
| --- | --- | --- |
| Single-page composition and handcrafted generic controls | Feature folders and Ant Design 6.6.2 controls | Follow the requested React structure and reuse maintained UI primitives |
| Green visual direction | Blue ConfigProvider tokens, white navigation, pale canvas | Requested blue palette with an uncomplicated plan-to-report flow |
| Unstructured backend failures could reach UI | Approved product codes and local recovery guidance | Avoid leaking stack traces, credentials or private paths |
| Closing an import could allow its late response to navigate | Import-session guard; background completion refresh only | Preserve the user's current action |
| Fixed-interval list requests could cancel slow reads | Schedule the next poll after the current read | Allow slow servers to return useful results |
| Terminal SSE followed by failed GET could leave stale progress | Bounded terminal-read recovery and cleanup | Preserve the final state without replaying Subject calls |
| Conditional overlay unmount could lose keyboard focus | Explicit trigger restoration | Keep keyboard navigation predictable |

## Visual and interaction evidence

Prototype: `blue-workspace-prototype.png`, native 1536×1024. Implementation checked at
1536×1024, DPR 1, light theme, Chinese, and at 390×844. Screenshots are local development
artifacts under `output/playwright/blue-*.png` (not runtime dependencies).

- Discovery, real plan execution, completed report, sample evidence and catalogs checked in Chrome.
- Real cancellation produces a partial report with completed results retained.
- Browser export of `artifacts/phase1/blue-browser.seven.eval` verified by the CLI.
- Import into a separate viewer running with `SEVEN_EVAL_PYTHON=/nonexistent/python` succeeds.
- Corrupt-file rejection hides raw details; choosing a valid file in the same dialog recovers.
- Mobile report, discovery and drawer have no document-level horizontal overflow; tables scroll locally.
- Escape restores the originating sample/asset button. Reduced-motion setting exercised.
- Long identifiers, filtered empty state and controlled unknown API error checked using browser-only fixtures.
- Independent review rechecked import isolation, polling, focus, typed server failures and terminal SSE recovery.

The generated reference defines the home screen's hierarchy, not every state. Ant Design controls,
actual asset counts and actual plan order intentionally differ from the initial mockup. This is a
same-size composition review, **not a claim of pixel-exact reproduction or approval of all screens**.
Full accessibility audit, true 200% browser zoom and cross-browser/platform release testing remain open.
The portable, self-contained HTML report retains its earlier static styling; it does not load Ant Design
or require the React application. Production continuous scoring remains outside this frontend phase.

## Verification commands

Final local verification on 2026-09-05: `make setup && make test && go vet ./... &&
go build -o bin/seven-eval ./cmd/seven-eval` exited 0. Go race suites passed; frontend
8/8 and Python 7/7 passed; TypeScript and Vite production build passed with the size warning below.
`./bin/seven-eval verify artifacts/phase1/blue-browser.seven.eval` exited 0 and returned
`valid: true`, status `completed`, run `20260905T060203-1e743b3e3d89e3a8`.
All repository source files are currently untracked on `codex/offline-eval-phase1`; no remote
repository, commit or push was created in this frontend work.

Run `make setup`, `make test`, `go vet ./...`, and `make build` from the repository root.
Frontend tests use Node's built-in test runner: safe failures, preset semantics, missing/zero distinctions,
processor replacement, and non-retrying mutations. Go tests cover server errors and execution contracts.
Browser failures deliberately injected for recovery tests are expected console HTTP errors.

The Vite production build currently reports an approximately 839 kB main chunk (272 kB gzip).
Report and import views are lazy loaded; the warning is retained rather than hidden. Further dependency
profiling is warranted before production distribution, without splitting Ant Design into circular chunks.
