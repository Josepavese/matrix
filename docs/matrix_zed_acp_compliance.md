# Matrix Zed ACP Compliance Notes

Last reviewed: 2026-05-04.

Matrix follows the Zed Agent Client Protocol documented at
https://agentclientprotocol.com and the official SDK references.

Current source anchors:

- Protocol docs: https://agentclientprotocol.com/protocol/overview
- Stable schema release v0.12.2: https://github.com/agentclientprotocol/agent-client-protocol/releases/tag/v0.12.2
- Schema reference: https://agentclientprotocol.com/protocol/schema
- Session config options: https://agentclientprotocol.com/protocol/session-config-options
- Terminals: https://agentclientprotocol.com/protocol/terminals
- File system: https://agentclientprotocol.com/protocol/file-system
- Official SDK index: https://agentclientprotocol.com/libraries/typescript
- Session fork RFD: https://agentclientprotocol.com/rfds/session-fork
- Additional directories RFD: https://agentclientprotocol.com/rfds/additional-directories
- Request cancellation RFD: https://agentclientprotocol.com/rfds/request-cancellation
- Next Edit Suggestions RFD: https://agentclientprotocol.com/rfds/next-edit-suggestions
- Elicitation RFD: https://agentclientprotocol.com/rfds/elicitation

## Matrix Position

ACP is Matrix's operational default for real coding agents today. A2A remains a
strategic protocol target, but current day-to-day coding-agent support is better
through ACP.

Matrix must stay protocol-neutral at the product boundary:

- channels and users do not choose ACP wire methods directly;
- providers advertise capabilities;
- Matrix maps those capabilities into neutral actions;
- unsupported ACP surfaces return typed unsupported results instead of being
  simulated.

## No `side` Primitive

ACP does not define `side`, `session/side`, or a hidden side-session primitive.

Matrix terms:

- `sidecar`: Matrix-owned protocol-neutral auxiliary context.
- `session/fork`: ACP branch primitive for a separate provider session.
- `attach_context`: Matrix live-context action that requires provider-specific
  proof and is not guaranteed by baseline ACP.

Mapping:

- sidecar prompt context -> ACP `session/prompt` visible content plus `_meta`
  correlation;
- sidecar branch/artifact work -> ACP `session/fork` when advertised;
- mid-turn live context -> provider-specific extension, cancel/restart, or
  next-turn context; not baseline ACP.

## Implemented ACP Surface

`pkg/zedacp` and the Matrix ACP adapter currently cover:

- `initialize`
- `authenticate`
- `session/new`
- `session/load`
- `session/list`
- `session/resume`
- `session/prompt`
- `session/cancel`
- `session/close`
- `session/set_config_option`
- `session/fork`
- `session/delete`
- `session_info_update`
- `config_option_update`
- `plan`
- `available_commands_update`
- `current_mode_update`
- client filesystem requests, including `line` and `limit` on `fs/read_text_file`
- client terminal lifecycle requests: `terminal/create`, `terminal/output`,
  `terminal/wait_for_exit`, `terminal/kill`, and `terminal/release`

Matrix prefers stable session config options over legacy `modes`. If a provider
returns `configOptions`, Matrix selects through `session/set_config_option` and
uses `modes` only as a fallback for transition-period agents.

## Tracked Latest Schema Deltas

Matrix models the current unstable/draft fields needed for forward compatibility:

- `additionalDirectories` on new/load/fork/list requests and session info;
- `messageId` on prompt requests;
- `userMessageId`, `usage`, and `_meta` on prompt responses;
- `nextCursor` and request filters on `session/list`.

Usage rules:

- `additionalDirectories` must be sent only when the provider advertises
  `sessionCapabilities.additionalDirectories`.
- `messageId` is optional and should be generated only when Matrix needs
  explicit user-message correlation.
- generic `$/cancel_request` is tracked but must not replace `session/cancel`
  for prompt-turn semantics until ACP stabilizes that transition.

Stable lifecycle deltas confirmed on 2026-05-04:

- `session/list` is stable and remains capability-gated by
  `sessionCapabilities.list`.
- `session/resume` is stable and capability-gated by
  `sessionCapabilities.resume`; Matrix prefers it before `session/load`.
- `session/close` is stable and capability-gated by
  `sessionCapabilities.close`.
- `session_info_update` is stable through `session/update`.
- `session/set_config_option` and `config_option_update` are stable. The
  response/update carries the full `configOptions` state.

## Future Surfaces

Matrix should treat these as optional, capability-gated integrations:

- provider configuration;
- logout;
- NES/document events;
- elicitation;
- Streamable HTTP.
- `session/set_model`;
- `usage_update`.

None of these replaces Matrix sidecar capsules or channel-neutral session
actions.

## Test Expectations

Any ACP compliance change must include at least one of:

- package-level wire/schema test in `pkg/zedacp`;
- adapter-level capability/projection test in `internal/providers/agents`;
- real-provider smoke with at least three available providers when the change
  affects runtime behavior.

Real-provider smoke command:

```bash
MATRIX_SMOKE_TEST=1 \
MATRIX_REAL_ACP_PROVIDERS='opencode=opencode acp --pure;codex=codex-acp;gemini=gemini --acp --yolo' \
go test ./tests/integration -run TestSmoke_RealACPProviderLifecycleCompliance -v -count=1 -timeout 20m
```

Latest recorded evidence:

- 2026-05-04: OpenCode `1.4.1`, `@zed-industries/codex-acp 0.13.0`
  over `@openai/codex 0.128.0`, and Gemini CLI `0.40.1` all completed real
  ACP initialize/new/prompt flows and returned provider-specific LLM proof
  tokens from temporary workspaces.
- OpenCode advertised `list`, `resume`, and draft `fork`; `session/list` and
  `session/resume` succeeded in the real probe.
- Codex advertised `list`, `close`, and `loadSession`; `session/list` returned
  an empty persisted-session set for the temporary workspace and prompt
  processing succeeded after upgrading `@zed-industries/codex-acp` from
  `0.11.1` to `0.13.0`.
- Gemini advertised `loadSession`; for a fresh temporary workspace it returned
  the provider error "No previous sessions found", then completed prompt
  processing and requested ACP `session/request_permission`.
- OpenCode and Codex did not call Matrix client-side `fs/*` or `terminal/*`
  requests in the probe; they used provider-native tool calls and emitted
  structural `tool_call` updates. Gemini requested ACP permission and emitted
  tool updates. Matrix must therefore preserve structural updates and avoid
  assuming every real provider uses client request methods for tool execution.
