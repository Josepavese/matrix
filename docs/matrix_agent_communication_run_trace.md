# Matrix Agent Communication Run Trace

Matrix owns protocol-neutral communication evidence.

This document adopts the external Noema proposal as a Matrix-native product surface. Noema is one expected consumer, but the feature is not Noema-specific.

```text
Matrix = communication plane
Noema = cognition compiler
```

Matrix records what happened while humans, channels, agents, protocols, sessions, tools, and supervisory systems communicated. External systems may compile that evidence into memory, cognition, compliance, analytics, replay, or governance.

## Product Claim

Matrix is not only a chat gateway.

Matrix is the local-first Agent Communication Matrix: the central crossing point where human-to-agent and agent-to-agent flows become observable, controllable, and protocol-neutral.

An external AI supervisor should be able to ask:

- which run is active;
- which agent was selected;
- why routing selected that agent;
- which session was reused;
- which protocol carried the exchange;
- which tool or artifact events occurred;
- whether the run should be cancelled;
- what final outcome was produced.

The supervisor must not need to know whether the provider is ACP, A2A, Telegram-triggered, HTTP-triggered, CLI-triggered, or a future channel.

## Final Contract

The contract family is:

```http
POST /v1/runs
GET  /v1/runs/{run_id}/trace
GET  /v1/runs/{run_id}/events
POST /v1/runs/{run_id}/actions
POST /v1/event-sinks
```

`POST /v1/runs` starts one Matrix communication run and always materializes a stable `run_id`.

Supported execution modes:

```text
sync   = execute and wait for final output, while still creating run evidence
async  = start the run and return immediately with observation URLs
stream = return live events directly, backed by the same run_id and trace model
```

Optional emergency fuse:

```json
{
  "emergency_kill_seconds": 0
}
```

Omitted or `0` means no absolute turn timeout. Matrix intentionally does not define a default hard timeout for communication runs because long-running autonomous agents are valid workloads. Production control is based on run events, stream/async observation, explicit `cancel`/`stop`, and channel or supervisor policy. Wall-clock termination is only allowed when the caller explicitly sends `emergency_kill_seconds`.

Timeout and recovery rules are tracked in [matrix_timeout_recovery_policy.md](matrix_timeout_recovery_policy.md).

Every response includes:

```json
{
  "run_id": "run-...",
  "status": "running|completed|failed|cancelled",
  "output": "optional sync output",
  "trace_url": "/v1/runs/run-.../trace",
  "events_url": "/v1/runs/run-.../events",
  "actions_url": "/v1/runs/run-.../actions",
  "cleanup": {
    "logical_session_id": "optional",
    "remote_session_id": "optional",
    "clean": true,
    "strong_cleanup": true,
    "cleanup_strength": "strong",
    "remote_deleted": false,
    "remote_closed": false,
    "remote_canceled": true,
    "remote_close_attempted": false,
    "process_reap_attempted": true,
    "process_reaped": true,
    "process_retention_allowed": false,
    "local_forgotten": true,
    "warnings": ["remote_cancel_session_not_found_after_process_reap"],
    "failure_code": "optional_machine_code"
  }
}
```

`cleanup` appears when the caller requests an ephemeral run lifecycle, for example `session_policy=new_ephemeral_delete_after_run`. Sync and stream error responses also carry `cleanup` when Matrix already created an ephemeral session before the agent failure; callers should inspect `clean`, `strong_cleanup`, `cleanup_strength`, `weak_cleanup_reason`, `local_forgotten`, `remote_deleted`, `remote_closed`, `remote_canceled`, process fields, `warnings`, and `failure_code` instead of inferring cleanup from HTTP status alone. Cleanup after `/v1/runs/{run_id}/actions` `cancel` uses a bounded context detached from the canceled run context, so remote cleanup and process reap are not run under an already-canceled context.

Provider readiness failures are also projected into the run response and event
stream. Matrix emits `provider.preflight.failed` with `code`, `agent_id`,
`protocol`, `phase`, and safe adapter diagnostics when an adapter/provider
failure prevents task execution. Typical codes are `provider_model_unavailable`,
`provider_auth_mismatch`, and `agent_preflight_failed`.

`clean=true` means Matrix reached an operational cleanup state for that lifecycle. `strong_cleanup=true` means Matrix has hard evidence from the provider or OS process layer. For ephemeral interrupt/resume flows, Matrix requires strong proof; local-only forgetting fails with `failure_code=cleanup_clean_without_remote_or_process_proof`. For non-ephemeral shared sessions, retained clients are allowed but explicitly marked with `cleanup_strength=retained` and `weak_cleanup_reason=process_retained`.

For local stdio ACP agents, Matrix does not spawn a fresh workspace client only to clean up a session owned by a now-dead process. If process reap proves the old agent is gone, cleanup remains strong and may include typed warnings such as `remote_lifecycle_skipped_no_reusable_cached_agent_client` and `remote_cancel_session_not_found_after_process_reap`. Expected async cancellation is logged as `run_cancelled`, not as a generic `matrix async run bridge failed` error.

## Canonical State

The trace is not a second history.

Canonical Matrix records are:

- run records;
- run events;
- workspace timeline entries;
- logical session records;
- remote provider session records;
- routing decisions;
- artifacts and content references;
- channel ingress metadata.

The exported trace is a versioned projection:

```text
Matrix canonical operational records
  -> matrix.agent_communication_run_trace.v0
  -> external consumers
```

## Schema

The public projection schema is:

```text
matrix.agent_communication_run_trace.v0
```

The schema is an operational communication trace, not a cognitive trace. It must not contain Noema concepts such as scars, doctrine, routines, promotion, semantic lift, or replay decisions.

## Event Taxonomy

Matrix keeps the primary taxonomy operational and protocol-neutral:

- `run.started`
- `run.completed`
- `run.failed`
- `run.cancelled`
- `routing.decision`
- `session.created`
- `session.resumed`
- `agent.prompt.sent`
- `agent.message.delta`
- `agent.message.final`
- `tool.call.requested`
- `tool.result.received`
- `artifact.created`
- `run.context.attached`
- `sidecar.capsule.delivered`

Provider-specific details belong in `protocol_meta`.

## Sidecar Capsule Events

External systems can attach sidecar capsules to `/v1/runs` without coupling
Matrix to that system's semantics. Matrix records delivery through
`sidecar.capsule.delivered`.

The event keeps capsule identity in top-level trace fields so redaction can
hide content while preserving audit evidence:

- `sidecar_provider`
- `sidecar_id`
- `sidecar_schema`
- `sidecar_version`
- `sidecar_carrier`
- `sidecar_visibility`

Default frontend metadata is `frontend_visible=false`, `audit_visible=true`,
and `trace_visible=true`. Normal chat timelines should show the human task,
not the raw sidecar carrier. Debug and trace views can inspect the event.

Protocol projection is backend-specific but Matrix-owned:

- ACP receives model-visible fallback text for `llm_visible` capsules and
  `_meta` correlation under `matrix.dev/sidecar` and `<provider>.dev/sidecar`.
- A2A receives data parts, message/request metadata, the Matrix sidecar
  extension URI, and model-visible fallback text for `llm_visible` capsules.

See [matrix_sidecar_capsules.md](matrix_sidecar_capsules.md) for the API
contract and projection rules.

Live context attachment uses `run.context.attached` for request, delivery,
late delivery, failure, or unsupported evidence. The event metadata includes
`delivery_id`, `delivery_status`, `reason`, optional `source_event_id` /
`source_sequence`, and frontend/audit visibility flags. Successful in-run live
attachments also emit `sidecar.capsule.delivered` events with the same
`delivery_id`. If a provider processes the injected context after the run has
already completed, Matrix records `status=late` and does not claim in-run
capsule delivery.

## Frontend Event Contract

Matrix exposes a provider-agnostic frontend event contract for agent activity. It is intended for Zed ACP facades, web consoles, terminal UIs, telemetry dashboards, and future channel adapters.

Every exported event has:

- `id`: unique Matrix event id;
- `sequence`: monotonic run-local event order;
- `kind`: protocol-neutral event family;
- `status`: normalized lifecycle status when applicable;
- `metadata.frontend_visible`: whether a frontend should normally render the event;
- `metadata.audit_visible`: whether the event is useful for audit even when hidden from the main frontend timeline.

Tool events use stable correlation fields:

- `tool_call_id`: stable id shared by `tool.call.requested` and related `tool.result.received` updates;
- `tool_name`: normalized tool name such as `write_file`, `read_file`, `edit_file`, `search`, `list_files`, or `shell`;
- `tool_kind`: ACP-aligned primary category: `read`, `edit`, `delete`, `move`, `search`, `execute`, `think`, `fetch`, `switch_mode`, or `other`;
- `tool_semantic_kind`: optional provider-neutral semantic class such as `validate`, `vcs`, `network`, or `agent` when the provider or upstream event supplies it;
- `tool_effect`: structural effect class such as `read_only`, `write`, `execute`, `control`, or `unknown`;
- `tool_subject_kind`: target subject such as `workspace`, `filesystem`, `process`, `network`, `agent_session`, `agent_reasoning`, or `unknown`;
- `tool_classification_source`: `protocol_metadata`, `heuristic_fallback`, or `unknown`;
- `tool_classification_confidence`: `high` for protocol metadata, `low` for heuristic fallback;
- `summary`: short safe UI text;
- `inputs`: structured safe input fields;
- `outputs`: structured safe output fields;
- `artifact_refs`: externally addressable artifacts when known.

Downstream consumers should trust `tool_kind` and the structural fields first. Name/content heuristics are only a fallback and are marked low confidence so external systems do not need provider-specific string parsing.

Permission events use stable correlation fields:

- `permission_id`: stable id shared by `permission.requested` and `permission.resolved`;
- `summary`: short audit text;
- `inputs`: safe requested-operation fields;
- `outputs`: decision fields such as `decision` and `option_id`;
- `metadata.frontend_visible=false` by default;
- `metadata.audit_visible=true` by default.

Lifecycle status vocabulary:

```text
pending
running
completed
failed
```

Frontend consumers should use `kind`, `tool_call_id`, `permission_id`, `sequence`, and visibility metadata instead of parsing provider-specific `session/update` payloads or natural-language logs.

Tool enrichment is intentionally layered:

- provider structured tool fields are used first;
- provider metadata is used when it carries safe fields such as `path`, `file`, `command`, or `operation`;
- metadata-only ACP tool updates are still emitted; Matrix does not require a text chunk when `title`, `toolCallId`, `kind`, `status`, `rawInput`, or locations identify a tool action;
- ACP client-handled tools such as `fs/read_text_file`, `fs/write_text_file`, and `terminal/create` are also projected into neutral tool events when Matrix executes them on behalf of the agent;
- a pending tool call can be enriched from the correlated permission request for the same active tool window;
- file artifact references are emitted as `file://...` only for completed edit/delete/move-style operations with a safe absolute path;
- Matrix avoids arbitrary natural-language inference when no structured source exists.

Native protocol payloads are preserved only as evidence in `protocol_meta`. ACP updates are stored under `protocol_meta.acp` when available. A2A or future providers should follow the same pattern, for example `protocol_meta.a2a`, while keeping the Matrix event kind and fields provider-neutral. Frontends that need ACP-native projection can build it from Matrix neutral events plus `protocol_meta.acp`; generic channel adapters should consume the neutral contract.

## Privacy

Default policy:

```text
refs first, inline only by explicit policy
```

Trace consumers should receive stable references, digests, event metadata, routing facts, timestamps, status, and artifact links. Raw prompts, files, tool output, or private channel data are not emitted inline unless the caller explicitly requests and is allowed to use `inline` mode.

Supported content modes:

```text
refs      = references and digests only
redacted  = redacted placeholders or summaries
inline    = raw content allowed by explicit policy
```

Frontend projection contract:

- `content_mode=inline` guarantees the final agent answer is directly renderable from `agent.message.final.message`.
- `content_mode=inline` also exposes the final answer in `outcome.summary` for clients that render a single terminal message.
- `outcome.summary_ref` and event `content_ref` remain present for audit and replay, but frontend consumers do not need to dereference `matrix://...` refs to display the final answer.
- `include_protocol_meta=false` strips protocol/debug metadata from trace event projections while preserving protocol-neutral event kinds and renderable agent content.
- Matrix technical events such as `run.started`, `routing.decision`, and `session.resumed` remain part of the trace; frontend facades can filter them without losing final content.

## Run Actions

Run actions are operational controls.

Mandatory action:

```json
{
  "action": "cancel",
  "reason": "consumer_policy"
}
```

`attach_context` is accepted only for active runs with a known logical and
remote session. Matrix returns a typed `unsupported` response when live context
cannot be delivered safely. Matrix records `late` when delivery returns only
after the original run has already reached a terminal status. Optional future
actions such as `annotate` or
`set_mode` are only acceptable if they remain protocol-neutral operational
controls. Matrix must not turn run actions into cognitive policy APIs.

For live delivery, the run's `logical_session_id + remote_session_id` is the
operational SSOT. The vault mirror can lag while an active provider turn is
still streaming and only persist the latest remote id at turn completion. Matrix
therefore routes `attach_context` to the run-bound remote session, not to a
possibly stale channel mirror. If the provider cannot use that exact remote
session, Matrix reports typed unsupported/failed evidence instead of recovering
silently into a replacement session.

`cancel` and `attach_context` are intentionally different. ACP standardizes
`session/cancel` for interrupting/stopping an ongoing turn; it does not
standardize injecting a second prompt into a running turn and requiring the LLM
to consume it before final output. Matrix therefore records live context as a
best-effort provider capability, not as a protocol guarantee. See
[matrix_live_context_interrupt_policy.md](matrix_live_context_interrupt_policy.md).

## Event Sinks

`POST /v1/event-sinks` registers generic external run-event consumers.

Expected consumers include:

- supervisory agents;
- Noema-like cognition compilers;
- observability systems;
- audit collectors;
- UI timelines;
- compliance or benchmark systems.

Sinks consume Matrix events and trace references, not Noema objects.

## Implementation Notes

The initial Matrix implementation introduces `internal/logic/runtrace` as the internal store and projection module.

Supporting libraries:

- `internal/logic/frontendevents`: provider-neutral normalization for frontend/audit tool and permission fields;
- `internal/logic/runnotifier`: bridge from live `ThoughtNotifier` updates into run trace events;
- `internal/logic/memstore`: reusable in-memory `middleware.Storage` for tests and embedded servers.

The HTTP server persists run records and ordered run events to Matrix vault storage when the daemon wires `WithTraceStorage`. Tests and embedded servers fall back to an in-memory storage implementation.

Current surfaces:

- `POST /v1/runs` creates `run_id`, records start/routing/prompt/final/completion events, and returns trace/event/action URLs.
- `POST /v1/runs` can force isolated evaluation turns with `session_policy=new_ephemeral_delete_after_run` and `cleanup_policy=delete_remote_or_cancel_and_forget_local`.
- `POST /v1/runs` is also the canonical provider preflight surface: send a
  minimal prompt through the target `agent_id` and inspect typed `code/details`
  before launching a large external batch.
- `GET /v1/runs/{run_id}/trace` returns `matrix.agent_communication_run_trace.v0`.
- `GET /v1/runs/{run_id}/events` returns ordered run events.
- `POST /v1/runs/{run_id}/actions` supports `cancel`, `attach_context`, and `append_context`.
- `emergency_kill_seconds` is the only wall-clock kill path and is disabled by default.
- `POST /v1/event-sinks` persists generic sink registration and queues HTTP delivery for matching run events through the persistent delivery outbox.
- non-interactive `/v1/runs` never emits first-run wizard prompts as agent output; when `system.configured` is false or missing, sync callers receive HTTP `409` with `code=SETUP_REQUIRED`.

Captured event sources:

- Matrix run lifecycle: `run.started`, `run.completed`, `run.failed`, `run.cancelled`.
- Matrix routing: `routing.decision`.
- Matrix prompt dispatch: `agent.prompt.sent`.
- Provider streaming updates through `ThoughtNotifier`: `agent.message.delta`.
- Provider tool updates through `ThoughtNotifier`: `tool.call.requested`, `tool.result.received`.
- Session status enrichment after route completion: `session.created` or `session.resumed`.
- Session isolation and cleanup: `session.policy.applied` and `session.cleanup`.

Run enrichment:

- selected protocol is resolved through the configured agent endpoint resolver when available;
- remote session ids are captured from notifier headers when providers emit them;
- logical session, remote session, protocol, workspace, mode, and status are copied from the channel-neutral session status surface when available;
- cleanup evidence records logical session id, remote session id, remote delete attempt/result, remote cancel fallback, workspace-bound agent client/process reap or allowed retention, local mirror removal, `clean`, optional `warnings`, optional `failure_code`, and cleanup errors;
- trace events keep protocol-specific details in `protocol_meta`, never as primary schema concepts.
- ACP `session/update` payloads are preserved in `protocol_meta.acp` for lossless inspection when `include_protocol_meta=true`;
- permission payloads can enrich the currently active tool event with path/operation, but permission events remain hidden from frontend timelines by default.

Event reads:

- `GET /v1/runs/{run_id}/events` supports cursor reads with `after=<event_id>`;
- responses include `next_cursor`;
- clients can request server-sent events with `?stream=sse` or `Accept: text/event-stream`;
- SSE emits `id`, `event`, and JSON `data` records and closes when the run reaches a terminal state.

Event sink delivery:

- sinks can subscribe to specific event kinds or `*`;
- sinks must register an absolute `http` or `https` URL;
- delivery is asynchronous and backed by a persistent delivery outbox;
- each delivery is a JSON `POST` containing the stored Matrix run event;
- failed deliveries do not block the run;
- failed deliveries are retried with backoff;
- deliveries move to a dead state after the configured max attempts;
- a daemon worker scans due deliveries after restart, so transient sink outages do not lose queued events.

Redaction enforcement:

- `refs` and `redacted` modes suppress event `message` fields in trace projections;
- `redacted` mode also strips frontend tool summaries, structured inputs/outputs, artifact refs, and event metadata;
- an omitted trace policy defaults to `content_mode=refs`, `redaction_profile=default`, and `include_protocol_meta=true`;
- `include_protocol_meta=false` strips protocol metadata from exported trace events;
- `inline` mode keeps final agent content in `agent.message.final.message` and `outcome.summary`;
- `inline` plus `redaction_profile=frontend` keeps safe tool fields such as `tool_name`, `tool_kind`, `summary`, lifecycle `status`, `inputs`, and `outputs`;
- raw prompt/tool/file content is not stored inline by the run trace path.

Headless setup behavior:

- interactive channels may still enter the first-run wizard when Matrix is not configured;
- `/v1/runs` is treated as non-interactive machine ingress and returns a structured setup error instead of wizard text;
- the structured response uses HTTP `409` and `code=SETUP_REQUIRED`;
- `matrix bootstrap doctor` exposes `system_configured` so installers and sidecars can block traffic until setup is complete;
- headless deployments must provision at least one active agent and complete setup before routing production `/v1/runs` traffic.

Concurrency and durability:

- event appends are serialized inside the run trace store before updating the event index, so concurrent provider deltas and lifecycle events cannot drop index references in a single daemon;
- sink delivery enqueueing is durable before delivery is attempted;
- sink retries are daemon-local operational work and remain safe to resume after restart because pending deliveries stay in vault storage.

The design target remains the full contract above: sync, async, stream, live event observation, run control, trace projection, redaction policy, and event sinks as one neutral Matrix capability.

## Real Validation

The run trace surface has been validated against a real ACP-backed agent through Matrix, using `opencode` as the provider endpoint.

Validated paths:

- sync run through `POST /v1/runs`, with final answer persisted in the run outcome;
- stream run through `execution_mode=stream`, with NDJSON start/completion frames;
- async run through `execution_mode=async`, with live `GET /v1/runs/{run_id}/events?stream=sse`;
- ordered event reads with `limit` and `after` cursors;
- redacted trace projection with `content_mode=redacted` and `include_protocol_meta=false`;
- default trace projection with omitted policy, confirming `include_protocol_meta=true`;
- run cancellation through `POST /v1/runs/{run_id}/actions`;
- HTTP event sink registration and delivery of lifecycle, routing, prompt, delta, session, final, completion, and cancellation events.

Observed real agent outputs:

- `MATRIX_RUNTRACE_OK` for sync run validation;
- `MATRIX_STREAM_OK` for stream run validation;
- `MATRIX_SSE_OK` for async SSE validation;
- `MATRIX_REDACTED_OK` for redaction validation;
- `MATRIX_UPDATED_OK` for post-patch binary validation.

Operational notes:

- a fresh Matrix daemon still routes interactive channel traffic through onboarding until `system.configured=true`;
- non-interactive `/v1/runs` traffic fails with `SETUP_REQUIRED` until setup is complete, so external frontends never receive wizard prompts as fake agent answers;
- after onboarding, provider selection remains channel-neutral and resolves through the Matrix agent catalog/session layer;
- real provider latency depends on the external agent, so run surfaces must be treated as asynchronous operational surfaces even when the caller chooses `sync`;
- no absolute run timeout is applied by default; emergency wall-clock termination is opt-in through `emergency_kill_seconds`.
