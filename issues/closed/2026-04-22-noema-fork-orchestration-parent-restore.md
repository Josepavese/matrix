# Noema request: active-session-safe fork orchestration with parent restore and cleanup proof

Date: 2026-04-22
Reporter: Noema integration
Priority: high for Noema experimental fork provider
Type: API improvement / lifecycle orchestration

## Summary

Matrix now exposes true provider fork through `/v1/session-actions` with:

```json
{
  "channel_id": "example",
  "action": "fork",
  "target": "parent-session-id"
}
```

This is enough as a lifecycle primitive, and Noema can begin an experimental local provider for OpenCode.

However, Noema's target paradigm needs a safer orchestration surface:

```text
main active Matrix run/session
-> fork a temporary child from the current context
-> send one internal interpreter prompt to the child
-> collect a concise artifact
-> cleanup/close/delete the child with proof
-> restore or preserve the parent session as active
-> never contaminate the main task/session
```

The current primitive forks the remote session and mirrors the child as a normal Matrix logical session that becomes active for the channel. That is workable for manual lifecycle use, but risky for automated Noema sidecar/interpreter usage during active evaluations.

## Why Noema Needs This

Noema wants to use fork as an optional provider behind:

```text
internal/experienceinterpreter
-> MatrixForkProvider
-> Matrix lifecycle API
-> ACP session/fork when provider supports it
```

Noema does not want Matrix to interpret Noema evidence. Matrix should remain transport/lifecycle only.

The needed Matrix behavior is not cognitive. It is lifecycle safety:

- fork child from a known parent
- run exactly one child turn or expose a child session handle safely
- cleanup child with strong proof
- preserve or restore parent channel state
- expose durable trace/lifecycle evidence
- return typed unsupported when fork is unavailable

## Problem With Current Primitive For Automation

Matrix maintainer response says:

- true ACP `session/fork` is called only when advertised
- `fork` is Draft/capability-gated
- forked child is mirrored as a normal Matrix logical session
- child becomes active for the channel
- automatic fork prompt/response artifact pipeline is not implemented yet
- automatic child cleanup after interpreter task is not implemented yet

For Noema, making the child active on the same channel during a live run can be unsafe:

- the main run may still be active
- subsequent `/v1/runs` calls on the same channel may accidentally target the child
- cleanup may remove the child but not restore parent activity
- failure between fork and cleanup can leave the channel in the wrong active session
- Noema has to reconstruct orchestration semantics that Matrix can enforce more safely

## Requested API Direction

Exact shape is up to Matrix, but Noema needs one of these.

### Option A: run-scoped fork artifact action

```text
POST /v1/runs/{run_id}/forks
```

Request sketch:

```json
{
  "reason": "temporary_interpreter",
  "prompt": "...",
  "visibility": "internal",
  "cleanup_policy": "close_or_cancel_and_forget_local",
  "restore_parent": true
}
```

Response sketch:

```json
{
  "status": "completed|unsupported|failed",
  "stability": "draft",
  "capability": "acp.session/fork",
  "parent_session_id": "...",
  "child_session_id": "...",
  "artifact": {
    "kind": "interpreter_response",
    "content": "..."
  },
  "cleanup": {
    "clean": true,
    "strong_cleanup": true,
    "remote_closed": true
  },
  "parent_restored": true
}
```

### Option B: session action fork flags

Extend `/v1/session-actions` `action=fork` with flags:

```json
{
  "channel_id": "example",
  "action": "fork",
  "target": "parent-session-id",
  "make_active": false,
  "ephemeral": true,
  "cleanup_policy": "close_or_cancel_and_forget_local"
}
```

Then Noema can run a child turn explicitly by child session id and cleanup it.

## Required Semantics

- Do not emulate fork by replaying prompt/history; expose `unsupported` instead.
- Keep `stability=draft` while ACP keeps `session/fork` Draft.
- Preserve parent session as active, or restore it atomically after child cleanup.
- Cleanup child session with strong proof where provider/process support allows it.
- Return cleanup fields equivalent to existing session cleanup proof.
- Expose trace evidence for fork creation, child run, cleanup, and parent restore.
- Return typed unsupported for Codex/Gemini when fork is not advertised.

## Non-goals

- Matrix should not interpret Noema evidence.
- Matrix should not decide suggestion quality.
- Matrix should not produce Noema guidance.
- Matrix should not promote draft ACP fork to stable product capability.

## Acceptance Criteria

- Noema can call a Matrix fork workflow without manually switching the user's active session away from the parent.
- Noema can verify child cleanup and parent restoration from machine-readable fields.
- Unsupported providers return typed unsupported/blocked evidence.
- OpenCode remains the first experimental path; Codex/Gemini remain gated fallback paths.

## Maintainer Response

Accepted and implemented as the session-action fork workflow, not as a new
cognitive endpoint.

Matrix keeps `session/fork` a Draft, capability-gated provider operation. The
new automation-safe fields on `/v1/session-actions action=fork` are:

- `make_active=false`: mirror the fork child without switching the user's
  active channel session.
- `restore_parent=true`: restore/preserve the parent active session after child
  work.
- `input`: optional one-turn prompt routed to the fork child only.
- `ephemeral` / `cleanup_policy`: when supplied with `input`, Matrix cleans the
  child after the child turn and returns `fork.cleanup`.

The response now carries:

- `fork.parent_logical_session_id`
- `fork.parent_remote_session_id`
- `fork.child_logical_session_id`
- `fork.make_active`
- `fork.artifact.content` for the raw child response when `input` is supplied
- `fork.cleanup` when automatic child cleanup ran
- `fork.parent_restored`

Matrix does not emulate fork by replaying prompt/history and does not interpret
Noema evidence. Unsupported providers still return typed `unsupported` fork
results. Fork-child cleanup is also parent-safe: Matrix can retain the shared
provider process when another local parent session still references the same
`agent_id + workspace_path`, while requiring remote/process proof for the child
cleanup result.
