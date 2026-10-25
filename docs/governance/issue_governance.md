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

## Acceptance is not a commitment to implement

Accepting a report records the decision to investigate and qualify it; it is not a
promise to build every part of it, or even some of it. The maintainer builds the
smallest generic change that keeps Matrix's philosophy — provider neutral, honest
about what it can and cannot observe, evidence over narration — and leaves the
rest out even when the report is otherwise valid:

- A request that would make Matrix claim a guarantee it cannot keep (for example
  the absence of tools in a provider's request body) is answered with the limit
  and the provider-side mechanism, not with code that narrates the guarantee.
- A request shaped around one consumer's vocabulary, one provider, or one
  channel becomes a generic contract or nothing; no hidden vertical path is
  created to satisfy it.
- A partial implementation is a delivery with its own stated limits, never a
  half-promise: when only the structural part is philosophically sound, only
  that part is built and the remainder is recorded as not built, by design.

The closure states what was built, what was answered with a limit, and why.
"Accepted" never means "everything requested will exist".
