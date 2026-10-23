# Contributing

This is a local Alpha candidate licensed under [Apache-2.0](LICENSE). Public contribution
intake will open when the repository is published. Contributions intentionally submitted for
inclusion are licensed under Apache-2.0 unless explicitly stated otherwise, as described in
section 5 of the license. Only submit material you are entitled to contribute; do not submit
confidential code/data. See [licensing scope](docs/licensing.md).

## Development

Use Go 1.25, Python 3.11 via uv, Node 20.19+ and pnpm 10.24.0. From the root:

```sh
make setup
make test
go vet ./...
make build
python3 scripts/acceptance.py
make acceptance-rag
make acceptance-typescript
make acceptance-compare
make acceptance-live
make acceptance-skill
```

Tests use controlled loopback targets and synthetic inputs. `make acceptance-seven` tests
controlled SSE/Episode behavior, not a real deployment. Large capacity tests are opt-in;
do not add million-observation workloads to PR CI.

Run `python3 scripts/public_content_check.py` and `git diff --check` before review.
The CI uses read-only permissions, pinned actions and no publishing credentials.
For clean-source and installed-package checks see [release preparation](docs/open-source-readiness.md).
For AI-assisted extension development, read the portable [SDK Skill](skills/seven-eval-sdk/SKILL.md)
and [setup guide](docs/ai-development.md). Keep templates aligned with the SDK contract and run
`make acceptance-skill` when changing them. The selected release candidate is `v0.1.0-alpha.1`.

## Change expectations

- Keep Schema, Go, Python/TS, UI and fixtures consistent.
- Preserve missing/error/zero semantics and full metric identity including K/judge configuration.
- Statistics belong to Processors; UI must not invent replacement statistics or conclusions.
- Include focused regressions and a reproducible fixture for new adapters.
- No automatic Subject retries, production replay or global environment resets.
- Use existing Ant Design controls and ECharts lifecycle management.
- Do not commit secrets, real traces, runtime state, downloaded reports or caches.

When public issues are enabled, include commit, OS/architecture, command, expected/observed
behavior and a small synthetic Package. For vulnerabilities follow [SECURITY.md](SECURITY.md).

There is no automatic publish/deploy workflow. Tags, release uploads and registry publication
require a separate maintainer action. Source preparation is not production acceptance.
