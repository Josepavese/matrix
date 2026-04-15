# zedacp

`pkg/zedacp` is Matrix's extracted Go implementation of the Zed Agent Client Protocol.

## Scope

The package currently exposes:

- ACP schema types
- a typed client-side ACP connection
- stdio, websocket, and unix transports
- request-handler and session-observer interfaces
- session lifecycle methods including `session/list`, `session/load`, `session/cancel`, and draft `session/delete`

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
- `session_info_update` is accepted and surfaced so Matrix can mirror remote session metadata in the vault
- `session/delete` is implemented as an optional draft method; callers should gate it on advertised `sessionCapabilities.delete`
- Matrix-specific ingress such as `/v1/runs` is intentionally outside this package
