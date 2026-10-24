# Security boundaries and reporting

This Alpha candidate is a **local trusted-user tool**. It has no remote authentication,
multi-tenant isolation or untrusted-code sandbox. Do not expose it to the Internet.
Python/TS extensions run with the process user's permissions; discovery executes module top-level code.

Evidence may contain confidential inputs, outputs and tool arguments. Key/value redaction is
not general PII detection. Review evidence before sharing bundles. Reference inputs are excluded
from default Subject requests, but user extensions can explicitly send their inputs.
Judges send evidence to the configured endpoint; no paid judge runs by default.

The viewer validates bundle paths, hashes and limits and does not launch imported extensions.
Validation is not a claim of universal content safety.

## Reporting status

GitHub private vulnerability reporting is enabled for `CaixyPromise/seven-agent-eval`
(verified through the GitHub API on 2026-10-24). Use
[Report a vulnerability](https://github.com/CaixyPromise/seven-agent-eval/security/advisories/new)
or Security → Advisories → Report a vulnerability. If the action is temporarily unavailable,
use an existing private maintainer contact; do not post secrets, private traces or production
data in public issues.
There is no response SLA or stable release line.

Supply the affected version, synthetic reproduction and impact. Only test systems you own or
are authorized to test. Automated tests and secret scans are not a full security audit.
