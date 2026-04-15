# Matrix Decision Trace

## Goal

Matrix should never feel like it is routing work "at random".

When Matrix:

- resumes a session
- reuses an active channel session
- creates a new session
- selects a workspace default specialist
- falls back to a global default

it should record and expose **why** that happened.

## Product Reason

This is not a debugging-only feature.

Decision trace is part of the product promise because Matrix is positioning itself as:

- a local-first Agent Session Fabric
- an orchestration substrate for humans and supervisory AI

That means Matrix must expose not only state and history, but also:

- routing rationale
- orchestration rationale
- fallback rationale

Without this, Matrix risks looking opaque and heuristic-driven.

## Current Model

Decision traces are recorded as first-class workspace timeline events:

- event type: `decision.recorded`

Each decision trace currently includes:

- `kind`
- `source`
- `explanation`
- `requested_agent_id`
- `selected_agent_id`
- `selected_session_id`
- `selected_mode`
- `fallback_used`
- `created_at`

## Current Decision Kinds

Matrix currently emits decisions such as:

- `reuse-active-session`
- `resume-workspace-session`
- `create-session`

These are deterministic orchestration decisions, not model-generated reasoning.

## Surfaces

Decision trace is exposed on all primary product surfaces.

Chat:

- `/why`
- `/decisions [workspace]`
- `/now` includes the latest decision

HTTP:

- `GET /v1/workspace-decisions`
- `GET /v1/workspace-state` includes the latest decision in materialized state

CLI:

- `matrix workspace decisions <workspace> --limit N`

Snapshots:

- workspace snapshots now preserve the latest decision trace along with state, events, turns, and handoff metadata

## Non-Goals

Decision trace does not currently attempt to expose:

- LLM chain-of-thought
- speculative planner reasoning
- hidden protocol internals

It is strictly about **operational orchestration decisions** made by Matrix itself.

## Next Logical Extension

The next step after this foundation is to attach decision traces to higher-level transitions such as:

- review specialist selection
- handoff target choice
- policy-profile-driven routing
- snapshot-informed resume decisions

That will let Matrix explain not just session routing, but full orchestration behavior.
