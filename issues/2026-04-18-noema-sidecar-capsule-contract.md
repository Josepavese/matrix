# Issue: support protocol-neutral Noema sidecar capsules

## Requesting project

Noema uses Matrix as the routing and trace substrate for coding-agent sidecar flows:

`Frontend -> Noema -> Matrix -> coding agent`

Noema is introducing a compact sidecar capsule format that should be visible to the downstream LLM/agent while also remaining machine-trackable in traces.

For now, Noema wants `<noema>...</noema>` to be the primary carrier because real coding agents are not guaranteed to forward ACP/A2A metadata into the model context. ACP `_meta` and A2A `metadata`/`Part.data` are useful, but they are not a reliable substitute for model-visible guidance.

This request asks Matrix to consider a protocol-neutral contract for carrying these capsules without needing Noema to know whether the selected backend route is ACP, A2A, or another provider protocol.

## Current Noema direction

Noema sidecar capsules are not generic prompt optimization text.

They carry meaning-oriented execution guidance:

- resolved intent
- relevant evidence
- success criteria
- constraints
- avoid/do-not-do guidance
- optional read-only helper commands for additional inspection

Example compact prompt shape:

```text
TASK:
Aggiorna il parser config per supportare timeout opzionale. Mantieni compatibilita.

<noema v="0.1" id="caps_7f31" schema="sidecar.intent.v0" help="noema sidecar explain caps_7f31" inspect="noema sidecar inspect caps_7f31 --json" policy="read_only optional">
intent: evolve_config_parser
meaning:
- preserve existing config behavior
- add optional timeout field
- fail closed on invalid timeout values
evidence:
- prior fixes failed when missing fields changed default behavior
- compatible evolution is preferred over schema breakage
success:
- existing tests still pass
- new timeout tests cover missing, valid, invalid values
avoid:
- do not rename existing config keys
- do not make timeout mandatory
</noema>
```

The helper commands in the opening tag are intentionally read-only and optional. They let an agent ask Noema for more structured context only if it already has shell/tool access and decides the capsule is insufficient.

## Desired Matrix behavior

Matrix should expose a provider-neutral way for upstream systems such as Noema to submit sidecar capsules separately from the human task body, while still allowing Matrix to project them into the backend protocol safely.

Preferred conceptual request shape:

```json
{
  "agent_id": "opencode",
  "input": {
    "text": "Aggiorna il parser config per supportare timeout opzionale. Mantieni compatibilita."
  },
  "sidecar_capsules": [
    {
      "provider": "noema",
      "id": "caps_7f31",
      "schema": "sidecar.intent.v0",
      "version": "0.1",
      "visibility": "llm_visible",
      "format": "noema_xml",
      "content": "<noema v=\"0.1\" id=\"caps_7f31\" schema=\"sidecar.intent.v0\" help=\"noema sidecar explain caps_7f31\" inspect=\"noema sidecar inspect caps_7f31 --json\" policy=\"read_only optional\">...</noema>",
      "metadata": {
        "intent": "evolve_config_parser",
        "policy": "read_only optional"
      }
    }
  ],
  "trace_policy": {
    "content_mode": "inline",
    "redaction_profile": "frontend",
    "include_protocol_meta": false
  }
}
```

The exact API shape is Matrix-owned. The important requirement is that Noema can send:

- the task body as task body;
- the Noema capsule as sidecar context;
- stable capsule identifiers for trace correlation;
- visibility intent: `llm_visible`, `trace_only`, or future values.

## Protocol projection expectations

Matrix does not need to understand or validate Noema's internal meaning schema.

Matrix should only preserve and route the capsule according to the selected backend protocol.

### ACP backend

For ACP routes, the safe default is:

- append or inject the `<noema>...</noema>` block into the model-visible prompt sent to the backend agent;
- attach a parallel `_meta["noema.dev/sidecar"]` object when possible;
- preserve capsule id/schema/version in trace.

Conceptual ACP projection:

```json
{
  "prompt": [
    {
      "type": "text",
      "text": "TASK:\n...\n\n<noema ...>...</noema>"
    }
  ],
  "_meta": {
    "noema.dev/sidecar": {
      "capsule_ids": ["caps_7f31"],
      "primary_carrier": "noema_xml",
      "schema": "sidecar.intent.v0"
    }
  }
}
```

Important: `_meta` should be treated as machine correlation, not as the only delivery path.

### A2A backend

For A2A routes, Matrix can use native structure more naturally:

- task/message `metadata`;
- `Part.data`;
- `media_type: application/vnd.noema.sidecar+json`;
- `extensions`.

But Noema still wants a model-visible fallback unless the selected A2A agent explicitly advertises that it consumes the structured sidecar data semantically.

Conceptual A2A projection:

```json
{
  "parts": [
    {
      "text": "Aggiorna il parser config per supportare timeout opzionale."
    },
    {
      "data": {
        "noema": {
          "version": "0.1",
          "capsule_id": "caps_7f31",
          "schema": "sidecar.intent.v0"
        }
      },
      "media_type": "application/vnd.noema.sidecar+json"
    },
    {
      "text": "<noema ...>...</noema>"
    }
  ],
  "metadata": {
    "noema_capsule_ids": ["caps_7f31"]
  },
  "extensions": ["https://noema.dev/extensions/sidecar/v0"]
}
```

## Trace requirements

Matrix traces should expose capsule delivery as explicit evidence, not as opaque prompt text only.

Suggested normalized trace event:

```json
{
  "kind": "sidecar.capsule.delivered",
  "actor": "matrix",
  "status": "completed",
  "provider": "noema",
  "capsule_id": "caps_7f31",
  "schema": "sidecar.intent.v0",
  "carrier": "noema_xml",
  "visibility": "llm_visible",
  "protocol": "acp",
  "summary": "Delivered Noema sidecar capsule caps_7f31",
  "metadata": {
    "frontend_visible": false,
    "audit_visible": true,
    "trace_visible": true
  }
}
```

The capsule content may be redacted according to trace policy, but delivery, carrier, id, schema, and visibility should remain observable.

## Frontend expectations

The `<noema>` capsule should generally not be rendered as normal assistant/user chatter in frontend timelines.

Preferred frontend behavior:

- user sees their task;
- frontend may show a small optional indicator such as `Noema sidecar context attached`;
- detailed capsule content remains inspectable only through trace/debug views;
- Matrix lifecycle/session/routing events remain hidden from normal chat output unless explicitly requested.

This preserves the user experience while still giving the downstream model real guidance.

## Non-goals

- Matrix should not parse Noema meaning semantics.
- Matrix should not generate Noema capsules.
- Matrix should not execute Noema helper commands.
- Matrix should not depend on Noema binaries.
- Matrix should not assume all providers understand ACP `_meta` or A2A structured data.
- Matrix should not expose hidden cognitive internals to frontend users by default.

## Why this belongs in Matrix

Noema can concatenate `<noema>` blocks itself, but doing so loses protocol-neutral traceability and makes every route-specific adapter responsible for the same carrier decisions.

Matrix is the better place to provide:

- a single neutral run API field for sidecar capsules;
- backend-specific projection into ACP, A2A, or future protocols;
- trace evidence that the capsule was delivered;
- frontend filtering rules that prevent capsule noise from appearing as user-visible chat.

This lets Noema remain focused on generating meaning capsules, while Matrix remains the protocol-neutral communication substrate.

## Acceptance criteria

- Matrix exposes a provider-neutral way to accept one or more sidecar capsules with id/schema/version/visibility.
- ACP routes preserve `<noema>` as model-visible guidance and optionally attach `_meta` correlation.
- A2A routes preserve equivalent metadata/data parts while keeping model-visible fallback unless explicitly unsupported by policy.
- Traces include a normalized `sidecar.capsule.delivered`-style event or equivalent.
- Frontend consumers can reliably hide sidecar capsule internals from normal chat output while keeping trace/debug access.
- The implementation remains optional and does not couple Matrix to Noema internals.
