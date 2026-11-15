# Matrix Zed ACP Protocol Tracking

Last checked: 2026-11-15.

The combined ACP/A2A feature matrix now lives in
[`protocol_coverage.md`](protocol_coverage.md). This file records ACP-specific
drift decisions.

## Source snapshot

- Stable ACP v1 schema: `schema-v1.32.0` (2026-11-14).
- Protocol release: `v1.18.0` (the Rust crate version is separate from the wire
  protocol version, which remains 1).
- Verified stable source commit:
  `3d911e0c126ecc71e5872e38212e4a7345f36aed`.
- Draft ACP v2 schema: `schema-v2.0.0-alpha.16`, source commit
  `6921fb746d7a9108eb2ded9acbb586bb7083a11e`. Matrix keeps its explicit v2
  experimental surface and does not chase draft-only additions as stable.
  Version 2 remains a qualified preview against released peers, not a stable
  promotion.
- Overview: https://agentclientprotocol.com/protocol/overview
- Initialization: https://agentclientprotocol.com/protocol/v1/initialization
- Transports: https://agentclientprotocol.com/protocol/v1/transports


## Version 2 adoption state (2026-11-15)

Matrix negotiates version 2 by default; it does not assume it. On a conforming
peer the live evidence is:

- Codex ACP 2.6.0: initialize, `session/new`, prompt acknowledgement with a
  terminal `state_update`, `session/resume`, `session/close`, and the advertised
  system-prompt append path.
- Claude Agent ACP 0.107.0: initialize and `session/new`; the prompt path is
  limited by an expired local OAuth session, not by the adapter.
- Kimi CLI 1.3: declares version 2 but sends the version 1 payload, so strict
  conformance rejects the handshake. A per-agent version 1 pin is the supported
  rollback and the doctor then reports the negotiated generation.

Version 2 operations and content are reported with `stability: draft` on a
version 2 connection; version 1 remains `stability: stable`. The negotiated
generation is carried as `protocol_version` in the capability report and as
`provider_protocol_version` in the doctor handshake result. The opt-in real-peer
test is `tests/integration/acp_v2_commercial_real_test.go`; the mock peers keep
covering cancellation, terminal authentication and synthetic edge cases.

Pin and clear an agent with:

```bash
matrix agent protocol-version <agent-id> 1
matrix agent protocol-version <agent-id> --clear
```

## Stable result

Stable drift from `schema-v1.26.0` to `schema-v1.32.0` is small: the session
lifecycle requests gained a client-owned `systemPrompt` field (stabilized in
`schema-v1.30.0`), and the schema gained descriptions and serialization hints.
No stable method was added or removed. Matrix now models and delivers the
system prompt when an operator configures one and the agent advertises
`sessionCapabilities.systemPrompt`; append is the default mode and override is
sent only when the agent advertises it. The remaining schema deltas were
unstable-only additions (archive/unarchive, status, set_title, proxy, tool-call
and terminal cancellation, background work, prompt suggestions) and are not
advertised as stable coverage.

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

Current notice/compaction, v2 patch/lifecycle and request-validation evidence is
recorded in [the protocol coverage ledger](protocol_coverage.md). Schema release
versions are distinct from the stable wire protocol version, which remains 1.
