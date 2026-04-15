# Matrix Orchestration Surface

## Purpose

Matrix should be operable both by humans and by a supervisory AI.

That means Matrix needs a stable, machine-readable description of:

- what it can do
- which surfaces expose those capabilities
- how an orchestrator should think about Matrix

The goal is not to turn Matrix into the main planning AI. The goal is to make Matrix a reliable orchestration substrate that a planner, operator, or meta-agent can call.

## Position

Matrix is a **local-first Agent Session Fabric**.

In orchestration terms, Matrix is:

- a control plane for workspace-scoped work
- a local mirror of session state, memory, timeline, and snapshots
- a neutral execution substrate for ACP and A2A agents
- a shared command surface that can be driven by chat, HTTP, CLI, or another AI

Matrix is not:

- the main cognitive engine
- a generic workflow DSL
- a raw protocol gateway

## Stable Machine Surface

Matrix now exposes a stable orchestration profile at:

```text
GET /v1/orchestration-capabilities
```

and locally through:

```bash
matrix orchestration capabilities
```

The profile is currently versioned as `v1` in code via:

- [surface.go](/home/jose/hpdev/Libraries/matrix/internal/logic/orchestration/surface.go)

## Capability Model

The profile is intentionally compact. It describes:

- `name`
- `category`
- `role`
- `capabilities`
- `surfaces`

Each capability includes:

- `id`
- `category`
- `description`
- `surfaces`

Each surface includes:

- `id`
- `description`
- `actions`

This is designed to be easy for:

- a human operator to inspect
- a meta-agent to load and reason over
- future SDKs or wrappers to mirror

## Current Capability Groups

Matrix currently declares these capability groups:

- `conversation.route`
- `session.lifecycle`
- `workspace.control`
- `workspace.state`
- `workspace.timeline`
- `workspace.memory`
- `workspace.snapshots`
- `intent.high_level`

These are deliberately semantic, not protocol-specific.

An orchestrator should ask:

- route a turn
- inspect the current work state
- inspect recent work history
- inspect recent work memory
- inspect snapshots
- request a high-level intent like `review` or `handoff`

It should not need to think in terms of:

- ACP vs A2A
- Telegram vs HTTP
- remote session ids vs local logical sessions

## Why This Matters

This is the bridge between Matrix as infrastructure and Matrix as product.

Without a formal orchestration surface, Matrix is only “usable by an AI” in an implicit way.

With a formal orchestration surface, Matrix becomes:

- describable
- introspectable
- callable
- replaceable at the API boundary

This supports the long-term product thesis:

> Matrix is an orchestration substrate that can be driven directly by a human or by a supervisory AI.

## Design Rules

The orchestration profile should remain:

- small
- stable
- semantic
- transport-neutral
- channel-neutral

The profile should not expose internal implementation details such as:

- vault key layout
- provider-specific session ids as primary concepts
- protocol wire specifics

Those belong below the orchestration layer.

## Next Evolution

The next logical step after this profile is not another endpoint. It is to add stronger execution traceability for the same capabilities:

- decision trace
- routing explanation
- why a session was resumed
- why a handoff was created
- which snapshot or memory segment informed a transition

That will let a supervisory AI not only call Matrix, but also audit Matrix’s behavior.

Decision trace is now part of the product surface through:

- `GET /v1/workspace-decisions`
- `/why`
- `/decisions [workspace]`
