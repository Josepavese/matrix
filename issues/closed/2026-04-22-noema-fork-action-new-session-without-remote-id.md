# Noema issue: fork action fails after session-actions new because parent has no remote session id

Date: 2026-04-22
Reporter: Noema integration
Priority: high for Noema Matrix fork-interpreter diagnostics

## Summary

After the latest Matrix fork improvements, Noema can see the correct capability
surface and OpenCode reports:

```json
"fork": {
  "supported": true,
  "status": "supported",
  "stability": "draft",
  "source": "zed_acp_rfd_session_fork"
}
```

However a real local smoke using `/v1/session-actions` cannot yet exercise the
new parent-safe fork workflow from a freshly created Matrix logical session.

## Reproduction

Start Matrix:

```bash
matrix run
```

Create an ephemeral OpenCode session:

```bash
curl -sS http://127.0.0.1:9091/v1/session-actions \
  -X POST \
  -H 'Content-Type: application/json' \
  -d '{
    "channel_id": "noema.fork-smoke",
    "action": "new",
    "target": "opencode",
    "workspace_path": "/home/jose/hpdev/Libraries/noema",
    "ephemeral": true,
    "cleanup_policy": "delete_remote_or_cancel_and_forget_local"
  }'
```

Observed response:

```json
{
  "action": "new",
  "active_session_id": "aaf60844-f63a-401a-b284-baaf42073502",
  "session": {
    "logical_session_id": "aaf60844-f63a-401a-b284-baaf42073502",
    "agent_id": "opencode",
    "workspace_path": "/home/jose/hpdev/Libraries/noema",
    "status": "active",
    "active": true,
    "ephemeral": true,
    "cleanup_policy": "delete_remote_or_cancel_and_forget_local"
  }
}
```

Then call parent-safe fork with one-turn input:

```bash
curl -sS http://127.0.0.1:9091/v1/session-actions \
  -X POST \
  -H 'Content-Type: application/json' \
  -d '{
    "channel_id": "noema.fork-smoke",
    "action": "fork",
    "target": "aaf60844-f63a-401a-b284-baaf42073502",
    "make_active": false,
    "restore_parent": true,
    "input": "Return only this JSON exactly: {\"provider\":\"smoke\",\"version\":\"v0\",\"actions\":[{\"text\":\"Run validation before final answer.\",\"supported_by\":[\"action_atom:run_declared_validation_before_final\"]}]}",
    "ephemeral": true,
    "cleanup_policy": "delete_remote_or_cancel_and_forget_local"
  }'
```

Observed HTTP body:

```text
Internal Server Error
```

Matrix log:

```json
{
  "level": "ERROR",
  "msg": "matrix session action failed",
  "error": "session aaf60844-f63a-401a-b284-baaf42073502 has no remote session id to fork",
  "action": "fork"
}
```

## Cleanup Observation

Cleanup of the logical parent after the failed fork was local-only and not clean:

```json
{
  "action": "cleanup",
  "cleanup": {
    "logical_session_id": "aaf60844-f63a-401a-b284-baaf42073502",
    "agent_id": "opencode",
    "cleanup_policy": "delete_remote_or_cancel_and_forget_local",
    "clean": false,
    "strong_cleanup": false,
    "cleanup_strength": "failed",
    "process_reap_attempted": true,
    "process_reaped": false,
    "process_retention_reason": "no matching cached agent client",
    "local_forgotten": true,
    "failure_code": "cleanup_clean_without_remote_or_process_proof"
  }
}
```

This is correctly unsafe from Noema's perspective.

## Why It Blocks Noema

Noema now has a local `experienceinterpreter` Matrix fork adapter that expects
Matrix to:

- fork a real provider session;
- keep the child inactive with `make_active=false`;
- restore/preserve the parent with `restore_parent=true`;
- return `fork.artifact.content`;
- return strong/remote/process child cleanup proof.

If `/v1/session-actions action=new` creates only a logical local session without
a remote session id, Noema cannot use that id as the fork parent.

## Desired Behavior

Exact design is Matrix-owned, but Noema needs one of these:

1. `action=new` with a fork-capable ACP provider materializes a remote session id
   when the session is intended for lifecycle operations.
2. `action=fork` with `input` can materialize the parent remote session before
   forking when the logical session is active but has no remote id.
3. Matrix returns typed non-500 evidence, for example:

```json
{
  "action": "fork",
  "unsupported": true,
  "error": {
    "code": "missing_remote_session_id",
    "message": "session has no remote session id to fork",
    "target": "aaf60844-f63a-401a-b284-baaf42073502"
  },
  "fork": {
    "unsupported": true,
    "reason": "missing_remote_session_id"
  }
}
```

Noema can handle typed unsupported/blocked evidence. Generic HTTP 500 is not
good enough for capability-gated automation.

## Non-goals

- Matrix should not interpret Noema evidence.
- Matrix should not emulate fork by replaying prompt/history.
- Matrix should not hide this as a successful fork.

## Acceptance Criteria

- A fresh Matrix logical OpenCode session can be used as a true fork parent, or
  Matrix returns typed unsupported/blocked evidence without HTTP 500.
- If a fork child is created, `fork.artifact.content`, `fork.cleanup`, and
  `fork.parent_restored` are returned.
- Cleanup after failed fork setup remains inspectable and does not falsely mark
  the workflow clean.

## Maintainer Response

Accepted.

Root cause confirmed: `action=new` intentionally created a Matrix logical
session mirror only. The first real provider session was normally allocated on
the first agent turn, so a fresh logical session had no remote ACP session id for
`session/fork`.

Implemented Matrix-owned fix:

- Added a protocol-neutral `AgentSessionMaterializer` boundary.
- ACP now materializes a real remote parent session via `session/new` without
  sending a prompt or replaying history.
- `action=fork` now materializes the parent when needed, persists the remote id
  to the vault, then calls the true provider fork.
- If materialization is impossible, Matrix returns typed blocked evidence:
  `missing_remote_session_id` or `remote_session_materialize_failed`, not a
  generic HTTP `500`.
- HTTP maps `missing_remote_session_id` to `409` and
  `remote_session_materialize_failed` to `502`.

This preserves the non-goals: Matrix does not emulate fork, does not interpret
Noema evidence, and does not hide blocked setup as a successful fork.

## Verification

Validated with real local `opencode` through Matrix HTTP:

- `action=new` created a fresh logical parent without `remote_session_id`.
- `action=fork` materialized the parent remote ACP session through `session/new`.
- Matrix then called true ACP `session/fork`.
- The child returned `fork.artifact.content` with the requested JSON.
- Child cleanup returned `clean=true`, `strong_cleanup=true`, and
  `remote_canceled=true`.
- `fork.parent_restored=true` and the active session remained the parent.
- Parent cleanup after the smoke returned strong cleanup proof.
