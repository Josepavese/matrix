# Matrix Zed ACP Protocol Tracking

Last checked: 2026-07-19.

The combined ACP/A2A feature matrix now lives in
[`protocol_coverage.md`](protocol_coverage.md). This file records ACP-specific
drift decisions.

## Source snapshot

- Stable ACP v1 schema: `schema-v1.19.0`.
- Protocol release: `v1.4.0`.
- Verified official repository commit:
  `b06dc4b4e1c2dd9ab388847111bfee0a016782bd`.
- Overview: https://agentclientprotocol.com/protocol/overview
- Initialization: https://agentclientprotocol.com/protocol/v1/initialization
- Transports: https://agentclientprotocol.com/protocol/v1/transports

## Stable result

All stable agent, client, and protocol methods are represented by the Matrix
ACP package and adapter. Optional lifecycle, prompt-content, MCP, logout,
filesystem, and terminal operations are capability-gated. Matrix now reports
the complete adapter surface through neutral `operations`, `content`,
`transports`, and `session` capability maps.

ACP authentication emits the stable `{methodId}` request only. The retired
credentials payload and the retired top-level `capabilities` initialization
field fail closed. Stable auth methods are agent-owned; draft env-var and
terminal auth descriptors are not exposed through the stable neutral auth
control.

Text and resource links are baseline prompt content. Image, audio, and embedded
resource content are rejected before `session/new` unless the provider
advertises the corresponding prompt capability. HTTP and SSE MCP servers are
rejected before session creation unless the matching MCP capability exists.

## Explicit experimental surface

`session/fork` remains draft. Matrix exposes it only through the named fork
action, marks it `draft` in capability reports, and requires an advertised
provider capability. It is never used as a fallback for stable lifecycle
operations.

Provider configuration, model selection, NES/document integration,
elicitation, MCP-over-ACP, and Streamable HTTP remain draft or unstable. Matrix
does not advertise them as stable runtime coverage. Promotion requires an
upstream stable contract or an explicitly named non-default experimental
surface under ZERO-LEGACY governance.

## Drift procedure

Compare the official stable `schema.json` and `meta.json` method sets with:

- `pkg/zedacp.ClientAPI` and `pkg/zedacp.Client`;
- `internal/providers/agents/default_handler.go`;
- `internal/providers/agents/acp_protocol_capabilities.go`;
- package wire tests and adapter capability-gating tests.

Any upstream replacement is a one-way migration: remove the superseded wire
shape, runtime fallback, test fixture, and active documentation in the same
change.
