# Architecture Guardrails

Architecture guardrails are automated ratchets for Matrix invariants.

They do not prove the architecture is perfect. They prevent silent drift.

Current guardrails:

- Protocol SDK imports must not spread into core logic. ACP and A2A SDK details belong in provider adapters or explicit discovery seams.
- Telegram provider imports must stay behind channel runtime composition. Telegram remains one channel, not a product spine.
- PAL home resolution must stay centralized. Direct `os.UserHomeDir` calls are allowed only in the PAL home resolver and filesystem providers.

Guardrails are configured in `governance/manifest.toml` as `pattern_budget.*` sections.

Budget rule:

- `max = 0` means no new occurrence outside `allowed_files`.
- `allowed_files` are explicit baseline seams, not permission to expand the pattern.
- Increasing a budget is a governance change and must update the reason.

When a guardrail fails, fix the architectural drift first. Increase the budget only if the new dependency is intentionally becoming a stable seam.
