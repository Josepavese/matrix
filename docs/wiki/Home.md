# Matrix Wiki

Matrix is a local communication hub between people or supervisory software and
existing coding agents. It routes tasks through ACP/A2A and maintains inspectable
runs, session identities and workspace state.

This is the versioned Wiki: its Markdown sources live in `docs/wiki/`, are
reviewed with the code, and can be read at the revision you installed. Guidance
below covers the v0.1.53 PAL features, v0.1.54 CLI correction and recent session/notification changes.

## Start here

1. [Getting Started](Getting-Started.md): install, configure an agent and send a task.
2. [Core Concepts](Core-Concepts.md): distinguish the agent, its model, a run and a session.
3. [Examples](Examples.md): a project review, delegation and state inspection.

## Guides

| Page | Purpose |
|---|---|
| [Using Agents](Using-Agents.md) | Discovery, installation, launch policy, model selection and readiness |
| [Delegation and Notifications](Delegation-and-Notifications.md) | Submit async work, wait for outcomes or input, retain delivery cursors |
| [Sessions and Recovery](Sessions-and-Recovery.md) | Import external conversations, resume exact IDs, handle restart interruptions |
| [Handoff](Handoff.md) | Pass a bounded operational brief to another specialist |
| [Sidecar Capsules](Sidecar-Capsules.md) | Attach structured supervisory context and inspect delivery |
| [Workspaces](Workspaces.md) | Roots, bindings, memory, timeline and metadata snapshots |
| [Channels](Channels.md) | Telegram, authenticated HTTP and CLI access |
| [PAL Execution and Observability](PAL-Execution-and-Observability.md) | Linux/macOS/Windows sandbox, host capacity, semantic state and OTLP logs |
| [API Reference](API-Reference.md) | HTTP requests, actions, events and response guarantees |
| [CLI Reference](CLI-Reference.md) | Commands, flags and platform-specific availability |
| [FAQ](FAQ.md) | Limits, provider credits, storage and troubleshooting |
| [Governance](Governance.md) | Product boundaries, release gates and qualification evidence |

## Recent changes to understand

- From v0.1.54, `matrix run submit` supplies a stable channel, submits async work,
  accepts HTTP 202 and supports explicit channel/workspace flags. See
  [Delegation and Notifications](Delegation-and-Notifications.md).

- External session import verifies the provider's exact remote ID. Resume
  failures do not silently open replacement conversations.
- Async runs have durable outcome notifications on the Unix local socket;
  input requests carry a bounded question. Consumer cursors and acknowledgements
  have explicit, different delivery guarantees.
- A daemon restart marks previously active runs `outcome_unknown`; it does not
  replay their prompts.
- Model traces distinguish the requested selector, effective selection and
  provider confirmation. They do not prove account balance or physical model identity.
- Optional execution policies, runtime capacity admission, read-only semantic
  views and OTLP log export are documented together in the PAL guide.
- Vault encrypted values bind to their storage keys (`ENCV2`); backup/migration
  procedures retain the canonical PAL-owned state.

## Technical evidence and design

- [Protocol coverage](../protocol_coverage.md) and
  [ACP compliance](../matrix_zed_acp_compliance.md).
- [Run trace contract](../matrix_agent_communication_run_trace.md) and
  [live context interrupt policy](../matrix_live_context_interrupt_policy.md).
- [Timeout/recovery policy](../matrix_timeout_recovery_policy.md).
- [v0.1.53 release evidence](../governance/releases/2026-10-08-v0.1.53.md) and
  [PAL implementation evidence](../governance/pal_implementation_2026-10-08.md).
- [README](../../README.md), [installation details](../matrix_installation.md) and
  [release runbook](../matrix_release_runbook.md).

Roadmaps and specifications may include planned work. Use runtime capabilities,
code and release evidence to establish what your installation supports.

## Contributing

Update these Markdown sources with the behavior they describe. Examples should
name an existing workspace and configured agent, authenticate HTTP requests,
and label provider-dependent or platform-specific features.
