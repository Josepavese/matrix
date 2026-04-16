# Matrix

<p align="center">
  <strong>The communication crossroads for humans and AI agents.</strong><br/>
  One matrix for human→multi-agent and agent→agent work across channels, protocols, and workspaces.
</p>

<p align="center">
  <img src="docs/assets/readme/hero.png" alt="Matrix communication matrix hero" width="1000" />
</p>

> **Warning**
> Matrix is still in an experimental testing phase. APIs, commands, storage layout, agent integrations, and deployment procedures may change quickly. Use it for evaluation, prototyping, and controlled local workflows, not unattended production operations.

<p align="center">
  <strong>Human-to-agent.</strong> <strong>Agent-to-agent.</strong> <strong>Protocol-neutral.</strong>
</p>

## Why Matrix

Most teams do not need another agent framework.

They need a central junction where every agent communication flow can meet:

- humans talking to one or many agents
- agents handing work to other agents
- Telegram, HTTP, CLI, ACP, and A2A sharing one operating model
- sessions, workspaces, memory, snapshots, and decisions staying connected

**Matrix is the communication matrix for real agents already in the wild.**

## The Claim

> Every channel. Every agent. One communication matrix.

That is the product.

Not another agent DSL.  
Not another workflow builder.  
Not another chat wrapper.

## What Makes It Different

- **Real agents, not toy demos**
  Codex, Claude, Gemini, OpenCode, ACP peers, A2A peers.

- **A central communication junction**
  Human-to-agent, human-to-multi-agent, and agent-to-agent flows share one routing core.

- **One workspace fabric**
  Channels and protocols change. The work context does not.

- **Visible orchestration**
  Timeline, memory, snapshots, and decision trace stay local-first.

- **One communication surface**
  Telegram, HTTP, CLI, and future channels share the same semantics.

## How It Works

<p align="center">
  <img src="docs/assets/readme/workflow.png" alt="Matrix workflow infographic" width="1000" />
</p>

1. A human, channel, agent, or supervisory AI enters through Telegram, HTTP, CLI, ACP, or A2A.
2. Matrix resolves workspace, session, intent, mode, provider, and communication path.
3. Matrix routes the flow to the right agent or agent group and keeps the work visible.

## The Operator Loop

<p align="center">
  <img src="docs/assets/readme/operator-loop.png" alt="Matrix operator loop infographic" width="1000" />
</p>

This is the loop Matrix is built for:

- implement
- review
- handoff
- snapshot
- resume

## Built For

- developers using more than one coding agent
- teams tired of agent sprawl
- supervisory AI systems that need a reliable communication substrate
- agent systems that need a neutral place to exchange work
- local-first operations where visibility matters

## Quick Start

```bash
go build -o matrix ./cmd/matrix
./matrix bootstrap doctor
./matrix run
```

## Core Surfaces

- `POST /v1/runs`
- `POST /v1/session-actions`
- `POST /v1/workspace-actions`
- `GET /v1/workspace-state`
- `GET /v1/workspace-timeline`
- `GET /v1/workspace-memory`
- `GET /v1/workspace-snapshots`
- `GET /v1/workspace-decisions`
- `GET /v1/orchestration-capabilities`
- `POST /a2a`

## Read The Product

- [Product profile](PRODUCT.md)
- [Category thesis](docs/matrix_category_thesis.md)
- [Product roadmap](docs/matrix_product_roadmap_2026_2027.md)
- [Chat UX](docs/matrix_chat_ux_spec.md)
- [Workspace affinity](docs/matrix_workspace_affinity_spec.md)
- [Workspace timeline](docs/matrix_workspace_timeline_spec.md)
- [Protocol-neutral runtime](docs/matrix_v2_protocol_neutral_runtime.md)
- [Orchestration surface](docs/matrix_orchestration_surface_spec.md)
- [Decision trace](docs/matrix_decision_trace_spec.md)
- [Production readiness](docs/matrix_production_readiness.md)
- [Deployment policy](docs/matrix_deployment_policy.md)
- [Threat model](docs/matrix_threat_model.md)
- [Brand direction](docs/brand_direction.md)

## Visual Direction

- **Tone:** sharp, operator-first, technical, controlled
- **Primary colors:** `#0B1020`, `#00D1B2`, `#3B82F6`, `#F5F7FB`
- **Accent colors:** `#FF7A59`, `#A3E635`

The README images are product visuals generated from the visual direction above.
