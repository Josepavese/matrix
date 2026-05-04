# Matrix Wiki

Welcome to the Matrix wiki. This is the developer guide for **Matrix** -- the local-first communication hub for AI coding agents.

## What Matrix Does

You use multiple AI coding agents: Claude, Gemini, OpenCode, Codex. Each one has its own CLI, its own sessions, its own way of working. There is no shared context. No way to hand off work from one to another. No single place to see what is happening.

Matrix fixes that. It runs locally, connects to your agents, and gives you one communication surface for all of them -- via Telegram, HTTP API, or CLI.

**One command. Any agent. One workspace.**

## Pages

| Page | What You Will Learn |
|------|-------------------|
| [Getting Started](Getting-Started.md) | Install, configure, and run your first agent conversation in under 5 minutes |
| [Core Concepts](Core-Concepts.md) | Understand workspaces, sessions, agents, and channels without the jargon |
| [Using Agents](Using-Agents.md) | Search, install, configure, and switch between coding agents |
| [Handoff](Handoff.md) | Transfer work from one agent to another with full context |
| [Sidecar Capsules](Sidecar-Capsules.md) | Attach machine-trackable context to runs without polluting chat |
| [Zed ACP Compliance](../matrix_zed_acp_compliance.md) | Current ACP support, unstable fields, and why Matrix has no ACP `side` primitive |
| [Live Context Interrupt Policy](../matrix_live_context_interrupt_policy.md) | Understand cancel vs live attach, provider limits, and tested fallback behavior |
| [Workspaces](Workspaces.md) | Organize work by project, track timelines, create snapshots, and build memory |
| [Channels](Channels.md) | Set up Telegram, use the HTTP API, or drive Matrix from the CLI |
| [API Reference](API-Reference.md) | Complete HTTP API documentation with request/response examples |
| [CLI Reference](CLI-Reference.md) | Every `matrix` command explained with examples |
| [Governance](Governance.md) | Understand the release gates and product invariants maintainers enforce |
| [Examples](Examples.md) | Step-by-step walkthroughs of common workflows |
| [FAQ](FAQ.md) | Common questions and troubleshooting |

## Quick Links

- [Install Matrix](Getting-Started.md#installation)
- [Run your first prompt](Getting-Started.md#your-first-prompt)
- [Set up Telegram](Channels.md#telegram)
- [Hand off work between agents](Handoff.md)
- [Attach sidecar context](Sidecar-Capsules.md)
- [Review Zed ACP compliance](../matrix_zed_acp_compliance.md)
- [Check live context interrupt policy](../matrix_live_context_interrupt_policy.md)
- [Review governance](Governance.md)
- [Browse the API](API-Reference.md)

## Contributing

Found a gap in this wiki? Open an issue or a pull request. Every page is a Markdown file under `docs/wiki/`.
