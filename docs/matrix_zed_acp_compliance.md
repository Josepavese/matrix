# Matrix Zed ACP Compliance Notes

Last reviewed: 2026-07-19 against stable ACP v1 schema `schema-v1.19.0`.

Matrix follows the Zed Agent Client Protocol documented at
https://agentclientprotocol.com/protocol/overview. The exhaustive current
method and feature inventory is
[`protocol_coverage.md`](protocol_coverage.md).

## Matrix position

ACP is a first-class coding-agent protocol behind Matrix's protocol-neutral
boundary. Channels never emit ACP methods directly. The adapter reports live
provider capabilities, rejects unsupported optional content before creating a
session, and never simulates an unavailable lifecycle primitive.

The project-wide
[`ZERO-LEGACY`](governance/zero_legacy_governance.md) policy applies to ACP
wire evolution. Matrix accepts `agentCapabilities`, stable `{methodId}`
authentication, and the current stable session contracts. It does not retry a
retired credential payload or read a retired initialization alias.

## Stable coverage

The ACP package and adapter cover every stable v1 method in both directions:

- initialization, agent-owned authentication, and capability-gated logout;
- session new/load/resume/list/delete/close, prompt/cancel, mode and config
  selection;
- session updates and permission requests;
- filesystem reads/writes;
- terminal create/output/wait/kill/release;
- JSON-RPC request cancellation;
- stable text/resource-link prompt content and capability-gated image, audio,
  and embedded resource content;
- stdio MCP configuration plus capability-gated HTTP and SSE configuration;
- paginated session discovery and stable `additionalDirectories` handling.

`session/set_config_option` is preferred when a session returns config options.
`session/set_mode` remains a separate stable ACP operation and is used when a
session exposes modes instead.

`session/update` remains authoritative for streamed output. Matrix keeps the
observer registered through a short post-response idle drain because a provider
may emit the final update immediately after the prompt response.

## Capability rules

- Text and `resource_link` are baseline prompt types.
- Image, audio, and embedded resource blocks fail before `session/new` unless
  their `promptCapabilities` flags are true.
- HTTP/SSE MCP endpoints fail before `session/new` unless their
  `mcpCapabilities` flags are true.
- `additionalDirectories` is sent only when the stable session capability is
  present, and every supplied path must be absolute.
- Filesystem and terminal client methods are advertised only when Matrix has the
  corresponding host backend.
- Unknown incoming methods return JSON-RPC `-32601` unless an explicit Matrix
  extension handler owns them.

## No `side` primitive

ACP does not define `side`, `session/side`, or a hidden side-session method.
Matrix `sidecar` context is projected through ordinary prompt content and
`_meta`. A provider branch uses the explicitly named draft `session/fork`
action only when advertised. Mid-turn context injection remains provider-bound
and cannot be inferred from baseline ACP.

## Experimental boundary

`session/fork` is draft and is reported as such. It is available only through
the explicit fork workflow, never as a fallback.

Provider configuration, model selection, NES/document events, elicitation,
MCP-over-ACP, and Streamable HTTP are draft or unstable. Matrix does not claim
them as stable coverage. Any experiment must be separately named, non-default,
capability-gated, and removable under ZERO-LEGACY governance.

## Verification

Protocol changes require package wire tests, adapter capability/projection
tests, governance, and a real provider smoke when runtime behavior changes:

```bash
go run ./scripts/governance_check --manifest governance/manifest.toml
go test ./pkg/zedacp ./internal/providers/agents
MATRIX_SMOKE_TEST=1 go test ./tests/integration -run TestSmoke_RealACPProviderLifecycleCompliance -v -count=1 -timeout 20m
```
