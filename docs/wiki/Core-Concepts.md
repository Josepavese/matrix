# Core Concepts

Matrix has four main concepts. Once you understand them, everything else follows naturally.

## Agents

An **agent** is an external AI coding tool that Matrix connects to. Matrix does not build agents. It talks to the ones you already have.

Supported agents out of the box:

| Agent | Command | Protocol | Status |
|-------|---------|----------|--------|
| OpenCode | `opencode acp` | ACP (stdio) | Active by default |
| Gemini CLI | `gemini --experimental-acp` | ACP (stdio) | Active |
| Claude Code | `claude acp` | ACP (stdio) | Available |
| Kimi | `kimi acp` | ACP (stdio) | Available |

You can also discover and install agents from the ACP Registry or A2A catalogs.

Key ideas:

- **You bring your own agents.** Matrix connects to them. It does not replace them.
- **Agents speak protocols.** Matrix handles ACP and A2A so you do not have to think about it.
- **You pick the default.** During setup, you choose which agent handles new conversations.

Read more: [Using Agents](Using-Agents.md)

## Sessions

A **session** is one conversation between you and an agent. When you send a prompt, Matrix either creates a new session or continues an existing one.

You can:

- **Create** a new session with `/new` or `POST /v1/session-actions`
- **List** active sessions with `/session list` or `matrix session attach`
- **Switch** between sessions
- **Cancel** a running session with `/stop`
- **Delete** a session
- **Name** a session for easy reference

Sessions persist in the local vault. If you stop and restart Matrix, your sessions are still there.

## Workspaces

A **workspace** is a project. It binds sessions to real work context -- a repository, a project folder, or a named task.

Without workspaces, sessions are just conversations. With workspaces, sessions become work:

- **Timeline** -- see what happened, when, and why
- **Memory** -- turn-by-turn summaries that persist across sessions
- **Snapshots** -- named checkpoints you can return to
- **Decisions** -- a trace of why Matrix chose a particular agent or routing

You can:

- **Create** a workspace: `matrix workspace add project-name --path /path/to/project`
- **Switch** between workspaces: `/use project-name` in chat
- **Inspect** workspace state: `/status` or `/now`
- **Create snapshots**: `/snapshot before-refactor`

Read more: [Workspaces](Workspaces.md)

## Channels

A **channel** is how you talk to Matrix. All channels share the same sessions and workspaces.

| Channel | Status | Best For |
|---------|--------|----------|
| HTTP API | Active | Scripts, integrations, programmatic access |
| Telegram | Active | Chat-based access from your phone or desktop |
| CLI | Active | Quick commands, inspection, configuration |

Start a conversation on Telegram, continue it via the HTTP API, inspect the results from the CLI. Same session. Same workspace. Same state.

Read more: [Channels](Channels.md)

## Sidecar Capsules

A **sidecar capsule** is optional context sent alongside a task by an upstream system or supervisory agent. It is not normal chat text.

Matrix keeps the task body separate from the sidecar, projects the capsule into ACP or A2A, and records a trace event proving delivery. Supervisors can also attach sidecar context to an active async run through run actions. Frontends should hide raw capsule internals from normal chat timelines while keeping trace/debug access.

Use sidecar capsules when an upstream system needs to attach intent, evidence, constraints, success criteria, or read-only inspection hints without becoming tied to one backend protocol.

Read more: [Sidecar Capsules](Sidecar-Capsules.md)

## How They Fit Together

```
You
 |
 +-- Channel (Telegram, HTTP, CLI)
      |
      +-- Session (one conversation with one agent)
           |
           +-- Agent (Claude, Gemini, OpenCode, ...)
                |
                +-- Workspace (project context, timeline, memory)
```

1. You send a message through a **channel**
2. Matrix resolves it to a **session** (or creates one)
3. The session is routed to the right **agent**
4. Everything is bound to a **workspace** for continuity

## The Operator Loop

Matrix is built for a repeating work pattern:

1. **Implement** -- send a prompt, let the agent code
2. **Review** -- `/review` to switch into review mode
3. **Hand off** -- `/handoff gemini` to pass the work to another agent
4. **Snapshot** -- `/snapshot before-deploy` to save state
5. **Resume** -- `/continue` or `/resume` to pick up where you left off

This loop works the same whether you are on Telegram, HTTP, or CLI.

## Glossary

| Term | Meaning |
|------|---------|
| ACP | Agent Client Protocol -- the primary protocol Matrix uses to talk to agents |
| A2A | Agent-to-Agent protocol -- an alternative protocol for agent communication |
| PAL Home | The directory where Matrix stores all its data |
| Vault | The local encrypted database (BoltDB) that stores all Matrix state |
| Run | One execution cycle: a prompt goes in, an agent processes it, a result comes back |
| Sidecar Capsule | Machine-trackable context attached to a run and projected into ACP/A2A without becoming normal chat |
| Handoff | Transferring active work from one agent to another with context preservation |
| Intent | A high-level operation like `continue`, `review`, `explain`, `triage`, or `handoff` |
| Mode | The current work mode: implementation, review, explanation, triage |
| Meta-agent | The agent designated to handle system administration tasks via `/action` |
