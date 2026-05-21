# Noema Matrix fork interpreter cleanup still retained without related-session proof

## Status

Closed.

Resolved in Matrix by changing the `action=fork` child cleanup proof contract.
Fork child cleanup no longer reports an ambiguous `clean=true` +
`cleanup_strength=retained` proof when the only retained process is the shared
parent workspace ACP client.

New behavior:

- If the fork child remote session is deleted, closed, or canceled and the child
  mirror is forgotten, Matrix reports `fork.cleanup.strong_cleanup=true` and
  `cleanup_strength=strong`.
- Matrix records the parent session as a non-retained `related_sessions` entry
  with reason `fork_parent_agent_client_owner`.
- If the child cleanup lacks strong provider proof, Matrix fails closed with
  structured retained related-session evidence instead of returning ambiguous
  retained cleanup.

Regression coverage:

- `TestSessionManager_ForkInputCanCleanupChildAndRestoreParent` now asserts a
  strong child cleanup proof, no `process_retained`, and a parent owner
  `related_sessions` proof.

Validation:

```text
go test ./internal/logic/session ./internal/logic/sessioncleanup ./internal/providers/runapi ./internal/logic/runreconcile -count=1
go test ./...
go run ./scripts/code_governance.go --config code-governance.toml
git diff --check
```

## Reporter

Noema experience-layer evaluation.

## Summary

Matrix `0.1.17-snapshot` fixed the `/v1/runs` post-run reconcile path: when a
Matrix-backed judge run retains an OpenCode ACP client, Matrix now fails closed
with `failure_code=run_related_session_retained` and a retained
`related_sessions` entry.

However, the `session action fork` path used by Noema's `matrix_fork`
interpreter still returns retained cleanup without structured related-session
proof:

- `clean=true`
- `cleanup_strength=retained`
- `strong_cleanup=false`
- `process_retained=true`
- `process_retention_reason="fork child uses parent agent client"`
- `failure_code` empty/omitted
- `related_sessions` absent/null
- an `opencode acp` process remains alive under `matrix run`

Noema correctly rejects this as `matrix fork cleanup proof insufficient`, so
experience guidance cannot be delivered and no benchmark can be accepted.

## Matrix Version

```text
matrix 0.1.17-snapshot
commit be97bb41dabb98c5bfbbe299b25c69c40f0fd350
built 2026-05-04T10:54:54Z
```

Matrix daemon:

```text
461195 /home/jose/.local/share/matrix/bin/matrix run
```

## Noema Command

```bash
cd /home/jose/hpdev/Libraries/noema/programs/evaluation-platform
env NOEMA_SEMANTIC_PROVIDER=ollama \
  NOEMA_SEMANTIC_OLLAMA_MODEL=embeddinggemma:latest \
  NOEMA_OUTCOME_CRITIC_PROVIDER=auto \
  NOEMA_OUTCOME_CRITIC_MATRIX_URL=http://127.0.0.1:9091 \
  go run ./cmd/noema-eval run plan \
  --batch-plan examples/live/phase91-experience-web-research-matrix-fix-cold-resume-plan.json \
  --output-dir artifacts/phase91-experience-web-research-matrix-fix-cold-resume-v2 \
  --matrix \
  --active-interpreter matrix_fork \
  --require-llm-guidance \
  --parallelism 1
```

Batch artifact:

```text
/home/jose/hpdev/Libraries/noema/programs/evaluation-platform/artifacts/phase91-experience-web-research-matrix-fix-cold-resume-v2/batch-execution.json
```

## Relevant Run IDs

Noema warm run:

```text
phase91-experience-web-research-matrix-fix-cold-resume-phase91-web-research-tight-budget-001-opencode-active-learned-resume-seed-7
```

Matrix `/v1/runs` id for the warm run:

```text
run-8470b06b-9806-4984-9b4d-d14ddca9e3e6
```

Parent session used by the Matrix fork interpreter:

```text
e37164aa-7f8c-46bf-922e-d7d1de51c17e
```

Problem cleanup logical session:

```text
b67bf62e-7bd9-4a4f-90a5-9fb35c1a1ea5
```

Problem cleanup remote session:

```text
ses_20d550bdfffehA9iim4t80Tx8H
```

## Failing Warm Run Facts

```text
arm_id=active_learned_resume
status=failed
stop_reason=noema_active_sidecar_strict_guidance_render_failed
patterns_available=1
active_interpreter_fork_attempts=1
active_interpreter_fork_accepted=0
active_interpreter_fork_rejected=1
active_interpreter_fork_reject_reason=matrix fork cleanup proof insufficient
candidate_suggestions_render_failed=1
suggestions_generated=0
suggestions_resumed=0
resume_intervention_proven=false
```

## Full Problem Cleanup JSON

From the Noema execution record:

```json
{
  "agent_id": "opencode",
  "clean": true,
  "cleanup_policy": "delete_remote_or_cancel_and_forget_local",
  "cleanup_strength": "retained",
  "local_forgotten": true,
  "logical_session_id": "b67bf62e-7bd9-4a4f-90a5-9fb35c1a1ea5",
  "process_reap_attempted": false,
  "process_reaped": false,
  "process_retained": true,
  "process_retention_allowed": true,
  "process_retention_reason": "fork child uses parent agent client",
  "protocol_kind": "acp",
  "remote_cancel_attempted": true,
  "remote_canceled": true,
  "remote_close_attempted": true,
  "remote_close_unsupported": true,
  "remote_closed": false,
  "remote_delete_attempted": true,
  "remote_delete_unsupported": true,
  "remote_deleted": false,
  "remote_session_id": "ses_20d550bdfffehA9iim4t80Tx8H",
  "strong_cleanup": false,
  "weak_cleanup_reason": "process_retained"
}
```

Matrix trace confirms the same cleanup event:

```text
kind=session.cleanup
cleanup_strength=retained
failure_code=""
related_sessions=null
process_retained=true
process_retention_reason="fork child uses parent agent client"
strong_cleanup=false
```

Trace path:

```text
/home/jose/hpdev/Libraries/noema/programs/evaluation-platform/artifacts/phase91-experience-web-research-matrix-fix-cold-resume-v2/runs/phase91-experience-web-research-matrix-fix-cold-resume-phase91-web-research-tight-budget-001-opencode-active-learned-resume-seed-7/matrix-trace.json
```

## Post-Run Process Tree

Immediately after the Noema run completed:

```text
461195 /home/jose/.local/share/matrix/bin/matrix run
471854 /home/jose/.local/bin/opencode acp
```

Detailed process state:

```text
PID     PPID    PGID    SID     STAT ELAPSED CMD
461195  5923    461195  461195  Ssl  13:05   /home/jose/.local/share/matrix/bin/matrix run
471854  461195  471854  461195  Sl   02:52   /home/jose/.local/bin/opencode acp
```

## What Is Fixed

The Matrix-backed outcome critic in the same Noema run now fails closed in the
expected structured way. This is positive evidence that `/v1/runs` reconcile was
fixed.

The outcome critic error includes:

```json
{
  "status": "failed",
  "error": "session cleanup failed: run_related_session_retained",
  "cleanup": {
    "clean": false,
    "strong_cleanup": false,
    "cleanup_strength": "failed",
    "process_retained": true,
    "process_retention_reason": "run_related_session_retained",
    "related_sessions": [
      {
        "agent_id": "opencode",
        "workspace_path": "/home/jose",
        "retained": true,
        "reason": "run_related_session_retained"
      }
    ],
    "failure_code": "run_related_session_retained",
    "error": "run cleanup retained a related agent client"
  }
}
```

So the remaining gap is narrower: it is not the `/v1/runs` reconcile path in
general; it is the cleanup proof returned by the `session action fork` /
`session.cleanup` path used by Noema's active interpreter.

## Likely Cause

The previous fix was applied at `internal/logic/runreconcile/reconcile.go` and
`internal/providers/runapi`. That path only runs for `/v1/runs` aggregate
cleanup.

The active interpreter uses Matrix fork capability rather than a plain
post-run `/v1/runs` judge. The problematic retained cleanup appears to be the
fork child/session cleanup itself. That cleanup is produced by the session
manager path, not by the run-level reconcile path that now marks
`Reconcile.Retained`.

Most likely relevant files:

- `internal/logic/session/manager_fork_workflow.go`
- `internal/logic/session/manager_cleanup_process.go`
- `internal/logic/session/manager_cleanup_result.go`
- `internal/logic/session/manager_cleanup.go`
- `internal/logic/session/manager_fork.go`

The exact observed reason comes from the fork-child retention branch:

```go
process_retention_reason = "fork child uses parent agent client"
```

That action-level retained cleanup may be legitimate while the parent is still
alive, but the returned proof is not production-safe for Noema and is not
structured enough to distinguish a safe temporary child retention from a leaked
parent/fork ACP client.

## Expected

For Matrix fork interpreter cleanup, one of these must happen:

- Production-safe cleanup:
  `strong_cleanup=true`, `cleanup_strength=strong`,
  `process_retained=false`, empty `failure_code`, no retained related sessions,
  and no post-run `opencode acp`.
- Structured fail-closed cleanup:
  `clean=false`, `cleanup_strength=failed`,
  `failure_code=run_related_session_retained`, and
  `related_sessions[].retained=true`.
- If the retained child cleanup is intentionally action-local and safe, Matrix
  must still expose enough structured parent/fork lifecycle evidence for Noema
  to prove that the aggregate run cleanup later reaped or accounted for the
  parent ACP client.

## Actual

The fork interpreter cleanup returns:

```text
clean=true
cleanup_strength=retained
strong_cleanup=false
process_retained=true
failure_code=""
related_sessions=null
```

and a Matrix child `opencode acp` remains alive after the Noema run completes.

## Requested Fix

- Extend the retained-client structured proof fix to the session fork cleanup
  path, not only `/v1/runs` reconcile.
- If a fork child retains the parent agent client, either attach a structured
  parent/fork related-session proof or fail closed with
  `failure_code=run_related_session_retained`.
- Ensure the cleanup response consumed by Noema's Matrix fork interpreter is
  production-safe under the same criteria as `/v1/runs` cleanup.
- Add a regression test where session action `fork` cleans a child that uses the
  parent ACP client and verify the returned cleanup is not ambiguous.

## Acceptance Criteria

Noema can proceed with the benchmark only when this same canary produces:

- `active_interpreter_fork_accepted=1`
- `suggestions_generated>=1`
- `suggestions_resumed>=1`
- `resume_intervention_proven=true`
- all Matrix cleanup proofs used by the fork interpreter are production-safe, or
  they fail closed with structured retained related-session evidence
- no post-run `opencode acp` remains under `matrix run`

Until then, Noema must keep rejecting `matrix_fork` guidance and cannot claim
experience benchmark improvement.
