# Noema diagnostic: post-task fork critic parent cleanup returns HTTP 500 after active resume

Date: 2026-04-25

## Summary

During a Noema experience-only run with the opt-in post-task `matrix_fork`
outcome critic, Matrix successfully executed the task and returned a fork-child
critic artifact. The run validation passed and Noema normalized the Matrix fork
review into a learnable structural outcome fact.

The final cleanup of the manually preserved parent Matrix session then failed:

```text
cleanup matrix post-task fork parent: matrix http status=500: Internal Server Error
```

Noema did not modify Matrix source. This issue records observed behavior from
the installed Matrix runtime and keeps the blocker on the Matrix side.

## Why Noema Preserves The Parent Session

The post-task fork critic needs the parent session to remain alive after the
main task reaches terminal evidence. If Noema uses
`session_policy=new_ephemeral_delete_after_run`, Matrix cleans the parent before
the fork critic can run.

For `NOEMA_OUTCOME_CRITIC_PROVIDER=matrix_fork`, Noema therefore uses a manual
parent lifecycle:

- create a parent session through `/v1/session-actions action=new`;
- run the main Matrix task without automatic session cleanup;
- run the post-task fork critic against the still-live parent;
- cleanup the parent only after the critic returns.

This path worked for `active_cold` in the same fixture. The cleanup failure
appeared on the seeded `active_learned_resume` canary.

## Reproduction Command

```bash
cd /home/jose/hpdev/Libraries/noema/programs/evaluation-platform
NOEMA_OUTCOME_CRITIC_PROVIDER=matrix_fork /tmp/noema-eval run plan \
  --config-dir ./configs \
  --batch-plan ./examples/live/phase90-experience-noncoding-opencode-plan.json \
  --agents opencode \
  --arms active_learned_resume \
  --matrix \
  --max-runs 1 \
  --active-seed-records ./artifacts/phase90-experience-posttask-matrix-fork-critic-v3/runs/phase90-experience-noncoding-opencode-phase90-incident-summary-001-opencode-active-cold-seed-7/execution-record.json \
  --output-dir ./artifacts/phase90-experience-posttask-matrix-fork-critic-seeded-v1
```

## Noema Artifact

```text
/home/jose/hpdev/Libraries/noema/programs/evaluation-platform/artifacts/phase90-experience-posttask-matrix-fork-critic-seeded-v1/batch-execution.json
```

Diagnostic experience report:

```text
/home/jose/hpdev/Libraries/noema/programs/evaluation-platform/artifacts/phase90-experience-posttask-matrix-fork-critic-seeded-v1/experience-proof-report.json
```

The report keeps `market_ready=false` and includes the limit:

```text
Strong Matrix cleanup proof is incomplete.
```

Run result:

```json
{
  "run_id": "phase90-experience-noncoding-opencode-phase90-incident-summary-001-opencode-active-learned-resume-seed-7",
  "status": "succeeded",
  "error": "cleanup matrix post-task fork parent: matrix http status=500: Internal Server Error",
  "matrix_run_id": "run-6d2bf8c8-9008-4595-a0ac-a549eeb82a1b",
  "session_isolation_id": "d1e37612-1789-4ae2-bd67-3901d22b4394",
  "matrix_channel_id": "noema-eval-channel-11dce2f45a068ffc2a0634aa7a48fbf1",
  "matrix_session_canceled": false,
  "matrix_session_deleted": false
}
```

Relevant notes:

```text
matrix_session_policy=manual_parent_ephemeral
matrix_cleanup_policy=manual_after_outcome_critic
outcome_critic_matrix_fork_lifecycle=true
outcome_critic_matrix_fork_parent_created=true
outcome_critic_matrix_fork_parent_session_id=d1e37612-1789-4ae2-bd67-3901d22b4394
matrix_cleanup_deferred_until_outcome_critic=true
validation_passed=true
active_sidecar_patterns_available=1
active_sidecar_patterns_learned=1
outcome_critic_provider=noema.outcomecritic.matrix_fork_judge
outcome_critic_intent_satisfied=yes
outcome_critic_risk=low
outcome_critic_learnable_pattern=true
outcome_critic_parent_matrix_cleanup_proof=false
```

The Matrix fork critic response was accepted before cleanup failed:

```json
{
  "provider": "noema.outcomecritic.matrix_fork_judge",
  "intent_satisfied": "yes",
  "risk": "low",
  "learnable_pattern": true,
  "failure_scar": null,
  "facts": [
    {
      "id": "note-1",
      "kind": "intent_satisfied",
      "severity": "info",
      "structural_signature": "kind:intent_satisfied|severity:info|passed:true|provider:matrix_fork_judge|experience_review:true",
      "evidence_refs": [
        "validation[0].facts[0]",
        "artifacts[0]",
        "artifacts[4]"
      ],
      "passed": true
    }
  ]
}
```

## Control Evidence

The same manual post-task fork critic lifecycle succeeded for `active_cold`:

```text
/home/jose/hpdev/Libraries/noema/programs/evaluation-platform/artifacts/phase90-experience-posttask-matrix-fork-critic-v3/batch-execution.json
```

That run had:

```text
status=succeeded
validation_passed=true
outcome_critic_provider=noema.outcomecritic.matrix_fork_judge
outcome_critic_parent_matrix_cleanup_proof=true
outcome_critic_parent_matrix_cleanup_clean=true
matrix_cleanup_clean=true
matrix_cleanup_strong=true
matrix_cleanup_remote_canceled=true
matrix_cleanup_process_reaped=true
```

## Expected Contract

For a manually preserved ephemeral parent session used by a post-task fork
critic:

- cleanup after the fork critic should return typed cleanup proof, not HTTP 500;
- if cleanup cannot complete, Matrix should return a machine-readable failure
  code explaining whether the parent, remote ACP session, process, or child fork
  cleanup state blocked cleanup;
- `clean=true` must still require strong remote/process evidence;
- Noema should not need to classify a generic HTTP 500 after a successful critic
  artifact.

## Why This Blocks Noema

The post-task fork critic lane is useful only if Noema can preserve the parent
long enough to run the critic and still prove strict cleanup afterward.

This failure means the seeded `active_learned_resume` canary is diagnostic only:
it proves seed restoration and Matrix fork critic artifact acceptance, but it
cannot be used as clean product evidence because parent cleanup proof is absent.

## Requested Behavior

Please make `/v1/session-actions action=cleanup` for this parent session return
one of:

- successful strict cleanup proof with remote/process evidence;
- or a typed cleanup failure/warning that names the failed lifecycle phase.

Acceptance criteria:

- rerunning the command above either produces clean parent cleanup proof, or a
  typed cleanup failure without HTTP 500;
- Matrix runtime logs expose the exact parent/remote/child cleanup state;
- Noema can preserve the same strict cleanup gate without parsing raw HTTP
  error text.

## Matrix Maintainer Response

Status: accepted and fixed.

Root cause: `cleanupSessionTyped` already produced a rich `cleanup` proof, but
converted some non-clean cleanup outcomes into a raw Go error. The HTTP
`/v1/session-actions` handler treated raw router errors as generic
`500 Internal Server Error`, so callers lost `failure_code`, phase, and cleanup
state.

Fix:

- `delete` and `cleanup` now return `SessionActionResult` with both `error` and
  `cleanup` when Matrix has cleanup state to report.
- Cleanup phase errors now preserve phase-level `failure_code` values such as
  `remote_delete`, `remote_close`, `remote_cancel`, `local_forget`,
  `local_status`, `process_reap`, and `process_reap_refs`.
- Clean fallback paths still clear terminal `error`/`failure_code` when a later
  close/cancel/process proof reaches `clean=true`.
- HTTP status mapping now returns typed `502`/`409` lifecycle responses instead
  of generic `500`.
- Runtime logs now emit structured cleanup failure state including logical
  session, remote session, agent, clean/strong flags, remote lifecycle flags,
  process flags, and failure code.

Verification:

- `go test ./internal/logic/session`
- `go test ./internal/providers/matrixapi`
- `go test ./...`
- Real isolated Noema/OpenCode repro using current Matrix source on
  `http://127.0.0.1:9092`:

```bash
NOEMA_OUTCOME_CRITIC_PROVIDER=matrix_fork \
NOEMA_MATRIX_URL=http://127.0.0.1:9092 \
go run ./cmd/noema-eval run plan \
  --config-dir ./configs \
  --batch-plan ./examples/live/phase90-experience-noncoding-opencode-plan.json \
  --agents opencode \
  --arms active_learned_resume \
  --matrix \
  --matrix-base-url http://127.0.0.1:9092 \
  --max-runs 1 \
  --active-seed-records ./artifacts/phase90-experience-posttask-matrix-fork-critic-v3/runs/phase90-experience-noncoding-opencode-phase90-incident-summary-001-opencode-active-cold-seed-7/execution-record.json \
  --output-dir ./artifacts/phase90-experience-posttask-matrix-fork-critic-seeded-matrix-fix-v1
```

Result: no cleanup HTTP 500. The Noema run reached parent cleanup proof:

```text
outcome_critic_parent_matrix_cleanup_proof=true
outcome_critic_parent_matrix_cleanup_clean=true
outcome_critic_parent_matrix_cleanup_local_forgotten=true
outcome_critic_parent_matrix_cleanup_process_reaped=true
```

The reproduced run status was `failed` only because the benchmark rubric
validation failed, not because Matrix cleanup failed.
