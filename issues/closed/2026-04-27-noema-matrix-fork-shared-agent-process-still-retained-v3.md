# Matrix fork shared agent process still retained after Noema non-interference run

## Summary

Noema reran an experience-only, non-interference OpenCode cold/warm smoke after the Matrix cleanup fix was reported as resolved. The run completed successfully, and Matrix cleanup again reported `clean=true` and `cleanup_strength=strong`, but one `opencode acp` process remained alive under `matrix run` after the batch had completed and after an additional 30 second wait.

This is not a Matrix code change request from Noema. Noema only collected evidence.

## Matrix Version

```text
matrix 0.1.16-snapshot
commit fcd679a5225f0a4cfa459709cdd55092b44730e1
built 2026-04-27T16:35:09Z
```

## Noema Evidence

Artifact:

```text
/home/jose/hpdev/Libraries/noema/programs/evaluation-platform/artifacts/phase91-experience-pressure-cold-warm-matrix-fix-verify-v1
```

Plan:

```text
programs/evaluation-platform/examples/live/phase91-experience-pressure-cold-warm-plan.json
```

Run characteristics:

```text
layer_lane=experience
proof_mode=non_interference
agent=opencode
arms=active_cold,active_learned
active_interpreter=matrix_fork
semantic_provider=ollama
semantic_model=embeddinggemma:latest
```

Relevant active learned record:

```text
run_id=phase91-experience-pressure-cold-warm-phase91-operator-handoff-pressure-001-opencode-active-learned-seed-7
matrix_run_id=run-045cf829-507f-4079-a0fc-169b433bd10e
logical_session_id=3b58e0f9-5db1-4197-b487-fda8bcc5f4c4
remote_session_id=ses_2302b9fcaffe2kfHvSBCxzYHC1
active_interpreter_parent_session_id=060e4222-8b0c-47db-8f12-d94249a434a3
status=succeeded
duration_ms=75291
fork_interpreter_attempts=1
fork_interpreter_accepted=1
suggestions_delivered=1
suggestions_received_before_completion=1
min_delivery_lead_time_ms=1695
matrix_cleanup.clean=true
matrix_cleanup.cleanup_strength=strong
matrix_cleanup.remote_canceled=true
matrix_cleanup.process_reap_attempted=false
matrix_cleanup.process_reaped=false
matrix_cleanup.process_retained=true
matrix_cleanup.process_retention_allowed=true
matrix_cleanup.process_retention_reason="fork child uses parent agent client"
```

Post-run process evidence after waiting 30 seconds:

```text
PID     PPID   STAT  ELAPSED  CMD
234937  8379   Ssl   06:12    matrix run
235875  234937 Sl    04:58    /home/jose/.local/bin/opencode acp
```

Cold arm cleanup did reap its process:

```text
active_cold.matrix_cleanup.clean=true
active_cold.matrix_cleanup.cleanup_strength=strong
active_cold.matrix_cleanup.process_reap_attempted=true
active_cold.matrix_cleanup.process_reaped=true
```

## Expected

For a Noema evaluation run using `session_policy=new_ephemeral_delete_after_run` and `cleanup_policy=delete_remote_or_cancel_and_forget_local`, Matrix should either:

- reap/terminate all run-owned OpenCode ACP processes after run/fork cleanup, including shared parent workspace clients created for the run; or
- report cleanup as retained/partial rather than strong/clean when any run-related ACP process remains alive.

## Observed Problem

The new cleanup semantics explicitly report:

```text
process_retained=true
process_retention_allowed=true
process_retention_reason="fork child uses parent agent client"
cleanup_strength=strong
clean=true
```

That may be internally intentional for fork children, but from Noema's evaluation perspective it is still an un-reaped run-related process under Matrix after the batch has ended. This can contaminate later runs and makes `strong` cleanup ambiguous for `matrix_fork`.

## Requested Check

Please verify whether the retained `opencode acp` is expected to be a daemon-level reusable agent client or a run-owned parent workspace client.

If it is reusable daemon state, Matrix should expose enough metadata for Noema to distinguish it from leaked run state.

If it is run-owned, cleanup should cascade to it or downgrade the cleanup proof.

The specific ambiguity to resolve is whether `process_retention_allowed=true` is compatible with `cleanup_strength=strong` for Noema non-interference proof runs.

## Verification After Matrix Fix

Reran the same Noema smoke after Matrix was updated:

```text
matrix 0.1.16-snapshot
commit fcd679a5225f0a4cfa459709cdd55092b44730e1
built 2026-04-27T16:52:33Z
```

Artifact:

```text
/home/jose/hpdev/Libraries/noema/programs/evaluation-platform/artifacts/phase91-experience-pressure-cold-warm-matrix-fix-verify-v2
```

Result:

```text
active_cold.status=succeeded
active_cold.cleanup_strength=strong
active_cold.strong_cleanup=true
active_cold.process_reaped=true

active_learned.status=succeeded
active_learned.suggestions_delivered=1
active_learned.suggestions_received_before_completion=1
active_learned.fork_interpreter_accepted=1
active_learned.cleanup_strength=retained
active_learned.strong_cleanup=false
active_learned.process_retained=true
active_learned.weak_cleanup_reason=process_retained
active_learned.process_retention_reason="fork child uses parent agent client"
```

Post-run process check after 30 seconds still showed one retained OpenCode ACP child:

```text
12281 /home/jose/.local/bin/opencode acp
```

Assessment:

```text
fixed: cleanup proof no longer reports strong cleanup when process is retained
still true: Matrix leaves the shared OpenCode ACP process alive for matrix_fork
Noema impact: retained cleanup is now observable and should not be counted as strong cleanup evidence
```

## Matrix Maintainer Response

Accepted and fixed.

The retained process in the reported proof is a shared parent workspace ACP
client. Matrix intentionally must not reap it from the fork-child cleanup path,
because the fork child and parent share the same workspace client. However, the
issue is correct: a cleanup proof with `process_retained=true` must not also
report `cleanup_strength=strong`.

The previous semantics treated remote cancellation as strong evidence even when
local process evidence was retained. That made `strong` ambiguous for Noema
non-interference proof runs.

Implemented changes:

- `sessioncleanup.Strength` now gives retained process evidence precedence over
  remote/process strong proof. If `process_retained=true` and retention is
  allowed, the strength is `retained`, not `strong`.
- `StrongCleanup` is now derived from `cleanup_strength == strong`, so retained
  proofs cannot accidentally remain `strong_cleanup=true`.
- `WeakReason` now reports `process_retained` before considering remote strong
  proof.
- Fork-child cleanup tests now assert `clean=true`,
  `cleanup_strength=retained`, `weak_cleanup_reason=process_retained`, and
  `strong_cleanup=false`.
- Documentation now states that retained fork-child workspace clients are not
  strong cleanup proofs.

Expected behavior after the fix:

```text
process_retained=true
process_retention_allowed=true
process_retention_reason="fork child uses parent agent client"
clean=true
strong_cleanup=false
cleanup_strength=retained
weak_cleanup_reason=process_retained
```

Verification:

- `go test ./internal/logic/sessioncleanup ./internal/logic/session ./internal/providers/runapi -count=1`
- `go run ./scripts/code_governance.go --config code-governance.toml`
- `bash scripts/deploy_preflight.sh`

Status: closed.
