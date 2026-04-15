# Matrix Workspace Affinity Spec

## Goal

Workspace affinity makes Matrix sessions attach to **real work context**, not only to a channel identity.

Today the primary mapping is:

- `channel_id -> active logical session`

With workspace affinity, the model becomes:

- `channel_id -> active logical session`
- `logical session -> workspace identity`
- `workspace identity -> preferred sessions, policies, and default agent behavior`

This is the feature that most clearly moves Matrix from "advanced routing bridge" to "Agent Session Fabric for real work."

## Product Intent

Without workspace affinity, a session is mostly:

- the current conversation for a channel

With workspace affinity, a session becomes:

- the active operating context for a repo, workspace, or machine-scoped task

That makes it possible to say:

- "continue the session for this repo"
- "switch this Telegram thread onto the billing-api workspace"
- "use the review policy for this workspace"
- "resume the session tied to branch X"

## Design Principles

### 1. Session Remains The Primary Runtime Object

Workspace affinity does not replace sessions.

It adds a stable work context that sessions can attach to.

### 2. Channel Is An Access Surface, Not The Identity Of Work

Channels should continue to be ingress identities, but the durable work context should live elsewhere.

### 3. Workspace Must Be Protocol-Neutral

ACP and A2A sessions can both attach to the same workspace model.

### 4. Workspace Must Be Channel-Neutral

Telegram, HTTP, CLI, and future channels should all resolve against the same workspace rules.

### 5. Local-First

Workspace identity, bindings, and policies belong in the vault.

## Definitions

### Workspace

A logical work context known to Matrix.

Examples:

- a repository root
- a project folder
- a named machine role
- a long-lived task space

### Workspace Binding

The association between a session and a workspace.

### Workspace Affinity

The rule that says a session should preferentially route, resume, and be controlled within a specific workspace context.

## Data Model

## 1. WorkspaceMeta

Add a new vault-backed object:

- key: `workspace.meta.<workspace_id>`

Suggested shape:

```json
{
  "id": "billing-api",
  "name": "billing-api",
  "kind": "repository",
  "root_path": "/home/jose/src/billing-api",
  "repo_url": "git@github.com:org/billing-api.git",
  "default_branch": "main",
  "labels": ["backend", "payments"],
  "policy_profile": "review",
  "default_agent_id": "codex",
  "created_at": "2026-04-15T12:00:00Z",
  "updated_at": "2026-04-15T12:00:00Z"
}
```

Core fields:

- `id`
- `name`
- `kind`
- `root_path`
- `policy_profile`
- `default_agent_id`
- timestamps

Optional fields:

- `repo_url`
- `default_branch`
- `labels`
- arbitrary metadata

## 2. SessionMeta Extension

Extend `session.meta.<session_id>` with:

```json
{
  "workspace_id": "billing-api",
  "workspace_path": "/home/jose/src/billing-api",
  "workspace_branch": "feature/invoice-fix",
  "workspace_role": "primary",
  "workspace_bound_at": "2026-04-15T12:30:00Z"
}
```

Recommended fields:

- `WorkspaceID`
- `WorkspacePath`
- `WorkspaceBranch`
- `WorkspaceRole`
- `WorkspaceBoundAt`

Notes:

- `WorkspaceID` is the stable local identifier
- `WorkspacePath` is a convenience mirror for operator visibility
- `WorkspaceBranch` is opportunistic metadata, not a hard requirement
- `WorkspaceRole` allows future multi-machine or multi-root usage

## 3. ChannelState Extension

Current `ChannelState` should stay lightweight.

Optional additions:

- `PreferredWorkspaceID`
- `LastWorkspaceID`

This enables a channel to remember:

- which workspace it usually operates on
- which workspace was last active

## 4. Workspace Indexes

Add vault-backed helper indexes:

- `workspace.sessions.<workspace_id>` -> recent logical session ids
- `workspace.channels.<workspace_id>` -> recent channel ids

These are denormalized indexes for fast list and attach flows.

## Resolution Model

When a message arrives, Matrix should resolve in this order:

1. current active session for the channel
2. workspace preference for the channel
3. explicit workspace target from the ingress or command
4. fallback to default agent and create a fresh unbound session

### Important Rule

Once a session exists and is bound to a workspace, the workspace becomes part of the session identity for routing and operator UX.

## Lifecycle

## 1. Create Workspace

Possible sources:

- explicit CLI command
- HTTP typed action
- channel command
- auto-discovery from working directory in CLI-oriented surfaces

Suggested commands:

- `matrix workspace add <path>`
- `matrix workspace create <name> --path <path>`
- `/workspace add <name>`

## 2. Bind Session To Workspace

Possible ways:

- create a new session already bound to a workspace
- attach an existing session to a workspace
- import a remote session and bind it during import

Suggested commands:

- `matrix session bind <session> <workspace>`
- `/session bind <workspace>`

## 3. Switch Workspace

Switching workspace should not always mean changing session, but it often will.

Suggested commands:

- `matrix workspace switch <workspace>`
- `/workspace switch <workspace>`

Behavior:

- if the channel already has an active session bound to the workspace, attach to it
- otherwise create or import the most appropriate session for that workspace

## 4. Inspect Workspace

Suggested commands:

- `matrix workspace list`
- `matrix workspace show <workspace>`
- `/workspace list`
- `/workspace status`

The operator should see:

- current workspace
- active session for the workspace
- recent sessions
- preferred/default agent
- policy profile

## 5. Detach Or Rebind

Suggested commands:

- `matrix session unbind <session>`
- `matrix session rebind <session> <workspace>`

## Channel And Ingress Changes

## 1. ChannelMessage

Current `ChannelMessage` is minimal:

- `ChannelID`
- `DefaultAgentID`
- `Input`
- `Notifier`

Recommended extension:

```go
type ChannelMessage struct {
    ChannelID          string
    DefaultAgentID     string
    WorkspaceID        string
    WorkspacePath      string
    Input              string
    Notifier           ThoughtNotifier
}
```

Notes:

- `WorkspaceID` is the stable logical hint
- `WorkspacePath` is useful for local CLI or future desktop integrations
- chat channels like Telegram will often omit both
- HTTP and CLI may provide them explicitly

## 2. SessionActionRequest

Recommended extension:

```go
type SessionActionRequest struct {
    ChannelID    string
    Action       string
    Target       string
    WorkspaceID  string
}
```

This allows typed session operations to be scoped to a workspace when needed.

## 3. HTTP Ingress

Current `POST /v1/runs` should be allowed to accept optional:

- `workspace_id`
- `workspace_path`

Current `POST /v1/session-actions` should be allowed to accept optional:

- `workspace_id`

## 4. Telegram And Future Chat Channels

Chat channels will mostly use commands rather than structured fields.

That is acceptable as long as they map into the same shared core semantics.

## Routing Rules

When Matrix must choose an agent for a new session, it should resolve:

1. explicit agent request
2. workspace default agent
3. global default agent

When Matrix must choose a session for a channel, it should resolve:

1. explicit session target
2. current channel active session
3. current channel preferred workspace
4. workspace's last active session
5. create a new session bound to the workspace

## CLI Surface

Recommended new top-level command:

- `matrix workspace`

Suggested subcommands:

- `matrix workspace add <path>`
- `matrix workspace list`
- `matrix workspace show <workspace>`
- `matrix workspace switch <workspace>`
- `matrix workspace remove <workspace>`
- `matrix workspace attach <workspace> --channel <channel>`

These are intentionally operator-oriented, not Git-oriented.

## Chat Surface

Recommended new slash commands:

- `/workspace list`
- `/workspace status`
- `/workspace switch <workspace>`
- `/workspace bind <workspace>`

Existing `/session` commands should show workspace context when present.

## Session UX Changes

`/session status` and `matrix session inspect` should include:

- workspace id
- workspace path
- workspace branch
- workspace policy profile

`/session list` should group or annotate sessions by workspace.

## Policy Integration

Workspace affinity is the first clean insertion point for policy routing.

Each workspace should eventually be able to define:

- default agent
- routing profile
- trust mode preference
- preferred reviewer
- tool access posture

This is why workspace affinity should be implemented before advanced policy routing.

## Timeline Integration

Workspace affinity also prepares the timeline model.

Once sessions are tied to a workspace, Matrix can later support:

- workspace session timelines
- per-workspace checkpoints
- rollback on a repo-specific task flow

Without workspace affinity, timelines remain too channel-centric.

## Implementation Plan

## Step 1: Data Model

- add `WorkspaceMeta` vault model
- extend `SessionMeta`
- add workspace index helpers

## Step 2: Read Path

- extend session manager resolution logic
- support optional workspace hints in ingress
- annotate `SessionEntry` with workspace data

## Step 3: Write Path

- add workspace CRUD in CLI
- add bind/rebind flows
- add channel commands

## Step 4: Operator UX

- richer session status and listing
- workspace inspection
- workspace-aware switching

## Step 5: Policy Hook

- resolve default agent by workspace before global fallback

## Migration Direction

Workspace affinity becomes part of the primary product model.

Sessions without workspace context can still exist temporarily during development,
but the preferred direction is:

- new ingress always carries workspace-aware semantics
- new channels target the shared typed core
- workspace-bound operation becomes the default product path

## Risks

### 1. Over-Coupling To Filesystem Paths

Not every useful workspace is a local path.

So `workspace_id` must be primary and `root_path` secondary.

### 2. Too Much CLI Bias

Telegram and HTTP must remain first-class.

Workspace semantics should be shared even when the channel has no natural cwd.

### 3. Hidden Complexity In Resolution Rules

Resolution order must stay simple and inspectable.

Matrix should explain why a given session or workspace was selected.

## Success Criteria

Workspace affinity is successful when:

- a session can be clearly attached to a workspace
- a channel can resume work by workspace, not only by prior thread state
- default agent selection can be workspace-aware
- operator surfaces show where work actually belongs
- future policy routing and timelines can build on the same model

## Decision

The recommended architectural move is:

**Make workspace a first-class object in the vault and a first-class annotation on sessions, while keeping channels as ingress surfaces rather than the identity of work.**

That is the cleanest path from the current Matrix runtime to a true Agent Session Fabric.
