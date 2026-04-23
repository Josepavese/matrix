# Noema Phase 87: active fork interpreter succeeds, then live attach reports remote-session mismatch

## Context

Noema reran the Phase 87 Matrix fork interpreter lane after the fix for:

`issues/closed/2026-04-23-noema-active-run-fork-interpreter-http-500.md`

The previous HTTP `500` blocker appears resolved. Matrix now exposes the expected capability truth and Noema can attempt the active-parent fork interpreter path:

- `matrix_fork_interpreter_supported=true`
- `matrix_fork_interpreter_status=supported`
- `matrix_fork_interpreter_stability=draft`
- `matrix_fork_interpreter_active_parent_safe=true`
- `matrix_fork_interpreter_requires_idle_parent=false`
- `matrix_fork_interpreter_artifact_turn=true`

## Rerun

Repo:

```text
/home/jose/hpdev/Libraries/noema
```

Command:

```bash
cd /home/jose/hpdev/Libraries/noema/programs/evaluation-platform
go run ./cmd/noema-eval run plan \
  --config-dir ./configs \
  --batch-plan ./examples/live/phase72-active-sidecar-short-plan.json \
  --agents opencode \
  --arms active_cold,active_learned \
  --matrix \
  --active-interpreter matrix_fork \
  --output-dir ./artifacts/phase87-active-interpreter-matrix-fork-opencode-short-v2
```

Artifact directory:

```text
/home/jose/hpdev/Libraries/noema/programs/evaluation-platform/artifacts/phase87-active-interpreter-matrix-fork-opencode-short-v2
```

Relevant run:

```text
phase72-active-sidecar-short-phase72-active-short-slug-001-opencode-active-learned-seed-7
```

Relevant files:

```text
/home/jose/hpdev/Libraries/noema/programs/evaluation-platform/artifacts/phase87-active-interpreter-matrix-fork-opencode-short-v2/runs/phase72-active-sidecar-short-phase72-active-short-slug-001-opencode-active-learned-seed-7/execution-record.json
/home/jose/hpdev/Libraries/noema/programs/evaluation-platform/artifacts/phase87-active-interpreter-matrix-fork-opencode-short-v2/runs/phase72-active-sidecar-short-phase72-active-short-slug-001-opencode-active-learned-seed-7/matrix-trace.json
/home/jose/hpdev/Libraries/noema/programs/evaluation-platform/artifacts/phase87-active-interpreter-matrix-fork-opencode-short-v2/runs/phase72-active-sidecar-short-phase72-active-short-slug-001-opencode-active-learned-seed-7/active-sidecar-report.json
```

## What Improved

The original generic HTTP `500` is gone.

The active learned run completed and validation passed:

- status: `succeeded`
- duration: `127820ms`
- cleanup: `clean=true`
- `process_retained=true`
- `process_retention_allowed=true`
- `process_retention_reason=other local sessions still reference agent client`

Fork interpreter stats:

- attempts: `3`
- accepted: `1`
- rejected: `2`
- fallbacks: `2`

The two rejections are Noema-side schema validation rejections, not Matrix HTTP failures:

```text
matrix fork interpreter artifact rejected: experience interpreter response has 7 instructions, max 6
```

## New Blocking Symptom

After the fork interpreter path, Noema generated one sidecar suggestion, but Matrix live attach did not complete:

```json
{
  "suggestions_generated": 1,
  "suggestions_delivered": 0,
  "suggestions_blocked": 1,
  "matrix_live_injection_supported": false,
  "blocker": "noema_active_sidecar_suggestion"
}
```

The Matrix trace shows the same delivery id first accepted and then rejected as unsupported:

```json
[
  {
    "kind": "run.context.attached",
    "status": "accepted",
    "metadata": {
      "delivery_id": "ctx-4d22595e-6309-4c98-8eb2-11f1c447b077",
      "delivery_status": "accepted",
      "reason": "noema_active_sidecar_suggestion",
      "source_event_id": "evt-22f2014d-4d80-46f5-88b4-f73cf8f6af26"
    }
  },
  {
    "kind": "run.context.attached",
    "status": "unsupported",
    "message": "run remote session does not match active session",
    "metadata": {
      "delivery_id": "ctx-4d22595e-6309-4c98-8eb2-11f1c447b077",
      "delivery_status": "unsupported",
      "message": "run remote session does not match active session",
      "reason": "noema_active_sidecar_suggestion",
      "source_event_id": "evt-22f2014d-4d80-46f5-88b4-f73cf8f6af26"
    }
  }
]
```

The trace still reports the parent logical/remote session on the run:

```json
{
  "run": {
    "id": "run-d0df6eb0-f5a5-42b4-a899-56be379a443a",
    "agent_id": "opencode",
    "protocol": "acp",
    "logical_session_id": "93aecc82-8eda-4f06-9a1f-e45bcf6b0ac2",
    "remote_session_id": "ses_245fa4a37ffe0G58AnucBGIqLZ",
    "status": "completed"
  },
  "routing": {
    "selected_agent_id": "opencode",
    "selected_session_id": "93aecc82-8eda-4f06-9a1f-e45bcf6b0ac2",
    "selected_protocol": "acp",
    "selected_remote_session_id": "ses_245fa4a37ffe0G58AnucBGIqLZ"
  }
}
```

And session cleanup references the same logical/remote session:

```json
{
  "kind": "session.cleanup",
  "metadata": {
    "logical_session_id": "93aecc82-8eda-4f06-9a1f-e45bcf6b0ac2",
    "remote_session_id": "ses_245fa4a37ffe0G58AnucBGIqLZ",
    "clean": true,
    "process_retained": true,
    "process_retention_allowed": true
  }
}
```

## Expected Matrix Contract

If Matrix advertises:

- `active_parent_safe=true`
- `requires_idle_parent=false`
- `artifact_turn=true`

then the path should compose safely:

```text
active parent run -> fork child artifact turn -> parent preserved/restored -> attach_context to active parent
```

Noema needs one of these outcomes:

1. Live attach succeeds after a successful/accepted fork artifact path:
   - `run.context.attached status=delivered`
   - `sidecar.capsule.delivered`
   - parent logical and remote session remain compatible with the active run.

2. Or Matrix exposes an explicit capability/blocker:
   - active fork artifacts are supported, but active live attach after fork is not safe for this provider/session.
   - typed response should explain the incompatibility before Noema tries to compose the two capabilities.

## Why This Matters

The HTTP `500` issue is fixed enough for Noema to trust the Matrix fork interpreter as a diagnostic artifact provider.

However, the market-relevant active sidecar path needs both:

```text
fork interpretation accepted + live suggestion delivered into the same active run
```

Right now Noema can prove the fork interpreter adapter path, but cannot claim active in-run guidance for this lane because the suggestion is blocked after generation.

## Request

Please verify whether `session/fork` or the fork artifact turn mutates, restores, or races the active session metadata in a way that makes `AttachRunContext` compare against a different `AgentSessionID`.

Specific questions:

- Is the `accepted -> unsupported` sequence expected for the same `delivery_id`?
- During active-parent fork artifact turns, can Matrix preserve the parent `logical_session_id + remote_session_id` used by the originating run until live attach has completed or failed deterministically?
- Should Matrix expose a combined capability such as `fork.active_parent_safe && live_context.after_fork_safe`, or is this intended to be guaranteed by `active_parent_safe=true`?
- If the mismatch is expected under OpenCode ACP Draft fork behavior, can Matrix return typed capability evidence before Noema attempts this composition?

Noema-side handling stays conservative:

- deterministic interpreter remains default/fallback
- `matrix_fork` remains opt-in diagnostic
- suggestions are not counted as delivered unless Matrix proves delivery
- this evidence is not market efficacy proof

---

## Matrix Maintainer Response

Accepted.

The `accepted -> unsupported` sequence for the same `delivery_id` is expected at
the event-model level: `accepted` means Matrix queued the live attach attempt,
and `delivered`, `late`, `failed`, or `unsupported` is the final delivery
evidence. The final `unsupported` state in this run, however, exposed a real
composition bug.

Root cause:

- The active run trace received the real OpenCode ACP remote session id from
  streaming `session/update` events.
- The Matrix vault mirror for the logical session is persisted at turn
  completion.
- While the run was still active, `attach_context` compared the run-bound remote
  id against the vault mirror.
- If the mirror still held an older/stale remote id, Matrix rejected live attach
  with `run remote session does not match active session`, even though the run
  trace contained the correct active remote session id.

Implemented contract change:

- For live attach, the run trace's `logical_session_id + remote_session_id` is
  the operational SSOT.
- Matrix routes `attach_context` to the run-bound remote session, not to a
  potentially stale channel/session mirror.
- Live attach now uses strict remote-session routing: if the provider cannot
  use the exact run-bound remote session, Matrix does not silently recover into
  a replacement remote session and claim in-run delivery.
- On successful live attach, Matrix repairs the local mirror with the
  run-bound remote session id.
- `run.context.attached` metadata now includes run session evidence
  (`logical_session_id`, `remote_session_id`, `agent_id`, and `workspace_id`
  when available) so future delivery failures are auditable.

Expected Noema behavior after this fix:

- Active fork artifact generation and live attach can compose when the provider
  supports both.
- `fork.active_parent_safe=true` remains about safe parent preservation during
  fork artifact work.
- Live attach proof remains separate and must still be determined by final
  delivery evidence: `run.context.attached status=delivered` plus
  `sidecar.capsule.delivered` before run completion.
- `accepted` alone must not be counted as delivered.

Verification added in Matrix:

- unit test: live attach uses the run remote id when the vault mirror lags
- unit test: strict live routing does not recover into a replacement remote
  session when the run-bound remote session is missing
- documentation updated in run-trace, live-context policy, API reference, and
  sidecar docs
