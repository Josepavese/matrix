# Noema live attach accepted immediately but delivered at terminal boundary

Date: 2026-04-27

## Summary

Noema reran an experience-only non-interference OpenCode active-sidecar smoke after
the Matrix fork/cleanup fixes.

The previous fork-interpreter blocker is no longer the primary issue in this
run: Noema now records that the Matrix `attach_context` action returned in
`1ms`.

However, Matrix accepted the context at `21:10:45.445Z` and only emitted the
agent-visible delivery at `21:11:06.353Z`, about `20.9s` later and only
`1496ms` before `agent.message.final` / `run.completed`. No post-delivery
agent/tool activity occurred, so Noema correctly records:

```text
suggestions_delivered=1
suggestions_received_before_completion=1
suggestions_post_delivery_activity=0
matrix_live_interrupt_proven=false
```

This means the current evidence proves accepted transport, but not useful live
intervention.

## Matrix Version

```text
matrix 0.1.16-snapshot
commit fcd679a5225f0a4cfa459709cdd55092b44730e1
built 2026-04-27T16:52:33Z
```

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
  --output-dir ./artifacts/phase91-experience-pressure-cold-warm-matrix-fix-verify-v3
```

Artifact:

```text
/home/jose/hpdev/Libraries/noema/programs/evaluation-platform/artifacts/phase91-experience-pressure-cold-warm-matrix-fix-verify-v3
```

Warm record:

```text
run_id=phase91-experience-pressure-cold-warm-phase91-operator-handoff-pressure-001-opencode-active-learned-seed-7
matrix_run_id=run-49938101-64dd-4d91-9c4c-586c1ab60df2
logical_session_id=4b7db3f7-90bb-43a6-920a-e602e8c1a316
remote_session_id=ses_22f384263ffe4YuHvp9U4t6M9e
delivery_id=ctx-13182463-0a91-4931-9728-5f242eabb5b0
suggestion_id=sug_726348c814cbd688
```

## Timing Evidence

From Noema `active-sidecar-report.json`:

```json
{
  "created_at": "2026-04-27T21:10:45.444970774Z",
  "delivery": {
    "action_started_at": "2026-04-27T21:10:45.445497517Z",
    "action_returned_at": "2026-04-27T21:10:45.446942049Z",
    "action_latency_ms": 1,
    "delivered_at": "2026-04-27T21:11:06.355711641Z",
    "terminal_at": "2026-04-27T21:11:07.851718018Z",
    "delivery_lead_time_ms": 1496
  }
}
```

From Matrix trace:

```text
2026-04-27T21:10:45.445721128Z run.context.attached delivery_status=accepted delivery_id=ctx-13182463-0a91-4931-9728-5f242eabb5b0
2026-04-27T21:11:06.353501336Z sidecar.capsule.delivered delivery_status=delivered delivery_id=ctx-13182463-0a91-4931-9728-5f242eabb5b0
2026-04-27T21:11:06.355711641Z run.context.attached delivery_status=delivered delivery_id=ctx-13182463-0a91-4931-9728-5f242eabb5b0
2026-04-27T21:11:07.851718018Z agent.message.final status=completed
2026-04-27T21:11:07.851718018Z run.completed status=completed
```

There are normal agent/tool events between `accepted` and `delivered`, so the
run was active while the accepted sidecar delivery was pending.

## Expected

When `POST /v1/runs/{run_id}/actions` with `attach_context` returns accepted
while the run is active, Matrix should either:

- deliver the sidecar capsule into the active agent context early enough for
  post-delivery agent/tool activity to be possible; or
- expose a precise pending/late/terminal-boundary delivery state so Noema can
  distinguish "accepted but not practically delivered" from useful live attach.

## Actual

Matrix accepted immediately, but delivery evidence arrived only at the terminal
boundary. Noema cannot claim live active-sidecar consumption from this run even
though the action was accepted and technically delivered before completion.

## Impact On Noema

Noema can currently claim:

- real agent run completed
- LLM fork interpreter produced a suggestion
- `attach_context` HTTP action returned quickly
- Matrix emitted delivery evidence before completion

Noema cannot claim:

- useful live sidecar interruption
- foreground consumption of the suggestion
- active learned benefit from this warm arm

## Requested Check

Please verify whether this is expected OpenCode/ACP behavior or a Matrix
delivery scheduling/observer issue.

If expected, Matrix capability metadata should probably downgrade or qualify
`live_attach` for OpenCode/fork-interpreter paths as "transport accepted but not
live-intervention suitable".

If not expected, the gap to investigate is:

```text
accepted_at=21:10:45.445Z
delivered_at=21:11:06.353Z
gap_ms~=20908
terminal_after_delivery_ms=1496
post_delivery_activity=0
```

## Matrix Maintainer Response

Accepted and fixed as a Matrix delivery-proof bug.

Root cause: Matrix previously treated "the provider `attach_context` call
returned while the run was still marked running" as `delivery_status=delivered`.
That was too weak. It proved transport/provider return, but it did not prove
useful live intervention or foreground model consumption.

Implemented changes:

- `run.context.attached` now distinguishes `accepted`, `delivered`,
  `terminal_boundary`, `late`, `failed`, and `unsupported`.
- `terminal_boundary` is emitted when the provider returns near run completion
  without live attach activity proof.
- `delivered` metadata now includes `delivery_class`,
  `live_consumption_proven`, `provider_activity_events`, and
  `terminal_boundary_window_ms`.
- Matrix wraps the live attach notifier and counts provider streaming/tool
  activity tied to the specific `delivery_id`.
- `sidecar.capsule.delivered` is not emitted for terminal-boundary delivery.
- Documentation now states that `accepted` is transport acceptance only and
  useful live attach requires `live_consumption_proven=true`.

Validation:

```text
go test ./internal/logic/runaction -count=1
go test ./internal/providers/runapi -count=1
go test ./internal/logic/session -count=1
go test ./internal/logic/runnotifier -count=1
go test ./internal/providers/agents -count=1
```
