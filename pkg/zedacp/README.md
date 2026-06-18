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
- typed current draft surfaces for `$/cancel_request`, `session/set_model`, provider configuration, and `logout`
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
- `SetSessionModelRequest`
- `SetSessionModelResponse`
- `ListProvidersRequest`
- `ListProvidersResponse`
- `SetProvidersRequest`
- `DisableProvidersRequest`
- `LogoutRequest`
- `CancelRequestNotification`
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

- Reviewed against the official ACP docs and latest release `schema-v1.14.0` on 2026-06-18.
- `session/list`, `session/load`, stable `session/resume`, stable `session/close`, and stable `session/delete` are first-class and used by Matrix for protocol-transparent session lifecycle actions.
- `session/cancel` is emitted as an ACP notification, matching current Zed ACP semantics
- `session/close` is implemented as an optional stable method; callers should gate it on advertised `sessionCapabilities.close`
- `session_info_update` is accepted and surfaced so Matrix can mirror remote session metadata in the vault
- `session/resume` is implemented as an optional stable method; Matrix prefers it when advertised and falls back to `session/load`
- `session/set_config_option` is implemented as a stable request and decodes the complete returned `configOptions` state; Matrix prefers config options over legacy `modes`
- `session/fork` is implemented as an optional draft method; callers should gate it on advertised `sessionCapabilities.fork`
- `session/delete` is implemented as an optional stable method; callers should gate it on advertised `sessionCapabilities.delete`
- `additionalDirectories` is modeled on current new/load/resume/fork requests and session info; `session/list` request intentionally does not carry it. Callers must gate actual usage on advertised `sessionCapabilities.additionalDirectories`
- `authenticate` uses the current request shape (`methodId` only) when no legacy credentials are supplied; auth env-var descriptors default `secret` to true and preserve unknown model-state drafts through raw response metadata
- `ExtRequest` and `ExtNotification` expose ACP extension methods without coupling Matrix to one vendor extension; unknown incoming methods should return JSON-RPC `-32601` unless an extension handler is explicitly installed
- ACP has no official `side` or `session/side` primitive; Matrix `sidecar` context must be projected through ordinary prompt/meta channels, provider-specific live extensions, or true `session/fork`
- generic `$/cancel_request`, provider configuration, and `session/set_model` have typed package calls but remain unstable/draft product surfaces; stable `logout` is typed but not exposed as a Matrix runtime action yet. NES/document events, elicitation, and full `usage_update` control remain tracked gaps. Stable Matrix behavior must remain capability-gated
- ACP tool updates surface official `kind`, `status`, `toolCallId`, `rawInput`, `rawOutput`, `locations`, and `content` variants (`content`, `diff`, `terminal`) so higher layers can build structural tool traces without string parsing
- ACP `plan`, `agent_thought_chunk`, `available_commands_update`, `current_mode_update`, `config_option_update`, `session_info_update`, and `usage_update` are decoded into typed update fields while preserving raw protocol metadata for audit
- ACP terminal requests follow the official lifecycle: `terminal/create` returns a `terminalId`, `terminal/output` reports retained output and exit status, `terminal/wait_for_exit` blocks for completion, and `terminal/kill` / `terminal/release` manage process lifetime
- ACP filesystem reads support official absolute `path`, 1-based `line`, and line `limit` fields
- ACP prompt content supports text plus `resource_link`, `resource`, `image`, and `audio` content blocks; callers must still respect the agent's advertised `promptCapabilities`
- `session/load` and `session/prompt` keep their observer registered through a short post-response idle drain so providers that emit trailing `session/update` notifications immediately after the JSON-RPC response do not lose the final text or metadata chunk
- capability flags accept the Zed object-style shape (`"close": {}`) and the older boolean shape used by some adapters
- Matrix-specific ingress such as `/v1/runs` is intentionally outside this package
