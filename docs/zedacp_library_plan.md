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

## Compliance Work Still Missing

### session/list

Protocol value:

- enumerate remote ACP sessions

Matrix impact:

- optional runtime introspection
- better diagnostics
- low direct impact on onboarding/channels

### session/load

Protocol value:

- re-attach to an existing ACP session after reconnect or restart

Matrix impact:

- high value for daemon restart recovery
- session vault becomes a real mirror of ACP session state rather than only a local cache
- touches router recovery and startup warm state

### session/cancel

Protocol value:

- interrupt an in-flight ACP turn

Matrix impact:

- requires a cancellable turn model in session queues
- affects channels, CLI commands, and `/v1/runs`

### session_info_update

Protocol value:

- real-time session metadata updates from the agent

Matrix impact:

- vault should mirror session titles/status/metadata
- improves `/session list`, aliases, history, and observability

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

1. keep extracting Matrix to consume `pkg/zedacp` only through adapters
2. implement `session/load`
3. mirror ACP session metadata into vault
4. implement `session/list`
5. implement `session_info_update`
6. add `session/cancel`
7. evaluate Streamable HTTP

This order maximizes value to Matrix while also making the ACP package more standalone.
