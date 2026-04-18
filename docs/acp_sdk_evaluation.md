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
- Python library page: https://agentclientprotocol.com/libraries/python
- Java library page: https://agentclientprotocol.com/libraries/java
- Community Go SDK repo: https://github.com/coder/acp-go-sdk

Local findings:

- `go list -m -versions github.com/coder/acp-go-sdk` currently returns published module versions through `v0.6.3`
- the repository README on `main` advertises `v0.10.8`, which suggests a gap between repository head and published module releases
- the published `v0.6.3` module exposes client and agent side connections and is useful for both ACP client and server implementations
- the published `v0.6.3` module does not expose the newer Matrix-needed surface around `session/list`, preview `session/close`, draft `session/delete`, and `session_info_update`

## What `coder/acp-go-sdk` Gets Right

- It is shaped like a real SDK, not an app-specific package.
- It supports both sides of the wire:
  - client-side connection
  - agent-side connection
- It models ACP with generated types and examples.
- It is community-visible and already used as a reference-quality Go implementation.

For a future standalone ACP project, this is the strongest available community candidate we have found.

## What Blocks Immediate Migration

### 1. Published surface is behind Matrix needs

Matrix now relies or is preparing to rely on:

- `session/load`
- `session/list`
- `session/close` preview support when capability-gated
- `session/delete` draft support when capability-gated
- `session_info_update`
- protocol-transparent remote session control in channel flows

The published `coder/acp-go-sdk@v0.6.3` surface is narrower than that.

### 2. Release maturity is not yet ideal

There is a mismatch between:

- the published versions visible to `go list`
- the newer version string shown in the repository README on `main`

That does not make the project unusable, but it does reduce confidence for a production migration where Matrix would depend on timely published releases.

### 3. Matrix already has custom runtime concerns

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

Matrix should **not migrate immediately** to `coder/acp-go-sdk`.

Matrix should:

1. keep `pkg/zedacp` as the current ACP backend
2. keep strengthening compliance against Zed ACP
3. use the new ACP backend port as the stable seam for future migration
4. revisit migration when a community or official Go SDK offers:
   - published support for `session/list`
   - published support for `session_info_update`
   - an acceptable story for preview `session/close`
   - an acceptable story for draft `session/delete`
   - stable releases that match repository head closely
