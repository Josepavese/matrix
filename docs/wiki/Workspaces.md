# Workspaces

Workspaces are what make Matrix more than a message router. They bind conversations to real project context so work persists across sessions, agents, and channels.

## Why Workspaces Matter

Without workspaces:
- Sessions are just conversations floating in space
- Switching agents means losing context
- There is no record of what happened, when, or why

With workspaces:
- Sessions are attached to real projects
- Work context survives agent switches and handoffs
- You get a timeline, memory, snapshots, and decision trace

## Creating a Workspace

### CLI

```bash
matrix workspace add my-project --path /home/user/my-project
```

This creates a workspace named `my-project` bound to that path.

### Chat

```
/workspace add my-project
```

### HTTP

```bash
curl -X POST http://127.0.0.1:9091/v1/workspace-actions \
  -H "Content-Type: application/json" \
  -d '{
    "action": "add",
    "workspace_id": "my-project",
    "root_path": "/home/user/my-project"
  }'
```

## Listing Workspaces

```bash
matrix workspace list
```

Or in chat:

```
/workspaces
```

## Switching Workspaces

```
/use my-project
```

Matrix switches the current channel to the `my-project` workspace. Future prompts go to the agent and session bound to that workspace.

## Workspace State

Check what is happening right now:

```
/now
```

Output:

```
Workspace: my-project
Mode: implementation
Agent: opencode
Session: 8fa3c1e2
Remote status: active
Last event: session resumed
Updated: 2026-04-16 10:30 UTC
```

Or via HTTP:

```bash
curl http://127.0.0.1:9091/v1/workspace-state?workspace_id=my-project
```

## Timeline

The timeline is an append-only event log for a workspace. It records every meaningful state transition:

```
/timeline
```

Output:

```
[1] handoff created opencode -> claude - Review the auth module [2026-04-16 14:45 UTC]
[2] entered review mode [2026-04-16 14:44 UTC]
[3] resumed session for opencode [2026-04-16 14:30 UTC]
[4] created session for opencode [2026-04-16 14:00 UTC]
```

Via HTTP:

```bash
curl http://127.0.0.1:9091/v1/workspace-timeline?workspace_id=my-project
```

### What gets recorded

| Event | When |
|-------|------|
| `session.created` | A new session is created for the workspace |
| `session.resumed` | An existing session is reattached |
| `session.switched` | The active session changes |
| `session.canceled` | A session is canceled |
| `mode.changed` | The work mode changes (implementation, review, etc.) |
| `intent.*` | A high-level intent fires (continue, review, explain, triage) |
| `handoff.created` | Work is handed to another agent |
| `handoff.applied` | The receiving agent consumes the handoff |

## Memory

Workspace memory stores turn-by-turn summaries so context persists across sessions:

```
/memory
```

Via HTTP:

```bash
curl http://127.0.0.1:9091/v1/workspace-memory?workspace_id=my-project
```

This is how Matrix remembers what happened in previous sessions without requiring the raw conversation transcript.

## Snapshots

Snapshots are named checkpoints of workspace state:

### Create a snapshot

```
/snapshot before-refactor
```

### List snapshots

```
/snapshots
```

Via HTTP:

```bash
curl http://127.0.0.1:9091/v1/workspace-snapshots?workspace_id=my-project
```

Snapshots capture:
- Active session
- Active agent
- Current mode
- Work status
- Any notes you attach

Use snapshots before risky operations so you can inspect the state later.

## Decisions

The decision trace shows why Matrix made routing and orchestration choices:

```
/decisions
```

Via HTTP:

```bash
curl http://127.0.0.1:9091/v1/workspace-decisions?workspace_id=my-project
```

Quick check on the latest decision:

```
/why
```

## Binding Sessions to Workspaces

When you create a session in the context of a workspace, it is automatically bound. You can also bind explicitly:

```
/workspace bind my-project
```

This associates the current session with the workspace, giving it access to the timeline, memory, and snapshots.

## Default Agent per Workspace

Each workspace can have its own default agent:

```bash
matrix workspace add my-project --default-agent claude
```

When you switch to that workspace, new sessions will use Claude by default instead of the global default.

## Workspace Resolution Order

When a message arrives, Matrix resolves the workspace in this order:

1. Explicit workspace target in the command or request
2. Current active session's bound workspace
3. Channel's preferred workspace
4. Global default (create an unbound session)

## CLI Commands

| Command | What It Does |
|---------|-------------|
| `matrix workspace add <id> --path <path>` | Create a workspace from a path |
| `matrix workspace list` | List all workspaces |
| `matrix workspace state <id>` | Show current workspace state |
| `matrix workspace switch <name>` | Switch to a workspace |
| `matrix workspace timeline <id>` | Show recent workspace events |
| `matrix workspace snapshots <id>` | List snapshots |
| `matrix workspace decisions <id>` | Show decision trace |
| `matrix workspace memory <id>` | Show workspace memory |
| `matrix workspace memory` | Show workspace memory |
| `matrix workspace retention` | Manage retention policies |

## Next

- [Handoff](Handoff.md) -- transfer work between agents within a workspace
- [Core Concepts](Core-Concepts.md) -- how workspaces fit into the bigger picture
- [Examples](Examples.md) -- real workspace workflows
