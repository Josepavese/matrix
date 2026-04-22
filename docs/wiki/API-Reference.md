# API Reference

Complete HTTP API reference for Matrix. The API server listens on `127.0.0.1:9091` by default.

## Authentication

All endpoints accept an optional API key via the `X-Matrix-Key` header:

```bash
curl -H "X-Matrix-Key: your-key" http://127.0.0.1:9091/_matrix/runtime
```

Configure the API key:

```bash
matrix config set acp_api_key your-key
```

## Health

### `GET /_matrix/runtime`

Runtime health report.

```bash
curl http://127.0.0.1:9091/_matrix/runtime
```

Returns a JSON health snapshot of the Matrix daemon.

---

## Runs

### `POST /v1/runs`

Execute a prompt on an agent. This is the primary endpoint for sending work to agents.

**Request:**

```bash
curl -X POST http://127.0.0.1:9091/v1/runs \
  -H "Content-Type: application/json" \
  -d '{
    "channel_id": "docs.http",
    "input": "List the files in the project root",
    "execution_mode": "sync",
    "agent_id": "opencode",
    "workspace_id": "my-project"
  }'
```

**Parameters:**

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `channel_id` | string | Yes | Stable caller/channel id, for example `docs.http` |
| `input` | string or object | Yes | The task body to send to the agent. Structured form is `{ "text": "..." }` |
| `execution_mode` | string | No | Execution mode: `sync` (default), `async`, `stream` |
| `agent_id` | string | No | Target agent (defaults to the configured default agent) |
| `workspace_id` | string | No | Target workspace |
| `workspace_path` | string | No | Workspace root path |
| `session_policy` | string | No | `new_ephemeral_delete_after_run` forces a fresh isolated session for the run |
| `cleanup_policy` | string | No | Cleanup policy for `session_policy=new_ephemeral_delete_after_run`; ignored as a destructive cleanup trigger when `session_policy` is omitted |
| `sidecar_capsules` | array | No | Protocol-neutral sidecar context projected into ACP/A2A and traced as `sidecar.capsule.delivered` |
| `emergency_kill_seconds` | number | No | Explicit wall-clock emergency fuse. Omitted means no hard run timeout |

**Response (sync mode):**

Returns the agent's response when the run completes.

For isolated evaluations, use:

```bash
curl -X POST http://127.0.0.1:9091/v1/runs \
  -H "Content-Type: application/json" \
  -d '{
    "channel_id": "eval.random-channel",
    "input": "Run the fixture task",
    "execution_mode": "sync",
    "agent_id": "opencode",
    "workspace_path": "/tmp/matrix-eval-fixture",
    "session_policy": "new_ephemeral_delete_after_run",
    "cleanup_policy": "delete_remote_or_cancel_and_forget_local"
  }'
```

The response may include `cleanup`, and the run trace records `session.policy.applied` and `session.cleanup`. If the agent fails after Matrix creates an ephemeral session, sync and stream error responses also include `cleanup` so callers can verify whether Matrix forgot the local mirror, canceled/deleted the provider session when supported, and reaped the workspace-bound agent process.

Cleanup proof includes:

```json
{
  "clean": true,
  "strong_cleanup": true,
  "cleanup_strength": "strong",
  "remote_delete_attempted": true,
  "remote_deleted": false,
  "remote_delete_unsupported": true,
  "remote_cancel_attempted": true,
  "remote_canceled": true,
  "process_reap_attempted": true,
  "process_reaped": true,
  "process_retention_allowed": false,
  "local_forgotten": true
}
```

**Sidecar capsules:**

Use `sidecar_capsules` when an upstream system or supervisory agent needs to attach machine-trackable context without making that context normal chat history.

```bash
curl -X POST http://127.0.0.1:9091/v1/runs \
  -H "Content-Type: application/json" \
  -d '{
    "channel_id": "supervisor.noema",
    "agent_id": "opencode",
    "execution_mode": "sync",
    "input": {
      "text": "Update the config parser to support an optional timeout."
    },
    "sidecar_capsules": [
      {
        "provider": "noema",
        "id": "caps_7f31",
        "schema": "sidecar.intent.v0",
        "version": "0.1",
        "visibility": "llm_visible",
        "format": "noema_xml",
        "content": "<noema id=\"caps_7f31\">intent: evolve_config_parser</noema>",
        "metadata": {
          "intent": "evolve_config_parser"
        }
      }
    ],
    "trace_policy": {
      "content_mode": "refs",
      "redaction_profile": "frontend",
      "include_protocol_meta": false
    }
  }'
```

Sidecar fields:

| Field | Required | Description |
|-------|----------|-------------|
| `provider` | Yes | Producer namespace, for example `noema` |
| `id` | Yes | Stable capsule id for trace correlation |
| `schema` | Recommended | Producer-owned schema id |
| `version` | Recommended | Producer-owned version |
| `visibility` | No | `llm_visible` or `trace_only`; empty defaults to `llm_visible`; unknown future values are accepted but not prompt-visible |
| `format` | No | Carrier format; inferred as `noema_xml` for `<noema...>` content |
| `content` | Required for `llm_visible` | Model-visible carrier text |
| `metadata` | No | Producer-owned structured metadata |

ACP routes append `llm_visible` content to the model prompt and attach `_meta` correlation. A2A routes send structured data parts plus metadata and also include a model-visible fallback for `llm_visible` capsules. Run traces include `sidecar.capsule.delivered`; normal frontend timelines should hide those internals by default.

**Response (async mode):**

Returns immediately with a `run_id`. Poll for results using the trace endpoint.

**Response (stream mode):**

Streams results as they arrive from the agent.

---

### `GET /v1/runs/{run_id}/trace`

Get the full trace for a run, including routing decisions, prompt, completion, and any failures.

```bash
curl http://127.0.0.1:9091/v1/runs/run-abc123/trace
```

---

### `GET /v1/runs/{run_id}/events`

Get the events for a run.

```bash
curl http://127.0.0.1:9091/v1/runs/run-abc123/events
```

---

### `POST /v1/runs/{run_id}/actions`

Perform an operational action on a run.

```bash
curl -X POST http://127.0.0.1:9091/v1/runs/run-abc123/actions \
  -H "Content-Type: application/json" \
  -d '{
    "action": "cancel"
  }'
```

Actions:

| Action | Description |
|--------|-------------|
| `cancel` / `stop` | Cancel an active run |
| `attach_context` / `append_context` | Attach live sidecar context to an active run session |

Live context example:

```bash
curl -X POST http://127.0.0.1:9091/v1/runs/run-abc123/actions \
  -H "Content-Type: application/json" \
  -d '{
    "action": "attach_context",
    "reason": "supervisor_suggestion",
    "source_event_id": "evt-source",
    "sidecar_capsules": [
      {
        "provider": "noema",
        "id": "sug_01",
        "schema": "noema.sidecar.suggestion.v0",
        "version": "0.1",
        "visibility": "llm_visible",
        "format": "noema_xml",
        "content": "<noema-suggestion>avoid loop</noema-suggestion>"
      }
    ]
  }'
```

`attach_context` returns `202` with a `delivery_id` when accepted. Delivery happens in the background and is visible in run events. `run.context.attached` uses `delivered` only when the context is delivered while the run is still active. If the provider processes it after the run becomes terminal, Matrix records `status=late` and does not emit `sidecar.capsule.delivered` for that run. Matrix returns `status=unsupported` when the run is not active, the session is not ready, or the runtime cannot attach live context.

`attach_context` is not the same as ACP `session/cancel`. ACP-compatible agents
are expected to support cancellation, but mid-turn live context consumption is
provider-specific. Treat `accepted` as queue/delivery acceptance only; treat
`delivered` before `run.completed` as in-run delivery proof; treat `late` as
"provider did not consume this context before the run ended." Current real
probes showed OpenCode consuming live context in-run, while Codex via
`codex-acp` and Gemini CLI ACP accepted the request but completed with `late`.
Use cancel-and-restart or next-turn context for providers without proven live
interrupt support.

---

### `POST /v1/event-sinks`

Register a webhook to receive run events.

```bash
curl -X POST http://127.0.0.1:9091/v1/event-sinks \
  -H "Content-Type: application/json" \
  -d '{
    "url": "https://example.com/webhook",
    "event_kinds": ["run.completed", "run.failed"]
  }'
```

---

## Sessions

### `POST /v1/session-actions`

Manage session lifecycle.

**List sessions:**

```bash
curl -X POST http://127.0.0.1:9091/v1/session-actions \
  -H "Content-Type: application/json" \
  -d '{
    "channel_id": "docs.http",
    "action": "list"
  }'
```

**Create a new session:**

```bash
curl -X POST http://127.0.0.1:9091/v1/session-actions \
  -H "Content-Type: application/json" \
  -d '{
    "channel_id": "docs.http",
    "action": "new",
    "target": "claude",
    "workspace_id": "my-project"
  }'
```

For ephemeral sessions without persistent workspace metadata:

```bash
curl -X POST http://127.0.0.1:9091/v1/session-actions \
  -H "Content-Type: application/json" \
  -d '{
    "channel_id": "eval.random-channel",
    "action": "new",
    "target": "opencode",
    "workspace_path": "/tmp/matrix-eval-fixture",
    "ephemeral": true,
    "cleanup_policy": "delete_remote_or_cancel_and_forget_local"
  }'
```

**Switch to a session:**

```bash
curl -X POST http://127.0.0.1:9091/v1/session-actions \
  -H "Content-Type: application/json" \
  -d '{
    "channel_id": "docs.http",
    "action": "switch",
    "target": "sess-123"
  }'
```

**Cancel a session:**

```bash
curl -X POST http://127.0.0.1:9091/v1/session-actions \
  -H "Content-Type: application/json" \
  -d '{
    "channel_id": "docs.http",
    "action": "cancel",
    "target": "sess-123"
  }'
```

**Delete a session:**

```bash
curl -X POST http://127.0.0.1:9091/v1/session-actions \
  -H "Content-Type: application/json" \
  -d '{
    "channel_id": "docs.http",
    "action": "delete",
    "target": "sess-123"
  }'
```

`delete` now returns a `cleanup` proof. If the provider does not support remote delete, Matrix attempts remote close when advertised by the protocol adapter, then remote cancel when policy allows it, then forgets the local mirror when requested by policy. After local deletion, Matrix closes the exact workspace-bound agent client when no other local session still references the same `agent_id + workspace_path`; otherwise it reports `process_retained=true`, `process_retention_allowed=true`, `cleanup_strength=retained`, and `weak_cleanup_reason=process_retained`. Cleanup proof can include `failure_code`; for example `agent_start_context_cancelled_during_cleanup` means a cleanup operation tried to start a provider while using an already-canceled context.

For async `/v1/runs/{run_id}/actions` `cancel`, Matrix uses a cleanup-specific
bounded context detached from the canceled run context. This allows
interrupt/resume clients to wait for `session.cleanup clean=true` before
starting the resume run. For ephemeral interrupt/resume flows, Matrix also
requires `strong_cleanup=true`; local-only forgetting fails with
`failure_code=cleanup_clean_without_remote_or_process_proof`.

**Cleanup a session:**

```bash
curl -X POST http://127.0.0.1:9091/v1/session-actions \
  -H "Content-Type: application/json" \
  -d '{
    "channel_id": "docs.http",
    "action": "cleanup",
    "target": "sess-123",
    "cleanup_policy": "delete_remote_or_cancel_and_forget_local",
    "force_forget_local": true
  }'
```

**Name a session:**

```bash
curl -X POST http://127.0.0.1:9091/v1/session-actions \
  -H "Content-Type: application/json" \
  -d '{
    "channel_id": "docs.http",
    "action": "name",
    "target": "auth-refactor"
  }'
```

**Provider capabilities:**

```bash
curl -X POST http://127.0.0.1:9091/v1/session-actions \
  -H "Content-Type: application/json" \
  -d '{
    "channel_id": "docs.http",
    "action": "capabilities",
    "target": "opencode"
  }'
```

The response contains `capabilities.session`, keyed by lifecycle feature. Each entry includes `supported`, `status`, `stability`, and `source`. ACP reports `list` and `info_update` as stable, `resume` and `close` as preview, and `fork` / `delete` as draft when the provider advertises them.

Unknown agent ids return a typed client error such as
`error.code=agent_not_found` instead of a generic server failure. Supervisory
clients should treat that as configuration failure, not as provider capability
absence.

**Fork a session:**

```bash
curl -X POST http://127.0.0.1:9091/v1/session-actions \
  -H "Content-Type: application/json" \
  -d '{
    "channel_id": "docs.http",
    "action": "fork",
    "target": "sess-123"
  }'
```

`fork` is a capability-gated experimental operation. Matrix calls a true provider fork only when the active protocol adapter advertises it; otherwise the response is typed with `unsupported=true` and `fork.unsupported=true`.

For automation, `fork` also accepts `make_active=false`, `restore_parent=true`,
and optional `input`. With `make_active=false`, Matrix mirrors the child without
switching the user's active channel session. If the logical parent has no remote
provider id yet, Matrix first creates a real remote parent session through the
provider session API, then forks that remote session. With `input`, Matrix runs
exactly one turn on the fork child and returns `fork.artifact.content`; when
`ephemeral` or `cleanup_policy` is supplied, Matrix cleans the child and returns
`fork.cleanup` proof. Matrix does not synthesize fork by prompt replay and does
not interpret the artifact content. If remote parent materialization is blocked,
Matrix returns typed evidence such as `error.code=missing_remote_session_id` or
`error.code=remote_session_materialize_failed` instead of HTTP `500`.

**Reconcile cached provider clients:**

```bash
curl -X POST http://127.0.0.1:9091/v1/session-actions \
  -H "Content-Type: application/json" \
  -d '{
    "channel_id": "docs.http",
    "action": "reconcile"
  }'
```

`reconcile` closes cached agent clients that no longer have vault session references. It returns `reconcile.reaped` and `reconcile.retained`.

**Parameters:**

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `channel_id` | string | Yes | Stable caller/channel id |
| `action` | string | Yes | `new`, `list`, `switch`, `cancel`, `delete`, `cleanup`, `name`, `capabilities`, `fork`, `reconcile` |
| `target` | string | No | Action operand: agent id, session selector, or alias |
| `workspace_id` | string | No | Workspace to bind the session to |
| `workspace_path` | string | No | Workspace root path for sessions that do not need persistent workspace metadata |
| `ephemeral` | boolean | No | Marks a new session as temporary/evaluation-only |
| `cleanup_policy` | string | No | Cleanup behavior for `delete`, `cleanup`, or ephemeral runs |
| `force_forget_local` | boolean | No | Removes the Matrix local mirror even when remote delete is unsupported |
| `make_active` | boolean | No | Fork only: whether the child becomes active. Defaults to `true` unless `input` is supplied |
| `restore_parent` | boolean | No | Fork only: restore/preserve the parent as active after child work |
| `input` | string | No | Fork only: one child turn to run for artifact-producing workflows |

Cleanup proof fields distinguish provider state from Matrix mirror state: `clean`, `strong_cleanup`, `cleanup_strength`, `weak_cleanup_reason`, `remote_deleted`, `remote_closed`, `remote_canceled`, `remote_*_attempted`, `remote_*_unsupported`, process reaping fields, and `local_forgotten`.

---

## Workspaces

### `POST /v1/workspace-actions`

Manage workspace lifecycle.

**List workspaces:**

```bash
curl -X POST http://127.0.0.1:9091/v1/workspace-actions \
  -H "Content-Type: application/json" \
  -d '{
    "channel_id": "docs.http",
    "action": "list"
  }'
```

**Get workspace status:**

```bash
curl -X POST http://127.0.0.1:9091/v1/workspace-actions \
  -H "Content-Type: application/json" \
  -d '{
    "channel_id": "docs.http",
    "action": "status"
  }'
```

**Create a snapshot:**

```bash
curl -X POST http://127.0.0.1:9091/v1/workspace-actions \
  -H "Content-Type: application/json" \
  -d '{
    "channel_id": "docs.http",
    "action": "snapshot",
    "target": "before-refactor"
  }'
```

**Switch workspace:**

```bash
curl -X POST http://127.0.0.1:9091/v1/workspace-actions \
  -H "Content-Type: application/json" \
  -d '{
    "channel_id": "docs.http",
    "action": "switch",
    "target": "my-project"
  }'
```

**Bind session to workspace:**

```bash
curl -X POST http://127.0.0.1:9091/v1/workspace-actions \
  -H "Content-Type: application/json" \
  -d '{
    "channel_id": "docs.http",
    "action": "bind",
    "target": "my-project"
  }'
```

---

### `GET /v1/workspace-state`

Get the current workspace state.

```bash
curl "http://127.0.0.1:9091/v1/workspace-state?workspace_id=my-project"
```

---

### `GET /v1/workspace-timeline`

Get the workspace event timeline.

```bash
curl "http://127.0.0.1:9091/v1/workspace-timeline?workspace_id=my-project"
```

---

### `GET /v1/workspace-memory`

Get workspace memory (turn summaries).

```bash
curl "http://127.0.0.1:9091/v1/workspace-memory?workspace_id=my-project"
```

---

### `GET /v1/workspace-snapshots`

List workspace snapshots.

```bash
curl "http://127.0.0.1:9091/v1/workspace-snapshots?workspace_id=my-project"
```

---

### `GET /v1/workspace-decisions`

Get the orchestration decision trace.

```bash
curl "http://127.0.0.1:9091/v1/workspace-decisions?workspace_id=my-project"
```

---

## Intents

### `POST /v1/intents`

Trigger a high-level intent.

```bash
curl -X POST http://127.0.0.1:9091/v1/intents \
  -H "Content-Type: application/json" \
  -d '{
    "channel_id": "docs.http",
    "intent": "handoff",
    "target": "claude",
    "workspace_id": "my-project"
  }'
```

Available intents:

| Intent | Description |
|--------|-------------|
| `continue` | Continue current work context |
| `resume` | Resume workspace context |
| `review` | Enter review mode |
| `explain` | Enter explain mode |
| `triage` | Enter triage mode |
| `handoff` | Hand off to another agent |

---

## Orchestration

### `GET /v1/orchestration-capabilities`

Get a machine-readable description of Matrix's capabilities. Useful for supervisory AI systems.

```bash
curl http://127.0.0.1:9091/v1/orchestration-capabilities
```

---

### `POST /v1/modes`

Switch work mode.

```bash
curl -X POST http://127.0.0.1:9091/v1/modes \
  -H "Content-Type: application/json" \
  -d '{
    "channel_id": "docs.http",
    "mode": "review"
  }'
```

---

## A2A

### `POST /a2a`

A2A protocol endpoint. Other A2A-compatible agents can call this to interact with Matrix.

The A2A agent card is available at the standard well-known path.

---

## Next

- [CLI Reference](CLI-Reference.md) -- the same operations from the terminal
- [Channels](Channels.md) -- set up Telegram and other channels
- [Examples](Examples.md) -- real API usage patterns
