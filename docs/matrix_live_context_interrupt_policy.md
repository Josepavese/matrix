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

## Engineering Rule

Provider capability names should stay explicit:

- `supports_cancel`: provider can receive a protocol cancel/stop request;
- `supports_live_context_interrupt`: provider has been proven to consume
  Matrix live context before the current run completes;
- `supports_next_turn_context`: provider can receive the same context on a
  later turn.

Do not infer `supports_live_context_interrupt=true` from ACP compatibility
alone.
