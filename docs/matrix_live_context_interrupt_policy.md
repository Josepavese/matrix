# Matrix Live Context Interrupt Policy

Last reviewed: 2026-05-21.

## Decision

Matrix treats live context delivery as a provider capability, not as a baseline
ACP guarantee.

ACP guarantees a cancellation path for an active prompt turn. It does not define
a standard "append this new prompt/context into the currently running turn and
make the model consume it before final answer" operation.

The latest ACP docs and schema release v0.13.2 also do not define a `side`,
`session/side`, or equivalent inline side-channel primitive. The official
branching primitive is draft `session/fork`; it creates a separate session and
does not solve mid-turn live context injection by itself.

ACP `session/update` notifications are session-scoped, not request-scoped. A
client receiving updates for `sessionId=s1` cannot prove which concurrent
`session/prompt` request caused an update unless the agent and client negotiate
a custom `_meta` correlation extension. Matrix therefore forbids concurrent ACP
`session/prompt` calls for the same remote session.

Matrix therefore separates these controls:

- `cancel`: interrupt/stop the active turn through provider protocol support;
- `attach_context`: best-effort live sidecar delivery only when the provider has
  a safe live-interrupt path;
- `unverified`: provider returned without live attach activity proof;
- `terminal_boundary`: provider returned near run completion without live
  attach activity proof;
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

ACP request cancellation RFD proposes `$/cancel_request` for cancelling a
specific JSON-RPC request, but it is still an RFD and does not add mid-turn
context injection.

Reference: https://agentclientprotocol.com/rfds/request-cancellation

Community prior art: `acpx` solves active-session concurrency with per-session
prompt queueing and cooperative `session/cancel`. It does not treat a second
concurrent `session/prompt` as live injection.

Reference: https://github.com/openclaw/acpx

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
`/v1/runs/{run_id}/actions` using `scripts/probe_live_attach.sh`. Earlier
probes that issued a second ACP `session/prompt` on the active session are no
longer considered valid proof, because ACP updates are session-scoped and can be
observed by multiple overlapping request observers.

| Agent | Wire path | Observed live context result | Matrix interpretation |
| --- | --- | --- | --- |
| ACP baseline | `session/prompt` on active session | unsafe/ambiguous | Matrix rejects live attach while a prompt is active |
| `opencode` | ACP | no negotiated Matrix live-injection extension | use cancel/restart or next-turn context |
| `codex` | `codex-acp` | no negotiated Matrix live-injection extension | use cancel/restart or next-turn context |
| `gemini` | Gemini CLI ACP | no negotiated Matrix live-injection extension | use cancel/restart or next-turn context |

`unsupported` for ACP live attach during an active prompt is intentional. It
means Matrix refused to create a false live-intervention claim. The caller must
choose `cancel`, cancel-and-restart, or next-turn context.

## Product Semantics

Matrix must never present `accepted` as proof that the target LLM consumed the
context. Useful live attach proof requires:

- `run.context.attached status=delivered`;
- `live_consumption_proven=true`;
- `delivery_class=live_activity_observed`;
- `provider_activity_events > 0`.

For ACP, those activity events are valid only when Matrix knows there is no
overlapping prompt observer for the same remote session, or when a negotiated
provider extension carries per-request/turn correlation. Baseline ACP exposes no
standard turn id on `session/update`.

`unverified` means the provider returned while the run stayed active, but Matrix
still lacks hard evidence that the LLM used the context. `terminal_boundary`
means the provider returned near completion with no attach activity. Clients
must not count either state as useful live intervention.

If a provider records `late`, clients should choose one of:

- start a new turn with the attached context;
- cancel and restart the run with the new context;
- queue the context as next-turn state.

If a provider records `unsupported` with reason `conversation turn already
active`, clients should make the same fallback choice immediately instead of
waiting for the current turn to finish.

`accepted` followed by a terminal delivery state for the same `delivery_id` is
expected. `accepted` means Matrix accepted the action and queued the delivery
attempt; `delivered`, `unverified`, `terminal_boundary`, `late`, `failed`, or
`unsupported` is the final evidence.
For active runs, Matrix treats the run trace's `logical_session_id` and
`remote_session_id` as the delivery SSOT. A channel/session mirror may lag until
the active turn finishes, especially after provider recovery or forked artifact
workflows, so live attach must not reject solely because the mirror still shows
an older remote id.

For cancel-and-restart / interrupt-resume flows, Matrix must expose cleanup
proof before the resume run is trusted. The cancelled run must produce
`session.cleanup clean=true strong_cleanup=true` with at least one remote/process
cleanup proof such as `remote_deleted`, `remote_closed`, `remote_canceled`, or
`process_reaped`. `process_absent=true` is also valid proof only when no remote
session id was materialized and `process_absence_reason=no matching cached agent
client`; with a known remote session id it is diagnostic data, not remote
lifecycle proof. Cleanup runs under a bounded context detached from the canceled
run context. If cleanup cannot complete, `failure_code` gives the
machine-readable class; `cleanup_clean_without_remote_or_process_proof` means
Matrix only forgot local state and therefore refused to claim strong cleanup for
an ephemeral flow. `agent_start_context_cancelled_during_cleanup` identifies the
historical bug where provider cleanup tried to start an agent under an
already-canceled context.

For local stdio ACP providers, Matrix treats the provider process as the owner
of its workspace sessions. Cleanup must target the exact reusable workspace
client; Matrix must not spawn a fresh ACP process just to send `session/cancel`
for a session owned by a reaped process. Cached provider clients are owned by the
router lifecycle, not by one `/v1/runs` request context; canceling an active run
must not cancel the cached ACP process before cleanup can prove ownership. If a
turn context is cancelled while `session/prompt` is active, Matrix deliberately
closes and evicts the exact workspace client and tombstones the known remote
session id; this converts a potentially poisoned provider client into explicit
process proof and guarantees the next same-agent request starts from a fresh
client. If process reap already proves the old
session unreachable, cleanup can remain `clean=true strong_cleanup=true` and
carry typed warnings such as
`remote_lifecycle_skipped_no_reusable_cached_agent_client` or
`remote_cancel_session_not_found_after_process_reap`. If keepalive evicts the
dead client before cleanup runs, Matrix keeps a short-lived tombstone bound to
the agent, workspace, and tracked remote session ids; cleanup may consume it as
process proof only for the matching target session. Remote lifecycle lookup also
evicts and tombstones a dead exact workspace client before returning
`remote_lifecycle_skipped_no_reusable_cached_agent_client`, so the later process
reap step can still produce strong proof. If no remote session exists yet and
the workspace client is absent, Matrix records process absence instead of
fabricating remote cancel/delete evidence.

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
  later prompt turn;
- `supports_concurrent_prompt`: provider explicitly supports concurrent
  prompt-request correlation for one remote session. Baseline ACP does not.

ACP adapter rule:

- one active `session/prompt` per remote session;
- normal user prompts wait behind the active prompt for the same session;
- `attach_context` never waits behind an active ACP prompt and never sends a
  concurrent prompt; it returns typed `unsupported` so supervisors can apply
  cancel/restart or next-turn fallback.

Do not infer `supports_live_context_interrupt=true` from ACP compatibility
alone.
