# Product Governance

Matrix is the local-first crossroads for AI communication flows.

Canonical product keywords: human-to-agent, agent-to-agent, one-to-many, many-to-many.

The product must keep these promises:

- Human-to-agent: a user can enter from Telegram, HTTP, CLI, or another channel without caring which provider protocol is underneath.
- Agent-to-agent: Matrix can be used as a tool by an AI or supervisor to ask other agents to work, review, fork, hand off, or continue.
- One-to-many: one instruction can fan out to multiple target agents when the workflow needs parallel reasoning or execution.
- Many-to-many: multiple agents can cooperate through Matrix without binding the product to one vendor or one chat surface.
- Workspace continuity: switching agents must not force the user to manually reconstruct project context.

Feature acceptance rule:

A feature is product-fit only if it strengthens Matrix as a communication matrix, orchestration surface, or continuity layer. Features that make Matrix a vertical wrapper for one agent, one channel, or one protocol should be rejected or redesigned.

ZERO-LEGACY design:

This repo is in active development. Active product surfaces converge on the
final product shape and reject obsolete layouts, aliases, packages, and
compatibility paths. The project-wide release policy is defined in
[`zero_legacy_governance.md`](zero_legacy_governance.md).
