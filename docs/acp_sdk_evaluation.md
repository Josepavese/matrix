# ACP Go SDK Evaluation

Last verified: 2026-07-19.

## Decision

Matrix keeps `pkg/zedacp` as its active ACP backend behind the protocol adapter
port. A third-party SDK may replace it only through an atomic backend migration;
Matrix will not maintain two ACP wire implementations or a compatibility
fallback.

## Current inputs

- Official ACP stable schema: `schema-v1.19.0`.
- Maintained Matrix coverage SSOT: [`protocol_coverage.md`](protocol_coverage.md).
- Community candidate: `github.com/coder/acp-go-sdk`; the latest published Go
  module observed with `go list -m -versions` is `v0.13.5`.
- Official SDK and protocol references:
  - https://agentclientprotocol.com/protocol/overview
  - https://agentclientprotocol.com/protocol/v1/initialization
  - https://agentclientprotocol.com/protocol/v1/transports
  - https://agentclientprotocol.github.io/typescript-sdk/
  - https://github.com/coder/acp-go-sdk

## Why the local backend remains active

The existing backend already implements Matrix's governed stable surface,
including initialization, method-ID authentication, session lifecycle,
capability-gated content and MCP transports, filesystem and terminal callbacks,
structured updates, cancellation, and Matrix remote endpoint transports. It is
also integrated with client pooling, Vault session mirrors, cleanup proof, and
the protocol-neutral router.

The community SDK remains a valid replacement candidate, but its version number
alone is not proof of Matrix parity. No migration is approved until a current
adapter spike demonstrates the complete coverage table without provider-specific
behavior leaking into middleware or session logic.

## Replacement gate

A candidate backend must:

1. model only the current stable ACP contract on the default surface;
2. reject retired initialization aliases and credential payloads;
3. preserve all capability gates, content blocks, MCP transports, callbacks,
   session metadata, cancellation semantics, and raw JSON-RPC IDs;
4. pass the full Matrix unit, race, governance, and real-provider lifecycle
   suites; and
5. replace `pkg/zedacp` in one change, deleting the superseded implementation.

Draft features require separately named non-default interfaces. `session/fork`
is the only current Matrix draft opt-in; it must not become a stable fallback.

## Next evaluation

When an SDK replacement is proposed, pin its exact module version and upstream
commit, implement the adapter on a temporary branch, compare its live capability
report with `docs/protocol_coverage.md`, then choose one backend. A partial or
dual-backend rollout is prohibited by ZERO-LEGACY governance.
