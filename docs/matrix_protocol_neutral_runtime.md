# Matrix Protocol-Neutral Runtime

## Goal

Matrix now separates:

- the **conversation core** from any specific agent protocol
- the **agent protocol adapters** from the daemon/session logic
- the **discovery layer** from both protocol execution and catalog shape
- the **channel runtime** from any specific messaging provider

This document defines the strategic boundary used by the codebase.

## Architectural Split

### 1. Protocol-Neutral Core

The neutral contract lives in `internal/middleware/protocol.go`.

Core concepts:

- `ProtocolEndpoint`: how an agent is reached
- `ConversationTurn`: one logical user turn
- `ConversationResult`: one logical agent result
- `ConversationClient`: protocol-specific adapter hidden behind a stable contract
- `ConversationSessionControl`: optional protocol-neutral session lifecycle control
- `ConversationFactory`: creator for ACP, A2A, and future protocols

The rest of Matrix should reason in terms of:

- logical session
- remote session token
- text input/output
- tool calls
- provider session/task inventory when available
- host capabilities (fs/process/trust mode)

It should **not** reason directly in terms of:

- ACP `initialize/session/new/session/prompt`
- A2A `SendMessage/Task/Event`
- transport framing details

### 2. Protocol Adapters

Current adapters:

- `internal/providers/agents/acp_adapter.go`
- `internal/providers/a2aclient/adapter.go`

Responsibilities of adapters:

- translate neutral turns into protocol-specific requests
- manage protocol-specific session state
- expose provider-native session/task lifecycle when available
- expose streaming updates as neutral thought updates when possible
- translate protocol-specific outputs back into `ConversationResult`

Protocol-specific details must stop at this layer.

### 3. Channel-Neutral Runtime

The neutral channel model lives in `internal/middleware/link.go`.

Current runtime bootstrap:

- `internal/logic/channelruntime/runtime.go`

Current provider:

- Telegram via `internal/providers/telegram`

Telegram is no longer started directly by `cmd/matrix/run.go`; it is created through the channel runtime registry like any other gateway.

### 4. Discovery-Neutral Layer

The discovery model now lives in `internal/logic/agentdiscovery`.
The onboarding-facing aggregation and activation boundary lives in `internal/logic/agentcatalog`.

Supported discovery sources:

- `local`: agents already registered in SSOT
- `acp_registry`: ACP public registry/catalog
- `a2a_card`: direct A2A discovery via Agent Card URL or base URL
- `a2a_catalog`: pluggable catalog provider for A2A-style directories

This is an intentional split:

- **protocol** decides how Matrix talks to the agent at runtime
- **discovery** decides how Matrix finds metadata/endpoints
- **catalog** is just one possible discovery backend

For A2A, Matrix now treats the Agent Card as the standard discovery artifact and catalogs as optional provider-specific indexes.

### 5. Onboarding-Neutral Selection

First-run onboarding no longer depends structurally on the ACP Registry.

The wizard now consumes:

- a discovery interface that aggregates `local`, `acp_registry`, and optional `a2a_catalog` sources
- an activation interface that decides how a selected entry becomes available locally

This means channel-driven onboarding, including Telegram, now follows a source policy instead of an ACP-specific code path.

Current activation rules:

- `local`: already available, no activation needed
- `acp_registry`: install through the ACP installer
- `a2a_catalog` or `a2a_card`: register a remote endpoint in SSOT

## Persistence Model

Agent configuration now distinguishes:

- `kind`: protocol family, currently `acp` or `a2a`
- `transport`: protocol binding or process transport

Protocol selection is therefore **SSOT-driven**:

1. an agent entry is loaded from `agent.config.<agent_id>` in the vault
2. `internal/logic/agentcfg/normalize.go` maps stored fields into `ProtocolEndpoint`
3. the router selects the adapter from `ProtocolEndpoint.Kind`

In other words, Matrix does not guess ACP vs A2A from traffic. It resolves the protocol from SSOT.

### Operational commands

- `matrix agent show <id>`: inspect effective config and normalized endpoint
- `matrix agent set-binary <id> <path> --kind acp --transport stdio`
- `matrix agent set-endpoint <id> <url> --kind a2a --transport JSONRPC`
- `matrix install <id>`: ACP Registry install flow
- `matrix install <id> --a2a-url <url>`: register a remote A2A endpoint in SSOT
- `matrix install <id> --a2a-card-url <base-or-card-url>`: discover endpoint from A2A Agent Card, then persist it in SSOT
- `matrix agent search --source local|acp_registry|a2a_catalog`
- `matrix agent info <ref> --source acp_registry|local|a2a_card|a2a_catalog`

Normalization logic lives in `internal/logic/agentcfg/normalize.go`.

## Inbound Surface

Matrix exposes:

- Matrix HTTP run bridge v1 on `/v1/runs`
- Matrix HTTP session actions v1 on `/v1/session-actions`
- Matrix HTTP workspace state v1 on `/v1/workspace-state`
- Matrix HTTP workspace timeline v1 on `/v1/workspace-timeline`
- Matrix HTTP workspace decisions v1 on `/v1/workspace-decisions`
- Matrix HTTP workspace memory v1 on `/v1/workspace-memory`
- Matrix HTTP workspace snapshots v1 on `/v1/workspace-snapshots`
- Matrix HTTP intents v1 on `/v1/intents`
- Matrix HTTP modes v1 on `/v1/modes`
- Matrix HTTP orchestration profile v1 on `/v1/orchestration-capabilities`
- A2A JSON-RPC API on `/a2a`
- A2A Agent Card on `/.well-known/agent-card.json`

Important distinction:

- outbound ACP support is Zed ACP over JSON-RPC transports such as `stdio`, `ws`, and `unix`
- `/v1/runs` is the canonical Matrix ingress API that routes into the session manager; it is not the ACP wire protocol defined by Zed

### Current Ingress Contract

`POST /v1/runs` accepts a Matrix envelope:

- `channel_id`: physical ingress identity or routing key
- `input`: latest user message
- `sidecar_capsules`: optional protocol-neutral context capsules kept separate from the task body, projected into ACP/A2A, and traced as `sidecar.capsule.delivered`
- `agent_id`: optional requested agent for new sessions
- `workspace_id` or `workspace_path`: optional work context
- `session_policy`: optional lifecycle policy. `new_ephemeral_delete_after_run` forces a fresh random logical session and schedules cleanup after the turn
- `cleanup_policy`: optional cleanup policy for ephemeral run lifecycles. Supported values are `delete_remote_or_cancel_and_forget_local`, `delete_remote_or_forget_local`, `delete_remote`, and `forget_local`; by itself it does not clean up a normal active session
- provider readiness failures are typed in sync/stream run responses with `code`
  and `details`. `provider_model_unavailable`, `provider_auth_mismatch`, and
  `agent_preflight_failed` identify adapter/provider problems and should not be
  scored as task failures by external evaluators.
- cancelled/deadline turns must not poison cached provider clients. For local
  stdio ACP, Matrix evicts the exact workspace client after cancellable prompt
  failures and preserves remote-session tombstone proof for cleanup; immediate
  follow-up requests create a fresh provider client.
- lane preflight is a normal `/v1/runs` call with a minimal prompt plus
  `session_policy=new_ephemeral_delete_after_run`, so it exercises the same
  channel-neutral Matrix path and still produces cleanup evidence.

`POST /v1/session-actions` accepts a typed action envelope:

- `channel_id`: physical ingress identity or routing key
- `action`: currently `cancel`, `delete`, `cleanup`, `switch`, `list`, `status`, `new`, `name`, `capabilities`, `fork`, `fork_status`, or `reconcile`
- `target`: optional action operand
- `workspace_id` or `workspace_path`: optional binding for new sessions
- `ephemeral`: optional flag for temporary sessions
- `cleanup_policy`: optional lifecycle cleanup policy
- `force_forget_local`: optional local mirror removal override for cleanup
- `make_active`: optional fork flag; defaults to `true` for plain fork handles and `false` when `input` is supplied
- `restore_parent`: optional fork flag to restore the previous/parent active session after child work
- `async`: optional fork flag; backgrounds the child artifact turn and returns a pollable `fork.job_id`
- `input`: optional one-turn prompt for fork-child artifact workflows

Current target semantics:

- `cancel`, `delete`, `cleanup`, `switch`: local or remote session selector
- `new`: requested agent id
- `name`: alias for the active logical session
- `capabilities`: optional agent id; unknown agents return typed `agent_not_found`
- `fork`: parent local or remote session selector; true provider fork only, never prompt replay
- `fork_status`: async fork job id
- `reconcile`: no target required

Behavior:

- if first-run is not completed, interactive channel requests are intercepted by the onboarding wizard
- if first-run is not completed, non-interactive HTTP `/v1/runs` requests fail with HTTP `409` and `code=SETUP_REQUIRED`
- once configured, the request is routed through the session manager
- the session manager resolves or creates the logical session for `channel_id`
- the active session agent wins over `agent_id` after the session exists
- slash commands such as `/session`, `/help`, `/wizard`, and `/action` are handled before agent routing
- chat slash commands are parsed by exact first token, so `/session-list` cannot accidentally trigger `/session`
- `/session list` shows the local vault mirror and, when supported by the current provider, the remote session/task inventory
- `/session switch <target>` can reattach to local history or import a remote ACP/A2A session/task into the local mirror
- `/session cancel [target]` cancels the active or selected remote session/task while preserving the local mirror
- `/cancel` and `/stop` are UX aliases for `/session cancel`
- `/session delete [target]` removes the local mirror and calls the closest remote lifecycle operation available
- `cleanup` produces explicit proof fields: `clean`, `remote_deleted`, `remote_canceled`, `process_reaped`, `process_retained`, `process_retention_allowed`, `local_forgotten`, `remote_delete_unsupported`, optional `warnings`, and optional `failure_code`
- `delete` and `cleanup` failures return typed JSON with `error.code` plus the `cleanup` proof whenever Matrix has cleanup state to report; lifecycle phase codes include `remote_delete`, `remote_close`, `remote_cancel`, `local_forget`, `local_status`, `process_reap`, and `process_reap_refs`
- `/session new [agent]`, `/session name <alias>`, and `/session status` are exposed by the same typed session-action core used by HTTP and future channel adapters

Defaulting:

- if `agent_id` is omitted on `/v1/runs`, Matrix uses the configured default agent
- if A2A metadata omits `agent_id`, Matrix also falls back to the configured default agent

Response model:

- `/v1/runs` always materializes a `run_id` and returns `trace_url`, `events_url`, and `actions_url`
- `/v1/runs` supports `sync`, `async`, and `stream` execution modes under one Matrix envelope
- `/v1/runs` has no default absolute turn timeout; callers may opt into an emergency wall-clock fuse with `emergency_kill_seconds`
- `/v1/runs` can also opt into `activity_timeout_seconds`, an idle-progress watchdog that cancels with `stop_reason=activity_timeout` when no agent/tool activity is observed for the configured duration
- synchronous `/v1/runs` responses include `output` when the run completes inline
- isolated `/v1/runs` success and error responses may include `cleanup`; traces record `session.policy.applied` and `session.cleanup`. A `session.cleanup` event with `status=failed` and `clean=false` is explicit evidence that provider/process cleanup was incomplete. When `session_policy=new_ephemeral_delete_after_run` creates a policy-owned session, routing is explicitly bound to that prepared logical session even if fork, judge, or sidecar workflows change the channel's active session before the run finishes. If cancellation races with provider startup and Matrix observes a different late-selected remote session, cleanup targets that selected logical/remote session so the proof does not lose the real provider target.
- sidecar capsule traces expose `sidecar_provider`, `sidecar_id`, `sidecar_schema`, `sidecar_version`, `sidecar_carrier`, and `sidecar_visibility` as top-level event fields so redaction can hide raw content without losing audit evidence
- synchronous `/v1/runs` returns structured HTTP `409` `SETUP_REQUIRED` instead of wizard text when `system.configured=false`
- provisioned headless installs can mark setup complete with `matrix vault set system.configured true`
- `GET /v1/runs/{run_id}/trace` returns `matrix.agent_communication_run_trace.v0`
- `GET /v1/runs/{run_id}/events` returns ordered run events
- tool and permission events expose provider-neutral frontend fields such as `sequence`, `tool_call_id`, `permission_id`, `tool_name`, `tool_kind`, `summary`, `inputs`, `outputs`, `artifact_refs`, and visibility metadata
- ACP tool events are accepted even when the provider sends only structured metadata and no text content. Matrix also emits neutral tool events for ACP client-handled `fs/read_text_file`, `fs/write_text_file`, and `terminal/create` calls, so coding runs expose read/edit/execute pressure without requiring consumers to parse agent prose.
- `POST /v1/runs/{run_id}/actions` exposes operational run controls such as `cancel` and live sidecar context attachment through `attach_context` / `append_context`
- `POST /v1/event-sinks` registers generic run-event consumers
- `/v1/session-actions` returns a synchronous typed JSON object describing the session action result
- `/v1/workspace-state`, `/v1/workspace-timeline`, `/v1/workspace-decisions`, `/v1/workspace-memory`, and `/v1/workspace-snapshots` return synchronous typed read models
- the same typed action surface is shared by the session manager for chat-style channels and HTTP callers
- `/a2a` returns A2A JSON-RPC events/messages as defined by the A2A SDK

Auth and callbacks:

- `/v1/runs` can be protected with `X-Matrix-Key`
- `/v1/auth/openrouter/callback` is the versioned auxiliary HTTP callback endpoint used by the onboarding/auth flow, not a general ingress surface

Readiness:

- `matrix bootstrap doctor` reports `system_configured`, active agents, and setup guidance before traffic is sent;
- `matrix agent doctor <id>` reports effective protocol endpoint data and probes ACP stdio commands with a safe `--help` invocation;
- ACP stdio probe fields are `command_probe_ok`, `command_probe_exit_code`, and `command_probe_error`;
- a failed ACP stdio probe means Matrix can see a path but the command is not runtime-ready, for example because the binary is corrupt or the configured subcommand is wrong.

Versioning policy:

- new clients should target `/v1/runs`
- new clients should target `/v1/session-actions` for typed session lifecycle operations instead of synthesizing slash-commands over `/v1/runs`
- future breaking envelope changes should introduce `/v2/...` rather than mutate the `v1` contract

## Session Mirror Model

Matrix stores session state in the vault as the local source of truth for channels, while also treating it as a mirror of remote provider state.

Current mirror fields include:

- logical session id
- agent id
- remote session/task token
- protocol kind
- mirror status
- remote title
- remote updated timestamp
- last synchronized timestamp

Current behavior:

- ACP remote sessions are enumerated through `session/list`, resumed through stable `session/resume` when advertised, and fall back to `session/load` when needed
- ACP remote sessions can be closed when the provider advertises stable `sessionCapabilities.close`; Matrix uses this before `session/cancel` when `session/delete` is unavailable
- ACP remote sessions can also be interrupted through `session/cancel`, which Matrix sends as a JSON-RPC notification
- ACP lifecycle support is reported through a protocol-neutral capability model with `supported`, `status`, `stability`, and `source` for `list`, `info_update`, `load`, `cancel`, `close`, `delete`, `resume`, and `fork`
- ACP session configuration prefers stable `configOptions` plus `session/set_config_option`; legacy `modes` are fallback only
- Fork capability descriptors also expose Matrix orchestration truth: `active_parent_safe`, `requires_idle_parent`, `artifact_turn`, `async_supported`, `blocking`, `artifact_streaming`, and `live_intervention_suitable`
- ACP `session/fork` is wired only as a Draft capability-gated operation; Matrix returns typed unsupported results unless the provider advertises it
- A2A remote tasks are enumerated through `ListTasks`, imported through `GetTask`, and deleted through `CancelTask`
- channel users do not select ACP or A2A explicitly; Matrix resolves the provider from SSOT and the active session

As of 2026-06-18, the current Zed ACP source of truth is the official
`agentclientprotocol.com` protocol docs plus the `agentclientprotocol` schema
release `Schema v1.13.7`, published 2026-06-16.
`session/list`, `session_info_update`, `session/resume`, `session/close`, and
`session/set_config_option` are stable lifecycle/configuration operations with
capability checks where applicable.
Matrix iterates `session/list` pagination through `nextCursor` and can propagate
configured MCP servers into ACP session setup/resume/fork calls.
ACP prompt projection supports additional content blocks supplied by channel or
HTTP ingress (`resource_link`, `resource`, `image`, `audio`) while channel
adapters remain responsible for provider capability checks.
ACP runtime updates are not collapsed into text-only streams: plan changes,
thought chunks, usage updates, session config/info updates, commands, tool
calls, diffs, and terminal references are projected into protocol-neutral run
events while raw ACP payloads stay in protocol metadata.
`session/fork`, `session/delete`, `additionalDirectories`, message ids,
provider configuration, logout, NES/document events, elicitation,
`session/set_model`, `usage_update`, and generic `$/cancel_request` remain
RFD/unstable/draft surfaces unless the official docs move them to
completed/stable. Matrix records this lifecycle state instead of collapsing it
into booleans; `additionalDirectories` is currently propagated only when the
agent advertises `sessionCapabilities.additionalDirectories`.
Unknown ACP agent-to-client methods are not treated as success. Matrix returns
JSON-RPC `-32601 Method not found` unless an explicit extension handler has
been registered for that method.

There is no ACP primitive named `side`, `session/side`, or `side session` in
the official docs or schema. Matrix uses `sidecar` as a protocol-neutral
product concept for auxiliary context. When Matrix needs a separate provider
conversation that does not pollute parent history, the ACP projection is the
real capability-gated `session/fork` method. When Matrix needs mid-turn live
context, ACP baseline still does not standardize that as `side`; Matrix must
use measured provider-specific live attach, cancel/restart, next-turn context,
or async fork artifact workflows.

Cleanup is also capability-aware. For ephemeral interrupt/resume flows, `clean=true` requires at least one strong provider or process proof: `remote_deleted`, `remote_closed`, `remote_canceled`, `process_reaped`, or `process_absent` when no remote session was ever materialized. Local-only forgetting is reported as failed or weak evidence, not as strong cleanup. `process_absent=true` is only strong when paired with an empty `remote_session_id` and `process_absence_reason=no matching cached agent client`; if Matrix knows a remote session id, absence of a cached process is diagnostic data, not proof that the remote session is gone. If the target remote session is deleted, closed, or canceled and a different local session still owns the same workspace provider client, Matrix reports that owner as non-retained `related_sessions[]` evidence with reason `shared_agent_client_owner`; the target cleanup remains strong and does not expose `process_retained=true`. Non-ephemeral retained clients without strong target proof can still be operationally clean, but carry `cleanup_strength=retained` and `weak_cleanup_reason=process_retained`. For local stdio ACP providers, Matrix targets only the exact reusable workspace client for remote lifecycle cleanup; it does not start a fresh provider process just to cancel a session owned by a reaped process, and records typed cleanup `warnings` when process proof satisfies the lifecycle. Provider clients are router-lifetime resources rather than request-lifetime resources, so canceling one `/v1/runs` request does not cancel the cached ACP process. If keepalive evicts a dead workspace client before cleanup runs, if remote lifecycle lookup observes a dead exact workspace client, if cleanup reaps a shared client for one remote session, or if a later request replaces that dead client, Matrix keeps a short-lived tombstone keyed by agent, workspace, and tracked remote sessions; cleanup can consume it as `process_reaped=true` only for matching remote sessions. A workspace-only reap cannot consume an explicit remote-session tombstone. Cleanup/delete is fork-aware: child sessions forked from the target are cleaned first, and the parent cleanup proof includes `fork_children_cleaned` plus nested `fork_children` cleanup records. Ephemeral `/v1/runs` cleanup also records `related_sessions`: run-created same-agent sessions in the run channel are cleaned as supplemental targets even when they were not explicitly flagged ephemeral, while pre-existing or shared related sessions fail the run cleanup with `clean=false`, `failure_code=run_related_session_retained`, `process_retained=true`, and reason `run_related_session_retained`. Run-internal snapshots use local-only session lists, not `status`, so cleanup accounting never creates local-only ghost sessions and never spawns provider discovery clients. After ephemeral cleanup, Matrix runs client reconciliation; unreferenced provider clients in the run agent/workspace scope are recorded as `related_sessions` with reason `run_unreferenced_agent_client_reaped`, retained clients outside that scope do not fail the run proof, and retained in-scope clients fail cleanup with full logical/remote/workspace ownership details.

Supervisor-facing cleanup must fail closed when `process_retained=true`.
Matrix may use retained cleanup internally for ordinary shared-session
operation, but HTTP/session-action consumers and `/v1/runs` cleanup proofs must
not treat retained process state as production-safe. A non-retained
`shared_agent_client_owner` entry is acceptable evidence only when the target
remote session has strong provider proof and the other owner is explicit. A standalone fork-child
cleanup that still depends on its parent agent client returns `clean=false`,
`cleanup_strength=failed`, and `failure_code=run_related_session_retained`
unless Matrix can convert it to strong proof. For fork subtrees, a forced
standalone child cleanup is converted to strong proof when the child remote
session is deleted, closed, or canceled and the child mirror is forgotten. Local
stdio ACP providers may also prove the same child lifecycle by reporting
`remote_lifecycle_skipped_no_reusable_cached_agent_client`: there is no reusable
child client left to cancel, and Matrix must not spawn a fresh stdio provider
only to synthesize a cancel. Matrix records the shared parent owner as a
non-retained related session instead of treating the live parent process as
child retention. The parent owner remains responsible for final cleanup;
destructive fallback cleanup of the parent is still restricted to run-owned
ephemeral parent subtrees.

Fork child cleanup never reaps the workspace agent client directly because ACP
fork children share the parent workspace client. When child remote cleanup has
strong provider proof (`remote_deleted`, `remote_closed`, `remote_canceled`, or
`remote_lifecycle_skipped_no_reusable_cached_agent_client`) and the child mirror
is forgotten, Matrix reports the child cleanup as `strong_cleanup=true` and
adds a non-retained `related_sessions` entry with reason
`fork_parent_agent_client_owner` for the parent session that owns the shared
client. During parent subtree cleanup, if child remote proof is already
unavailable because the shared parent client is gone, the parent
`process_reaped=true` proof is projected into the child cleanup record and the
child is still reported as strong. If neither child provider/quiescence proof
nor parent process proof exists, Matrix fails closed with
`failure_code=run_related_session_retained` instead of returning ambiguous
retained cleanup.

Channels and HTTP can request `action=capabilities`, `action=fork`, `action=fork_status`, and `action=reconcile` through the same session-action contract. HTTP uses `/v1/session-actions`; text channels use `/session capabilities`, `/session fork`, `/session fork-status`, and `/session reconcile`. `reconcile` closes cached provider clients that no longer have a Matrix vault session reference.

Fork is safe for automation when callers set `make_active=false`. Matrix mirrors
the child, keeps or restores the parent as active, and returns
`fork.parent_restored=true` when the channel active session is preserved. If the
logical parent exists but has not yet opened a provider session, Matrix first
materializes a real remote parent session through the provider session API. It
does not fake fork by replaying prompt history. If the request includes `input`,
Matrix routes exactly one child turn and returns the raw child response as
`fork.artifact.content`; when `ephemeral=true` or `cleanup_policy` is supplied,
Matrix then cleans the child and returns `fork.cleanup`. Matrix still does not
evaluate or interpret the artifact.

For live sidecar workflows, `fork` can run the child artifact turn
asynchronously by setting `async=true` with `input`. Matrix still performs a
true provider fork before acknowledging the request, but then returns
`fork.job_id` immediately and runs the child turn in the background. Callers poll
`action=fork_status` to retrieve `fork.job.status`, `fork.job.artifact`,
`fork.job.cleanup`, and `fork.job.error`. This fixes the synchronous live
intervention bug where a provider-bound artifact turn blocked the caller until
the active parent run had already completed. `active_parent_safe=true` now means
state safety only; it is not a latency guarantee. Current ACP fork descriptors
therefore set `blocking=true`, `artifact_streaming=false`, and
`live_intervention_suitable=false` while also setting `async_supported=true`.
This is intentionally not described as ACP `side`: the official protocol name is
`session/fork`, while Matrix `sidecar` remains the neutral orchestration layer.

Active-parent fork is supported when `capabilities.session.fork.active_parent_safe=true`.
Parent cleanup is subtree cleanup: Matrix first cleans mirrored fork children,
then cleans the parent, then reaps the shared provider client when no local
session still references the same `agent_id + workspace_path`. Child cleanup
records are finalized after the parent process proof, so the final parent
cleanup never reports fork children as retained when the shared process was
actually reaped. If an async fork job later notices that its child was already
cleaned by parent cleanup, it records `fork_child_cleanup_already_missing`
instead of leaking the process or failing cleanup accounting.

If parent materialization is impossible, Matrix returns typed blocked evidence
instead of a generic server failure. Current codes include
`missing_remote_session_id` and `remote_session_materialize_failed`. If the fork
child turn or child cleanup fails after a provider child has been created, Matrix
returns typed evidence such as `fork_child_turn_failed` or
`fork_child_cleanup_failed`, includes any available `fork.cleanup` proof, and
does not collapse the path into HTTP `500`.

The A2A ingress is implemented with the official Go SDK:

- module: `github.com/a2aproject/a2a-go/v2`

## Market State

Matrix is intentionally ready for both ACP and A2A at the runtime boundary, but the operational state of the market is not symmetric.

### Operational Standard Today

ACP is the current operational standard for real coding agents in this environment.

Available ACP products and adapters include:

- Codex via `codex-acp`
- Gemini CLI via `gemini --acp`
- Claude via `@agentclientprotocol/claude-agent-acp`
- OpenCode via `opencode acp`

The latest three-provider Matrix smoke evidence is recorded below for OpenCode,
Codex ACP, and Gemini CLI.

For day-to-day usage, ACP should be treated as the default production path.

### Strategic Readiness

A2A remains strategically important and is already supported in Matrix at the protocol, routing, discovery, and ingress layers.

However, for the real products currently used with Matrix, A2A support is not yet mature enough to be treated as the default operational standard.

Current state:

- Matrix runtime: A2A-ready
- Matrix discovery: A2A-ready
- Matrix ingress: A2A-ready
- Real market availability across coding agents: still uneven

Therefore A2A should be documented as:

- implemented in the core
- suitable for experimentation and future adoption
- pending broader and more stable market support from vendors and adapters

### Adoption Trigger

Matrix should promote A2A from strategic readiness to operational standard only when at least one of these becomes true:

- major coding agents expose stable native A2A endpoints
- stable vendor-supported A2A adapters become common and well documented
- A2A discovery and deployment patterns become operationally simpler than ACP in real environments

Until then:

- use ACP by default
- keep A2A available without making it the primary recommended path

### Real Provider Lifecycle Probe

The 2026-05-04 lifecycle probe was executed against real ACP providers through
`pkg/zedacp`, temporary workspaces, and real LLM prompts:

- `opencode` 1.4.1 via `opencode acp --pure`: advertised `loadSession`,
  `session/list`, stable `session/resume`, and draft `session/fork`;
  `session/list`, `session/resume`, prompt processing, file-token retrieval, and
  terminal-token retrieval succeeded.
- `codex` via `@zed-industries/codex-acp` 0.13.0 over `@openai/codex` 0.128.0:
  advertised `loadSession`, `session/list`, and stable `session/close`;
  prompt processing succeeded after upgrading `codex-acp` from 0.11.1. For a
  fresh temporary workspace, `session/list` returned zero persisted sessions and
  `session/load` returned resource-not-found, which Matrix treats as provider
  state, not as a protocol simulation opportunity.
- `gemini` 0.40.1 via `gemini --acp --yolo`: advertised `loadSession`, then
  returned "No previous sessions found" for a fresh temporary workspace;
  prompt processing succeeded and Gemini requested ACP `session/request_permission`.

The same probe showed that OpenCode and Codex can satisfy file/terminal tasks
through provider-native tools while emitting structural ACP `tool_call` updates,
without invoking Matrix client-side `fs/*` or `terminal/*` request methods.
Matrix must therefore treat client requests and provider tool updates as two
valid execution evidence channels.

This is the intended contract: Matrix exposes one channel-neutral lifecycle
surface, but every provider action remains capability-gated and evidence-based.

## Design Rules

- Session logic may depend on `middleware.AgentRouter`, not on ACP or A2A SDK types.
- Agent protocol packages may depend on ACP/A2A specifics, but only inside adapters.
- Discovery code may depend on registry formats or Agent Card schemas, but not on the session manager or protocol adapters.
- Channel gateways may depend on provider SDKs, but the daemon boot process must depend only on the channel runtime registry.
- New protocols must be added by implementing `ConversationFactory`, not by branching the session manager.
- New discovery backends must be added by implementing `agentdiscovery.Provider`, not by hardcoding another branch into the CLI.
- New onboarding discovery policies must be expressed by source ordering and activation rules, not by embedding protocol-specific logic in the wizard.
- New channels must be added by implementing a runtime `Factory`, not by editing the daemon startup flow with provider-specific code paths.

## Current Supported Matrix

### Outbound agent protocols

- ACP
- A2A

### Inbound client protocols

- A2A
- Matrix run bridge

### Messaging channels

- Telegram

The runtime is now neutral even if only one messaging gateway is currently bundled.
