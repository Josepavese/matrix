# Matrix Governance

Governance is a release gate, not a meeting artifact.

The SSOT is `governance/manifest.toml`. It defines the documents, scripts, CI hooks, release hooks, and keywords that must stay present before Matrix can be called releasable.

Run the gate locally:

```bash
go run ./scripts/governance_check --manifest governance/manifest.toml
```

Governance layers:

- Product: Matrix must remain the crossroads for human-to-agent and agent-to-agent communication.
- Architecture: protocol-neutral, channel-neutral, PAL-based, vault-backed, and local-first.
- Code: budgets are enforced by `code-governance.toml`.
- Deploy: releases must produce cross-platform artifacts and local install evidence.
- Tests: protocol, channel, and session changes need real evidence, not only unit coverage.
- Issues: accepted work becomes tracked design or code; rejected work explains why.
- Security: secrets, vault material, and installer behavior are governed explicitly.

If a change weakens one layer, it must strengthen another layer and document the tradeoff.
