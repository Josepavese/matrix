# Noema fork/resume cleanup reports strong while parent ACP remains alive

## Status

Open.

## Reporter

Noema experience-layer evaluation.

## Summary

Matrix fixed the simple OpenCode cleanup-proof mismatch from:

```text
issues/closed/2026-05-04-noema-opencode-cleanup-proof-says-reaped-but-acp-process-remains.md
```

The simple cold/warm canary now leaves no `opencode acp` process after batch
completion. However, a fork/resume run still leaves an OpenCode ACP process alive
while the final run cleanup proof reports `clean=true`, `strong_cleanup=true`,
and `process_reaped=true`.

This appears specific to the `active_learned_resume` path where the initial
resume run retains a parent/fork client.

## Environment

Matrix binary:

```text
matrix 0.1.16-snapshot
commit fcd679a5225f0a4cfa459709cdd55092b44730e1
built 2026-05-04T07:52:40Z
```

Matrix daemon:

```text
PID 217429, started Mon May 4 09:53:19 2026
exe /home/jose/.local/share/matrix/bin/matrix
```

Noema repository:

```text
/home/jose/hpdev/Libraries/noema
```

## Passing Control Case

Command:

```bash
env NOEMA_SEMANTIC_PROVIDER=ollama \
  NOEMA_SEMANTIC_OLLAMA_MODEL=embeddinggemma:latest \
  NOEMA_OUTCOME_CRITIC_PROVIDER=auto \
  NOEMA_OUTCOME_CRITIC_MATRIX_URL=http://127.0.0.1:9091 \
  go run ./cmd/noema-eval run plan \
  --batch-plan examples/live/phase91-experience-nolegacy-cold-resume-plan.json \
  --output-dir artifacts/phase91-experience-nolegacy-cold-resume-matrix-fix-v1 \
  --matrix \
  --active-interpreter matrix_fork \
  --require-llm-guidance \
  --parallelism 1
```

Result:

```text
active_cold             succeeded clean=true strong=true process_reaped=true process_retained=false
active_learned_resume   succeeded clean=true strong=true process_reaped=true process_retained=false
```

Post-batch process state:

```text
217429 matrix run
```

No OpenCode ACP process remained. This control case is accepted.

## Failing Fork/Resume Case

Command:

```bash
env NOEMA_SEMANTIC_PROVIDER=ollama \
  NOEMA_SEMANTIC_OLLAMA_MODEL=embeddinggemma:latest \
  NOEMA_OUTCOME_CRITIC_PROVIDER=auto \
  NOEMA_OUTCOME_CRITIC_MATRIX_URL=http://127.0.0.1:9091 \
  go run ./cmd/noema-eval run plan \
  --batch-plan examples/live/phase91-experience-web-research-tight-budget-cold-resume-plan.json \
  --output-dir artifacts/phase91-experience-web-research-tight-budget-matrix-fix-v1 \
  --matrix \
  --active-interpreter matrix_fork \
  --require-llm-guidance \
  --parallelism 1
```

Batch artifact:

```text
/home/jose/hpdev/Libraries/noema/programs/evaluation-platform/artifacts/phase91-experience-web-research-tight-budget-matrix-fix-v1/batch-execution.json
```

Warm run facts:

```text
arm: active_learned_resume
status: failed
stop_reason: noema_active_sidecar_wall_timeout
patterns_available: 1
failure_scars_learned: 1
suggestions_generated: 1
suggestions_resumed: 1
resume_intervention_proven: true
```

The run correctly exercised the fork/resume path:

```text
active_resume_preflight_failure_scars=1
active_resume_preflight_actionable_failure_scars=1
active_resume_preflight_decision=interrupt_resume
active_interpreter_fork_attempts=1
active_interpreter_fork_accepted=1
```

Initial resume cleanup explicitly retained a process:

```text
active_sidecar_resume_initial_matrix_cleanup_process_retained=true
active_sidecar_resume_initial_matrix_cleanup_strength=retained
active_sidecar_resume_initial_matrix_cleanup_process_retention_reason=fork child uses parent agent client
```

Final cleanup proof then reports strong cleanup:

```json
{
  "agent_id": "opencode",
  "clean": true,
  "cleanup_strength": "strong",
  "logical_session_id": "d935a550-f4a5-489c-ab2d-18d163ab0ed3",
  "process_reap_attempted": true,
  "process_reaped": true,
  "process_retained": false,
  "protocol_kind": "acp",
  "remote_session_id": "ses_20df4b334ffesNh9fGeeosLWY0",
  "strong_cleanup": true
}
```

After batch completion and an additional 20 second wait, process state was:

```text
217429 matrix run
244648 /home/jose/.local/bin/opencode acp
```

Process details:

```text
PID     PPID    STARTED                    STAT ELAPSED TIME     CMD
244648  217429  Mon May 4 10:10:37 2026    Sl   08:34   00:00:11 /home/jose/.local/bin/opencode acp
```

This process start time matches the warm fork/resume phase, not the earlier
control canary.

## Expected

One of:

- Final cleanup reaps all ACP clients created by the fork/resume run, including
  parent/fork clients retained during initial resume handoff.
- If the parent/fork ACP client is intentionally retained, final cleanup reports
  `clean=false` or `cleanup_strength=retained/failed`, with structured
  `related_sessions`/retention evidence.
- If retention is safe, Matrix exposes explicit safe-retention proof and does
  not report `process_reaped=true` for the whole run.

## Actual

- Final run cleanup reports strong/reaped.
- A Matrix child `opencode acp` process remains alive after batch completion.
- `related_sessions` length is `0` in the final Noema record.

## Why This Matters

Noema can accept the simple-run cleanup fix, but cannot treat fork/resume cleanup
as production-grade while a retained ACP parent survives a supposedly strong
cleanup proof. This blocks clean long-run evidence for experience guidance that
uses Matrix fork interpretation and active learned resume.

## Requested Fix

- Extend the post-cleanup reconcile to include ACP clients retained by
  fork/resume parent-child handoff.
- Make final run cleanup proof account for the retained parent/fork client.
- Add a regression test where a fork/resume child uses a parent agent client and
  final cleanup cannot report strong while the parent ACP process remains alive.

## Resolution

Closed after Matrix cleanup reconciliation work landed after this report.

The current implementation addresses the reported stale strong-cleanup proof in
three layers:

- `/v1/runs` captures local-only session snapshots before and after ephemeral
  runs, so active/fork sessions created during the run are accounted for without
  spawning discovery ACP clients.
- Run cleanup records `related_sessions`; newly created owned related sessions
  are cleaned, while pre-existing/shared retained sessions downgrade the cleanup
  proof to failed with `failure_code=run_related_session_retained`.
- Post-cleanup reconcile closes cached ACP provider clients that no longer have
  a Matrix vault reference and records them as related cleanup evidence with
  reason `run_unreferenced_agent_client_reaped`.

Regression evidence:

```text
go test ./internal/providers/runapi -run 'TestHandleRuns_NewEphemeralDeleteAfterRun|TestHandleRuns_EphemeralCleanupTargetsPolicySessionWhenActiveChanges|TestHandleRuns_EphemeralCleanupIncludesNewOwnedRelatedSessions' -count=1 -v
go test ./internal/logic/session -run 'TestSessionManager_(ParentCleanupCascadesToRunningAsyncForkChild|EphemeralParentCleanupCascadesToForkChild|CleanupFallsBackToCancelAndForgetsLocalMirror|CleanupPrefersCloseBeforeCancelWhenDeleteUnsupported)' -count=1 -v
go test ./internal/providers/agents -run 'Test.*Reap|Test.*Tombstone|Test.*SessionTracking|Test.*Concurrent' -count=1 -v
```

All targeted tests passed. The cleanup contract is now: Matrix must either reap
the fork/resume ACP client, report it as cleaned related evidence, or fail the
run cleanup as retained. It cannot report `strong_cleanup=true` while an
unaccounted run-owned ACP client remains alive.
