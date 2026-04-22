# Noema request: evaluate experimental ACP `session/fork` for temporary context-preserving interpreter threads

Date: 2026-04-22
Reporter: Noema integration
Priority: high for research, medium for production
Status requested: Matrix design review / optional experimental implementation

## Summary

Noema has a product need that maps closely to the new ACP `session/fork` RFD:

```text
main active agent session
-> fork temporary child session with current context
-> ask the agent to interpret Noema evidence into concise guidance
-> collect the response/artifact
-> close/delete the fork
-> keep the main session history clean
```

This would let Noema use the same agent as a context-aware interpreter without polluting the user-visible task thread.

The ACP `session/fork` RFD is still Draft, so Noema should not depend on it as a production baseline. But it is important enough that Matrix should evaluate whether to expose it experimentally when an adapter/provider supports it.

## Official Sources

Primary source:

- `session/fork` RFD: https://agentclientprotocol.com/rfds/session-fork

Related official lifecycle sources:

- ACP RFD updates: https://agentclientprotocol.com/rfds/updates
- `session/resume` RFD, Preview: https://agentclientprotocol.com/rfds/session-resume
- `session/close` RFD, Preview: https://agentclientprotocol.com/rfds/session-close
- ACP protocol schema: https://agentclientprotocol.com/protocol/schema
- Zed ACP overview: https://zed.dev/acp
- Zed ACP client page: https://zed.dev/acp/editor/zed

Important upstream facts from the official RFD:

- `session/fork` is Draft.
- The RFD proposes adding the ability to fork a new session based on an existing one.
- The stated motivation is to use the current conversation as context to generate summaries or similar artifacts without polluting user history.
- The RFD explicitly mentions future use cases such as summaries and potentially subagents.
- Proposed method: `session/fork`.
- Proposed capability shape in the RFD: `session: { fork: {} }`.
- Proposed request includes `sessionId`, `cwd`, and `mcpServers`, aligned with `session/load`-style options.

## Why This Matters To Noema

Noema currently has an `experienceinterpreter.Provider` layer.

Today the safe default is:

```text
structured evidence
-> deterministic/local provider
-> concise <noema schema="noema.agent.v1"> guidance
```

But a stronger future provider could be:

```text
structured Noema evidence
-> Matrix ACP fork provider
-> same agent, same context, temporary child session
-> concise natural-language guidance
-> fork closed
-> guidance delivered to main run only if verified
```

This avoids two current problems:

1. A brand-new session loses context.
2. Asking the main session to interpret evidence pollutes the task history and may affect future reasoning.

## Desired Matrix Abstraction

Please consider a Matrix-owned experimental surface that hides the ACP draft details behind provider capability gating.

Possible Matrix-level contract:

```text
POST /v1/runs/{run_id}/forks
```

or a run action:

```json
{
  "action": "fork_context",
  "reason": "temporary_interpreter",
  "prompt": "...",
  "cleanup_policy": "close_after_response",
  "visibility": "internal"
}
```

Possible response:

```json
{
  "run_id": "run-main",
  "fork_run_id": "run-fork",
  "status": "completed|unsupported|failed",
  "capability": "acp.session/fork",
  "stability": "draft",
  "cleanup": {
    "requested": true,
    "closed": true,
    "proof": "session.close|provider-native|matrix-local"
  },
  "artifact": {
    "kind": "interpreter_response",
    "content": "..."
  }
}
```

Exact API shape is up to Matrix. The key requirement is capability truth and cleanup proof.

## Required Semantics

The fork must:

- inherit enough current session context to be useful
- avoid writing into the main session history
- be explicitly marked experimental while ACP `session/fork` is Draft
- close/cleanup the child session after the artifact is produced
- record trace evidence for fork creation, prompt, completion, and cleanup
- report unsupported providers explicitly
- avoid pretending a reconstructed fresh session is equivalent to a true fork

## Noema Safety Requirements

Noema will not trust fork output blindly.

Noema still needs:

- evidence-ref verification where applicable
- minimal agent-facing output
- audit-only internal evidence
- no raw Noema trace dumps into the main agent prompt
- no production claim unless Matrix proves fork support and cleanup

The fork provider should be one provider behind an abstraction, not a hard dependency.

Canonical Noema layer shape:

```text
internal/experienceinterpreter
-> Provider interface
-> MatrixForkProvider
-> Matrix experimental fork API
-> ACP session/fork when supported
```

## Capability States Noema Needs

For each provider/adapter:

- `fork_supported`: true/false
- `fork_stability`: draft/preview/stable/matrix-specific
- `fork_source`: acp.session/fork/provider-native/matrix-emulated/unsupported
- `fork_cleanup_supported`: true/false
- `fork_context_fidelity`: true_fork/reconstructed_context/unsupported

If Matrix can only emulate fork by creating a new session and replaying or summarizing context, please expose that as `reconstructed_context`, not as true fork.

## Acceptance Criteria

- Matrix reviews whether `session/fork` can fit its philosophy as a provider-neutral session lifecycle capability.
- If implemented, Matrix marks it experimental while upstream ACP keeps it Draft.
- Matrix exposes fork support per provider/adapter.
- Matrix records fork lifecycle events in traces.
- Matrix closes/cleans the fork and provides cleanup evidence.
- Unsupported providers return typed unsupported evidence.
- Noema can call the feature without knowing Codex/OpenCode/Claude/Gemini-specific internals.

## Non-goals

- Matrix should not interpret Noema evidence.
- Matrix should not decide whether the fork-generated guidance is correct.
- Matrix should not expose this as stable production capability while ACP marks it Draft.
- Matrix should not contaminate the main agent session with temporary interpreter prompts.

## Matrix maintainer response

Status: accepted and implemented as an experimental capability-gated lifecycle
operation.

Matrix exposes fork through the channel-neutral `/v1/session-actions` surface:

```json
{
  "channel_id": "example",
  "action": "fork",
  "target": "optional-local-session-id"
}
```

Semantics:

- Matrix calls true ACP `session/fork` only when the provider advertises
  `sessionCapabilities.fork`.
- The capability is reported as `stability=draft`.
- Unsupported providers return `unsupported=true` and `fork.unsupported=true`.
- Matrix does not emulate fork by replaying context or creating a fresh session.
- The forked remote session is mirrored as a normal Matrix logical session and
  becomes active for the channel.
- Cleanup remains an explicit session lifecycle action with strong cleanup proof.

Real provider result:

- `opencode` advertises `fork=true`; Matrix successfully called ACP
  `session/fork` and received a child remote session.
- Fork response is now JSON snake_case, for example
  `fork.child.remote_session_id`.
- `codex-acp` and `gemini` currently report fork unsupported.

Scope intentionally not implemented yet:

- Dedicated `/v1/runs/{run_id}/forks`.
- Automatic fork prompt/response artifact pipeline.
- Automatic child cleanup after an interpreter task.

Those are product-level orchestration features on top of the now-working
lifecycle primitive. The primitive is ready for a Noema experimental provider,
but not a stable production baseline while ACP keeps `session/fork` in Draft.

Validation:

- Unit test coverage added for `pkg/zedacp` `session/fork`.
- Real OpenCode ACP fork completed through installed Matrix.
- Forked child and parent sessions were cleaned afterward with strong proof.
