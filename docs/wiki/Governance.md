# Governance

Matrix governance is enforced before release.

Run the local gate:

```bash
go run ./scripts/governance_check --manifest governance/manifest.toml
```

What it protects:

- Product fit: Matrix remains a human-to-agent and agent-to-agent crossroads, not a wrapper for one provider.
- Architecture: protocol-neutral, channel-neutral, PAL home SSOT, vault-backed runtime state.
- Code quality: `code-governance.toml` keeps package, file, function, parameter, and complexity budgets visible.
- Deploy discipline: CI, GoReleaser, cross-platform artifacts, local install, and release evidence.
- Test evidence: protocol, provider, channel, session, and runtime changes need evidence appropriate to their risk.
- Issue handling: accepted, rejected, and closed issues must leave a maintainable trail.

Primary docs:

- [Governance index](../governance/README.md)
- [Product governance](../governance/product_governance.md)
- [Architecture governance](../governance/architecture_governance.md)
- [Protocol and channel governance](../governance/protocol_channel_governance.md)
- [Deploy governance](../governance/deploy_governance.md)
