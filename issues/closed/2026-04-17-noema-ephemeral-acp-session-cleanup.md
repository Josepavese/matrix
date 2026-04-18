# NOEMA evaluation needs strict ephemeral ACP sessions with cleanup guarantees

## Context

NOEMA now has a real evaluation harness that can route coding-agent test runs through Matrix instead of talking to each agent directly.

The official test requirement is strict:

- each `OFF`, `MEMORY`, and `NOEMA` arm must run in a separate conversation/session;
- the session identifier must be ephemeral, random, and not derived from stable run IDs;
- after the run, Matrix must cancel/delete/forget the session so the agent cannot resume that conversation later;
- the artifact must prove whether cleanup happened.

This is required to avoid cross-arm contamination in scientific and marketing evaluation reports.

## What NOEMA tried

NOEMA calls:

1. `POST /v1/session-actions`
   payload:
   `{"channel_id":"noema-eval-channel-<random>","action":"new","target":"opencode"}`

2. `POST /v1/runs`
   payload includes:
   `channel_id=noema-eval-channel-<same-random>`,
   `agent_id=opencode`,
   `workspace_path=<isolated fixture workspace>`,
   `execution_mode=sync`.

3. `POST /v1/session-actions`
   payload:
   `{"channel_id":"noema-eval-channel-<random>","action":"delete","target":"<logical_session_id>"}`

If delete fails, NOEMA now falls back to:

`{"channel_id":"noema-eval-channel-<random>","action":"cancel","target":"<logical_session_id>"}`

NOEMA still marks the run non-official if delete fails.

## Evidence

Successful Matrix execution with failed delete:

- NOEMA artifact:
  `/home/jose/hpdev/Libraries/noema/programs/evaluation-platform/artifacts/phase59-real-coding-matrix-opencode-noema-smoke-v3/`
- Matrix run:
  `run-3b36baa7-74c6-410c-8ca1-9f8e1505a714`
- Logical session:
  `245c2b5c-fc44-434d-a76f-7158e565c4f2`
- Remote ACP session:
  `ses_26445f86fffe5dTx7otgFvIxkR`

Matrix log excerpt:

`matrix session action failed: ACP agent does not advertise session/delete`

Manual fallback cancel succeeded:

`{"action":"cancel","message":"Canceled session: 245c2b5c-fc44-434d-a76f-7158e565c4f2", ... "remote_status":"canceled"}`

Delete still fails after cancel.

Additional evidence from 2026-04-18 strict smoke:

- NOEMA artifact:
  `/home/jose/hpdev/Libraries/noema/programs/evaluation-platform/artifacts/phase59-real-coding-matrix-opencode-noema-smoke-v5/`
- Matrix run:
  `run-82ab85cb-6e21-49e6-89c6-bac5284a6694`
- NOEMA result:
  agent solved the task, `validation_passed=true`, `matrix_session_deleted=false`, `matrix_session_canceled=true`, official status `failed`
- After cancel, Matrix still had a live child process:
  `/home/jose/.local/bin/opencode acp`
- That child still held `gopls` descendants.

This makes cleanup stronger than local session deletion only: Matrix should also terminate/reap the target ACP agent process when an ephemeral session/run is cancelled, or expose explicit process-retention state so callers know cleanup is incomplete.

## Additional issue found

Calling `session-actions:new` with an unregistered `workspace_id` fails:

`failed to bind workspace: workspace <id> not found`

For NOEMA evaluations, we do not want to create stable Matrix workspace records for every ephemeral test run unless Matrix also supports automatic ephemeral workspace cleanup.

NOEMA can pass `workspace_path` to `/v1/runs`, but the current Matrix ACP router appears to use the Matrix PAL home as agent CWD and does not make the run workspace path the actual agent working directory. NOEMA can prompt the agent to `cd` into the path, but that is weaker than a protocol-level workspace binding.

## Desired Matrix contract

Prefer one of these designs.

Option A: one-shot run policy on `/v1/runs`

```json
{
  "channel_id": "noema-eval-channel-<random>",
  "agent_id": "opencode",
  "workspace_path": "/abs/isolated/workspace",
  "session_policy": "new_ephemeral_delete_after_run",
  "cleanup_policy": "delete_remote_or_forget_local"
}
```

Matrix should:

- create a new logical session;
- create/load a new remote ACP session;
- bind the workspace path as the real agent cwd/tool cwd;
- run the prompt;
- delete the remote session if supported;
- otherwise cancel it and remove/forget the local Matrix session mirror;
- expose cleanup result in the run trace.

Option B: explicit session lifecycle API

Extend `/v1/session-actions` `new` to accept:

```json
{
  "channel_id": "noema-eval-channel-<random>",
  "action": "new",
  "target": "opencode",
  "workspace_path": "/abs/isolated/workspace",
  "ephemeral": true,
  "cleanup_policy": "delete_remote_or_forget_local"
}
```

Then expose a cleanup action such as:

```json
{
  "channel_id": "noema-eval-channel-<random>",
  "action": "cleanup",
  "target": "<logical_session_id>",
  "force_forget_local": true
}
```

## Acceptance criteria

- NOEMA can create a Matrix session per run without stable IDs.
- NOEMA can bind an absolute workspace path without pre-registering persistent workspace metadata.
- The agent executes with that workspace as the effective cwd or tool cwd.
- After the run, Matrix can remove the logical session from its local session state even when the remote ACP agent does not support `session/delete`.
- If remote deletion is unsupported, Matrix clearly reports `remote_deleted=false`, `remote_canceled=true`, and `local_forgotten=true`.
- Ephemeral cleanup terminates or reaps agent subprocesses created only for that ephemeral run/session, or reports `process_reaped=false` and keeps the run non-clean.
- The run trace includes logical session id, remote session id, and cleanup result.
- Matrix CLI/log tooling can inspect this while the daemon is running without bbolt timeout issues.

## Why this matters

Without this, NOEMA can run real Matrix-backed tests, but cannot honestly call them official comparative evidence because an agent may retain or resume prior arm context.

For NOEMA, `cancel` is useful but not sufficient as the final cleanup contract unless Matrix also forgets the local logical session and declares the remote deletion limitation explicitly.

## Matrix maintainer response

Status: accepted and implemented as a generic Matrix contract, not as a NOEMA-specific integration.

Matrix now exposes the requested lifecycle in two protocol-neutral ways:

- `POST /v1/runs` supports `session_policy=new_ephemeral_delete_after_run`.
- `POST /v1/runs` supports `cleanup_policy`, scoped to the ephemeral run policy so it does not accidentally destroy a normal active session.
- `POST /v1/session-actions` `new` accepts `workspace_path`, `ephemeral`, and `cleanup_policy`.
- `POST /v1/session-actions` `cleanup` accepts `cleanup_policy` and `force_forget_local`.

Implementation notes:

- Logical Matrix session IDs are random UUIDs.
- `workspace_path` can bind an isolated filesystem root without requiring persistent workspace metadata.
- The agent router keys protocol clients by `agent_id + cwd`, so ACP session/new, session/load, tools, and terminal/fs handlers run against the requested workspace path.
- Remote lifecycle calls are workspace-aware when the session has a workspace path, so delete/cancel targets the same workspace-bound client instead of an arbitrary client for the same agent.
- If remote delete is unsupported, cleanup falls back to remote cancel when policy allows it.
- Local Matrix session mirror removal is explicit and also removes workspace session index entries.
- Cleanup attempts to close/reap the exact workspace-bound agent client after local mirror deletion when no other local session still references the same `agent_id + workspace_path`; for stdio ACP this closes the transport and terminates the child process path.
- Cleanup proof includes `clean`, `remote_deleted`, `remote_delete_unsupported`, `remote_canceled`, `process_reaped`, `process_retained`, `process_retention_allowed`, `local_forgotten`, and any cleanup error.
- Run traces record `session.policy.applied` and `session.cleanup`; `session.cleanup` is marked failed when `clean=false`.

Acceptance mapping:

- Separate conversation per evaluation arm: covered by `session_policy=new_ephemeral_delete_after_run`.
- Ephemeral/random identifier: covered by UUID logical sessions.
- No stable workspace metadata requirement: covered by direct `workspace_path`.
- Real agent CWD/tool CWD: covered by workspace-bound router clients and ACP `cwd`.
- Remote delete unsupported: reported as `remote_delete_unsupported=true`, with cancel fallback and local forget where policy allows.
- Cleanup artifact proof: covered by sync response `cleanup` and run trace events.
- Agent process retention: covered by `process_reaped`/`process_retained`/`process_retention_allowed`/`clean`.

Residual provider limitation:

Remote hard deletion is still provider capability-dependent. If an ACP/A2A provider does not expose delete semantics, Matrix cannot claim `remote_deleted=true`; it can only cancel, reap the local protocol client when possible, forget the local mirror, and report the exact proof.
