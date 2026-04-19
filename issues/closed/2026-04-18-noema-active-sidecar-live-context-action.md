# Noema Active Sidecar Requires Live Context/Suggestion Injection For Active Runs

Date: 2026-04-18
Reporter: Noema integration

## Summary

Noema needs a protocol-neutral Matrix surface that can attach a sidecar suggestion/context capsule to an already active `/v1/runs/{run_id}` execution.

Matrix already supports the parts around this:

- `/v1/runs` creates a stable run id.
- `execution_mode=async` and `GET /v1/runs/{run_id}/events?stream=sse` allow live observation.
- `sidecar_capsules` allow initial task-time context delivery.
- `/v1/runs/{run_id}/actions` currently supports operational `cancel`.

The missing surface is live, in-run context delivery:

```text
Noema observes Matrix run events
-> Noema detects a learned pattern or loop risk
-> Noema emits a short suggestion
-> Matrix delivers it into the same active agent session
-> Matrix records delivery/ack/effect evidence
```

Without this, Noema can only do initial capsules or post-run analysis. That blocks the active sidecar product path where Noema learns from prior traces and intervenes during long-running agent work without taking over the task.

## Desired Final Contract

Keep this Matrix-owned and provider-neutral. Noema semantics should remain payload data.

### Option A: Extend `/v1/runs/{run_id}/actions`

Add an action such as `attach_context` or `append_context`.

Example request:

```json
{
  "action": "attach_context",
  "reason": "supervisor_suggestion",
  "sidecar_capsules": [
    {
      "provider": "noema",
      "id": "sug_01J...",
      "schema": "noema.sidecar.suggestion.v0",
      "version": "0.1",
      "visibility": "llm_visible",
      "format": "noema_xml",
      "content": "<noema-suggestion kind=\"learned_pattern\" confidence=\"0.82\">...</noema-suggestion>",
      "metadata": {
        "run_id": "run-...",
        "source_event_id": "evt-...",
        "policy": "conservative"
      }
    }
  ]
}
```

Expected response:

```json
{
  "run_id": "run-...",
  "status": "running",
  "action": "attach_context",
  "accepted": true,
  "delivery_id": "ctx_..."
}
```

Expected trace events:

- `sidecar.capsule.delivered` or `run.context.attached`
- top-level provider/capsule identity fields already used by initial sidecar capsules
- `source_event_id` / `source_sequence` if provided
- delivery status: `accepted`, `delivered`, `rejected`, `unsupported`
- provider-neutral reason/error when not deliverable

### Option B: Dedicated Context Endpoint

Alternative:

```text
POST /v1/runs/{run_id}/context
```

with the same payload shape as above.

This may be cleaner if Matrix wants run actions to remain strictly lifecycle controls.

## Semantics Matrix Should Not Own

Matrix should not:

- parse Noema intent;
- classify whether the suggestion is correct;
- enforce Noema policy;
- know about Noema cognition, replay, doctrine, scars, motifs, or routines.

Matrix only needs to:

- accept provider-owned sidecar context;
- deliver it into the active agent communication channel when possible;
- record a durable trace event;
- report unsupported provider/session states explicitly.

## Delivery Requirements

The operation should be safe for real coding-agent sessions:

- It must target the same logical/remote session as the active run.
- It must not create a new conversation unless explicitly requested.
- It must preserve the original run id and event ordering.
- It must be observable by `GET /v1/runs/{run_id}/events`.
- It must work with async/SSE observation.
- It must return a typed unsupported response when the current protocol/provider cannot accept live context.
- It must preserve Matrix cleanup/session isolation semantics.

## Why Noema Needs This

Noema's active sidecar goal is not just initial prompt framing.

Target behavior:

```text
experience from prior runs
-> learned patterns / failure signatures
-> live intent timeline
-> conservative suggestion during current run
-> measured effect after the suggestion
-> replay-backed adoption or rejection
```

For long-running coding tasks, useful interventions often happen after the first failed validation, after repeated tool loops, or when the agent is about to mutate tests/oracles. A start-only `sidecar_capsules` payload cannot express that.

## Acceptance Criteria

- A caller can start an async run, read events, and attach a provider-owned sidecar capsule while the run is still active.
- The agent receives the live context in the same session when the backend supports it.
- The run trace shows the context delivery event with correlation metadata.
- Unsupported backends return an explicit non-2xx or accepted-with-unsupported status, not silent success.
- Existing `cancel` behavior remains unchanged.
- Initial `sidecar_capsules` behavior remains unchanged.

## Suggested First Slice

Implement `attach_context` for the `/v1/runs/{run_id}/actions` surface, even if the first backend support is limited to ACP agents that can receive another user/message update in the active session.

If ACP live append is not safe for some agents, Matrix should still expose a clean `unsupported` delivery result so Noema can record that active guidance was generated but not delivered.

## Matrix maintainer response

Status: accepted and implemented as a generic Matrix live context feature.

The request fits Matrix because it extends the existing protocol-neutral run supervision surface. The implementation does not add Noema semantics to Matrix: provider, schema, content, and metadata stay opaque sidecar data.

Implemented contract:

- `POST /v1/runs/{run_id}/actions` now accepts `attach_context` and `append_context`.
- Payload uses the same `sidecar_capsules` contract as initial `/v1/runs`.
- Matrix accepts only active runs with known logical and remote session ids.
- Matrix returns `202` plus a `delivery_id` immediately when delivery is accepted.
- Delivery runs asynchronously against the same logical/remote session.
- Unsupported states return typed `status=unsupported` instead of silent success.
- Existing `cancel` / `stop` behavior remains supported.

Trace evidence:

- `run.context.attached` records accepted, delivered, late, failed, or unsupported delivery state.
- Successful in-run delivery also emits `sidecar.capsule.delivered` for each capsule.
- Provider-late delivery is explicit: if the provider processes the injected context after the run has completed, Matrix records `status=late` and does not claim the capsule was delivered into the active run.
- Events include `delivery_id`, `delivery_status`, reason, capsule count, and optional source event correlation.
- Frontend metadata remains hidden-by-default: `frontend_visible=false`, `audit_visible=true`, `trace_visible=true`.

Runtime behavior:

- Session Manager exposes a run-context attachment path that targets an existing logical session and remote session.
- Matrix does not create a new conversation for live context.
- If the active agent/provider rejects concurrent or live delivery, Matrix records failure evidence in the run events.
- Run notifier now stores the logical session id early, so active async runs can become attachable before completion.

Documentation:

- `docs/matrix_sidecar_capsules.md`
- `docs/matrix_agent_communication_run_trace.md`
- `docs/wiki/API-Reference.md`
- `docs/wiki/Sidecar-Capsules.md`
- `docs/wiki/Core-Concepts.md`
- `docs/wiki/Examples.md`

Acceptance mapping:

- Start async and observe events: existing `/v1/runs` async/SSE remains unchanged.
- Attach during active run: covered by `attach_context`.
- Same session delivery: covered by logical/remote session targeting.
- Trace delivery: covered by `run.context.attached` and `sidecar.capsule.delivered`.
- Unsupported providers/session states: covered by typed `unsupported`.
- Existing initial sidecars and cancel behavior: unchanged.
