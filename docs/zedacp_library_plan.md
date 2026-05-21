# Zed ACP Library Plan

## Goal

Extract Matrix's Zed ACP implementation into a package that can eventually become:

- a standalone repository
- a drop-in replacement point if an official Go SDK appears
- a thin dependency used by Matrix rather than an internal protocol tangle
- a backend that can be swapped behind Matrix's ACP port without touching channel or session logic

Related evaluation:

- `docs/acp_sdk_evaluation.md`

## Current Split

### Package layer

`pkg/zedacp`

Contains:

- ACP schema types
- typed client-side ACP JSON-RPC methods
- stdio transport
- websocket transport
- unix transport
- request handler and observer interfaces

### Matrix adapter layer

`internal/providers/agents/acp_adapter.go`

Contains:

- conversion between Matrix-neutral turns and ACP turns
- conversion between ACP tool calls and Matrix tool calls
- host capability injection
- session recreation policy for Matrix runtime behavior

### Matrix host integration

`internal/providers/agents/default_handler.go`

Contains:

- trust/permission decisions
- filesystem operations
- terminal operations

This remains Matrix-specific on purpose.

### Matrix ingress

`internal/providers/matrixapi/server.go`

Contains:

- Matrix-owned `/v1/runs` ingress
- versioned auth callback path

This is not part of the ACP library and should stay outside it.

## Why This Shape

This mirrors the separation exposed conceptually by the official ACP SDKs:

- protocol/schema models
- connection/client
- transport bindings
- host/runtime integration

Reference pages:

- TypeScript SDK: https://agentclientprotocol.github.io/typescript-sdk/
- Python library: https://agentclientprotocol.com/libraries/python
- Java library: https://agentclientprotocol.com/libraries/java

## Current Compliance Snapshot

Last reviewed against the official ACP docs and schema release v0.13.2 on
2026-05-21.

Implemented in `pkg/zedacp` and the Matrix ACP adapter:

- `initialize`
- `authenticate`
- `session/new`
- `session/load`
- `session/list`
- stable `session/resume`
- `session/prompt`
- `session/cancel` notification
- stable `session/close`
- stable `session/set_config_option`
- `session/fork`
- `session/delete`
- `session_info_update`
- client-side filesystem and terminal request handling used by Matrix
- stdio, websocket, and unix transports
- object-style capability parsing for current Zed `sessionCapabilities`

Newly tracked unstable/draft schema deltas:

- `additionalDirectories` on new/load/resume/fork request and session info shapes
- `messageId` / `userMessageId` on prompt request/response
- typed `$/cancel_request` as generic JSON-RPC request cancellation
- typed provider configuration, logout, and `session/set_model` package surfaces
- current structured auth method shapes
- NES/document event surfaces
- elicitation surfaces

Important semantic conclusion:

- ACP does not expose `side`, `session/side`, or a side-session lifecycle method.
- Matrix `sidecar` is a Matrix-owned protocol-neutral context concept.
- ACP branch/side work must use capability-gated `session/fork`; live mid-turn context remains provider-specific and cannot be inferred from baseline ACP compatibility.

## Compliance Work Still Open

### additionalDirectories policy

Protocol value:

- declare multi-root workspace scope without changing `cwd`

Matrix impact:

- the package and ACP adapter now model and propagate the field when callers
  provide roots and the provider advertises support
- Matrix still needs product policy for when PAL/workspace roots should be
  forwarded automatically
- usage must stay gated on `sessionCapabilities.additionalDirectories`; the
  field must not be sent on `session/list`

### Generic request cancellation

Protocol value:

- cancel individual JSON-RPC requests through `$/cancel_request`

Matrix impact:

- `pkg/zedacp` can emit the notification; Matrix could map Go
  `context.Context` cancellation to ACP request ids
- should not replace `session/cancel` for prompt-turn semantics until the official protocol makes that transition

### Provider configuration/logout/model/NES/elicitation

Protocol value:

- richer editor-agent integration surfaces

Matrix impact:

- provider configuration, logout, and model selection have typed package calls,
  but are not wired to Matrix runtime UX yet
- useful for future channel UX, but not required for current Matrix production runtime
- must remain optional and capability-gated

### Streamable HTTP

Protocol value:

- support the ACP transport track beyond stdio/custom transports

Matrix impact:

- endpoint normalization
- transport creation
- health checks and doctor/runtime reporting

## Vault Mirror Direction

Target model:

- Matrix vault is not the authority over ACP/A2A session state
- Matrix vault becomes the local mirror used for:
  - list
  - update
  - delete
  - recovery
  - diagnostics

That means:

1. ACP/A2A remote state changes should update the vault mirror
2. Matrix commands should prefer remote protocol operations first when available
3. vault records should explicitly track:
   - local logical session id
   - remote protocol kind
   - remote session/task id
   - mirrored metadata
   - sync status

## Recommended Sequence

1. keep Matrix consuming `pkg/zedacp` only through adapters
2. define product policy for when Matrix should supply `additionalDirectories`
3. evaluate generic `$/cancel_request` below the existing prompt-turn cancellation layer
4. evaluate provider configuration/logout/model selection only when a real agent requires them
5. evaluate NES/document events as editor-style context signals, not as Matrix sidecar replacement
6. evaluate elicitation as a structured user-input surface for channels
7. evaluate Streamable HTTP

This order maximizes value to Matrix while also making the ACP package more standalone.
