# ACP Go SDK Evaluation

## Goal

Evaluate whether Matrix should:

- keep the current in-repo ACP package `pkg/zedacp`
- migrate to a community-maintained ACP Go SDK such as `github.com/coder/acp-go-sdk`
- or adopt a hybrid model where Matrix keeps a stable local facade and swaps the backend later

## Inputs Considered

External references:

- Zed ACP introduction: https://agentclientprotocol.com/get-started/introduction
- Zed ACP transports: https://agentclientprotocol.com/protocol/transports
- Zed ACP registry: https://agentclientprotocol.com/get-started/registry
- TypeScript SDK docs: https://agentclientprotocol.github.io/typescript-sdk/
- TypeScript SDK version reviewed: v0.21.0
- Python library page: https://agentclientprotocol.com/libraries/python
- Java library page: https://agentclientprotocol.com/libraries/java
- Zed ACP `session/fork` RFD: https://agentclientprotocol.com/rfds/session-fork
- Zed ACP `additionalDirectories` RFD: https://agentclientprotocol.com/rfds/additional-directories
- Zed ACP request cancellation RFD: https://agentclientprotocol.com/rfds/request-cancellation
- Community Go SDK repo: https://github.com/coder/acp-go-sdk

Local findings:

- 2026-05-21: `go list -m -versions github.com/coder/acp-go-sdk`
  returns published module versions through `v0.13.0`
- 2026-05-21: the repository README on `main` advertises `v0.13.0`
- the published `v0.13.0` module should be re-evaluated against official ACP
  Schema v1.13.7 before any migration
- the published SDK exposes client and agent side connections and is useful for
  both ACP client and server implementations; Matrix should still keep its ACP
  backend port so the transition remains reversible

## What `coder/acp-go-sdk` Gets Right

- It is shaped like a real SDK, not an app-specific package.
- It supports both sides of the wire:
  - client-side connection
  - agent-side connection
- It models ACP with generated types and examples.
- It is community-visible and already used as a reference-quality Go implementation.

For a future standalone ACP project, this is the strongest available community candidate we have found.

## What Still Blocks Immediate Migration

### 1. Matrix has not yet validated the SDK adapter surface end-to-end

Matrix now relies or is preparing to rely on:

- `session/load`
- `session/list`
- stable `session/close` support when capability-gated
- stable `session/resume` support when capability-gated
- stable `session/set_config_option` support and complete `configOptions` state
- `session/delete` draft support when capability-gated
- `session_info_update`
- `session/fork` draft support when capability-gated
- `additionalDirectories` modeling for multi-root workspaces
- prompt `messageId` / `userMessageId` correlation
- protocol-transparent remote session control in channel flows

The published `coder/acp-go-sdk@v0.13.0` surface should cover substantially more
of this list than earlier versions. Before migration, Matrix still needs a
small adapter spike proving that its generated types, connection lifecycle, and
extension handling preserve Matrix's current behavior with OpenCode, Codex ACP,
and Gemini.

### 2. Dependency migration still needs a reversible seam

The release mismatch needs a fresh check: `go list` and the repository README
previously pointed to `v0.13.0`, while the current official ACP schema release
checked for this Matrix review is Schema v1.13.7. Matrix should still keep
`pkg/zedacp` behind the ACP backend port until the community SDK has passed
real-provider Matrix tests.

### 3. Matrix still has custom runtime concerns

Matrix does not only need ACP wire calls. It also needs:

- persistent client pooling
- stdio/ws/unix transport selection
- host request handling for fs and terminal methods
- session mirror semantics in the vault
- protocol-transparent bridging into `/session` channel commands

Even with a community SDK, Matrix would still need an adapter layer.

## What Blocks Keeping `pkg/zedacp` As-Is Forever

Keeping the in-repo package unchanged is also not ideal.

Current concerns:

- it is Matrix-maintained, so protocol catch-up is our burden
- `session/cancel` semantics must stay aligned with current Zed ACP notifications
- `session/close` semantics must stay aligned with the Preview RFD while it is not yet stable
- `session/fork` semantics must stay aligned with the Draft RFD and must not be confused with a non-existent ACP `side` primitive
- `additionalDirectories` and prompt message ids must stay optional and capability-gated while unstable
- long-term maintenance cost is on us unless we rebase or extract it cleanly

So the right answer is not "never migrate". The right answer is "do not couple Matrix directly to a single ACP backend."

## Recommended Decision

### Short term

Keep `pkg/zedacp` as the active backend.

Reason:

- it already supports the Matrix runtime shape
- it already covers the current Matrix session-control roadmap better than the published community Go SDK
- migrating now would likely lose capability or require carrying a fork

### Medium term

Treat `pkg/zedacp` as a facade, not as the permanent implementation.

Matrix should depend on:

- a local ACP port interface
- local ACP-neutral adapter types at the runtime boundary

and only one layer below that should know which Go ACP backend is used:

- current backend: `pkg/zedacp`
- future backend: `coder/acp-go-sdk`
- future backend: official Zed ACP Go SDK, if one appears

### Long term

If `coder/acp-go-sdk` reaches the needed protocol surface and release quality:

1. implement a backend adapter for it
2. run Matrix integration tests unchanged
3. swap the default ACP backend
4. keep `pkg/zedacp` only as a compatibility facade or deprecate it

## Work Already Done To Support This

Matrix now has a dedicated ACP backend port in:

- `internal/providers/agents/acp_port.go`

This isolates the runtime adapter from the concrete ACP implementation:

- `acpConversationClient` no longer needs to know how the underlying client is constructed
- future backend swaps can be confined to the ACP backend layer

This does **not** fully remove all backend coupling yet, but it makes migration materially safer.

## Migration Strategy

### Option A: no migration now

Recommended default.

- keep `pkg/zedacp`
- continue improving compliance
- monitor `coder/acp-go-sdk`

### Option B: hybrid migration

Recommended once the community SDK publishes the missing surface.

- keep Matrix adapters and session manager unchanged
- implement a new ACP backend behind `acp_port.go`
- compare:
  - protocol correctness
  - real agent compatibility
  - test stability
  - maintenance burden

### Option C: hard switch now

Not recommended.

Reasons:

- likely feature regression on session lifecycle support
- uncertain release cadence for the exact surface Matrix needs
- high chance of needing local patches anyway

## Final Recommendation

Matrix should **not hard-switch immediately** to `coder/acp-go-sdk`, but it
should treat the `v0.13.x` line as a serious migration candidate.

Matrix should:

1. keep `pkg/zedacp` as the current ACP backend
2. keep strengthening compliance against Zed ACP
3. use the new ACP backend port as the stable seam for future migration
4. run a follow-up adapter spike against `coder/acp-go-sdk@v0.13.0`
5. migrate only if the adapter passes Matrix's real-provider smoke suite and
   keeps draft/unstable features capability-gated
