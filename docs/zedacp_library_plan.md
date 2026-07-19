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

Last reviewed against ACP stable Schema v1.19.0 on 2026-07-19.

The normative operation/content/transport matrix is maintained in
[`protocol_coverage.md`](protocol_coverage.md). `pkg/zedacp` owns the stable wire
types and JSON-RPC client; `internal/providers/agents` owns capability checks and
neutral projection. Middleware contains no ACP SDK imports.

Stable authentication accepts a method ID only, initialization reads
`agentCapabilities` only, and optional session/content/MCP operations are
capability-gated before traffic is sent. Filesystem and terminal callbacks are
advertised only when their Matrix host backends exist.

`session/fork` is the sole explicitly named draft opt-in. Other draft surfaces,
including provider/model selection, NES/document events, elicitation,
MCP-over-ACP, and Streamable HTTP, are not advertised by the production adapter.
ACP has no stable `side` primitive; Matrix sidecars remain protocol-neutral.

## Remaining package work

There is no known stable ACP coverage gap in the maintained table. Future work
is packaging work, not a compatibility layer:

1. keep the protocol facade isolated from Matrix host/runtime policy;
2. run schema drift checks against the pinned upstream stable release;
3. evaluate a third-party Go SDK only through the atomic replacement gate in
   [`acp_sdk_evaluation.md`](acp_sdk_evaluation.md); and
4. promote a draft feature only after upstream stable promotion or behind a
   separately named, non-default experimental interface.

## Vault mirror direction

Matrix stores a local mirror for routing, recovery, diagnostics, and cleanup
proof. Remote protocol operations remain authoritative for remote lifecycle.
The mirror records the logical session, protocol kind, remote session/task ID,
metadata, and synchronization status; it must never emulate an unsupported
remote operation.
