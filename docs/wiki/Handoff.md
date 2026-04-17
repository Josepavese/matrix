# Handoff

Handoff is how you transfer work from one agent to another without losing context. It is the feature that makes multi-agent workflows practical.

## The Problem

You are working on a feature with OpenCode. The implementation is done, but you want a code review. You could:

1. Copy-paste the context into Claude manually
2. Start a fresh session and re-explain everything
3. **Use handoff** -- one command, full context transfer

Handoff is option 3.

## How It Works

When you trigger a handoff:

1. Matrix captures the current work context (active session, workspace, recent turns, current mode)
2. It creates a **handoff packet** -- a structured summary of what happened so far
3. The packet is stored in the workspace timeline
4. The next agent receives the packet as context on its first turn
5. The workspace timeline records the handoff event

The receiving agent gets everything it needs to continue where the previous agent left off.

## Using Handoff

### From Telegram

```
/handoff claude
```

Matrix hands off the current workspace session to Claude. You will see a confirmation:

```
Handoff: opencode -> claude
Context: Review the billing API patch
Workspace: billing-api
```

### From HTTP API

```bash
curl -X POST http://127.0.0.1:9091/v1/intents \
  -H "Content-Type: application/json" \
  -d '{
    "channel_id": "docs.http",
    "intent": "handoff",
    "target": "claude",
    "workspace_id": "billing-api"
  }'
```

### From CLI

```bash
matrix workspace switch billing-api --handoff claude
```

## When to Use Handoff

| Scenario | Command |
|----------|---------|
| Implementation done, need review | `/handoff claude` |
| Stuck on a hard bug, try a different model | `/handoff gemini` |
| One agent finished frontend, another handles backend | `/handoff opencode` |
| Running agent hit its limit, escalate | `/handoff claude` |

## What the Receiving Agent Sees

The handoff packet includes:

- **Source agent** -- which agent was working before
- **Workspace** -- project context
- **Mode** -- what mode was active (implementation, review, etc.)
- **Summary** -- a summary of recent work
- **Transfer context** -- deterministic context for the receiving agent

The receiving agent does not need to guess. It gets a clear brief.

## Tracking Handoffs

Every handoff is recorded in the workspace timeline:

```
/timeline
```

Output:

```
[1] handoff created opencode -> claude - Review the billing patch [2026-04-15 12:45 UTC]
[2] entered review mode [2026-04-15 12:44 UTC]
[3] resumed session for opencode [2026-04-15 12:30 UTC]
```

You can also inspect handoff decisions:

```
/decisions
```

## Handoff vs Agent Switch

Handoff is not the same as simply switching agents:

| | Agent Switch | Handoff |
|---|---|---|
| Context | Lost | Preserved |
| Timeline | Not recorded | Recorded |
| Receiving agent | Starts fresh | Gets a brief |
| Use case | Start something new | Continue existing work |

Use `/handoff` when you want continuity. Use `/new` or a direct agent switch when you want a fresh start.

## The Operator Loop

Handoff is a key part of the Matrix operator loop:

```
implement -> review -> handoff -> snapshot -> resume
```

1. Implement with one agent
2. Review the work
3. Hand off to another agent for the next phase
4. Snapshot the state
5. Resume later

Read more: [Core Concepts](Core-Concepts.md#the-operator-loop)

## Next

- [Using Agents](Using-Agents.md) -- configure and manage your agents
- [Workspaces](Workspaces.md) -- understand workspace timeline and memory
- [Examples](Examples.md) -- step-by-step handoff walkthroughs
