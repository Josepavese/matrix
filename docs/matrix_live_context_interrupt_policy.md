# Matrix Live Context Interrupt Policy

Last reviewed: 2026-04-19.

## Decision

Matrix treats live context delivery as a provider capability, not as a baseline
ACP guarantee.

ACP guarantees a cancellation path for an active prompt turn. It does not define
a standard "append this new prompt/context into the currently running turn and
make the model consume it before final answer" operation.

Matrix therefore separates these controls:

- `cancel`: interrupt/stop the active turn through provider protocol support;
- `attach_context`: best-effort live sidecar delivery into the active session;
- `late`: delivery did not complete before the original run became terminal;
- next-turn append/restart: safe fallback for providers that do not consume
  injected context mid-turn.

## Source Review

ACP official docs state that baseline agents support `session/new`,
`session/prompt`, `session/cancel`, and `session/update`.

Reference: https://agentclientprotocol.com/protocol/initialization

ACP prompt-turn docs define cancellation as `session/cancel`: the client may
cancel an ongoing prompt turn, and the agent should stop language model requests
and tool invocations as soon as possible. The same page says another
`session/prompt` continues the conversation after the current prompt turn
completes.

Reference: https://agentclientprotocol.com/protocol/prompt-turn

ACP extensibility allows `_meta` and custom capabilities, but custom extensions
must be explicitly negotiated and cannot be assumed for interoperability.

Reference: https://agentclientprotocol.com/protocol/extensibility

Gemini CLI ACP docs list `prompt` for sending a prompt and `cancel` for
cancelling an ongoing prompt. They do not document a standard mid-turn context
append/interruption operation.

Reference: https://geminicli.com/docs/cli/acp-mode/

Zed `codex-acp` README documents Codex ACP adapter features such as context
mentions, images, tool calls, following, edit review, TODO lists, slash
commands, client MCP servers, and authentication. It does not document a
standard mid-turn live context injection guarantee.

Reference: https://github.com/zed-industries/codex-acp

## Observed Matrix Runtime Results

Real-agent probes were run through Matrix `/v1/runs` and
`/v1/runs/{run_id}/actions` using `scripts/probe_live_attach.sh`.

| Agent | Wire path | Observed live context result | Matrix interpretation |
| --- | --- | --- | --- |
| `opencode` | ACP | delivered before `run.completed`; marker observed in provider output | supports live context interrupt for this probe |
| `codex` | `codex-acp` | request accepted while run active, then `late`; no provider-output marker | does not currently provide reliable mid-turn live context consumption |
| `gemini` | Gemini CLI ACP | request accepted while run active, then `late`; no provider-output marker | does not currently provide reliable mid-turn live context consumption |

`late` is not a provider failure. It means Matrix accepted a live-context
delivery request while the run was active, but the provider did not finish
consuming that context before the run completed.

## Product Semantics

Matrix must never present `accepted` as proof that the target LLM consumed the
context. Proof requires one of:

- `run.context.attached status=delivered` before `run.completed`;
- matching provider output in `run.context.attached.message`,
  `agent.message.delta`, or `agent.message.final` when inline traces are
  explicitly enabled.

If a provider records `late`, clients should choose one of:

- start a new turn with the attached context;
- cancel and restart the run with the new context;
- queue the context as next-turn state.

`accepted` followed by a terminal delivery state for the same `delivery_id` is
expected. `accepted` means Matrix accepted the action and queued the delivery
attempt; `delivered`, `late`, `failed`, or `unsupported` is the final evidence.
For active runs, Matrix treats the run trace's `logical_session_id` and
`remote_session_id` as the delivery SSOT. A channel/session mirror may lag until
the active turn finishes, especially after provider recovery or forked artifact
workflows, so live attach must not reject solely because the mirror still shows
an older remote id.

For cancel-and-restart / interrupt-resume flows, Matrix must expose cleanup
proof before the resume run is trusted. The cancelled run must produce
`session.cleanup clean=true strong_cleanup=true` with at least one remote/process
cleanup proof such as `remote_deleted`, `remote_closed`, `remote_canceled`, or
`process_reaped`. Cleanup runs under a bounded context detached from the
canceled run context. If cleanup cannot complete, `failure_code` gives the
machine-readable class; `cleanup_clean_without_remote_or_process_proof` means
Matrix only forgot local state and therefore refused to claim strong cleanup for
an ephemeral flow. `agent_start_context_cancelled_during_cleanup` identifies the
historical bug where provider cleanup tried to start an agent under an
already-canceled context.

For local stdio ACP providers, Matrix treats the provider process as the owner
of its workspace sessions. Cleanup must target the exact reusable workspace
client; Matrix must not spawn a fresh ACP process just to send `session/cancel`
for a session owned by a reaped process. If process reap already proves the old
session unreachable, cleanup can remain `clean=true strong_cleanup=true` and
carry typed warnings such as
`remote_lifecycle_skipped_no_reusable_cached_agent_client` or
`remote_cancel_session_not_found_after_process_reap`.

For shared non-ephemeral sessions, Matrix may retain a provider client when
other local sessions still reference the same `agent_id + workspace_path`. That
case is not strong cleanup proof; it is reported as
`cleanup_strength=retained`, `process_retained=true`, and
`weak_cleanup_reason=process_retained`. Supervisors can call `/v1/session-actions`
with `action=reconcile` to close cached clients that no longer have vault
references.

## Engineering Rule

Provider capability names should stay explicit:

- `supports_cancel`: provider can receive a protocol cancel/stop request;
- `supports_live_context_interrupt`: provider has been proven to consume
  Matrix live context before the current run completes;
- `supports_next_turn_context`: provider can receive the same context on a
  later turn.

Do not infer `supports_live_context_interrupt=true` from ACP compatibility
alone.
