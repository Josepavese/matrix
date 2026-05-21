# Matrix Zed ACP Protocol Tracking

Last checked: 2026-05-21.

Purpose: keep a fast comparison point between upstream Zed Agent Client Protocol
development and Matrix's local ACP surface.

## Source Snapshot

Official sources checked:

- ACP overview: https://agentclientprotocol.com/protocol/overview
- Stable schema metadata: https://raw.githubusercontent.com/agentclientprotocol/agent-client-protocol/main/schema/meta.json
- Unstable schema: https://raw.githubusercontent.com/agentclientprotocol/agent-client-protocol/main/schema/schema.unstable.json
- Unstable schema metadata: https://raw.githubusercontent.com/agentclientprotocol/agent-client-protocol/main/schema/meta.unstable.json
- Changelog: https://raw.githubusercontent.com/agentclientprotocol/agent-client-protocol/main/CHANGELOG.md
- Latest release: https://github.com/agentclientprotocol/agent-client-protocol/releases/tag/v0.13.2
- Additional directories RFD: https://agentclientprotocol.com/rfds/additional-directories
- Request cancellation RFD: https://agentclientprotocol.com/rfds/request-cancellation
- MCP-over-ACP RFD: https://agentclientprotocol.com/rfds/mcp-over-acp
- Session delete RFD: https://agentclientprotocol.com/rfds/session-delete
- Configurable providers RFD: https://agentclientprotocol.com/rfds/custom-llm-endpoint
- Logout RFD: https://agentclientprotocol.com/rfds/logout-method
- Authentication methods RFD: https://agentclientprotocol.com/rfds/auth-methods
- Elicitation RFD: https://agentclientprotocol.com/rfds/elicitation
- Next Edit Suggestions RFD: https://agentclientprotocol.com/rfds/next-edit-suggestions
- Zed external-agent docs: https://zed.dev/docs/ai/external-agents
- ACP Registry blog: https://zed.dev/blog/acp-registry

Upstream state:

- ACP wire protocol remains `protocolVersion: 1`.
- Latest GitHub release is `v0.13.2`, published 2026-05-17.
- Latest release delta from the local 2026-05-04 review: unstable
  `additionalDirectories` guidance update, unstable `session/delete`, unstable
  MCP-over-ACP message types, and v2 schema scaffolding.
- Latest `main` commit seen: `4244a28b47aa` on 2026-05-21, CI-only feature
  powerset gating update.
- Zed now documents ACP Registry as the preferred install path for external
  agents starting from `v0.221.x`; Agent Server extensions remain supported but
  are on a deprecation path.

## Local Surface Snapshot

Primary local anchors:

- Protocol-neutral runtime: `docs/matrix_protocol_neutral_runtime.md`
- Existing ACP notes: `docs/matrix_zed_acp_compliance.md`
- ACP package: `pkg/zedacp`
- ACP adapter: `internal/providers/agents`
- A2A client adapter: `internal/providers/a2aclient`
- A2A server endpoint: `internal/providers/a2a`
- Default agent config: `configs/agents.json`

Matrix local design:

- Matrix is protocol-neutral at the channel/session boundary.
- ACP is the most complete coding-agent runtime path today.
- A2A exists as both outbound client adapter and inbound server endpoint.
- ACP and A2A are selected from agent config SSOT; Matrix does not infer the
  protocol from traffic.

## Stable ACP Comparison

Implemented locally:

- Agent-side stable methods: `initialize`, `authenticate`, `session/new`,
  `session/load`, `session/list`, `session/resume`, `session/prompt`,
  `session/cancel`, `session/close`, `session/set_config_option`,
  `session/set_mode`.
- Client-side stable methods: `session/request_permission`,
  `fs/read_text_file`, `fs/write_text_file`, `terminal/create`,
  `terminal/output`, `terminal/wait_for_exit`, `terminal/kill`,
  `terminal/release`, `session/update`.
- Stable update projections include text/thought chunks, tool calls, plans,
  available commands, current mode, config option updates, and session info
  updates.
- Transports implemented locally: stdio, websocket, unix socket.
- Matrix correctly keeps unknown ACP methods behind explicit extension handlers;
  unknown incoming methods return JSON-RPC method-not-found.

Stable gaps:

- No blocking stable method gap found against the current stable `meta.json`.
- Local filesystem handler accepts relative paths by resolving under `cwd`.
  ACP says protocol file paths must be absolute, so this is a tolerant
  compatibility behavior, not a spec feature.

## Unstable/Draft Comparison

Implemented or partially implemented locally:

- `session/fork`: implemented as optional draft, capability-gated.
- `session/delete`: implemented as optional draft, capability-gated.
- `additionalDirectories`: modeled on current lifecycle/session structs,
  accepted by Matrix run/session ingress, normalized to unique absolute paths,
  and propagated by the ACP runtime only when
  `sessionCapabilities.additionalDirectories` is advertised.
- Prompt `messageId`, prompt response `userMessageId`, `usage`, and `_meta`.
- `usage_update` decoded as audit/projection data.
- ACP tool content variants: `content`, `diff`, `terminal`.
- Structured current auth method shapes are decoded in `pkg/zedacp`
  (`env_var.vars[]`, terminal args/env, auth `_meta`). Runtime env-var
  auto-auth now uses `env_var.vars[]`, sends current `authenticate` requests
  without legacy credentials, and falls back to legacy credential payloads for
  older adapters.
- `AuthEnvVar.secret` defaults to `true` when omitted, matching current
  unstable schema metadata.
- Typed unstable client surfaces exist for `$/cancel_request`,
  `providers/list`, `providers/set`, `providers/disable`, `logout`, and
  `session/set_model`.
- Extension request/notification escape hatch.

Open gaps and corrections:

1. MCP-over-ACP.
   Upstream unstable meta now includes `mcp/connect`, `mcp/disconnect`, and
   `mcp/message` client methods plus `mcp/message` agent method and ACP MCP
   server shapes. Matrix currently forwards `mcpServers` into session
   lifecycle requests, but does not implement ACP as an MCP transport channel.

2. Generic request cancellation.
   `pkg/zedacp` can emit `$/cancel_request`, but Matrix runtime still uses
   `session/cancel` for prompt-turn cancellation and Go context cancellation
   internally. Request-id mapping is not wired end-to-end.

3. Provider configuration.
   Upstream draft has `providers/list`, `providers/set`, and
   `providers/disable`, gated by `agentCapabilities.providers`. `pkg/zedacp`
   has typed calls, but Matrix provider routing is still config/env/onboarding
   driven and does not expose this ACP provider configuration surface.

4. Logout.
   Upstream logout RFD is Preview. `pkg/zedacp` has a typed call, but Matrix
   startup auth and local onboarding do not advertise/consume `auth.logout`.

5. Terminal auth.
   `pkg/zedacp` decodes terminal auth method shapes. Matrix does not yet
   advertise `clientCapabilities.auth.terminal` on the autonomous runtime path:
   agent-to-agent execution must not block on an implicit TUI. Human onboarding
   can still present terminal-auth instructions, and a Zed-compatible frontend
   may advertise this only when it can complete the interactive flow explicitly.
   Env-var auth is implemented for current `vars[]` plus legacy compatibility.

6. Elicitation.
   Upstream draft adds `elicitation/create` and `elicitation/complete` for
   transient structured user input. Matrix has no ACP client-side elicitation
   handler yet.

7. NES/document events.
   Upstream draft adds NES methods and document notifications:
   `nes/start`, `nes/suggest`, `nes/accept`, `nes/reject`, `nes/close`,
   `document/didOpen`, `document/didChange`, `document/didSave`,
   `document/didClose`, and `document/didFocus`. Matrix has no editor/NES
   surface today.

8. Model selection.
   Upstream unstable meta includes `session/set_model` and model state shapes.
   `pkg/zedacp` has typed calls and response models; Matrix runtime still
   prefers stable `session/set_config_option`.

9. Streamable HTTP.
   Existing Matrix ACP transports are stdio, websocket, and unix socket. ACP
   transport work beyond those remains an evaluation item.

## Priority Queue

Near term:

- Keep `additionalDirectories` drift checks focused on current schema:
  `session/new`, `session/load`, `session/resume`, `session/fork`, and
  `SessionInfo` carry the field; `session/list` request does not.
- Add request-id bookkeeping if Matrix wants Go context cancellation to emit
  `$/cancel_request`.
- Decide where typed provider configuration/logout belong in the product
  surface: runtime commands, onboarding, or admin API.

Medium term:

- Keep terminal auth policy split by caller mode: automatic Matrix/A2A runtime
  must prefer nonblocking env/config auth, while human Zed-compatible frontends
  may opt into terminal auth only when they own the interaction loop.
- If provider configuration is exposed, gate it on `agentCapabilities.providers`
  and keep Matrix config/env as the authority when the agent does not advertise
  that surface.
- Add protocol-level probes for typed draft calls against real providers once
  providers advertise those capabilities.

Long term:

- Evaluate MCP-over-ACP as a real transport bridge only after Matrix's current
  MCP forwarding model needs bidirectional MCP channels.
- Treat elicitation as a channel UX feature, not only an ACP package type.
- Treat NES/document events as editor-client integrations. They are not a
  replacement for Matrix sidecar capsules or A2A routing.
- Revisit Streamable HTTP when a real provider requires it.

## Fast Recheck Procedure

Use these commands for the next protocol-drift pass:

```bash
curl -fsSL https://raw.githubusercontent.com/agentclientprotocol/agent-client-protocol/main/schema/meta.json
curl -fsSL https://raw.githubusercontent.com/agentclientprotocol/agent-client-protocol/main/schema/meta.unstable.json
curl -fsSL https://raw.githubusercontent.com/agentclientprotocol/agent-client-protocol/main/CHANGELOG.md
curl -fsSL https://api.github.com/repos/agentclientprotocol/agent-client-protocol/releases/latest
curl -fsSL 'https://api.github.com/repos/agentclientprotocol/agent-client-protocol/commits?since=YYYY-MM-DDT00:00:00Z&per_page=30' | jq -r '.[] | [.commit.author.date, (.sha[0:12]), (.commit.message | split("\n")[0])] | @tsv'
```

Then compare:

- stable `agentMethods` and `clientMethods` against `pkg/zedacp.ClientAPI`,
  `pkg/zedacp.Client`, and `internal/providers/agents/default_handler.go`;
- unstable `agentMethods`, `clientMethods`, and `protocolMethods` against this
  tracking file;
- RFD status changes under https://agentclientprotocol.com/rfds;
- Zed external-agent and registry docs for distribution/auth changes.
