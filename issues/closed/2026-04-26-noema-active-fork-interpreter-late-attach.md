# Noema active fork interpreter returns too late for live attach

## Summary

Noema `active_learned` now emits a startup positive-routine prior anchored on `run.started`, but the Matrix fork interpreter path returns after the parent OpenCode run has already completed. The subsequent `attach_context` action fails with HTTP `409: run is not active`.

This makes the current Matrix fork interpreter unsuitable for short live active-sidecar guidance even though capability metadata reports:

- `fork_interpreter_status=supported`
- `fork_interpreter_stability=draft`
- `fork_interpreter_active_parent_safe=true`
- `fork_interpreter_requires_idle_parent=false`
- `fork_interpreter_artifact_turn=true`

## Repro Evidence

Repository: `/home/jose/hpdev/Libraries/noema`

Command:

```bash
NOEMA_OUTCOME_CRITIC_PROVIDER=auto \
NOEMA_SEMANTIC_PROVIDER=ollama \
NOEMA_SEMANTIC_OLLAMA_MODEL=embeddinggemma:latest \
NOEMA_SEMANTIC_PROFILE=embeddinggemma_local_text_300m \
go run ./cmd/noema-eval run plan \
  --config-dir ./configs \
  --batch-plan ./examples/live/phase91-experience-coding-pressure-cold-warm-plan.json \
  --agents opencode \
  --arms active_cold,active_learned \
  --matrix \
  --active-interpreter matrix_fork \
  --require-llm-guidance \
  --output-dir ./artifacts/phase91-non-interference-coding-pressure-cold-warm-v6
```

Artifact:

`/home/jose/hpdev/Libraries/noema/programs/evaluation-platform/artifacts/phase91-non-interference-coding-pressure-cold-warm-v6`

Warm Matrix run:

`run-eabe8231-71d1-4b04-9cdb-0b1c521879e1`

Relevant timeline:

- `21:35:38.726` `run.started`
- `21:36:34.219` `agent.message.final`
- `21:36:34.219` `run.completed`
- `21:36:40.702` `run.context.attached` with `status=unsupported`, `message="run is not active"`

Noema active report confirms the suggestion was correctly anchored to startup:

```json
{
  "notes": [
    "startup_positive_routine_prior=pat_4cee0ff2192ae5ca",
    "suggestion_delivery_blocked=matrix_live_context_action_missing",
    "kernel_proposals=1"
  ],
  "suggestions": [
    {
      "source_event_id": "evt-60961a40-e28a-4641-b65d-4fb87a35f963",
      "delivery": {
        "mode": "matrix_attach_context",
        "status": "failed",
        "supported": true,
        "delivered": false,
        "reason": "matrix http status=409: run is not active"
      }
    }
  ]
}
```

## Expected

For `make_active=false`, `restore_parent=true`, active-parent-safe fork interpreter calls should return while the parent run is still active, or Matrix should report a precise capability/blocker state showing that live active fork interpretation is not timely/available for the provider/session state.

## Actual

The fork interpreter produces an accepted artifact, but late enough that Noema can only attempt `attach_context` after parent completion, producing `409 run is not active`.

## Impact

Noema can prove:

- structural observation works
- startup prior selection works
- Matrix fork LLM artifact generation works
- Matrix cleanup remains strong

Noema cannot claim live Matrix fork interpreter consumption/benefit on short active runs until this timing behavior is fixed or represented as an explicit unsupported capability.

## Follow-Up Evidence

After Noema compacted the fork interpreter prompt to `compact_structural_request_v1`, v7 improved from failed attach to accepted/delivered attach, but still too late for useful live intervention:

Artifact:

`/home/jose/hpdev/Libraries/noema/programs/evaluation-platform/artifacts/phase91-non-interference-coding-pressure-cold-warm-v7`

Warm Matrix run:

`run-46172624-d284-401d-9d40-86b71983859c`

Relevant timeline:

- `21:44:23.187` `run.started`
- `21:45:28.415` `run.context.attached` with `status=accepted`
- `21:45:33.545` `sidecar.capsule.delivered`
- `21:45:33.547` `run.context.attached` with `status=delivered`
- `21:45:34.161` `run.completed`

Noema counters:

```json
{
  "suggestions_generated": 1,
  "suggestions_delivered": 1,
  "suggestions_received_before_completion": 1,
  "suggestions_post_delivery_activity": 0,
  "fork_interpreter_attempts": 1,
  "fork_interpreter_accepted": 1
}
```

The agent-visible delivery happened after the agent had already completed the meaningful work and immediately before finalization. The Matrix-delivered message text shows the agent treated the sidecar notification as unrelated/no-op. This confirms the prompt size was not the main blocker; the active fork interpreter path still needs materially earlier artifact delivery for live sidecar usefulness.

The same issue reproduces on a non-coding operator-handoff run:

Artifact:

`/home/jose/hpdev/Libraries/noema/programs/evaluation-platform/artifacts/phase91-non-interference-operator-handoff-cold-warm-v1`

Warm Matrix run:

`run-a6fe911e-63f4-4377-b429-6a5d91c3339b`

Relevant timeline:

- `21:49:15.475` `run.started`
- `21:50:10.316` `run.context.attached` with `status=accepted`
- `21:50:16.660` `run.completed`
- `21:50:16.855` `run.context.attached` with `status=late`

The task outcome was judged successful (`intent_satisfied=yes`, `risk=low`), but the Matrix fork sidecar produced no live-consumption proof (`suggestions_delivered=0`, `suggestions_late=1`, `post_delivery_activity=0`).

## Matrix Maintainer Response

Accepted and fixed.

Root cause:

- Provider latency is real because an artifact-producing fork runs a full child LLM turn.
- Matrix made that latency worse by exposing only a synchronous `fork` artifact path: the HTTP caller waited for provider `session/fork`, child turn, cleanup, and parent restore before it could attempt `attach_context`.
- Matrix also over-described the capability. `active_parent_safe=true` meant parent session state safety, but downstream systems reasonably interpreted it as live-intervention suitability.

Implemented fix:

- `POST /v1/session-actions` now accepts `async=true` for `action=fork` when `input` is supplied.
- Matrix still performs a true provider fork before returning, then runs the child artifact turn in the background.
- The immediate response includes `fork.async=true`, `fork.job_id`, and initial `fork.job` state.
- Callers can poll `action=fork_status` with the returned job id to obtain `fork.job.status`, `fork.job.artifact`, `fork.job.cleanup`, or `fork.job.error`.
- Parent restoration happens before returning from the async fork accept path, so the active parent remains usable while the child interpreter runs.
- ACP fork capability descriptors now distinguish state safety from live timing: they expose `async_supported=true`, `blocking=true`, `artifact_streaming=false`, and `live_intervention_suitable=false`.

Expected Noema usage:

1. On early active-run signal, call `fork` with `make_active=false`, `restore_parent=true`, `async=true`, and compact `input`.
2. Continue observing the parent run.
3. Poll `fork_status`.
4. If the job reaches `completed` while the parent run is still active, use `fork.job.artifact.content` for `attach_context`.
5. If the parent completes first, classify the suggestion as late without blocking the live path.

This does not make a slow provider fast, and it does not claim streaming artifact support. It removes the Matrix-side blocking bug and makes the remaining provider-bound latency explicit and observable.
