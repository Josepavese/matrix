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

Timeout and recovery rules are tracked in [matrix_timeout_recovery_policy.md](/home/jose/hpdev/Libraries/matrix/docs/matrix_timeout_recovery_policy.md).

Every response includes:

```json
{
  "run_id": "run-...",
  "status": "running|completed|failed|cancelled",
  "output": "optional sync output",
  "trace_url": "/v1/runs/run-.../trace",
  "events_url": "/v1/runs/run-.../events",
  "actions_url": "/v1/runs/run-.../actions"
}
```

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

Provider-specific details belong in `protocol_meta`.

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

## Run Actions

Run actions are operational controls.

Mandatory action:

```json
{
  "action": "cancel",
  "reason": "consumer_policy"
}
```

Optional actions such as `annotate`, `attach_context`, or `set_mode` are only acceptable if they remain protocol-neutral operational controls. Matrix must not turn run actions into cognitive policy APIs.

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

The HTTP server persists run records and ordered run events to Matrix vault storage when the daemon wires `WithTraceStorage`. Tests and embedded servers fall back to an in-memory storage implementation.

Current surfaces:

- `POST /v1/runs` creates `run_id`, records start/routing/prompt/final/completion events, and returns trace/event/action URLs.
- `GET /v1/runs/{run_id}/trace` returns `matrix.agent_communication_run_trace.v0`.
- `GET /v1/runs/{run_id}/events` returns ordered run events.
- `POST /v1/runs/{run_id}/actions` supports `cancel`.
- `emergency_kill_seconds` is the only wall-clock kill path and is disabled by default.
- `POST /v1/event-sinks` persists generic sink registration and queues HTTP delivery for matching run events through the persistent delivery outbox.

Captured event sources:

- Matrix run lifecycle: `run.started`, `run.completed`, `run.failed`, `run.cancelled`.
- Matrix routing: `routing.decision`.
- Matrix prompt dispatch: `agent.prompt.sent`.
- Provider streaming updates through `ThoughtNotifier`: `agent.message.delta`.
- Provider tool updates through `ThoughtNotifier`: `tool.call.requested`, `tool.result.received`.
- Session status enrichment after route completion: `session.created` or `session.resumed`.

Run enrichment:

- selected protocol is resolved through the configured agent endpoint resolver when available;
- remote session ids are captured from notifier headers when providers emit them;
- logical session, remote session, protocol, workspace, mode, and status are copied from the channel-neutral session status surface when available;
- trace events keep protocol-specific details in `protocol_meta`, never as primary schema concepts.

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
- `redacted` mode also strips tool names and event metadata;
- an omitted trace policy defaults to `content_mode=refs`, `redaction_profile=default`, and `include_protocol_meta=true`;
- `include_protocol_meta=false` strips protocol metadata from exported trace events;
- raw prompt/tool/file content is not stored inline by the run trace path.

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

- a fresh Matrix daemon still routes the first interaction through onboarding until `system.configured=true`;
- after onboarding, provider selection remains channel-neutral and resolves through the Matrix agent catalog/session layer;
- real provider latency depends on the external agent, so run surfaces must be treated as asynchronous operational surfaces even when the caller chooses `sync`;
- no absolute run timeout is applied by default; emergency wall-clock termination is opt-in through `emergency_kill_seconds`.
