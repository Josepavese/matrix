# Matrix Workspace Timeline Spec

## Goal

Workspace Timeline is the next category-defining layer for Matrix.

It makes the current Session Fabric visible and valuable to users by turning
work history from "chat turns" into a durable operational timeline.

The objective is not to add another log.

The objective is to let Matrix answer these questions cleanly:

- what is happening in this workspace now
- what happened before
- which specialist worked on it
- when did Matrix resume, review, cancel, or hand off
- what should the next specialist know

This is the first feature that makes Matrix obviously different from:

- agent builders
- generic orchestration runtimes
- protocol bridges
- chat frontends

## Product Thesis

Most systems treat history as:

- prompt history
- chat transcript
- runtime trace

Matrix should treat history as:

- operational continuity of work

That means the primary historical object is not the prompt.

It is the **workspace event timeline**.

## Product Outcome

With Workspace Timeline, a user on Telegram, HTTP, or CLI should be able to:

- ask what is currently active
- see recent work events
- understand handoffs and specialist changes
- inspect why the current agent/session was chosen
- resume work without re-reading raw chat

The operator should see:

- a short current-state view
- a recent timeline
- stable event semantics across protocols and channels

## Scope

Phase 1 of Workspace Timeline is intentionally narrow.

It covers:

- append-only workspace events in the vault
- timeline query and summary surfaces
- handoff, review, resume, continue, cancel, switch, and new-session events
- current-state materialization from recent events

It does not yet cover:

- rollback
- checkpoints
- branching
- diff-level snapshots
- provider-native trace import beyond mirrored metadata

Those belong to later phases.

## Definitions

### Workspace Timeline

An append-only event stream for one logical workspace.

### Workspace Event

A typed event that records a meaningful operational transition in work state.

Examples:

- session created
- session resumed
- mode changed
- handoff created
- remote session canceled
- workspace switched

### Current Work State

A materialized summary of the latest meaningful state of the workspace:

- active logical session
- active specialist
- current mode
- remote status
- latest handoff
- last meaningful operator event

## Design Principles

### 1. Timeline Is Workspace-First

Session history remains important, but the timeline is anchored on the workspace.

This is what makes Matrix feel like the operating layer for real work rather
than a better per-chat history.

### 2. Events Are Semantic, Not Raw Trace Spam

The timeline should not record every token or every ACP notification.

It should record operator-relevant state transitions.

### 3. Same Semantics On Every Channel

Telegram, HTTP, CLI, and future channels should surface the same event model.

Rendering can differ. Event meaning must not.

### 4. Vault Remains The SSOT

Timeline storage belongs in the vault, next to workspace and session state.

### 5. Timeline Must Support Explanation

Matrix must be able to explain:

- why this session is active
- why this specialist is active
- why the mode changed
- why a handoff happened

## Data Model

## 1. Workspace Event

Vault key:

- `workspace.event.<workspace_id>.<event_id>`

Suggested shape:

```json
{
  "id": "01HSXYZ...",
  "workspace_id": "billing-api",
  "type": "handoff.created",
  "channel_id": "telegram.user123",
  "logical_session_id": "sess-123",
  "remote_session_id": "remote-abc",
  "agent_id": "claude",
  "mode": "review",
  "message": "Workspace handed off to claude for review",
  "reason": "specialist-handoff",
  "metadata": {
    "from_agent_id": "codex",
    "to_agent_id": "claude",
    "summary": "Review the current billing patch"
  },
  "created_at": "2026-04-15T12:45:00Z"
}
```

Core fields:

- `id`
- `workspace_id`
- `type`
- `channel_id`
- `logical_session_id`
- `agent_id`
- `mode`
- `message`
- `reason`
- `metadata`
- `created_at`

Optional fields:

- `remote_session_id`

## 2. Workspace Event Index

Vault key:

- `workspace.timeline.<workspace_id>`

Value:

- newest-first ordered list of recent `event_id`

This mirrors the session/workspace index pattern already used elsewhere.

## 3. Workspace Current State

Vault key:

- `workspace.state.<workspace_id>`

Suggested shape:

```json
{
  "workspace_id": "billing-api",
  "active_logical_session_id": "sess-123",
  "active_agent_id": "claude",
  "active_mode": "review",
  "remote_status": "active",
  "last_event_type": "handoff.created",
  "last_event_at": "2026-04-15T12:45:00Z",
  "last_handoff": {
    "from_agent_id": "codex",
    "to_agent_id": "claude",
    "summary": "Review the current billing patch"
  }
}
```

This is a materialized convenience object derived from events and mirrored state.

It exists for fast UX, not as an independent truth source.

## Event Taxonomy

Phase 1 event types:

- `session.created`
- `session.resumed`
- `session.switched`
- `session.canceled`
- `session.deleted`
- `workspace.bound`
- `workspace.switched`
- `mode.changed`
- `intent.continue`
- `intent.resume`
- `intent.review`
- `intent.explain`
- `intent.triage`
- `handoff.created`
- `handoff.applied`

Rules:

- one event type = one semantic transition
- do not create provider-specific event names in the shared timeline
- provider details belong in `metadata`

## Write Rules

Matrix should write timeline events when:

### 1. A Session Is Created For A Workspace

Write:

- `session.created`

### 2. A Session Is Reattached Or Resumed

Write:

- `session.resumed`

### 3. The Active Session Changes

Write:

- `session.switched`

### 4. A Mode Changes

Write:

- `mode.changed`

### 5. A High-Level Intent Occurs

Write:

- `intent.continue`
- `intent.resume`
- `intent.review`
- `intent.explain`
- `intent.triage`

### 6. A Handoff Packet Is Created

Write:

- `handoff.created`

### 7. A Handoff Packet Is Consumed On First Routed Turn

Write:

- `handoff.applied`

### 8. A Remote Session Is Canceled Or Deleted

Write:

- `session.canceled`
- `session.deleted`

## Read Model

## 1. Current Workspace Status

High-level `/status` should remain compact.

It should read from:

- session mirror
- workspace current state

and optionally show:

- latest handoff
- last meaningful event

## 2. Workspace Timeline View

New surfaces:

- `/timeline`
- `/timeline [workspace]`
- `matrix workspace timeline <workspace>`
- `GET /v1/workspace-timeline?workspace_id=billing-api`

Behavior:

- default to current workspace for the channel
- return newest-first recent events
- show concise semantic entries, not raw provider dumps

## 3. Workspace Current-State View

New surfaces:

- `/now`
- `matrix workspace state <workspace>`
- `GET /v1/workspace-state?workspace_id=billing-api`

Behavior:

- show the current operating state of the workspace
- optimized for "what is happening now"

## UX Model

Users should not need to think in terms of:

- event IDs
- materialized state
- mirrored metadata

They should see:

- current work
- recent work
- last handoff

Recommended chat outputs:

### `/now`

```text
Workspace: billing-api
Mode: review
Agent: claude
Session: 8fa3c1e2
Remote status: active
Last event: handoff created
Event detail: Applied specialist handoff
Updated: 2026-04-15 12:45 UTC
Handoff: codex -> claude - Review the current billing patch
```

### `/timeline`

```text
[1] handoff created codex -> claude - Review the current billing patch [2026-04-15 12:45 UTC]
[2] entered review mode [2026-04-15 12:44 UTC]
[3] resumed session for claude [2026-04-15 12:44 UTC]
[4] created session for codex [2026-04-15 12:30 UTC]
```

## Channel And Ingress Surfaces

## Chat

Add:

- `/now`
- `/timeline`
- `/timeline <workspace>`

Keep these high-level.

Advanced operators can still use lower-level `session` and `workspace` commands.

## HTTP

Add:

- `GET /v1/workspace-state?workspace_id=...`
- `GET /v1/workspace-timeline?workspace_id=...`

Optional later:

- pagination and `limit`

## CLI

Add:

- `matrix workspace state <workspace>`
- `matrix workspace timeline <workspace>`

Optional later:

- `--json`

## Implementation Plan

## Step 1. Timeline Store

Add:

- event object type
- event index helpers
- current-state materializer

Location:

- `internal/logic/workspace/timeline.go`

## Step 2. Event Writes In Session Manager

Emit timeline events from:

- session create/resume/switch
- mode changes
- intents
- handoff creation and application
- cancel/delete flows

## Step 3. Typed Read Surfaces

Add shared core contracts for:

- workspace state
- workspace timeline

These should be channel-neutral before any HTTP or Telegram rendering.

## Step 4. Chat, HTTP, CLI

Expose:

- `/now`
- `/timeline`
- HTTP read endpoints
- CLI read commands

## Step 5. Product Polish

Refine:

- compact event wording
- event grouping
- explainability of routing and handoff

## Why This Is The Next Point

This is the right next move because it:

- makes the Session Fabric visible
- turns handoff into an understandable product feature
- gives Matrix a demoable operator loop
- creates a strong GTM story
- prepares later work on checkpoints, rollback, and branching

Without Workspace Timeline, Matrix remains architecturally strong but visually abstract.

With it, Matrix starts to look like its own product category.
