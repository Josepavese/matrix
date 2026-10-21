# ACP and A2A Protocol Coverage

Matrix implementation rechecked: 2026-10-21. Upstream snapshot: 2026-10-21.

This document is the Matrix source of truth for protocol feature coverage. The
runtime capability report is the live source of truth for a specific configured
provider. ZERO-LEGACY rules in
[`governance/zero_legacy_governance.md`](governance/zero_legacy_governance.md)
apply to every adapter.

## Upstream snapshot

- ACP stable wire protocol `1`, JSON schema `schema-v1.26.0` (2026-10-16),
  source commit `e2d83349bc9e28a4059ea3acefa1c91c966f74e2`. Rust crate `v1.12.0`
  is a separate artifact version, not the wire protocol version.
- ACP v2 remains draft: `schema-v2.0.0-alpha.9`, source commit
  `e2d83349bc9e28a4059ea3acefa1c91c966f74e2`. Matrix supports its negotiated
  client surfaces; subprocess fixtures prove interoperability, not commercial
  provider support or editor-complete history rendering.
- A2A specification `v1.0.1`, release commit
  `3303592588e388e62e0f69f701af531d2f4e3991` (2026-05-28).
- Matrix A2A SDK: `github.com/a2aproject/a2a-go/v2 v2.7.0` (2026-10-21).
  Minimum Go version is 1.26; repository qualification uses Go 1.27.1.

Authoritative sources:

- https://agentclientprotocol.com/protocol/overview
- https://agentclientprotocol.com/protocol/v1/initialization
- https://agentclientprotocol.com/protocol/v1/transports
- https://a2a-protocol.org/latest/specification/
- https://github.com/a2aproject/A2A/releases/tag/v1.0.1
- https://github.com/agentclientprotocol/agent-client-protocol/releases/tag/schema-v1.26.0
- https://github.com/agentclientprotocol/agent-client-protocol/releases/tag/schema-v2.0.0-alpha.9
- https://github.com/a2aproject/a2a-go/releases/tag/v2.7.0

## ACP v2 implementation evidence

Matrix tests the negotiated v2 initialize/authenticate/logout surface and the
asynchronous turn lifecycle against real stdio subprocess fixtures in
`tests/integration/acp_v2_*_real_test.go`. These are protocol peers built for
conformance tests, not evidence that a commercial agent supports v2. Terminal
authentication requires the explicit operator capability opt-in. ACP v1 keeps
its own lifecycle. Configure and inspect the actual provider before selecting v2.

## ACP v1 stable coverage

Matrix is the ACP client. Optional agent operations are capability-gated; a
method existing in the SDK never causes Matrix to advertise provider support.

| Direction | Stable surface | Matrix coverage |
| --- | --- | --- |
| Client to agent | `initialize`, `authenticate`, `session/new`, `session/load`, `session/set_mode`, `session/set_config_option`, `session/prompt`, `session/cancel`, `session/list`, `session/delete`, `session/resume`, `session/close`, `logout` | Complete; optional methods are gated by the initialization response |
| Agent to client | `session/request_permission`, `session/update`, `elicitation/create`, `fs/read_text_file`, `fs/write_text_file`, `terminal/create`, `terminal/output`, `terminal/release`, `terminal/wait_for_exit`, `terminal/kill` | Complete; filesystem and terminal support are advertised only when the host backend exists; elicitation is advertised only when a frontend port is wired |
| Protocol | `$/cancel_request` | Typed and available; prompt cancellation continues to use the semantic `session/cancel` operation |
| Content | text, resource link, image, audio, embedded resource | Complete; baseline text/resource-link always allowed, optional blocks rejected unless advertised |
| MCP session configuration | stdio, HTTP, SSE | Complete; HTTP/SSE are rejected unless the agent advertises the matching MCP capability |
| Session configuration | select groups and boolean options | Complete; Matrix advertises stable boolean config-option support |
| Transport | stdio plus Matrix remote websocket/unix adapters | Complete for configured Matrix ACP endpoints; upstream Streamable HTTP remains draft and is not advertised |
| Authentication | agent-owned method ID and capability-gated logout | Complete through the neutral authentication control; retired credential payloads are rejected by construction |

ACP message updates preserve optional stable `messageId` and exact chunk text.
Tool-call updates preserve the optional stable programmatic `name`
(stabilized upstream 2026-09-17) alongside `title`; it is projected into
neutral tool metadata as `tool_name`.
Matrix projects structured message-phase metadata into neutral
`progress`/`final` classifications without text heuristics. Providers that do
not expose final-phase evidence retain append-only ACP fallback semantics and
remain explicitly `unclassified`.

`session/fork` remains a named draft operation. It is available only through
the explicit fork action and only when the provider advertises it; it is not a
stable ACP baseline or an implicit fallback.

Elicitation (`elicitation/create`) stabilized upstream on 2026-07-24.
Matrix implements it as a layered surface: neutral SSOT types and the
`ElicitationFrontend` port live in `internal/middleware`, the in-memory
pending registry in `internal/logic/elicitation`, the ACP wire projection in
the agents adapter, and the HTTP API (`GET`/`POST /v1/elicitations`) as the
v1 channel frontend. `clientCapabilities.elicitation` is advertised exactly
when a frontend port is wired, with the modes the frontend reports; inbound
requests for unadvertised modes get JSON-RPC `-32602` per spec, and a missing
frontend still answers with an explicit decline. Form schemas are restricted
to flat string/number/boolean/enum properties; richer schemas are rejected.
URL mode validates absolute URLs and never prefetches or auto-opens them.
The registry owns request identity (concurrent questions in one session get
unique IDs), every pending entry names the asking agent as the spec requires,
and accepted values are validated against the displayed schema through the
single `middleware.ValidateElicitationValues` implementation. Unanswered
requests are bounded by `agent.elicitation_timeout_seconds`, and cancelling a
run revokes its pending question immediately: the ACP adapter binds each
in-flight turn context per remote session, so the blocked request resolves as
`cancel` and the registry entry disappears instead of lingering until the
timeout. Turn bindings are per session because concurrent sessions share one
client and `beginPrompt` serialises turns only within a session. Two channel frontends consume the same registry through its lifecycle events
(`opened`/`resolved`), so neither owns state: the HTTP surface stays
programmatic (`GET`/`POST /v1/elicitations`), and Telegram renders the question
in the chat that owns the remote session. Chat correlation reuses
`ThoughtNotifier.SetHeader`, which already receives the remote session id at
every turn start, so no new correlation channel was introduced.
The Telegram frontend renders constrained fields (enum, oneOf options,
boolean) as inline buttons with labels, requires an explicit `Invia` step so
the user reviews before sending, always offers decline and cancel, shows the
target host and full URL for URL mode without ever opening it, and consumes the
next chat message as the answer for a single free-text field. Forms that mix
free text with other fields are not answerable from Telegram: the chat offers
decline/cancel and the full surface remains the HTTP API. Requests for sessions
the bot does not own are skipped and logged rather than sent to a wrong chat.

Elicitation interop evidence (2026-09-21):

- The full round trip is exercised against a real ACP peer over a real stdio
  process boundary in `tests/integration/elicitation_interop_test.go`
  (`accept`, `decline`, and the frontend-less decline control), using the real
  transport, request handler, and pending registry.
- A real provider binary is used to prove the rollout itself is safe:
  `tests/integration/elicitation_real_provider_test.go` negotiates a live
  session with stable elicitation capabilities advertised. OpenCode 1.18.31
  completed `initialize` (protocol 1), `session/new`, `session/cancel`, and
  `session/close` unchanged, so advertising the new client capability breaks no
  existing conversation. Run it with
  `MATRIX_SMOKE_TEST=1 go test ./tests/integration -run RealACPAgentToleratesElicitation`.
- Ecosystem status, measured rather than assumed: the installed OpenCode build
  contains `elicitation/create` only inside its MCP client code and never in an
  ACP session context (no `sessionId`/`session/update` neighbours), and the
  `claude` ACP entry in the local vault pointed at a subcommand that Claude Code
  2.1.220 no longer ships (`claude acp` is unknown), so OpenCode cannot serve
  as an elicitation peer today.
- Claude Code is integrated through the official
  `@agentclientprotocol/claude-agent-acp` adapter (binary `claude-agent-acp`),
  which the repository seed already declared. The adapter implements the stable
  surface as well: it calls `methods.client.elicitation.create` and
  `elicitation/complete`, and it only starts its MCP OAuth flow when the client
  advertises `elicitation.url`. A stale vault endpoint that pointed at
  `claude acp` was corrected to the adapter path; the real adapter then
  negotiated protocol 1 with elicitation advertised.
- Real consumer found: `@agentclientprotocol/codex-acp` **does** implement the
  stable surface. Its `handleElicitation` path calls
  `methods.client.elicitation.create` for MCP-server input requests, and
  `shouldUseAcpElicitation` selects between elicitation and
  `session/request_permission` purely from the capabilities the client
  advertised (`clientSupportsFormElicitation` / `clientSupportsUrlElicitation`).
- Because of that routing, advertising elicitation **changes real agent
  behaviour**: an MCP input request that previously arrived as
  `session/request_permission` (answered immediately by Matrix policy, including
  `agent.trust_mode` auto-approval) would instead wait on a human and expire as
  `cancel`, silently denying a governed approval. Matrix therefore ships
  elicitation as an **opt-in, default-off** surface
  (`matrix config set agent.elicitation_enabled true`). With the flag off, no
  capability is advertised, the HTTP surface answers `503`, and behaviour is
  byte-for-byte the pre-existing permission path.
- Live emission proven against a real agent. With the capability advertised,
  codex-acp routed an MCP tool approval to Matrix as a stable elicitation
  (`Allow the mocksrv MCP server to run tool "choose_database"?`), and after
  Matrix answered it, codex forwarded the MCP server's own follow-up
  elicitation (`Which database should the mock tool use?`) through the same
  path. Both were projected into the neutral SSOT, validated, and answered
  through the registry.
- That live run exposed a projection defect no unit test had caught: codex-acp
  expresses an approval scope as `oneOf` of `{const, title}` options with a
  `title` and a `default`, not as a plain `enum`. The original projection
  dropped all three, degrading a three-way approval choice into a free-text
  field. `ElicitationField` now carries `Title`, `Default` and
  `Options[]ElicitationOption{Value, Label}`, and `wireOptions` folds `enum`
  and `oneOf`/`anyOf` into that one representation, so no frontend needs to
  know which spelling an agent chose.
- `TestSmoke_RealCodexACPEmitsElicitation` remains as the observable probe for
  the user-input path (that prompt made the model answer in prose instead of
  invoking its tool, so it skips with evidence rather than failing).

ACP draft/unstable provider selection, model selection, NES/document,
MCP-over-ACP, and Streamable HTTP
are not advertised as stable Matrix capabilities. Adding one requires a
separately named non-default experimental surface or promotion in the upstream
stable schema.

## ACP current presentation and lifecycle contracts

- V1 advertises `clientCapabilities.session.notices` and `.compaction` only
  when the connection-wide observer is wired; v2 includes them in its baseline
  and never emits the retired v1 `session`, filesystem or terminal capability
  fields during v2 initialize.
- Notices are live `agent.notice` events, never stored as session history or
  counted as answer/work evidence. Compaction updates and summary chunks retain
  first-seen position, replacement/append semantics, omitted values and explicit
  clears. Detached snapshots are exposed as neutral `context_compactions` in
  turn metadata and remote session information. They survive between turns on
  one connection and rebuild only from explicit peer replay after reconnect.
- V1 compaction `_meta` replaces the object; v2 patches top-level keys, with
  null key removal and whole-object null clearing. Message/tool/terminal
  entity metadata is materialized within the observed turn; Matrix does not
  maintain a private transcript. Nested objects replace without recursive merge.
- V2 session-info patches propagate explicit title/timestamp clearing and
  top-level metadata removal through the neutral contract into Matrix session
  mirrors. Omission preserves state; this is independent of aggregate turn data.
- V2 insertion acknowledgements require `messageId`. An idle/replay snapshot
  cannot complete work that never entered running/requires_action. Missing idle
  completion fails at the turn budget, retaining partial diagnostic output;
  post-insertion `error` stop reason fails rather than completing partial text.
  Received updates are drained before resume/close or final turn materialization.
- Actionable fs/terminal/permission values are validated before side effects.
  Missing/null file content cannot truncate a file; invalid args/env/ranges or
  paths are rejected. Permission decisions select only actual offered options.
  Malformed optional display metadata is absent. Provider `_meta` cannot replace
  Matrix's protocol identity or evidence classification.
- Native unit/race tests and real stdio mock peers cover these contracts. A real
  OpenCode/DeepSeek probe qualifies current v1 delivery; no installed commercial
  v2 agent support is inferred from synthetic v2 peers.

## A2A v1.0.1 coverage

Matrix implements both an outbound A2A client and an inbound A2A agent server.

| Stable RPC | Client | Server |
| --- | --- | --- |
| `SendMessage` | Complete | Complete |
| `SendStreamingMessage` | Complete; used only when the agent card advertises streaming, otherwise Matrix uses `SendMessage` without probing | Complete, including submitted/working/progress/artifact/completed events |
| `GetTask` | Complete | Complete |
| `ListTasks` | Complete, including all-page traversal | Complete, authenticated task ownership and pagination |
| `CancelTask` | Complete | Complete |
| `SubscribeToTask` | Complete through neutral task subscription | Complete through the SDK streaming handler |
| `CreateTaskPushConfig` | Complete through neutral push control | Opt-in; requires a governed callback store and sender |
| `GetTaskPushConfig` | Complete | Opt-in with push configuration |
| `ListTaskPushConfigs` | Complete | Opt-in with push configuration |
| `DeleteTaskPushConfig` | Complete | Opt-in with push configuration |
| `GetExtendedAgentCard` | Complete and preserved losslessly | Opt-in with an authenticated extended card |

Additional stable A2A surface:

- JSON-RPC and HTTP+JSON bindings are implemented by both discovery and server
  advertisement. gRPC is a valid optional A2A binding but Matrix does not
  advertise it because there is no governed gRPC ingress listener.
- Agent-card discovery negotiates the selected interface, protocol version,
  tenant, transport, headers, skills, security requirements, and extended card.
- Direct endpoints preserve the optional A2A tenant in the Vault and CLI.
- Text, raw file, URL file, and structured data parts are projected losslessly
  into neutral Matrix content, including artifact metadata.
- Message extension URIs and referenced task IDs survive neutral routing.
- Task states, status messages, artifacts, history identity, context IDs, task
  IDs, final markers, and progressive events remain distinct; intermediate
  working/thought updates never contaminate final output.
- Push callbacks are disabled by default. Enabling them requires an outbound
  URL-validation and delivery policy so protocol coverage cannot silently
  create an SSRF surface.

### Known A2A differences

One difference between Matrix's served A2A surface and the specification's letter is
accepted, tested, and recorded here rather than left implicit:

- **`ListTasks` with `includeArtifacts: true` and a task that has no artifacts.**
  §3.1.4 (A2A v1.0.1) says "the artifacts field should be included with its actual
  content (which may be an empty array if the task has no artifacts)". Matrix omits
  the field for an artifact-less task. The omission is the SDK wire type's, not the
  store's: both bindings marshal with `encoding/json` (SDK JSON-RPC and REST handlers) and `a2a.Task.Artifacts` carries
  `json:"artifacts,omitempty"` (in `a2a-go/v2 v2.7.0`), so an empty
  slice is dropped whether or not it is nil. Only a transport-level body rewrite or a
  fork of the SDK's core type could emit `[]`, and the sentence is a lowercase
  "should". The MUST in the same paragraph - omit the field when `includeArtifacts` is
  false - is met. Pinned by
  `TestListTasksOmitsTheArtifactsFieldForATaskWithoutArtifacts`
  (`internal/providers/a2a/methods_test.go`), which fails if the SDK's shape changes.

Terminal-task message/subscription errors now come directly from SDK 2.7.0:
`UnsupportedOperationError` (-32004 on JSON-RPC). The old correction interceptor
was removed; `TestProtocolSDKTerminalTaskCodesMatchCurrentSpecification` pins
native behavior. Matrix retains only its own one-running-turn policy; running
subscriptions and input-required task continuation remain valid.

Both served bindings now require the negotiated `A2A-Version: 1.0`; unsupported
versions are rejected, not silently interpreted as 1.0. JSON-RPC uses current
PascalCase method names. The retired 0.3 method translator is removed.
The SDK handles `application/a2a+json` negotiation, trailing JSON rejection,
UTC timestamps, canonical ProtoJSON security scopes, timestamp filters and
stream failure propagation. SDK 2.7's exact-only input matcher cannot match
Matrix's advertised MIME families: the adapter validates against the same card,
accepting `image/*` and `audio/*` while rejecting unadvertised media types.
Optional SDK JWS helpers do not make Matrix a card-signing or signature-trust
service; no cryptographic verification or signed-card capability is advertised.

## Runtime verification

`AgentCapabilities` reports separate `session`, `operations`, `content`, and
`transports` maps. Callers must use the report and explicit optional interfaces;
they must not probe support by sending speculative protocol traffic.

Required checks for a protocol change:

```bash
go run ./scripts/governance_check --manifest governance/manifest.toml
go test ./pkg/zedacp ./internal/providers/agents ./internal/providers/a2aclient ./internal/providers/a2a ./internal/providers/sidecarprojection
go test -race ./...
```
