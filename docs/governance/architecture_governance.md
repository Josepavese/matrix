# Architecture Governance

Matrix architecture is governed by five invariants.

Canonical architecture keywords: protocol-neutral, channel-neutral, PAL, vault, SSOT.

- Protocol-neutral: ACP, A2A, and future protocols sit behind provider ports. Business logic must not hardcode protocol-specific semantics.
- Channel-neutral: Telegram, HTTP, CLI, and future channels expose the same command model wherever their transport allows it.
- PAL home SSOT: runtime state, binaries, databases, logs, configs, and artifacts live under the Matrix PAL home, not inside a cloned repository.
- Vault-backed runtime SSOT: sessions, workspace bindings, memory, and provider mirrors are reconciled through the vault.
- Agent-orchestrable: Matrix must be usable by humans and by agents as a stable tool surface.

Adapters may contain protocol or channel details. Core services may depend only on neutral interfaces, capability contracts, and normalized events.

Unsupported provider behavior must be represented as an explicit capability result. It must not be hidden behind a fake success.
