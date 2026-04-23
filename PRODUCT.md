# Matrix Product Profile

## The Product

**Matrix is the communication matrix for real coding agents.**

It is the central junction where communication flows between:

- humans
- agents
- channels
- workspaces
- sessions
- protocols

The goal is simple:

> connect every human-to-agent and agent-to-agent flow through one durable communication surface

## The Category

Matrix should be positioned as a **local-first Agent Communication Matrix**.

That means:

- not an agent builder
- not a workflow DSL
- not just a protocol bridge
- not a vague "AI operating system"

Matrix is the layer that keeps communication, work, and state continuous while agents, channels, and protocols change.

Deeper strategy is documented in [docs/matrix_category_thesis.md](docs/matrix_category_thesis.md).

## The Wedge

The wedge is clear:

**route real coding-agent communication through one durable workspace/session matrix**

This is the problem Matrix solves:

- agent sprawl
- disconnected human-to-agent channels
- disconnected agent-to-agent handoffs
- fragmented sessions
- context loss across channels
- invisible orchestration
- too much protocol-specific glue

## Who It Is For

- developers using more than one coding agent
- small engineering teams with real multi-agent workflows
- AI-native teams that want a local-first communication layer
- supervisory AI systems that need a reliable communication substrate

## Core Promise

Matrix gives you:

- **one communication matrix**
  humans, channels, protocols, and agents meet in one routing layer

- **one workspace fabric**
  work stays attached to the workspace, not to one temporary chat

- **one session lifecycle**
  resume, switch, review, handoff, cancel, snapshot

- **one local source of truth**  
  vault, memory, timeline, snapshots, decision trace

- **one visibility layer**
  see what happened, why it happened, what state the work is in now, and which run evidence an external supervisor can consume

## What Matrix Supports

The supported product surface is centered on real operation:

- `matrix run`
- `matrix install`
- `matrix agent ...`
- `matrix doctor`
- `matrix bootstrap doctor`
- `matrix vault ...`
- `matrix session ...`
- `matrix workspace ...`

Operational APIs:

- `POST /v1/runs`
- `GET /v1/runs/{run_id}/trace`
- `GET /v1/runs/{run_id}/events`
- `POST /v1/runs/{run_id}/actions`
- `POST /v1/event-sinks`
- `POST /v1/session-actions`
- `POST /v1/workspace-actions`
- `GET /v1/workspace-state`
- `GET /v1/workspace-timeline`
- `GET /v1/workspace-memory`
- `GET /v1/workspace-snapshots`
- `GET /v1/workspace-decisions`
- `GET /v1/orchestration-capabilities`

## Protocol Position

Matrix is protocol-neutral in the core.

Today:

- **ACP is the operational default**
- **A2A is implemented and ready inside Matrix**
- **A2A market adoption is still catching up**

So the product stance is:

- use ACP today for production integrations
- keep A2A enabled as a forward path

## Channel Position

Matrix is channel-neutral in the core.

Today:

- Telegram is the first fully integrated channel
- HTTP is a first-class machine surface
- future channels should plug into the same session, intent, workspace, and mode model

## What Makes It Different

Most tools do one of these:

- build agents
- expose a single agent
- provide protocol plumbing

Matrix does something else:

**it connects external agents as one continuous system of communication and work**

That difference becomes visible through:

- workspace affinity
- timeline
- work memory
- snapshots
- decision trace
- handoff between specialists
- protocol-neutral communication run traces
- uniform human-to-agent and agent-to-agent flows

## Explicit Non-Goals

Matrix is not trying to be:

- a hosted SaaS control plane
- a generic multi-tenant agent platform
- a semantic filesystem product
- a broad no-code workflow builder
- a protocol standard itself

## Product Standard

Matrix is “on profile” when it is:

- clear to understand in under two minutes
- useful with real agents, not synthetic demos
- credible as the central communication junction for humans and agents
- local-first in state and control
- visible in its orchestration, not opaque
- consistent across channels and machine surfaces

## Next Reading

- [README.md](README.md)
- [docs/matrix_product_roadmap_2026_2027.md](docs/matrix_product_roadmap_2026_2027.md)
- [docs/matrix_workspace_affinity_spec.md](docs/matrix_workspace_affinity_spec.md)
- [docs/matrix_workspace_timeline_spec.md](docs/matrix_workspace_timeline_spec.md)
- [docs/matrix_orchestration_surface_spec.md](docs/matrix_orchestration_surface_spec.md)
- [docs/matrix_agent_communication_run_trace.md](docs/matrix_agent_communication_run_trace.md)
