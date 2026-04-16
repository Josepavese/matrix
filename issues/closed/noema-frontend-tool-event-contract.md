# Issue: expose frontend-ready tool and permission event contract for Noema/Zed ACP

## Requesting project

Noema uses Matrix as the routing and trace substrate for this sidecar flow:

`Zed ACP frontend -> Noema ACP facade -> Matrix HTTP /v1/runs -> coding agent ACP backend`

The current integration works end-to-end for normal messages and code responses. It also successfully executes tool calls through Matrix/OpenCode. The remaining problem is frontend fidelity: ACP frontends such as Zed need semantically complete tool/permission events, not just opaque session updates.

This request is not meant as a one-off fix for one prompt. It is a general Matrix contract improvement so any frontend layered above Matrix can render agent activity cleanly without understanding provider-specific ACP internals.

## Observed evidence

On 2026-04-16, Noema routed this Zed prompt through Matrix/OpenCode:

`crea uno script casuale in /tmp`

Matrix/OpenCode successfully created the file and returned a final answer. The relevant runtime log shows:

```text
20:43:15 update_type=tool_call text_len=0
20:43:15 session/request_permission params_len=445
20:43:15 permission_approved optionID=once
20:43:15 update_type=tool_call_update text_len=0
20:43:17 update_type=tool_call text_len=0
20:43:18 update_type=tool_call_update text_len=0
20:43:21 completed routed turn response_len=144
```

The frontend result was mostly clean, but Zed rendered two generic `session/update` rows before the final answer because the Matrix-facing event payloads did not carry enough normalized tool metadata for Noema to project them into rich ACP `tool_call` / `tool_call_update` UI events.

Noema can provide fallback titles, but fallback-only projection is not enough for a production-grade frontend contract.

## Desired Matrix behavior

When Matrix observes backend ACP tool and permission activity, `/v1/runs/{run_id}/events` and `/v1/runs/{run_id}/trace` should expose normalized, frontend-ready events with stable identifiers, semantic tool fields, lifecycle status, and safe structured payloads.

The consuming frontend should not need to parse raw provider messages, ACP internals, request-permission blobs, shell strings, or natural language to understand what happened.

## Proposed event contract

### Tool requested

When a backend agent starts a tool call:

```json
{
  "id": "tool-call-stable-id",
  "run_id": "run-id",
  "sequence": 42,
  "kind": "tool.call.requested",
  "actor": "agent",
  "status": "pending",
  "protocol": "acp",
  "protocol_method": "session/update",
  "tool_name": "write_file",
  "tool_kind": "edit",
  "summary": "Create /tmp/casual_script.sh",
  "inputs": {
    "path": "/tmp/casual_script.sh"
  },
  "metadata": {
    "source_update_type": "tool_call",
    "frontend_visible": true
  }
}
```

### Tool in progress

When Matrix receives execution progress:

```json
{
  "id": "tool-call-stable-id",
  "run_id": "run-id",
  "sequence": 43,
  "kind": "tool.result.received",
  "actor": "tool",
  "status": "running",
  "protocol": "acp",
  "protocol_method": "session/update",
  "tool_name": "write_file",
  "tool_kind": "edit",
  "summary": "Writing /tmp/casual_script.sh",
  "inputs": {
    "path": "/tmp/casual_script.sh"
  },
  "metadata": {
    "source_update_type": "tool_call_update",
    "frontend_visible": true
  }
}
```

### Tool completed

When the tool completes:

```json
{
  "id": "tool-call-stable-id",
  "run_id": "run-id",
  "sequence": 44,
  "kind": "tool.result.received",
  "actor": "tool",
  "status": "completed",
  "protocol": "acp",
  "protocol_method": "session/update",
  "tool_name": "write_file",
  "tool_kind": "edit",
  "summary": "Created /tmp/casual_script.sh",
  "outputs": {
    "path": "/tmp/casual_script.sh"
  },
  "artifact_refs": [
    "file:///tmp/casual_script.sh"
  ],
  "metadata": {
    "source_update_type": "tool_call_update",
    "frontend_visible": true
  }
}
```

### Permission requested / resolved

When the backend ACP agent asks for permission, Matrix currently auto-approves in some local configurations. That decision is operationally important and should be trace-visible, but it should not be rendered as opaque frontend noise.

Preferred normalized event:

```json
{
  "id": "permission-stable-id",
  "run_id": "run-id",
  "sequence": 43,
  "kind": "permission.requested",
  "actor": "agent",
  "status": "pending",
  "protocol": "acp",
  "protocol_method": "session/request_permission",
  "summary": "Permission requested for file edit",
  "inputs": {
    "permission_kind": "file_write",
    "path": "/tmp/casual_script.sh",
    "options_count": 3
  },
  "metadata": {
    "frontend_visible": false,
    "audit_visible": true
  }
}
```

And resolution:

```json
{
  "id": "permission-stable-id",
  "run_id": "run-id",
  "sequence": 44,
  "kind": "permission.resolved",
  "actor": "matrix",
  "status": "completed",
  "protocol": "acp",
  "protocol_method": "session/request_permission",
  "summary": "Permission approved once",
  "outputs": {
    "decision": "approved",
    "option_id": "once"
  },
  "metadata": {
    "frontend_visible": false,
    "audit_visible": true,
    "approval_mode": "auto"
  }
}
```

## Required semantics

- Tool event IDs must be stable across requested/progress/completed updates.
- `tool_name` should be provider-normalized where possible, for example `write_file`, `read_file`, `shell`, `edit_file`, `search`, `list_files`.
- `tool_kind` should map to frontend-level categories, for example `read`, `edit`, `delete`, `move`, `search`, `execute`, `fetch`, `think`, `other`.
- `status` should use frontend lifecycle terms: `pending`, `running`, `completed`, `failed`.
- `summary` should be short and safe for direct UI display.
- `inputs` and `outputs` should be structured and redacted according to the active trace policy.
- `artifact_refs` should be emitted when a tool creates or modifies externally addressable artifacts.
- Permission events should be available for trace/audit consumers but marked with `metadata.frontend_visible=false` unless Matrix intentionally wants them rendered.
- Matrix lifecycle/routing/session events should remain available in trace, but frontend consumers need a reliable flag or event family distinction to filter them without brittle string matching.

## Trace policy interaction

Noema currently asks Matrix for:

```json
{
  "content_mode": "inline",
  "redaction_profile": "frontend",
  "include_protocol_meta": false
}
```

Under this profile, the tool event contract should still include safe display fields:

- `tool_name`
- `tool_kind`
- `summary`
- lifecycle `status`
- safe path-like fields when allowed by the frontend redaction profile
- stable IDs and sequence numbers

Raw ACP protocol payloads can remain hidden unless `include_protocol_meta=true`.

## Why this belongs in Matrix

Noema should remain a thin adapter at this boundary:

`Matrix normalized event -> ACP frontend event`

If Noema has to infer tool names, tool kinds, affected paths, permission decisions, or UI visibility from raw logs or natural language, the architecture becomes brittle and provider-specific. Matrix is the correct layer to normalize backend ACP/A2A/provider signals into provider-agnostic run events.

This also benefits future Matrix frontends directly. A Zed frontend, terminal frontend, web frontend, or telemetry dashboard can all consume the same normalized Matrix event stream.

## Noema-side intended mapping

Once Matrix exposes the fields above, Noema can map directly:

```text
tool.call.requested  -> ACP StartToolCall(id, title, kind, pending)
tool.result.received -> ACP UpdateToolCall(id, title, kind, running/completed/failed)
agent.message.delta  -> ACP UpdateAgentThoughtText or message delta, depending on visibility policy
agent.message.final  -> ACP UpdateAgentMessageText
permission.*         -> audit/trace only by default, unless frontend_visible=true
```

Noema should not parse raw provider text and should not inspect shell commands as a semantic source of truth.

## Acceptance criteria

- `/v1/runs/{run_id}/events` emits non-empty `tool_name` or at least a stable normalized fallback for ACP tool calls.
- Tool updates share the same event/tool ID as the originating tool call.
- Tool status advances through `pending/running/completed/failed` instead of opaque update-only events.
- A file creation/edit operation exposes a short safe summary and structured path metadata under frontend redaction policy.
- Permission requests and auto-approval decisions are trace-visible and audit-visible, but not rendered as generic `session/update` rows by frontend consumers.
- No raw `session/update` strings are required by Noema to render tool UI.
- The contract is documented in Matrix docs as a provider-agnostic frontend event contract, not a Noema-only hack.

## Non-goals

- Do not make Matrix depend on Noema.
- Do not make Matrix expose cognitive/internal Noema concepts.
- Do not require Matrix to render UI.
- Do not require Matrix to preserve unredacted tool payloads under frontend redaction policy.
- Do not solve every provider-specific tool taxonomy immediately; a conservative normalized subset is enough.

## Matrix resolution

Implemented as a Matrix-native frontend/audit event contract, not as a Noema-specific adapter.

The final implementation adds:

- run-local monotonic `sequence` values assigned by the run trace store;
- stable `tool_call_id` correlation across tool requested/result events;
- stable `permission_id` correlation across permission requested/resolved events;
- normalized `tool_name`, `tool_kind`, lifecycle `status`, `summary`, `inputs`, `outputs`, and `artifact_refs`;
- visibility metadata with frontend tool events rendered by default and permission events audit-visible but frontend-hidden by default;
- provider metadata isolation through `protocol_meta`, still controlled by trace policy;
- a reusable `internal/logic/frontendevents` normalization package;
- a reusable `internal/logic/runnotifier` bridge from live agent updates into trace events;
- documentation in the Matrix run trace and protocol-neutral runtime docs.

This keeps Noema as a thin consumer of Matrix-normalized events:

```text
Matrix normalized run event -> frontend ACP/Zed/Web/CLI projection
```

No consumer needs to parse raw `session/update` rows, ACP-specific blobs, request-permission payloads, shell text, or natural language logs to render the common tool/permission lifecycle.
