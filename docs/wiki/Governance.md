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
- Architecture guardrails: pattern budgets block protocol, channel, and PAL home drift.
- ZERO-LEGACY: active surfaces fail closed on retired contracts and migrate only one way to PAL-owned canonical artifacts.

Primary docs:

- [Governance index](../governance/README.md)
- [Product governance](../governance/product_governance.md)
- [Architecture governance](../governance/architecture_governance.md)
- [Protocol and channel governance](../governance/protocol_channel_governance.md)
- [Zed ACP compliance](../matrix_zed_acp_compliance.md)
- [Deploy governance](../governance/deploy_governance.md)
- [Architecture guardrails](../governance/architecture_guardrails.md)
- [ZERO-LEGACY policy](../governance/zero_legacy_governance.md)

## Qualification and documentation

The README/Wiki describe implemented contracts. Roadmaps remain design direction.
Platform compilation or contract tests do not certify an installed OS driver:
record native tests and real container/mount qualification separately.

- [v0.1.53 release and local runtime evidence](../governance/releases/2026-10-08-v0.1.53.md)
- [PAL implementation evidence](../governance/pal_implementation_2026-10-08.md)
- [PAL guide](PAL-Execution-and-Observability.md)
