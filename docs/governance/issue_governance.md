# Issue Governance

Local issues are triaged as accepted, rejected, or closed.

Rules:

- Accepted: the issue strengthens Matrix product fit, architecture, safety, or installability. It becomes design, code, tests, and documentation.
- Rejected: the issue weakens Matrix scope or duplicates an existing capability. The rejection must explain the reason.
- Closed: the issue has been implemented, rejected with reason, or superseded. Local closed issues move to `issues/closed`.

Maintainer response format:

- Decision: accepted or rejected.
- Reason: product and architecture impact.
- Scope: files, surfaces, or commands affected.
- Evidence: tests, real-agent smoke, deploy status, or reason evidence is not applicable.

Use [issue_triage_template.md](issue_triage_template.md) for external or local issues that require maintainer response.

Issues must not create hidden vertical paths for one provider, one protocol, or one channel.
