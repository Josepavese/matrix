# Matrix V2 Protocol-Neutral Runtime

## Goal

Matrix V2 now separates:

- the **conversation core** from any specific agent protocol
- the **agent protocol adapters** from the daemon/session logic
- the **discovery layer** from both protocol execution and catalog shape
- the **channel runtime** from any specific messaging provider

This document defines the strategic boundary used by the codebase.

## Architectural Split

### 1. Protocol-Neutral Core

The neutral contract lives in `internal/middleware/protocol.go`.

Core concepts:

- `ProtocolEndpoint`: how an agent is reached
- `ConversationTurn`: one logical user turn
- `ConversationResult`: one logical agent result
- `ConversationClient`: protocol-specific adapter hidden behind a stable contract
- `ConversationSessionControl`: optional protocol-neutral session lifecycle control
- `ConversationFactory`: creator for ACP, A2A, and future protocols

The rest of Matrix should reason in terms of:

- logical session
- remote session token
- text input/output
- tool calls
- provider session/task inventory when available
- host capabilities (fs/process/trust mode)

It should **not** reason directly in terms of:

- ACP `initialize/session/new/session/prompt`
- A2A `SendMessage/Task/Event`
- transport framing details

### 2. Protocol Adapters

Current adapters:

- `internal/providers/agents/acp_adapter.go`
- `internal/providers/agents/a2a_adapter.go`

Responsibilities of adapters:

- translate neutral turns into protocol-specific requests
- manage protocol-specific session state
- expose provider-native session/task lifecycle when available
- expose streaming updates as neutral thought updates when possible
- translate protocol-specific outputs back into `ConversationResult`

Protocol-specific details must stop at this layer.

### 3. Channel-Neutral Runtime

The neutral channel model lives in `internal/middleware/link.go`.

Current runtime bootstrap:

- `internal/logic/channelruntime/runtime.go`

Current provider:

- Telegram via `internal/providers/telegram`

Telegram is no longer started directly by `cmd/matrix/run.go`; it is created through the channel runtime registry like any other gateway.

### 4. Discovery-Neutral Layer

The discovery model now lives in `internal/logic/agentdiscovery`.
The onboarding-facing aggregation and activation boundary lives in `internal/logic/agentcatalog`.

Supported discovery sources:

- `local`: agents already registered in SSOT
- `acp_registry`: ACP public registry/catalog
- `a2a_card`: direct A2A discovery via Agent Card URL or base URL
- `a2a_catalog`: pluggable catalog provider for A2A-style directories

This is an intentional split:

- **protocol** decides how Matrix talks to the agent at runtime
- **discovery** decides how Matrix finds metadata/endpoints
- **catalog** is just one possible discovery backend

For A2A, Matrix now treats the Agent Card as the standard discovery artifact and catalogs as optional provider-specific indexes.

### 5. Onboarding-Neutral Selection

First-run onboarding no longer depends structurally on the ACP Registry.

The wizard now consumes:

- a discovery interface that aggregates `local`, `acp_registry`, and optional `a2a_catalog` sources
- an activation interface that decides how a selected entry becomes available locally

This means channel-driven onboarding, including Telegram, now follows a source policy instead of an ACP-specific code path.

Current activation rules:

- `local`: already available, no activation needed
- `acp_registry`: install through the ACP installer
- `a2a_catalog` or `a2a_card`: register a remote endpoint in SSOT

## Persistence Model

Agent configuration now distinguishes:

- `kind`: protocol family, currently `acp` or `a2a`
- `transport`: protocol binding or process transport
- legacy `protocol`: retained for backward compatibility and normalized at load time

Protocol selection is therefore **SSOT-driven**:

1. an agent entry is loaded from `agent.config.<agent_id>` in the vault
2. `internal/logic/agentcfg/normalize.go` maps legacy and new fields into `ProtocolEndpoint`
3. the router selects the adapter from `ProtocolEndpoint.Kind`

In other words, Matrix does not guess ACP vs A2A from traffic. It resolves the protocol from SSOT.

### Operational commands

- `matrix agent show <id>`: inspect effective config and normalized endpoint
- `matrix agent set-binary <id> <path> --protocol acp --transport stdio`
- `matrix agent set-endpoint <id> <url> --kind a2a --transport JSONRPC`
- `matrix install <id>`: ACP Registry install flow
- `matrix install <id> --a2a-url <url>`: register a remote A2A endpoint in SSOT
- `matrix install <id> --a2a-card-url <base-or-card-url>`: discover endpoint from A2A Agent Card, then persist it in SSOT
- `matrix agent search --source local|acp_registry|a2a_catalog`
- `matrix agent info <ref> --source acp_registry|local|a2a_card|a2a_catalog`

Normalization logic lives in `internal/logic/agentcfg/normalize.go`.

This lets existing ACP definitions continue working while making A2A first-class.

## Inbound Surface

Matrix exposes:

- Matrix HTTP run bridge v1 on `/v1/runs`
- Matrix HTTP session actions v1 on `/v1/session-actions`
- Matrix HTTP workspace state v1 on `/v1/workspace-state`
- Matrix HTTP workspace timeline v1 on `/v1/workspace-timeline`
- Matrix HTTP workspace decisions v1 on `/v1/workspace-decisions`
- Matrix HTTP workspace memory v1 on `/v1/workspace-memory`
- Matrix HTTP workspace snapshots v1 on `/v1/workspace-snapshots`
- Matrix HTTP intents v1 on `/v1/intents`
- Matrix HTTP modes v1 on `/v1/modes`
- Matrix HTTP orchestration profile v1 on `/v1/orchestration-capabilities`
- A2A JSON-RPC API on `/a2a`
- A2A Agent Card on `/.well-known/agent-card.json`

Important distinction:

- outbound ACP support is Zed ACP over JSON-RPC transports such as `stdio`, `ws`, and `unix`
- `/v1/runs` is the canonical Matrix ingress API that routes into the session manager; it is not the ACP wire protocol defined by Zed

### Current Ingress Contract

`POST /v1/runs` accepts a Matrix envelope:

- `channel_id`: physical ingress identity or routing key
- `input`: latest user message
- `agent_id`: optional requested agent for new sessions

`POST /v1/session-actions` accepts a typed action envelope:

- `channel_id`: physical ingress identity or routing key
- `action`: currently `cancel`, `delete`, `switch`, `list`, `status`, `new`, or `name`
- `target`: optional action operand

Current target semantics:

- `cancel`, `delete`, `switch`: local or remote session selector
- `new`: requested agent id
- `name`: alias for the active logical session

Behavior:

- if first-run is not completed, the request is intercepted by the onboarding wizard
- once configured, the request is routed through the session manager
- the session manager resolves or creates the logical session for `channel_id`
- the active session agent wins over `agent_id` after the session exists
- slash commands such as `/session`, `/help`, `/wizard`, and `/action` are handled before agent routing
- `/session list` shows the local vault mirror and, when supported by the current provider, the remote session/task inventory
- `/session switch <target>` can reattach to local history or import a remote ACP/A2A session/task into the local mirror
- `/session cancel [target]` cancels the active or selected remote session/task while preserving the local mirror
- `/cancel` and `/stop` are UX aliases for `/session cancel`
- `/session delete [target]` removes the local mirror and calls the closest remote lifecycle operation available
- `/session new [agent]`, `/session name <alias>`, and `/session status` are exposed by the same typed session-action core used by HTTP and future channel adapters

Defaulting:

- if `agent_id` is omitted on `/v1/runs`, Matrix uses the configured default agent
- if A2A metadata omits `agent_id`, Matrix also falls back to the configured default agent

Response model:

- `/v1/runs` returns a synchronous JSON object with `output`
- `/v1/session-actions` returns a synchronous typed JSON object describing the session action result
- `/v1/workspace-state`, `/v1/workspace-timeline`, `/v1/workspace-decisions`, `/v1/workspace-memory`, and `/v1/workspace-snapshots` return synchronous typed read models
- the same typed action surface is shared by the session manager for chat-style channels and HTTP callers
- `/a2a` returns A2A JSON-RPC events/messages as defined by the A2A SDK

Auth and callbacks:

- `/v1/runs` can be protected with `X-Matrix-Key`
- `/v1/auth/openrouter/callback` is the versioned auxiliary HTTP callback endpoint used by the onboarding/auth flow, not a general ingress surface

Versioning policy:

- new clients should target `/v1/runs`
- new clients should target `/v1/session-actions` for typed session lifecycle operations instead of synthesizing slash-commands over `/v1/runs`
- future breaking envelope changes should introduce `/v2/...` rather than mutate the `v1` contract

## Session Mirror Model

Matrix stores session state in the vault as the local source of truth for channels, while also treating it as a mirror of remote provider state.

Current mirror fields include:

- logical session id
- agent id
- remote session/task token
- protocol kind
- mirror status
- remote title
- remote updated timestamp
- last synchronized timestamp

Current behavior:

- ACP remote sessions are enumerated through `session/list`, resumed through `session/load`, and can be deleted when the provider advertises draft `sessionCapabilities.delete`
- ACP remote sessions can also be interrupted through `session/cancel`, which Matrix sends as a JSON-RPC notification
- A2A remote tasks are enumerated through `ListTasks`, imported through `GetTask`, and deleted through `CancelTask`
- channel users do not select ACP or A2A explicitly; Matrix resolves the provider from SSOT and the active session

The A2A ingress is implemented with the official Go SDK:

- module: `github.com/a2aproject/a2a-go/v2`

## Market State

Matrix is intentionally ready for both ACP and A2A at the runtime boundary, but the operational state of the market is not symmetric.

### Operational Standard Today

ACP is the current operational standard for real coding agents in this environment.

This has been verified with real products and adapters:

- Codex via `codex-acp`
- Gemini CLI via `gemini --acp`
- Claude via `@zed-industries/claude-code-acp`
- OpenCode via `opencode acp`

For day-to-day usage, ACP should be treated as the default production path.

### Strategic Readiness

A2A remains strategically important and is already supported in Matrix at the protocol, routing, discovery, and ingress layers.

However, for the real products currently used with Matrix, A2A support is not yet mature enough to be treated as the default operational standard.

Current state:

- Matrix runtime: A2A-ready
- Matrix discovery: A2A-ready
- Matrix ingress: A2A-ready
- Real market availability across coding agents: still uneven

Therefore A2A should be documented as:

- implemented in the core
- suitable for experimentation and future adoption
- pending broader and more stable market support from vendors and adapters

### Adoption Trigger

Matrix should promote A2A from strategic readiness to operational standard only when at least one of these becomes true:

- major coding agents expose stable native A2A endpoints
- stable vendor-supported A2A adapters become common and well documented
- A2A discovery and deployment patterns become operationally simpler than ACP in real environments

Until then:

- use ACP by default
- keep A2A available without making it the primary recommended path

## Design Rules

- Session logic may depend on `middleware.AgentRouter`, not on ACP or A2A SDK types.
- Agent protocol packages may depend on ACP/A2A specifics, but only inside adapters.
- Discovery code may depend on registry formats or Agent Card schemas, but not on the session manager or protocol adapters.
- Channel gateways may depend on provider SDKs, but the daemon boot process must depend only on the channel runtime registry.
- New protocols must be added by implementing `ConversationFactory`, not by branching the session manager.
- New discovery backends must be added by implementing `agentdiscovery.Provider`, not by hardcoding another branch into the CLI.
- New onboarding discovery policies must be expressed by source ordering and activation rules, not by embedding protocol-specific logic in the wizard.
- New channels must be added by implementing a runtime `Factory`, not by editing the daemon startup flow with provider-specific code paths.

## Current Supported Matrix

### Outbound agent protocols

- ACP
- A2A

### Inbound client protocols

- A2A
- Matrix run bridge

### Messaging channels

- Telegram

The runtime is now neutral even if only one messaging gateway is currently bundled.
