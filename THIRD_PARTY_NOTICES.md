# Third-party material

Project licensing is pending; see [the recommendation](../seven-agent-docs/seven-agent-eval/docs/licensing.md).
Dependencies retain their own licenses independently of the project's eventual license.

`scripts/dependency_notices.py` inventories resolved Go modules, installed pnpm packages
(including build tools) and the Python wheels included in a distribution. The archive's
`third-party/inventory.json` records names, versions, metadata and copied license/notice paths.
Text is copied verbatim, not replaced by a guessed SPDX label. Build-only dependencies are
conservatively included even if not shipped as executable modules.

The build fails if a dependency has no discoverable license/notice material. This checks
completeness, not legal compatibility or vulnerabilities. Embedded attribution requirements
still require review. No blanket claim is made that dependencies use the proposed project license.

Fixtures are controlled examples, not downloaded BEIR corpora. Format support grants no rights
to user datasets. Design images and repository history need their own publication review.
