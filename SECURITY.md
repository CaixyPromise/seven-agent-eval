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

The selected reporting channel is GitHub private vulnerability reporting for
`CaixyPromise/seven-agent-eval`. Once the repository and channel are available, use its
Security → Advisories → Report a vulnerability action. During this local preparation the
repository/channel has not been created or verified. If the action is unavailable, use an
existing private maintainer contact; do not post secrets, private traces or production data
in public issues. The maintainer must enable and verify the channel before public release.
There is no response SLA or stable release line.

Supply the affected version, synthetic reproduction and impact. Only test systems you own or
are authorized to test. Automated tests and secret scans are not a full security audit.
