# Matrix Sidecar Capsules

Matrix supports protocol-neutral sidecar capsules on `/v1/runs`.

Sidecar capsules are optional context blocks supplied by an upstream system alongside the human task body. Matrix does not interpret the capsule semantics. It preserves identity, visibility, carrier, and trace evidence, then projects the capsule into the selected backend protocol.

This feature is generic. Noema is the first expected producer, but Matrix treats `provider` as data and does not depend on Noema binaries or schemas.

## Contract

Request shape:

```json
{
  "channel_id": "noema.http",
  "agent_id": "opencode",
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
      "content": "<noema v=\"0.1\" id=\"caps_7f31\" schema=\"sidecar.intent.v0\">...</noema>",
      "metadata": {
        "intent": "evolve_config_parser",
        "policy": "read_only optional"
      }
    }
  ],
  "trace_policy": {
    "content_mode": "refs",
    "redaction_profile": "frontend",
    "include_protocol_meta": false
  }
}
```

`input` may be the compact string form or the structured `{ "text": "..." }` form. Structured input is preferred for upstream systems because it keeps the task body visibly separate from sidecar context.

## Fields

| Field | Required | Meaning |
|-------|----------|---------|
| `provider` | Yes | Producer namespace, for example `noema` |
| `id` | Yes | Stable capsule id used for trace correlation |
| `schema` | Recommended | Producer-owned schema id |
| `version` | Recommended | Producer-owned schema/version value |
| `visibility` | Yes | `llm_visible` or `trace_only`; empty defaults to `llm_visible`; unknown future values are accepted but treated as non-prompt-visible by Matrix |
| `format` | No | Carrier format, for example `noema_xml`; inferred as `noema_xml` when content contains `<noema` |
| `content` | Required for `llm_visible` | Model-visible carrier text |
| `metadata` | No | Producer-owned structured correlation metadata |

## Projection

### ACP

ACP routes receive:

- the task body as the prompt text;
- every `llm_visible` capsule appended as model-visible text;
- `_meta` correlation under `matrix.dev/sidecar` and `<provider>.dev/sidecar`.

Matrix treats `_meta` as correlation only. The model-visible carrier remains the safe default because real coding agents do not consistently forward protocol metadata into model context.

### A2A

A2A routes receive:

- a text part for the task body;
- a data part for each capsule using media type `application/vnd.<provider>.sidecar+json`;
- message/request metadata under `matrix.sidecar`;
- the Matrix sidecar extension URI `https://matrix.dev/extensions/sidecar/v0`;
- a model-visible text fallback for each `llm_visible` capsule.

`trace_only` capsules are not appended as text but are still represented as structured sidecar data and trace evidence.

Unknown future visibility values are accepted for forward compatibility and are treated like trace-only by current Matrix releases. Only the explicit `llm_visible` value can inject capsule content into model-visible prompt text.

## Live Context Attachment

Supervisors can attach sidecar context to an active async run through `/v1/runs/{run_id}/actions`:

```json
{
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
}
```

Matrix accepts the request only for active runs with a known logical and remote session. It returns a `delivery_id` immediately, delivers in the background to the same session, and records `run.context.attached` plus `sidecar.capsule.delivered` evidence when delivery completes while the run is still active. If the provider processes the context after the run has already completed, Matrix records `run.context.attached` with `status=late` and does not mark the capsule as delivered into that run. If the run/session/backend cannot receive live context, Matrix returns a typed `unsupported` response instead of silent success.

ACP compatibility alone does not mean the provider can consume new context
inside an already running prompt turn. The ACP baseline includes
`session/cancel`, which interrupts/stops the current turn, but it does not
standardize mid-turn prompt/context injection. Matrix therefore treats live
context interrupt as a measured provider capability. Current real-agent probes:

- `opencode`: live context delivered before completion and marker observed in
  provider output;
- `codex` through `codex-acp`: request accepted while active, then `late`;
- `gemini` through Gemini CLI ACP: request accepted while active, then `late`.

See [matrix_live_context_interrupt_policy.md](matrix_live_context_interrupt_policy.md)
for source review, tested semantics, and fallback policy.

## Trace Evidence

Every accepted capsule creates a normalized event:

```json
{
  "kind": "sidecar.capsule.delivered",
  "actor": "matrix",
  "status": "completed",
  "sidecar_provider": "noema",
  "sidecar_id": "caps_7f31",
  "sidecar_schema": "sidecar.intent.v0",
  "sidecar_version": "0.1",
  "sidecar_carrier": "noema_xml",
  "sidecar_visibility": "llm_visible",
  "metadata": {
    "frontend_visible": false,
    "audit_visible": true,
    "trace_visible": true
  }
}
```

The capsule content follows normal trace policy:

- `refs`: stores content digest/ref only in exported traces;
- `redacted`: keeps capsule identity fields while hiding nonessential metadata;
- `inline`: may include raw capsule content in the event message.

Frontend consumers should hide `sidecar.capsule.delivered` from normal chat timelines and show at most an indicator such as `Sidecar context attached`.

## Non-Goals

Matrix does not:

- parse provider-specific capsule meaning;
- generate capsules;
- execute helper commands embedded in capsules;
- require Noema or any other producer;
- expose sidecar content as normal user/assistant chat by default.
