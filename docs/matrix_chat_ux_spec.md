# Matrix Chat UX Spec

## Goal

Make Matrix usable from Telegram and future chat channels without exposing protocol, session, or workspace internals by default.

The user should feel like they are operating:

- one bot
- many projects
- one continuous working surface

They should not feel like they are managing:

- ACP or A2A
- remote session ids
- logical session ids
- transport details
- one bot per project

## Core Principle

Matrix may have multiple internal layers, but chat UX must stay simple.

### Internal model

- workspace
- logical session
- remote agent session
- protocol adapter

### User-facing model

- current project
- current mode
- current specialist

## Bot Model

Matrix should use:

- one Telegram bot
- many workspaces
- many internal sessions

It should not require:

- one bot per project
- manual bot wiring per project
- protocol knowledge from the user

## Default Mental Model

The chat user should think:

1. choose the project
2. continue the work
3. ask for a different specialist only when needed

Not:

1. choose a protocol
2. choose a remote session
3. map a channel to a logical session

## Default Chat Commands

The recommended default command set is:

- `/status`
- `/now`
- `/timeline [workspace]`
- `/decisions [workspace]`
- `/why`
- `/memory`
- `/snapshots`
- `/snapshot [note]`
- `/use <workspace>`
- `/workspaces`
- `/continue`
- `/resume [workspace]`
- `/review [workspace]`
- `/explain [workspace]`
- `/triage [workspace]`
- `/handoff <agent>`
- `/new`
- `/stop`

Advanced commands may still exist, but should not be the first thing the user needs.

## Current Friendly Aliases

Matrix now exposes friendly chat aliases for the common path:

- `/status` -> high-level work summary
- `/now` -> current workspace operating state
- `/timeline [workspace]` -> recent workspace timeline events
- `/decisions [workspace]` -> recent orchestration decisions for the workspace
- `/why` -> latest routing or orchestration decision for the current workspace
- `/memory` -> recent workspace work memory
- `/snapshots` -> recent workspace snapshots
- `/snapshot [note]` -> create a workspace snapshot
- `/use <workspace>` -> switch to the workspace context
- `/workspaces` -> list configured workspaces
- `/continue` -> continue the current work context
- `/resume [workspace]` -> resume the current or selected workspace context
- `/review [workspace]` -> move into review mode for the current or selected workspace
- `/explain [workspace]` -> move into explain mode for the current or selected workspace
- `/triage [workspace]` -> move into triage mode for the current or selected workspace
- `/handoff <agent>` -> hand off the current workspace to another specialist with a local transfer packet

The lower-level forms still exist:

- `/workspace list`
- `/workspace status`
- `/workspace switch <workspace>`
- `/workspace bind <workspace>`
- `/session ...`

`/handoff <agent>` is not a raw agent switch. Matrix records a local transfer packet,
binds it to the target specialist session, and injects that context on the next turn.
This preserves operator continuity without pretending that different agents share the
same native memory.

## UX Rules

### 1. One Bot, Many Workspaces

The bot is the console.

The workspace is the project.

The session is internal state.

### 2. Remember Context Per Chat

Each chat should remember its preferred or last workspace.

This allows:

- minimal repeated setup
- fast resume
- less command noise

### 3. Prefer Intent-Level Commands

The chat interface should prefer:

- "use billing-api"
- "continue"
- "review this"
- "explain this"
- "triage this"

over infrastructure-heavy commands.

### 4. Hide Agent And Protocol Details By Default

Users should not be asked by default:

- which protocol is active
- which remote session id exists
- which transport is in use

Matrix can surface those details in advanced or diagnostic contexts.

### 5. Explain Decisions Briefly

When Matrix switches context or specialist, it should explain briefly:

- workspace selected
- mode selected
- agent selected

Example:

```text
Workspace: billing-api
Mode: implementation
Agent: Claude
```

## Base Mode Vs Advanced Mode

### Base Mode

For most chat users:

- few commands
- implicit reuse of workspace context
- silent reuse of the best session
- short status messages

### Advanced Mode

For power users and operators:

- `/session list`
- `/session inspect`
- `/workspace bind`
- `/workspace switch`
- explicit agent/session controls

These remain available, but should not dominate the default UX.

## Recommended Next UX Steps

The next layer to add on top of the current implementation should be:

### 1. Richer `/status`

A high-level status command that summarizes:

- workspace
- active session
- current agent
- remote status

without dumping raw ids unless needed.

### 2. Richer Intent Behavior

Natural aliases such as:

- `/review`
- `/continue`
- `/resume`
- `/explain`
- `/triage`

These should map into workspace-aware session logic and later into policy routing and handoff.

### 3. Short Decision Notices

Whenever Matrix does an important context switch, it should emit a concise notice instead of raw internals.

### 4. Progressive Disclosure

Normal users see:

- workspace
- agent
- mode

Advanced users can ask for:

- session ids
- remote ids
- protocol info
- timeline details

## Design Decision

The correct UX direction for Matrix chat channels is:

**one bot, many workspaces, minimal commands, hidden infrastructure, explicit but compact context cues.**

That preserves the power of the internal architecture without forcing users to navigate it manually.
