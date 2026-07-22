# ACP and A2A Protocol Coverage

Last verified: 2026-07-22.

This document is the Matrix source of truth for protocol feature coverage. The
runtime capability report is the live source of truth for a specific configured
provider. ZERO-LEGACY rules in
[`governance/zero_legacy_governance.md`](governance/zero_legacy_governance.md)
apply to every adapter.

## Upstream snapshot

- ACP v1 stable schema release `schema-v1.20.0`, protocol release `v1.4.0`,
  official repository commit `5e89c71497fe07dd4ae633c181a17224f4a8956d`.
- A2A specification `v1.0.1`, official repository commit
  `af112d9491c1fd4b2a568ac65755af4a62790490`.
- Matrix A2A SDK: `github.com/a2aproject/a2a-go/v2 v2.3.1`.

Authoritative sources:

- https://agentclientprotocol.com/protocol/overview
- https://agentclientprotocol.com/protocol/v1/initialization
- https://agentclientprotocol.com/protocol/v1/transports
- https://a2a-protocol.org/latest/specification/
- https://github.com/a2aproject/A2A

## ACP v1 stable coverage

Matrix is the ACP client. Optional agent operations are capability-gated; a
method existing in the SDK never causes Matrix to advertise provider support.

| Direction | Stable surface | Matrix coverage |
| --- | --- | --- |
| Client to agent | `initialize`, `authenticate`, `session/new`, `session/load`, `session/set_mode`, `session/set_config_option`, `session/prompt`, `session/cancel`, `session/list`, `session/delete`, `session/resume`, `session/close`, `logout` | Complete; optional methods are gated by the initialization response |
| Agent to client | `session/request_permission`, `session/update`, `fs/read_text_file`, `fs/write_text_file`, `terminal/create`, `terminal/output`, `terminal/release`, `terminal/wait_for_exit`, `terminal/kill` | Complete; filesystem and terminal support are advertised only when the host backend exists |
| Protocol | `$/cancel_request` | Typed and available; prompt cancellation continues to use the semantic `session/cancel` operation |
| Content | text, resource link, image, audio, embedded resource | Complete; baseline text/resource-link always allowed, optional blocks rejected unless advertised |
| MCP session configuration | stdio, HTTP, SSE | Complete; HTTP/SSE are rejected unless the agent advertises the matching MCP capability |
| Session configuration | select groups and boolean options | Complete; Matrix advertises stable boolean config-option support |
| Transport | stdio plus Matrix remote websocket/unix adapters | Complete for configured Matrix ACP endpoints; upstream Streamable HTTP remains draft and is not advertised |
| Authentication | agent-owned method ID and capability-gated logout | Complete through the neutral authentication control; retired credential payloads are rejected by construction |

ACP message updates preserve optional stable `messageId` and exact chunk text.
Matrix projects structured message-phase metadata into neutral
`progress`/`final` classifications without text heuristics. Providers that do
not expose final-phase evidence retain append-only ACP fallback semantics and
remain explicitly `unclassified`.

`session/fork` remains a named draft operation. It is available only through
the explicit fork action and only when the provider advertises it; it is not a
stable ACP baseline or an implicit fallback.

ACP draft/unstable provider selection, model selection, NES/document,
elicitation, MCP-over-ACP, and Streamable HTTP are not advertised as stable
Matrix capabilities. Adding one requires a separately named non-default
experimental surface or promotion in the upstream stable schema.

## A2A v1.0.1 coverage

Matrix implements both an outbound A2A client and an inbound A2A agent server.

| Stable RPC | Client | Server |
| --- | --- | --- |
| `SendMessage` | Complete | Complete |
| `SendStreamingMessage` | Complete; used only when the agent card advertises streaming, otherwise Matrix uses `SendMessage` without probing | Complete, including submitted/working/progress/artifact/completed events |
| `GetTask` | Complete | Complete |
| `ListTasks` | Complete, including all-page traversal | Complete, authenticated task ownership and pagination |
| `CancelTask` | Complete | Complete |
| `SubscribeToTask` | Complete through neutral task subscription | Complete through the SDK streaming handler |
| `CreateTaskPushConfig` | Complete through neutral push control | Opt-in; requires a governed callback store and sender |
| `GetTaskPushConfig` | Complete | Opt-in with push configuration |
| `ListTaskPushConfigs` | Complete | Opt-in with push configuration |
| `DeleteTaskPushConfig` | Complete | Opt-in with push configuration |
| `GetExtendedAgentCard` | Complete and preserved losslessly | Opt-in with an authenticated extended card |

Additional stable A2A surface:

- JSON-RPC and HTTP+JSON bindings are implemented by both discovery and server
  advertisement. gRPC is a valid optional A2A binding but Matrix does not
  advertise it because there is no governed gRPC ingress listener.
- Agent-card discovery negotiates the selected interface, protocol version,
  tenant, transport, headers, skills, security requirements, and extended card.
- Direct endpoints preserve the optional A2A tenant in the Vault and CLI.
- Text, raw file, URL file, and structured data parts are projected losslessly
  into neutral Matrix content, including artifact metadata.
- Message extension URIs and referenced task IDs survive neutral routing.
- Task states, status messages, artifacts, history identity, context IDs, task
  IDs, final markers, and progressive events remain distinct; intermediate
  working/thought updates never contaminate final output.
- Push callbacks are disabled by default. Enabling them requires an outbound
  URL-validation and delivery policy so protocol coverage cannot silently
  create an SSRF surface.

## Runtime verification

`AgentCapabilities` reports separate `session`, `operations`, `content`, and
`transports` maps. Callers must use the report and explicit optional interfaces;
they must not probe support by sending speculative protocol traffic.

Required checks for a protocol change:

```bash
go run ./scripts/governance_check --manifest governance/manifest.toml
go test ./pkg/zedacp ./internal/providers/agents ./internal/providers/a2aclient ./internal/providers/a2a ./internal/providers/sidecarprojection
go test -race ./...
```
