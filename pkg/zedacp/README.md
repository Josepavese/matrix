# zedacp

`pkg/zedacp` is Matrix's extracted Go implementation of the Zed Agent Client Protocol.

## Scope

The package currently exposes:

- ACP schema types
- a typed client-side ACP connection
- stdio, websocket, and unix transports
- request-handler and session-observer interfaces
- session lifecycle methods including `session/list`, `session/load`, `session/cancel`, preview `session/close`, draft `session/fork`, and draft `session/delete`

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
- `ForkSessionRequest`
- `ForkSessionResponse`
- `ListSessionsResponse`
- `SessionInfo`
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

- `session/list` and `session/load` are first-class and used by Matrix for protocol-transparent `/session list` and `/session switch`
- `session/cancel` is emitted as an ACP notification, matching current Zed ACP semantics
- `session/close` is implemented as an optional preview method; callers should gate it on advertised `sessionCapabilities.close`
- `session_info_update` is accepted and surfaced so Matrix can mirror remote session metadata in the vault
- `session/resume` is tracked by Matrix as a preview lifecycle capability; `pkg/zedacp` still uses stable `session/load` for current resume behavior
- `session/fork` is implemented as an optional draft method; callers should gate it on advertised `sessionCapabilities.fork`
- `session/delete` is implemented as an optional draft method; callers should gate it on advertised `sessionCapabilities.delete`
- ACP tool updates surface official `kind`, `status`, `toolCallId`, `rawInput`, and `locations` fields so higher layers can build structural tool traces without string parsing
- capability flags accept the Zed object-style shape (`"close": {}`) and the older boolean shape used by some adapters
- Matrix-specific ingress such as `/v1/runs` is intentionally outside this package
