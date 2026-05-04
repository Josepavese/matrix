# zedacp

`pkg/zedacp` is Matrix's extracted Go implementation of the Zed Agent Client Protocol.

## Scope

The package currently exposes:

- ACP schema types
- a typed client-side ACP connection
- stdio, websocket, and unix transports
- request-handler and session-observer interfaces
- session lifecycle methods including `session/list`, `session/load`, stable `session/resume`, stable `session/close`, `session/cancel`, draft `session/fork`, and draft `session/delete`
- stable session configuration through `configOptions`, `config_option_update`, and `session/set_config_option`
- current unstable schema fields such as lifecycle `additionalDirectories`, prompt `messageId` / `userMessageId`, and usage updates

Matrix consumes this package through adapters in `internal/providers/agents`.

## Design

The package is deliberately shaped like an SDK rather than a Matrix subsystem:

- `Client` owns JSON-RPC multiplexing and typed ACP methods
- transport implementations are independent of Matrix runtime concerns
- request handling is interface-driven
- Matrix-specific filesystem, terminal, trust, vault, and session logic stay outside

This matches the separation used by the official ACP SDKs conceptually:

- protocol/schema layer
- connection/client layer
- transport layer
- host integration layer

Reference SDK docs:

- TypeScript SDK: https://agentclientprotocol.github.io/typescript-sdk/
- Python library: https://agentclientprotocol.com/libraries/python
- Java library: https://agentclientprotocol.com/libraries/java

## Current API

Key exported pieces:

- `Client`
- `ClientAPI`
- `Transport`
- `RequestHandler`
- `SessionObserver`
- `InitializeRequest`
- `InitializeResponse`
- `NewSessionRequest`
- `NewSessionResponse`
- `LoadSessionRequest`
- `LoadSessionResponse`
- `ResumeSessionRequest`
- `ResumeSessionResponse`
- `ForkSessionRequest`
- `ForkSessionResponse`
- `ListSessionsRequest`
- `ListSessionsResponse`
- `SessionInfo`
- `ConfigOption`
- `SetSessionConfigOptionRequest`
- `SetSessionConfigOptionResponse`
- `PromptRequest`
- `PromptResponse`
- `ToolCall`
- `SessionNotification`
- `NewStdioTransport`
- `NewWSTransport`
- `NewUnixTransport`

## Stability

This package is intended to become separable from Matrix.

Near-term goals:

1. keep Matrix consuming `pkg/zedacp` through thin adapters only
2. close the remaining protocol-compliance gaps against current Zed ACP
3. make the package publishable as a standalone repository if needed

## Protocol Notes

- Reviewed against the official ACP docs, schema release `v0.12.2`, and official SDK index on 2026-05-04.
- `session/list`, `session/load`, stable `session/resume`, and stable `session/close` are first-class and used by Matrix for protocol-transparent session lifecycle actions.
- `session/cancel` is emitted as an ACP notification, matching current Zed ACP semantics
- `session/close` is implemented as an optional stable method; callers should gate it on advertised `sessionCapabilities.close`
- `session_info_update` is accepted and surfaced so Matrix can mirror remote session metadata in the vault
- `session/resume` is implemented as an optional stable method; Matrix prefers it when advertised and falls back to `session/load`
- `session/set_config_option` is implemented as a stable request and decodes the complete returned `configOptions` state; Matrix prefers config options over legacy `modes`
- `session/fork` is implemented as an optional draft method; callers should gate it on advertised `sessionCapabilities.fork`
- `session/delete` is implemented as an optional draft method; callers should gate it on advertised `sessionCapabilities.delete`
- `additionalDirectories` is modeled on new/load/fork/list and session info structs, but callers must gate actual usage on advertised `sessionCapabilities.additionalDirectories`
- ACP has no official `side` or `session/side` primitive; Matrix `sidecar` context must be projected through ordinary prompt/meta channels, provider-specific live extensions, or true `session/fork`
- generic `$/cancel_request`, providers configuration/logout, NES/document events, elicitation, `session/set_model`, and `usage_update` are tracked as unstable/draft surfaces; stable Matrix behavior must remain capability-gated
- ACP tool updates surface official `kind`, `status`, `toolCallId`, `rawInput`, and `locations` fields so higher layers can build structural tool traces without string parsing
- ACP terminal requests follow the official lifecycle: `terminal/create` returns a `terminalId`, `terminal/output` reports retained output and exit status, `terminal/wait_for_exit` blocks for completion, and `terminal/kill` / `terminal/release` manage process lifetime
- ACP filesystem reads support official absolute `path`, 1-based `line`, and line `limit` fields
- capability flags accept the Zed object-style shape (`"close": {}`) and the older boolean shape used by some adapters
- Matrix-specific ingress such as `/v1/runs` is intentionally outside this package
