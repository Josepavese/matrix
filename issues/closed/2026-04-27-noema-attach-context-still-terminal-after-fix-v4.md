# Noema verification failed: attach_context still delivered at terminal boundary after Matrix fix

Date: 2026-04-27

## Summary

Noema reran the same experience-only, non-interference OpenCode active-sidecar
smoke after Matrix was rebuilt and restarted with the reported fix.

The result still does not prove useful live attach:

```text
suggestions_delivered=1
suggestions_received_before_completion=1
suggestions_post_delivery_activity=0
matrix_live_interrupt_proven=false
```

Matrix now returns from `attach_context` immediately (`2ms`), but the actual
sidecar delivery still lands at the terminal boundary, with only `2199ms` lead
time before final output and no later agent/tool work.

This keeps OpenCode `live_attach` at diagnostic status for Noema.

## Matrix Version

```text
matrix 0.1.16-snapshot
commit fcd679a5225f0a4cfa459709cdd55092b44730e1
built 2026-04-27T21:33:12Z
```

## Related Closed Issue

```text
/home/jose/hpdev/Libraries/matrix/issues/closed/2026-04-27-noema-attach-context-accepted-but-delivered-at-terminal-boundary.md
```

That issue appears fixed only at the HTTP/action-return layer. Noema can now
prove the action call returns fast, but the agent-visible delivery remains too
late for live intervention.

## Noema Repro

Repository:

```text
/home/jose/hpdev/Libraries/noema
```

Command:

```bash
NOEMA_OUTCOME_CRITIC_PROVIDER=auto \
NOEMA_SEMANTIC_PROVIDER=ollama \
NOEMA_SEMANTIC_OLLAMA_MODEL=embeddinggemma:latest \
NOEMA_SEMANTIC_PROFILE=embeddinggemma_local_text_300m \
go run ./cmd/noema-eval run plan \
  --config-dir ./configs \
  --batch-plan ./examples/live/phase91-experience-pressure-cold-warm-plan.json \
  --agents opencode \
  --arms active_cold,active_learned \
  --matrix \
  --active-interpreter matrix_fork \
  --require-llm-guidance \
  --output-dir ./artifacts/phase91-experience-pressure-cold-warm-matrix-fix-verify-v4
```

Artifact:

```text
/home/jose/hpdev/Libraries/noema/programs/evaluation-platform/artifacts/phase91-experience-pressure-cold-warm-matrix-fix-verify-v4
```

Warm record:

```text
run_id=phase91-experience-pressure-cold-warm-phase91-operator-handoff-pressure-001-opencode-active-learned-seed-7
matrix_run_id=run-1eeb7bcb-14d2-41d8-8ff9-16a866e43149
logical_session_id=f122b0f9-15a7-4f91-934a-2e61d490c0d5
remote_session_id=ses_22f1e7a72ffeC0cc110QNmyEe7
delivery_id=ctx-32e7917f-a7c1-4374-ba14-39adb777c32b
suggestion_id=sug_e23998a6616ca6f2
```

## Timing Evidence

From Noema `active-sidecar-report.json`:

```json
{
  "created_at": "2026-04-27T21:39:15.883022015Z",
  "delivery": {
    "action_started_at": "2026-04-27T21:39:15.884313534Z",
    "action_returned_at": "2026-04-27T21:39:15.886789742Z",
    "action_latency_ms": 2,
    "delivered_at": "2026-04-27T21:39:32.276395079Z",
    "terminal_at": "2026-04-27T21:39:34.475504528Z",
    "delivery_lead_time_ms": 2199
  }
}
```

From Matrix trace:

```text
2026-04-27T21:39:32.275118233Z sidecar.capsule.delivered delivery_status=delivered delivery_id=ctx-32e7917f-a7c1-4374-ba14-39adb777c32b
2026-04-27T21:39:32.276395079Z run.context.attached delivery_status=delivered delivery_id=ctx-32e7917f-a7c1-4374-ba14-39adb777c32b
2026-04-27T21:39:34.475504528Z agent.message.final status=completed
2026-04-27T21:39:34.475504528Z run.completed status=completed
```

There are many normal agent/tool events before delivery, but none after delivery
and before terminal evidence.

## Outcome Evidence

Cold arm:

```text
status=succeeded
duration_ms=56131
outcome_critic.intent_satisfied=yes
outcome_critic.risk=low
matrix_cleanup.strong_cleanup=true
```

Warm active-learned arm:

```text
status=succeeded
duration_ms=112433
outcome_critic.intent_satisfied=no
outcome_critic.risk=high
active_sidecar.suggestions_delivered=1
active_sidecar.suggestions_post_delivery_activity=0
active_sidecar.matrix_live_interrupt_proven=false
matrix_cleanup.cleanup_strength=retained
matrix_cleanup.strong_cleanup=false
matrix_cleanup.process_retained=true
```

Noema generated:

```text
experience-proof-report.market_ready=false
provider-capability-report.opencode.live_attach.recommended_status=diagnostic
```

## Expected

For Noema to promote OpenCode `live_attach` from diagnostic to supported, Matrix
must show delivery early enough that at least one later agent/tool event occurs
before terminal evidence.

## Actual

`attach_context` returns quickly, but the sidecar capsule is still delivered at
the end of the run. This is transport proof, not live-consumption proof.

## Requested Check

Please investigate the remaining delivery path after action acceptance:

```text
action_returned_at=21:39:15.886Z
delivered_at=21:39:32.276Z
terminal_at=21:39:34.475Z
gap_action_return_to_delivery_ms~=16389
terminal_after_delivery_ms=2199
post_delivery_activity=0
```

If this is expected OpenCode/ACP behavior, Matrix should report the capability
as not live-intervention-suitable for this provider/path so Noema can avoid
testing it as a live sidecar lane.

## Matrix Maintainer Response

Accepted and fixed.

Root cause: the previous fix still allowed a weak `delivered` classification
when the provider returned just outside the fixed terminal-boundary window
(`2199ms` before final output vs the `2000ms` window), even though no attach
activity occurred after delivery. That made Matrix emit
`sidecar.capsule.delivered` without useful live-consumption proof.

Updated contract:

- `sidecar.capsule.delivered` is emitted only when Matrix has useful live attach
  proof: provider streaming/tool activity for the delivery or an explicit
  provider proof.
- Provider return without attach activity is now `run.context.attached`
  `delivery_status=unverified`, not `delivered`.
- Provider return near terminal completion remains
  `delivery_status=terminal_boundary`.
- `unverified`, `terminal_boundary`, and `late` must not be treated as useful
  live intervention.

This means OpenCode can remain diagnostic for Noema when it returns the attach
prompt late or without post-delivery activity. Matrix no longer marks that path
as delivered sidecar evidence.

Validation:

```text
go test ./internal/logic/runaction ./internal/providers/runapi -count=1
go run ./scripts/code_governance.go --config code-governance.toml
```

## Final Closure Addendum - 2026-04-28

The deeper investigation changed the final diagnosis from "late provider
return" to an ACP baseline semantics issue plus a Matrix observer-proof issue.

External review:

- ACP `session/prompt` is a prompt turn. The official prompt-turn docs state
  that a new `session/prompt` continues the conversation after the current turn
  completes; ACP standardizes `session/cancel` for interrupting an active turn,
  not mid-turn context injection.
- ACP `session/update` is scoped by `sessionId`, not by prompt request id.
  Without a negotiated custom `_meta` correlation extension, concurrent prompts
  on the same ACP session cannot produce reliable per-request delivery proof.
- Community prior art (`openclaw/acpx`) handles this by queueing prompts per
  session and using cooperative `session/cancel`, not by sending concurrent
  `session/prompt` requests to simulate live injection.

Final Matrix fix:

- ACP adapter now enforces one active `session/prompt` per remote session.
- Normal user prompts for the same remote session wait behind the active prompt.
- `attach_context` no longer waits behind an active ACP prompt and no longer
  sends a second concurrent prompt. It returns typed `unsupported`, allowing the
  supervisor to choose `cancel`, cancel-and-restart, or next-turn context.
- Matrix no longer treats overlapping ACP observer activity as valid live attach
  proof. A `sidecar.capsule.delivered` event requires unambiguous live proof or
  a future negotiated provider extension.
- Documentation now states that baseline ACP compatibility is not
  `supports_live_context_interrupt`, and that live attach requires a safe
  provider-specific path.

Files changed for the final fix:

- `internal/providers/agents/acp_prompt_guard.go`
- `internal/providers/agents/acp_adapter.go`
- `internal/logic/session/manager_live_context.go`
- `internal/middleware/agent.go`
- `internal/middleware/protocol.go`
- `internal/providers/agents/acp_adapter_concurrency_test.go`
- `docs/matrix_live_context_interrupt_policy.md`
- `docs/matrix_sidecar_capsules.md`
- `docs/matrix_agent_communication_run_trace.md`
- `docs/wiki/API-Reference.md`
- `docs/wiki/Sidecar-Capsules.md`

Final validation:

```text
go test ./... -count=1
go run ./scripts/code_governance.go
```

Result: both passed. This issue is closed as fixed by refusing unsafe ACP live
attach during an active prompt, rather than by relabeling late delivery as
successful.
