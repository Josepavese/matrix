# Noema diagnostic: OpenCode interrupt/resume cancel emits `session not found` after clean cleanup

Date: 2026-04-24

## Summary

During a Noema experience-only `active_learned_resume` run, Matrix successfully
performed the interrupt/resume path and later validation repair passed.

However, the Matrix runtime log shows an expected Noema cancel/resume sequence
also emitting an OpenCode ACP `session/cancel` error for a session that had
already disappeared:

```text
Error handling notification ... method: "session/cancel" ... sessionId: "ses_243a55ff0ffeWpUpth4Em05URY"
code: -32602
message: "Invalid params"
data: "{\"error\":\"Session not found: ses_243a55ff0ffeWpUpth4Em05URY\"}"
matrix async run bridge failed error="ACP prompt failed: context canceled"
```

This did not block Noema's run: cleanup proof was clean and the later repair run
validated successfully. Still, the runtime evidence is noisy and ambiguous for
Noema because an expected interrupt is logged as an ACP error plus bridge
failure.

Noema did not modify Matrix source. This issue records installed-runtime
behavior observed through Matrix PAL logs.

## Noema Command

```bash
cd /home/jose/hpdev/Libraries/noema/programs/evaluation-platform
/tmp/noema-eval run plan \
  --config-dir ./configs \
  --batch-plan ./examples/live/phase84-evidence-hardening-midrun-plan.json \
  --agents opencode \
  --arms active_cold,active_learned_resume \
  --max-runs 2 \
  --matrix \
  --output-dir ./artifacts/phase84-experience-only-matrix-resume-v5
```

The command was launched through `setsid`/`nohup` style wrapping so the runner
would survive accidental terminal closure.

## Noema Artifact

```text
/home/jose/hpdev/Libraries/noema/programs/evaluation-platform/artifacts/phase84-experience-only-matrix-resume-v5/batch-execution.json
```

Relevant outcome:

```json
{
  "arm_id": "active_learned_resume",
  "status": "succeeded",
  "duration_ms": 157663,
  "active_sidecar": {
    "patterns_available": 1,
    "suggestions_generated": 1,
    "suggestions_resumed": 1,
    "validation_repair_attempts": 1,
    "validation_repair_succeeded": true,
    "resume_intervention_proven": true
  },
  "notes": [
    "active_sidecar_resume_initial_run_id=run-92183435-c6fc-4f53-9012-4e3f9b4ee587",
    "active_sidecar_resume_initial_matrix_cleanup_proof=true",
    "active_sidecar_resume_initial_matrix_cleanup_clean=true",
    "active_sidecar_resume_initial_matrix_cleanup_remote_canceled=true",
    "active_sidecar_resume_initial_matrix_cleanup_process_reaped=true",
    "active_sidecar_resume_run_id=run-1c8a51b8-fee6-45cc-a92f-d2bab0bac06b",
    "active_sidecar_validation_repair_run_id=run-443c50de-150a-468f-9f68-c1e382a11192",
    "active_sidecar_validation_repair_validation_passed=true",
    "validation_passed=true"
  ]
}
```

## Matrix Log Evidence

Source:

```text
/home/jose/.local/share/matrix/logs/matrix-runtime.jsonl
```

Relevant local-time sequence:

```text
2026-04-23T23:59:46.693+02:00 updated stored acp session mapping
  logical_session=b54fbf5a-761f-48d9-9b37-b2cffcf942bd
  agent_session=ses_243a55ff0ffeWpUpth4Em05URY

2026-04-23T23:59:46.720+02:00 acp transport closed error="stdio receive error: EOF"
2026-04-23T23:59:46.720+02:00 stdio transport: agent process exited pid=166610 wait_err="signal: killed"

2026-04-23T23:59:47.837+02:00 conversation client initialized

2026-04-23T23:59:47.838+02:00 agent stderr:
  Error handling notification {
    jsonrpc: "2.0",
    method: "session/cancel",
    params: { sessionId: "ses_243a55ff0ffeWpUpth4Em05URY" }
  } {
    code: -32602,
    message: "Invalid params",
    data: "{\"error\":\"Session not found: ses_243a55ff0ffeWpUpth4Em05URY\"}"
  }

2026-04-23T23:59:48.031+02:00 matrix async run bridge failed
  error="ACP prompt failed: context canceled"
  run_id=run-92183435-c6fc-4f53-9012-4e3f9b4ee587

2026-04-23T23:59:48.209+02:00 routing channel input
  channel=noema-eval-resume-channel-f0cd1a2fcde03bed61568bce029f707d
  input_len=1831
```

## Why This Matters To Noema

Noema treats interrupt/resume evidence as production-grade only when cleanup is
strong and the resume capsule is actually delivered through a fresh isolated
run. In this case the final Noema evidence is good, but Matrix emits error-like
runtime evidence in the middle of the expected cancellation path.

That makes downstream classification harder:

- Was this an expected cancellation side effect?
- Was Matrix sending `session/cancel` to a new ACP process that never owned the
  old session id?
- Should Noema downgrade the run, ignore the stderr, or treat it as a Matrix
  provider-lifecycle warning?

The current evidence suggests the old provider process was killed/reaped, then a
new OpenCode ACP process received a `session/cancel` notification for the old
session id and rejected it.

## Expected Contract

For a requested Noema interrupt/resume:

- If Matrix has already reaped the ACP process or otherwise proven the old
  session unreachable, it should not send `session/cancel` to a fresh ACP
  process that does not own the old session id.
- If the provider returns `Session not found` after cleanup has already proven
  process/remote cleanup, Matrix should classify that as typed benign cleanup
  evidence or a typed cleanup warning, not as a generic bridge failure.
- Expected cancel-induced `context canceled` should be mapped to
  `run.cancelled` or cleanup evidence, not logged as an ambiguous
  `matrix async run bridge failed` error.

## Requested Behavior

Please make the interrupt/resume cancellation path produce one of:

- no provider stderr/error when cancellation is already satisfied by process
  reap or remote cleanup proof;
- or a typed machine-readable warning such as:

```text
remote_cancel_session_not_found_after_process_reap
```

The important property is that Noema can distinguish expected interrupt cleanup
from a real Matrix/provider failure without scraping provider stderr.

## Acceptance Criteria

- Noema `active_learned_resume` against OpenCode can cancel the initial run,
  start the resume run, and run validation repair without Matrix logging a
  generic `matrix async run bridge failed` error for the expected cancellation.
- If `session/cancel` returns `Session not found`, Matrix records a stable typed
  cleanup warning/failure code with the relevant cleanup proof fields.
- Clean cleanup remains strict: `clean=true` should still require remote/process
  evidence such as `remote_canceled=true` or `process_reaped=true`.
- The behavior is observable in `matrix-runtime.jsonl` and/or run cleanup
  metadata without Noema parsing OpenCode stderr text.

---

## Matrix Maintainer Resolution

Accepted and fixed.

Implementation:

- async expected cancellation is now logged as `matrix async run cancelled`
  with `event=run_cancelled`, cleanup proof fields, warnings, and failure code;
- generic `matrix async run bridge failed` is reserved for unexpected async
  bridge errors;
- local stdio ACP workspace lifecycle cleanup now targets only the exact
  reusable workspace client and does not spawn a fresh ACP process just to
  delete/close/cancel a session owned by an old process;
- dead local stdio ACP clients are evicted by keepalive without prewarming a
  fresh replacement that could receive lifecycle calls for stale sessions;
- cleanup metadata now carries typed `warnings`;
- `process_reaped=true` is accepted as strong remote/process cleanup proof for
  ephemeral local stdio provider sessions;
- docs now describe cleanup warnings and the exact-client local ACP rule.

Real validation:

- local Matrix runtime from source;
- real OpenCode ACP provider;
- async `/v1/runs` with
  `session_policy=new_ephemeral_delete_after_run`;
- OpenCode created remote session
  `ses_2438dc587ffeL91Dc71vHDXsEH`;
- OpenCode emitted real `session/update`, tool call, and tool result events;
- `/v1/runs/{run_id}/actions cancel` returned accepted cancellation;
- trace recorded `session.cleanup status=completed`;
- cleanup metadata recorded `clean=true`, `strong_cleanup=true`,
  `cleanup_strength=strong`, `process_reaped=true`, `failure_code=""`, and
  warnings:
  `remote_lifecycle_skipped_no_reusable_cached_agent_client`,
  `remote_cancel_session_not_found_after_process_reap`;
- runtime log recorded `matrix async run cancelled` for
  `run-6e4291ae-3d57-49fd-94b5-a4c7ba862849`;
- no new generic `matrix async run bridge failed` was emitted for that run.

Verification:

- `go test ./internal/logic/sessioncleanup`
- `go test ./internal/providers/agents`
- `go test ./internal/providers/runapi`
- `go test ./internal/logic/session`
- `go test ./internal/providers/acp`
- `go test ./cmd/matrix`
- `go test ./...`

Status: closed.
