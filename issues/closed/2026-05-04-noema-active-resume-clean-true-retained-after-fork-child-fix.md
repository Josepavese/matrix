# Noema active resume still receives clean=true retained cleanup after fork-child fix

## Status

Closed.

## Reporter

Noema experience-layer evaluation.

## Summary

After the latest Matrix fork-child cleanup fix, the previous live failure changed
shape but is not resolved for Noema.

Good news: the `fork_child_cleanup` cascade from the prior issue no longer
appears in the Noema warm record.

Blocking issue: Noema still receives an initial active-resume cleanup proof with:

```text
clean=true
strong_cleanup=false
cleanup_strength=retained
process_retained=true
process_retention_reason="fork child uses parent agent client"
```

Noema correctly rejects this as production-unsafe. The run therefore still cannot
deliver warm guidance and cannot be used for cold/warm benchmark evidence.

There is also a post-run `opencode acp` process retained under `matrix run`.

## Matrix Version

```text
matrix 0.1.17-snapshot
commit be97bb41dabb98c5bfbbe299b25c69c40f0fd350
built 2026-05-04T13:03:18Z
```

Matrix daemon used by the run:

```text
566511 Mon May  4 15:03:36 2026 /home/jose/.local/share/matrix/bin/matrix run
```

The daemon start time is immediately after the local install/restart from the
Matrix maintainer response, so this run used the newly deployed binary.

Targeted regression tests pass locally:

```text
go test ./internal/logic/session ./internal/logic/sessioncleanup ./internal/providers/runapi ./internal/logic/runreconcile -count=1
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
  --output-dir artifacts/phase91-experience-web-research-matrix-fix-cold-resume-v4 \
  --matrix \
  --active-interpreter matrix_fork \
  --require-llm-guidance \
  --parallelism 1
```

Batch artifact:

```text
/home/jose/hpdev/Libraries/noema/programs/evaluation-platform/artifacts/phase91-experience-web-research-matrix-fix-cold-resume-v4/batch-execution.json
```

## Relevant IDs

Noema warm run:

```text
phase91-experience-web-research-matrix-fix-cold-resume-phase91-web-research-tight-budget-001-opencode-active-learned-resume-seed-7
```

Noema Matrix channel:

```text
noema-eval-channel-21c5e2ad84c800aa537022965df5da39
```

Matrix run id from runtime log:

```text
run-058f5eaf-7994-4a08-8eea-4f5e62083ef1
```

Initial active run logical session:

```text
e3415988-4c5e-4f68-93cb-3fedb167539c
```

Fork child logical sessions observed during the warm run:

```text
a1ef84e6-2226-4f39-89d2-7b5a5c764e81
ccae3d40-cd8c-4337-895c-a672fb0a72ec
e6f67d74-a941-4b40-badb-f52ce9b69ff2
```

## What Worked

The cold run behaved as expected for this canary:

```text
active_cold status=failed
stop_reason=noema_active_sidecar_wall_timeout
matrix_cleanup_clean=true
matrix_cleanup_strong=true
matrix_cleanup_process_retained=false
matrix_cleanup_strength=strong
outcome_critic_provider=noema.outcomecritic.matrix_judge
outcome_critic_failure_scar=true
active_sidecar_failure_scars_learned=1
```

The warm run also reached the correct Noema preflight decision:

```text
active_resume_preflight_patterns=1
active_resume_preflight_positive_routines=0
active_resume_preflight_failure_scars=1
active_resume_preflight_actionable_failure_scars=1
active_resume_preflight_decision=interrupt_resume
```

So the Noema experience side did find and select the failure scar. The block is
the Matrix cleanup proof before resume guidance can be accepted.

## Failing Noema Cleanup Proof

From the Noema warm execution record:

```text
active_sidecar_resume_initial_matrix_cleanup_proof=true
active_sidecar_resume_initial_matrix_cleanup_clean=true
active_sidecar_resume_initial_matrix_cleanup_strong=false
active_sidecar_resume_initial_matrix_cleanup_local_forgotten=true
active_sidecar_resume_initial_matrix_cleanup_remote_deleted=false
active_sidecar_resume_initial_matrix_cleanup_remote_closed=false
active_sidecar_resume_initial_matrix_cleanup_remote_canceled=true
active_sidecar_resume_initial_matrix_cleanup_process_reaped=false
active_sidecar_resume_initial_matrix_cleanup_process_retained=true
active_sidecar_resume_initial_matrix_cleanup_process_retention_allowed=true
active_sidecar_resume_initial_matrix_cleanup_strength=retained
active_sidecar_resume_initial_matrix_cleanup_weak_reason=process_retained
active_sidecar_resume_initial_matrix_cleanup_process_retention_reason=fork child uses parent agent client
active_sidecar_resume_initial_matrix_cleanup_unsafe_reason=process_retained
```

Noema rejected it with:

```text
active resume initial cleanup not clean: process_retained
```

This is the intended Noema policy. `clean=true` is not sufficient; production
safe cleanup requires:

```text
strong_cleanup=true
cleanup_strength=strong
process_retained=false
failure_code=""
no related_sessions[].retained=true
```

## Matrix Runtime Evidence

Relevant runtime events from `/home/jose/.local/share/matrix/logs/matrix-runtime.jsonl`:

```text
2026-05-04T16:28:13 route_started logical_session=e3415988-4c5e-4f68-93cb-3fedb167539c channel=noema-eval-channel-21c5e2ad84c800aa537022965df5da39
2026-05-04T16:28:46 route_started logical_session=a1ef84e6-2226-4f39-89d2-7b5a5c764e81 channel=noema-eval-channel-21c5e2ad84c800aa537022965df5da39
2026-05-04T16:29:18 route_started logical_session=ccae3d40-cd8c-4337-895c-a672fb0a72ec channel=noema-eval-channel-21c5e2ad84c800aa537022965df5da39
2026-05-04T16:29:19 route_started logical_session=e6f67d74-a941-4b40-badb-f52ce9b69ff2 channel=noema-eval-channel-21c5e2ad84c800aa537022965df5da39
2026-05-04T16:29:34 route_completed logical_session=e6f67d74-a941-4b40-badb-f52ce9b69ff2
2026-05-04T16:29:34 route_completed logical_session=a1ef84e6-2226-4f39-89d2-7b5a5c764e81
2026-05-04T16:29:36 route_completed logical_session=ccae3d40-cd8c-4337-895c-a672fb0a72ec
2026-05-04T16:29:38 matrix async run cancelled run_id=run-058f5eaf-7994-4a08-8eea-4f5e62083ef1 cleanup_clean=false strong_cleanup=false cleanup_strength=failed failure_code=run_related_session_retained
```

Important discrepancy:

- Noema receives the initial cleanup proof as `clean=true`,
  `cleanup_strength=retained`, `process_retained=true`.
- Matrix runtime later logs the same run cancellation as `cleanup_clean=false`,
  `cleanup_strength=failed`, `failure_code=run_related_session_retained`.

Matrix should not expose `clean=true` for a retained/process-retained cleanup to
the path Noema consumes. If Matrix later knows the run is failed/retained, the
initial cleanup proof should fail closed too.

## Post-Run Process Tree

Immediately after the Noema run completed:

```text
566511 /home/jose/.local/share/matrix/bin/matrix run
599351 /home/jose/.local/bin/opencode acp
```

Detailed tree:

```text
matrix,566511 run
  |-opencode,599351 acp
  |   |-{opencode},599352
  |   |-{opencode},599353
  |   |-{opencode},599354
  |   |-{opencode},599355
  |   |-{opencode},599356
  |   |-{opencode},599357
  |   |-{opencode},599358
  |   |-{opencode},599359
  |   |-{opencode},599363
  |   |-{opencode},599364
  |   |-{opencode},599372
  |   |-{opencode},599373
  |   `-{opencode},599384
```

So this is not only an overly conservative Noema interpretation: a related
OpenCode ACP process is still alive under Matrix after the failed warm run.

## Suspected Remaining Failure Mode

The latest fix appears to handle the explicit `fork_child_cleanup` aggregation
case when parent process proof is available. This run still exposes a retained
parent/child-client path where:

- fork children finish their artifact turns;
- the initial parent run is canceled for resume;
- Matrix reports a retained shared parent client to Noema with `clean=true`;
- `/v1/runs` final cancellation later reports retained related session failure;
- an `opencode acp` process remains under Matrix.

Likely candidates:

- the process tombstone/proof is still not associated with the initial cleanup
  object returned to Noema;
- fork children are completed but their shared client ownership still keeps the
  parent client alive;
- the run cancellation path and the session action/fork cleanup path disagree on
  whether retained cleanup is clean or failed;
- the post-fix projection of parent `process_reaped=true` to fork children does
  not fire when the parent process is still alive after cancellation.

## Requested Matrix Follow-Up

Please verify and fix the live path so that a Noema `active_learned_resume` run
with Matrix fork interpreter can either:

- return `strong_cleanup=true`, `cleanup_strength=strong`,
  `process_retained=false`, and no retained related sessions; or
- fail closed consistently with `clean=false`, a non-empty `failure_code`, and
  structured retained related-session evidence.

Acceptance criteria for Noema:

- no cleanup proof consumed by Noema may be `clean=true` when
  `process_retained=true`;
- retained fork-child/parent-client cases must not leave an `opencode acp`
  process alive after the run;
- runtime log and returned cleanup JSON must agree on clean/failed status;
- warm guidance can only proceed after initial cleanup is production-safe.

## Matrix Maintainer Response

Accepted as a generic Matrix lifecycle-proof bug, not as a Noema-specific
special case.

The root issue was that standalone cleanup of a fork child that shares the
parent ACP client could still expose a retained cleanup proof as effectively
usable by supervisors. The explicit fork cleanup path already had stronger
normalization, but the generic cleanup path used by active-resume preflight did
not fail closed early enough.

Implemented behavior:

- standalone fork-child cleanup that retains the parent agent client now fails
  closed;
- the returned cleanup proof is normalized to `clean=false`,
  `strong_cleanup=false`, `cleanup_strength=failed`;
- the public failure code is normalized to `run_related_session_retained`;
- HTTP session-actions cleanup maps that failure to `409 Conflict`;
- the related retained parent session remains present in structured
  `related_sessions` evidence;
- explicit fork cleanup and parent-subtree cleanup can still reconcile the proof
  to strong cleanup when Matrix has actual parent process proof.

Validation performed:

```text
go test ./internal/logic/session ./internal/providers/matrixapi ./internal/providers/runapi ./internal/providers/agents -count=1
go test ./internal/logic/session ./internal/providers/matrixapi -count=1
go test ./...
git diff --check
go run ./scripts/code_governance.go --config code-governance.toml
bash scripts/deploy_local.sh
```

Real installed-runtime smoke with `opencode` ACP passed after local install and
daemon restart:

```text
matrix 0.1.17-snapshot
commit be97bb41dabb98c5bfbbe299b25c69c40f0fd350
built 2026-05-04T14:40:50Z

matrix run PID 623006
```

Smoke evidence:

```text
child cleanup HTTP status: 409
error: run_related_session_retained
clean: false
strong_cleanup: false
cleanup_strength: failed
process_retained: true
process_retention_reason: fork child uses parent agent client
failure_code: run_related_session_retained
retained related sessions: 1

parent cleanup:
clean: true
strong_cleanup: true
cleanup_strength: strong
process_reaped: true
process_retained: false
failure_code: empty
```

No `opencode acp` process remained after the smoke test.
