# Sidecar Capsules

Sidecar capsules let an upstream system attach structured context to a run without turning that context into normal chat history.

The main use case is:

```text
Frontend or supervisor -> Matrix -> coding agent
```

The upstream system sends the task body and one or more sidecar capsules. Matrix routes both through the selected protocol and records delivery in the run trace.

## Why It Exists

Some systems need to send meaning-oriented context:

- resolved intent
- evidence
- success criteria
- constraints
- avoid/do-not-do guidance
- optional read-only inspection hints

Putting that directly into chat loses traceability. Sending it only as protocol metadata is not reliable because real agents may not expose metadata to the model. Matrix supports both: structured correlation plus model-visible fallback when requested.

## HTTP Example

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

## Visibility

| Visibility | Behavior |
|------------|----------|
| `llm_visible` | Capsule content is projected into model-visible backend context and also traced |
| `trace_only` | Capsule identity and structured data are traced/projected when possible, but raw content is not appended as prompt text |

If omitted, visibility defaults to `llm_visible`.

Unknown future visibility values are accepted for forward compatibility and treated as non-prompt-visible by current Matrix releases.

## Protocol Projection

ACP:

- appends `llm_visible` capsule content to the prompt;
- adds `_meta` correlation under `matrix.dev/sidecar` and `<provider>.dev/sidecar`.

A2A:

- sends one text part for the task body;
- sends one data part per capsule with media type `application/vnd.<provider>.sidecar+json`;
- adds `matrix.sidecar` metadata and the Matrix sidecar extension URI;
- appends `llm_visible` capsule content as text fallback.

## Frontend Behavior

Normal chat views should not render raw sidecar content as user or assistant chatter.

Recommended frontend behavior:

- show the human task normally;
- optionally show `Sidecar context attached`;
- hide `sidecar.capsule.delivered` from the main timeline;
- expose capsule details only in trace/debug views.

## Trace

Each capsule creates a `sidecar.capsule.delivered` event with:

- `sidecar_provider`
- `sidecar_id`
- `sidecar_schema`
- `sidecar_version`
- `sidecar_carrier`
- `sidecar_visibility`

The event is `frontend_visible=false`, `audit_visible=true`, and `trace_visible=true`.

See also: [API Reference](API-Reference.md#post-v1runs) and [Run Trace Spec](../matrix_agent_communication_run_trace.md).

## Live Attach

For active async runs, supervisors can call `/v1/runs/{run_id}/actions` with `action=attach_context` and a `sidecar_capsules` array.

Matrix returns a `delivery_id`, delivers the context to the same logical/remote session when supported, and records:

- `run.context.attached` for accepted, delivered, late, failed, or unsupported delivery state;
- `sidecar.capsule.delivered` for each capsule delivered while the run is still active.

If the run is already terminal or the session is not ready, Matrix returns `status=unsupported` instead of pretending the context reached the agent.
If a provider accepts the additional turn but only processes it after the original run has completed, Matrix records `status=late` instead of treating that as in-run delivery.
`accepted` is not final delivery proof; it is followed by final evidence for the same `delivery_id`. During active runs, Matrix routes live attach to the run-bound logical/remote session because the channel mirror can lag until the active provider turn finishes.

Provider note: ACP standardizes `session/cancel` for interrupting/stopping an
active turn, not guaranteed mid-turn live context injection. Matrix therefore
measures live attach per provider. Current probes show OpenCode consuming live
context before completion; Codex via `codex-acp` and Gemini CLI ACP currently
complete with `late`.
