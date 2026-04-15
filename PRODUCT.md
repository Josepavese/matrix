# Matrix Product Profile

## The Product

**Matrix is the control plane for real coding agents.**

It gives developers one operating surface across:

- agents
- channels
- workspaces
- sessions
- protocols

The goal is simple:

> bring the best agent for the job, keep one workspace fabric

## The Category

Matrix should be positioned as a **local-first Agent Session Fabric**.

That means:

- not an agent builder
- not a workflow DSL
- not just a protocol bridge
- not a vague "AI operating system"

Matrix is the layer that keeps the work continuous while agents, channels, and protocols change.

Deeper strategy is documented in [docs/matrix_category_thesis.md](/home/jose/hpdev/Libraries/matrix/docs/matrix_category_thesis.md).

## The Wedge

The wedge is clear:

**operate real coding agents from one durable workspace/session surface**

This is the problem Matrix solves:

- agent sprawl
- fragmented sessions
- context loss across channels
- invisible orchestration
- too much protocol-specific glue

## Who It Is For

- developers using more than one coding agent
- small engineering teams with real multi-agent workflows
- AI-native teams that want a local-first control plane
- supervisory AI systems that need a reliable orchestration substrate

## Core Promise

Matrix gives you:

- **one workspace fabric**  
  work stays attached to the workspace, not to one temporary chat

- **one session surface**  
  resume, switch, review, handoff, cancel, snapshot

- **one local source of truth**  
  vault, memory, timeline, snapshots, decision trace

- **one operator view**  
  see what happened, why it happened, and what state the work is in now

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

**it operates external agents as one continuous system of work**

That difference becomes visible through:

- workspace affinity
- timeline
- work memory
- snapshots
- decision trace
- handoff between specialists

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
- local-first in state and control
- visible in its orchestration, not opaque
- consistent across channels and machine surfaces

## Next Reading

- [README.md](README.md)
- [docs/matrix_product_roadmap_2026_2027.md](/home/jose/hpdev/Libraries/matrix/docs/matrix_product_roadmap_2026_2027.md)
- [docs/matrix_workspace_affinity_spec.md](/home/jose/hpdev/Libraries/matrix/docs/matrix_workspace_affinity_spec.md)
- [docs/matrix_workspace_timeline_spec.md](/home/jose/hpdev/Libraries/matrix/docs/matrix_workspace_timeline_spec.md)
- [docs/matrix_orchestration_surface_spec.md](/home/jose/hpdev/Libraries/matrix/docs/matrix_orchestration_surface_spec.md)
