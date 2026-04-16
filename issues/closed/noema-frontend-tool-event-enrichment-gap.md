# Issue: enrich frontend tool event summaries and structured inputs

## Requesting project

Noema consumes Matrix as the protocol-neutral sidecar substrate for this flow:

`Zed ACP frontend -> Noema ACP facade -> Matrix HTTP /v1/runs -> coding agent ACP backend`

Matrix has already implemented the core provider-neutral frontend event contract requested by Noema:

- `tool.call.requested`
- `tool.result.received`
- `permission.requested`
- `permission.resolved`
- stable `tool_call_id`
- stable `permission_id`
- `tool_name`
- `tool_kind`
- `metadata.frontend_visible`
- `metadata.audit_visible`

This issue documents the remaining gap found during a real integration run.

## Real verification evidence

Date: 2026-04-16

Matrix commit line observed:

```text
15b0f93 Use Matrix home vault key by default
b1e9522 Add provider-neutral frontend trace events
```

The PAL-home vault fix works:

- Matrix no longer requires an external agent to export vault variables when launched from the normal PAL home.
- `matrix doctor` now reads the vault using `/home/jose/.local/share/matrix/configs/vault-master.key`.
- `matrix run` can stay alive in foreground.
- HTTP on `127.0.0.1:9091` responds.

Noema then executed a real Matrix HTTP run:

```text
POST /v1/runs
agent_id=opencode
execution_mode=async
input="crea uno script casuale in /tmp chiamato noema_matrix_contract.sh"
trace_policy.content_mode=inline
trace_policy.redaction_profile=frontend
trace_policy.include_protocol_meta=false
```

Run id:

```text
run-fd2ce2c7-5607-44f0-afb3-68857e47a3fd
```

Outcome:

```text
completed
summary: Creato e reso eseguibile `/tmp/noema_matrix_contract.sh`.
```

## What works now

The exported trace now contains provider-neutral frontend tool and permission events.

Example tool start:

```json
{
  "sequence": 8,
  "kind": "tool.call.requested",
  "actor": "agent",
  "status": "pending",
  "tool_call_id": "tool-b6b8fc5f8900518b",
  "tool_name": "write_file",
  "tool_kind": "edit",
  "summary": "write",
  "inputs": null,
  "metadata": {
    "content_length": 5,
    "frontend_visible": true,
    "source_update_type": "tool_call"
  }
}
```

Example tool result:

```json
{
  "sequence": 9,
  "kind": "tool.result.received",
  "actor": "tool",
  "status": "completed",
  "tool_call_id": "tool-b6b8fc5f8900518b",
  "tool_name": "write_file",
  "tool_kind": "edit",
  "summary": "write",
  "outputs": null,
  "metadata": {
    "content_length": 5,
    "frontend_visible": true,
    "source_update_type": "tool_call_update"
  }
}
```

Example permission event:

```json
{
  "sequence": 10,
  "kind": "permission.requested",
  "actor": "agent",
  "status": "pending",
  "permission_id": "perm-e964cd0793964307",
  "summary": "Permission requested for /tmp/noema_matrix_contract.sh",
  "inputs": {
    "path": "/tmp/noema_matrix_contract.sh",
    "options": [
      {"kind": "allow_once", "optionId": "once"},
      {"kind": "allow_always", "optionId": "always"},
      {"kind": "reject_once", "optionId": "reject"}
    ]
  },
  "metadata": {
    "audit_visible": true,
    "frontend_visible": false
  }
}
```

This is a strong improvement: Noema can now consume `tool_call_id`, filter `frontend_visible=false`, and avoid showing permission/audit events in the Zed timeline.

## Remaining gap

The tool events are structurally correct but still too poor for a production-grade frontend:

- `summary` is often only `write` or `bash`;
- `inputs` is often `null` even when Matrix later knows the path through the permission request;
- `outputs` is often `null` even when the tool completed successfully;
- `artifact_refs` is often absent even when a file was created or modified;
- the useful path is visible in the permission event, but not carried back into the correlated tool event.

For Zed and other frontends this means:

- the old `session/update` leakage can be removed by Noema;
- but the rendered tool rows may still look too generic;
- frontend adapters cannot show clear text such as `Create /tmp/noema_matrix_contract.sh` unless Matrix enriches the tool event itself.

## Desired behavior

Matrix should preserve the current provider-neutral event contract, but enrich tool events using structured ACP payloads and correlated permission context when available.

For the same run, the preferred tool start would be:

```json
{
  "kind": "tool.call.requested",
  "status": "pending",
  "tool_call_id": "tool-b6b8fc5f8900518b",
  "tool_name": "write_file",
  "tool_kind": "edit",
  "summary": "Create /tmp/noema_matrix_contract.sh",
  "inputs": {
    "path": "/tmp/noema_matrix_contract.sh"
  },
  "metadata": {
    "frontend_visible": true,
    "source_update_type": "tool_call"
  }
}
```

And the completion:

```json
{
  "kind": "tool.result.received",
  "status": "completed",
  "tool_call_id": "tool-b6b8fc5f8900518b",
  "tool_name": "write_file",
  "tool_kind": "edit",
  "summary": "Created /tmp/noema_matrix_contract.sh",
  "outputs": {
    "path": "/tmp/noema_matrix_contract.sh"
  },
  "artifact_refs": [
    "file:///tmp/noema_matrix_contract.sh"
  ],
  "metadata": {
    "frontend_visible": true,
    "source_update_type": "tool_call_update"
  }
}
```

For shell execution:

```json
{
  "kind": "tool.call.requested",
  "status": "pending",
  "tool_call_id": "tool-c856f4e8078b6c0f",
  "tool_name": "shell",
  "tool_kind": "execute",
  "summary": "Run chmod on /tmp/noema_matrix_contract.sh",
  "inputs": {
    "path": "/tmp/noema_matrix_contract.sh",
    "operation": "chmod"
  },
  "metadata": {
    "frontend_visible": true
  }
}
```

## Important design constraint

Matrix should not rely on natural-language inference as the primary source of truth.

Preferred enrichment sources, in order:

1. structured ACP tool call payload fields;
2. provider metadata already available in the ACP update;
3. correlated `session/request_permission` payload for the same pending tool;
4. safe file/artifact information already known to Matrix;
5. only then a conservative fallback title such as `write_file` or `shell`.

If Matrix cannot safely determine a path or operation from structured fields, it should leave those fields empty rather than hallucinate.

## Possible implementation direction

Matrix already emits permission events with structured `inputs.path`.

In the observed run:

- tool event has `tool_call_id=tool-b6b8fc5f8900518b`, `tool_name=write_file`, `summary=write`, `inputs=null`;
- immediately following permission event has `permission_id=perm-e964cd0793964307`, `summary=Permission requested for /tmp/noema_matrix_contract.sh`, `inputs.path=/tmp/noema_matrix_contract.sh`;
- this permission is part of the same backend tool action window.

Matrix could maintain a short-lived correlation context inside the run notifier:

- active tool call id;
- last pending tool event;
- permission request path/operation;
- completion status.

When the permission payload exposes a path, Matrix can enrich the active tool event or append later tool updates with the path. If immutable event storage makes retroactive update undesirable, the later `tool.result.received` event can carry the enriched `outputs` and `artifact_refs`.

## Acceptance criteria

- File creation/edit tool events expose a safe path in `inputs.path` or `outputs.path` when the ACP/permission payload contains it.
- Completed file creation/edit events expose `artifact_refs=["file://..."]` when safe under `redaction_profile=frontend`.
- Tool summaries are frontend-grade:
  - `Create /tmp/name.sh`
  - `Created /tmp/name.sh`
  - `Run chmod on /tmp/name.sh`
  - not just `write` or `bash`.
- Permission events remain `frontend_visible=false` by default.
- Frontend-visible tool events remain `frontend_visible=true`.
- Noema does not need to parse raw provider text, shell commands, or permission prose to render usable tool UI.
- The behavior is covered by Matrix tests around ACP tool + permission correlation.

## Non-goals

- Do not make Matrix depend on Noema.
- Do not expose unsafe raw tool payloads under frontend redaction policy.
- Do not infer arbitrary semantics from user prompts.
- Do not require every provider to have perfect tool metadata immediately; improve where structured ACP payloads or permission payloads already provide the data.


## Matrix maintainer response

Accepted and implemented.

Matrix keeps the protocol-neutral run trace as the public SSOT and does not expose ACP `session/update` as the primary frontend contract. Native ACP data is now preserved under `protocol_meta.acp` when protocol metadata is requested, so ACP-native facades can inspect or project it without forcing Telegram, HTTP, A2A, or future channels to depend on ACP shapes.

The runtime now enriches tool events from structured sources in this order: provider tool metadata, provider native metadata, the correlated active permission request, and safe Matrix-known file/artifact data. In the observed Noema flow this means a generic ACP `write` tool event can be enriched with `/tmp/noema_matrix_contract.sh` from the permission payload, producing frontend-grade neutral events such as `Create /tmp/noema_matrix_contract.sh` and `Created /tmp/noema_matrix_contract.sh` with `outputs.path` and `file://` artifact refs.

Permission events remain audit-visible and hidden from the default frontend timeline (`frontend_visible=false`), while tool events stay frontend-visible. This lets Noema render useful Zed rows without parsing raw provider prose or leaking permission/audit events.

A2A compatibility is preserved by design: A2A native data should be stored in its own protocol metadata namespace, while Matrix continues to emit the same neutral `tool.call.requested`, `tool.result.received`, `permission.requested`, and `permission.resolved` events. Any ACP-specific projection is therefore an adapter concern, not the Matrix core contract.

Implementation coverage added:

- active tool and permission correlation in the run notifier;
- safe structured path and command-operation extraction;
- enriched tool summaries, inputs, outputs, and artifact refs;
- raw ACP update preservation under protocol metadata;
- tests for ACP tool/permission correlation and ACP protocol metadata preservation.
