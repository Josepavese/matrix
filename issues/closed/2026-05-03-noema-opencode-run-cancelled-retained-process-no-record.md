# Noema OpenCode Matrix run cancels with retained process and no execution record

## Status

Closed.

## Reporter

Noema experience-layer evaluation.

## Summary

During a Noema experience-only non-interference run, Matrix returned a `capture.json` whose final event is `session.cleanup` with `clean=true` but `cleanup_strength=retained`, `strong_cleanup=false`, `process_retained=true`, and `weak_cleanup_reason=process_retained`.

The Noema runner then remained alive without writing `execution-record.json`. This blocks the batch and leaves OpenCode ACP processes running.

## Reproduction Context

Repository: `/home/jose/hpdev/Libraries/noema`

Command:

```bash
env NOEMA_SEMANTIC_PROVIDER=ollama \
  NOEMA_SEMANTIC_OLLAMA_MODEL=embeddinggemma:latest \
  NOEMA_OUTCOME_CRITIC_PROVIDER=auto \
  NOEMA_OUTCOME_CRITIC_MATRIX_URL=http://127.0.0.1:9091 \
  go run ./cmd/noema-eval run plan \
  --batch-plan examples/live/phase91-experience-normal-budget-fast-seed7-plan.json \
  --output-dir artifacts/phase91-experience-normal-budget-fast-seed7-v1 \
  --matrix \
  --active-interpreter matrix_fork \
  --require-llm-guidance \
  --parallelism 1
```

Run directory:

```text
/home/jose/hpdev/Libraries/noema/programs/evaluation-platform/artifacts/phase91-experience-normal-budget-fast-seed7-v1/runs/phase91-experience-normal-budget-fast-seed7-phase91-long-incident-runbook-001-opencode-active-cold-seed-7
```

Observed `capture.json`:

```json
{
  "RunID": "run-ca86f907-3ae9-48a4-adc5-17be423d9a5c",
  "SessionID": "6410450f-5086-4305-902d-647b9e8b75b2",
  "Outcome": {
    "Success": false,
    "Summary": "cancelled"
  }
}
```

Final event metadata:

```json
{
  "agent_id": "opencode",
  "clean": true,
  "cleanup_policy": "delete_remote_or_cancel_and_forget_local",
  "cleanup_strength": "retained",
  "local_forgotten": true,
  "process_reap_attempted": true,
  "process_reaped": true,
  "process_retained": true,
  "process_retention_allowed": true,
  "process_retention_reason": "run_related_session_retained",
  "protocol_kind": "acp",
  "remote_cancel_attempted": false,
  "remote_canceled": false,
  "remote_delete_attempted": true,
  "remote_deleted": false,
  "strong_cleanup": false,
  "weak_cleanup_reason": "process_retained",
  "warnings": [
    "remote_lifecycle_skipped_no_reusable_cached_agent_client",
    "remote_cancel_session_not_found_after_process_reap",
    "run_related_session_retained"
  ]
}
```

Processes still alive when inspected:

```text
noema-eval run plan
matrix run
opencode acp
opencode acp
```

## Expected

One of:

- Matrix provides strong cleanup proof and the Noema runner proceeds to write `execution-record.json`.
- Matrix reports a hard cleanup failure quickly enough that Noema can write a failed execution record and continue/fail-fast according to batch policy.

## Actual

- `capture.json` exists.
- `execution-record.json` is not written.
- Batch runner remains alive.
- OpenCode ACP processes remain alive.
- Cleanup metadata reports `clean=true` while also reporting retained process / weak cleanup.

## Why This Matters

Noema treats Matrix as the transport/lifecycle authority. In non-interference proof runs, Noema cannot manually repair or reinterpret Matrix lifecycle state without contaminating the evaluation. A retained process with `clean=true` but no terminal execution record blocks production-grade experience evidence.

## Requested Fix

- Make cleanup truth internally consistent: retained process should not be `clean=true` unless Matrix also exposes an explicit safe-retention proof contract that Noema can trust.
- Ensure async OpenCode runs always reach a terminal API state that lets clients write a final record.
- If a related session is retained, expose enough structured proof for the client to distinguish safe provider reuse from leaked active work.

## Resolution

Accepted and fixed in Matrix.

Matrix no longer reports run-level ephemeral cleanup as clean when a pre-existing
or shared related session is retained. The cleanup proof now returns
`clean=false`, `cleanup_strength=failed`, `failure_code=run_related_session_retained`,
`process_retained=true`, and a structured `related_sessions` entry. This makes
the API truth internally consistent for Noema and other callers: retained
related work is explicit cleanup failure, not isolated success.

`/v1/runs` cleanup handling now also treats typed cleanup errors and `clean=false`
proofs as real run cleanup failures instead of ignoring them when the transport
call itself returned nil error.

Docs updated:

- `docs/matrix_agent_communication_run_trace.md`
- `docs/matrix_protocol_neutral_runtime.md`
- `docs/wiki/API-Reference.md`

Validation:

- `go test ./internal/providers/runapi -count=1`
- `go test ./internal/logic/session ./internal/logic/sessioncleanup ./internal/providers/agents -count=1`
- `go test ./internal/providers/matrixapi -count=1`
- `go test ./...`
