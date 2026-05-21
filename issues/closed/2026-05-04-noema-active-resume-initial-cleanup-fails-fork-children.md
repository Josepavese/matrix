# Noema active resume initial cleanup fails after Matrix fork children

## Status

Closed.

## Reporter

Noema experience-layer evaluation.

## Summary

Matrix's latest session-fork cleanup fix improved the safety contract: Noema no
longer receives ambiguous `clean=true` retained cleanup for the fork path. In the
latest live Noema canary, Matrix now fails closed with structured evidence.

That is the correct direction, but the OpenCode `active_learned_resume` path is
still blocked in real use. The warm run reaches Noema preflight, finds one
failure scar, decides to use `interrupt_resume`, starts the initial Matrix run,
then Matrix cancels/fails that initial run because fork-child cleanup fails for
three related fork sessions.

Result: no warm guidance is delivered, so Noema cannot run a valid cold/warm
benchmark with `matrix_fork` yet.

## Matrix Version

```text
matrix 0.1.17-snapshot
commit be97bb41dabb98c5bfbbe299b25c69c40f0fd350
built 2026-05-04T12:11:56Z
```

Matrix daemon at validation time:

```text
502285 /home/jose/.local/share/matrix/bin/matrix run
```

Targeted Matrix regression tests pass locally:

```text
go test ./internal/logic/session ./internal/logic/sessioncleanup ./internal/providers/runapi ./internal/logic/runreconcile -count=1
```

So this appears to be a live integration gap not covered by the current unit
tests.

## Noema Command

```bash
cd /home/jose/hpdev/Libraries/noema/programs/evaluation-platform
env NOEMA_SEMANTIC_PROVIDER=ollama \
  NOEMA_SEMANTIC_OLLAMA_MODEL=embeddinggemma:latest \
  NOEMA_OUTCOME_CRITIC_PROVIDER=auto \
  NOEMA_OUTCOME_CRITIC_MATRIX_URL=http://127.0.0.1:9091 \
  go run ./cmd/noema-eval run plan \
  --batch-plan examples/live/phase91-experience-web-research-matrix-fix-cold-resume-plan.json \
  --output-dir artifacts/phase91-experience-web-research-matrix-fix-cold-resume-v3 \
  --matrix \
  --active-interpreter matrix_fork \
  --require-llm-guidance \
  --parallelism 1
```

Batch artifact:

```text
/home/jose/hpdev/Libraries/noema/programs/evaluation-platform/artifacts/phase91-experience-web-research-matrix-fix-cold-resume-v3/batch-execution.json
```

## Relevant IDs

Noema warm run:

```text
phase91-experience-web-research-matrix-fix-cold-resume-phase91-web-research-tight-budget-001-opencode-active-learned-resume-seed-7
```

Noema Matrix channel:

```text
noema-eval-channel-4d105f0e7f94715151191519c1f90a12
```

Matrix async run id from runtime log:

```text
run-1af81c05-8b2a-4c12-8a6a-144f514898b2
```

Initial run logical session:

```text
da5bc810-00c6-4aa7-8349-85a7a06acc47
```

Failed fork child sessions:

```text
363b142b-b558-47bf-b5e2-6d86406804bc
4c706d9c-c716-4f44-a461-d3c599751099
74b62fcb-92fe-462b-96f1-f14d2bb8a2cb
```

Corresponding remote ACP sessions from Matrix runtime log:

```text
da5bc810-00c6-4aa7-8349-85a7a06acc47 -> ses_20d063d87ffegeuNEKz2RGXpfE
363b142b-b558-47bf-b5e2-6d86406804bc -> ses_20d0561dfffejo5CrrLulPzgfe
4c706d9c-c716-4f44-a461-d3c599751099 -> ses_20d053820ffev377LSXPr14yu9
74b62fcb-92fe-462b-96f1-f14d2bb8a2cb -> ses_20d054812ffe3uTUfDrYXEi1tg
```

## What Noema Observed

The cold run is not the cleanup problem. It failed by task wall timeout and then
cleaned strongly:

```text
active_cold status=failed
stop_reason=noema_active_sidecar_wall_timeout
matrix_cleanup_clean=true
matrix_cleanup_strong=true
matrix_cleanup_process_retained=false
matrix_cleanup_strength=strong
outcome_critic_provider=noema.outcomecritic.matrix_judge
active_sidecar_failure_scars_learned=1
```

The warm run starts correctly and reaches an actionable Noema decision:

```text
active_resume_preflight_patterns=1
active_resume_preflight_positive_routines=0
active_resume_preflight_failure_scars=1
active_resume_preflight_actionable_failure_scars=1
active_resume_preflight_decision=interrupt_resume
```

Then Matrix initial cleanup fails:

```text
active_sidecar_resume_initial_matrix_cleanup_proof=true
active_sidecar_resume_initial_matrix_cleanup_clean=false
active_sidecar_resume_initial_matrix_cleanup_strong=false
active_sidecar_resume_initial_matrix_cleanup_local_forgotten=true
active_sidecar_resume_initial_matrix_cleanup_remote_deleted=false
active_sidecar_resume_initial_matrix_cleanup_remote_closed=false
active_sidecar_resume_initial_matrix_cleanup_remote_canceled=false
active_sidecar_resume_initial_matrix_cleanup_process_reaped=true
active_sidecar_resume_initial_matrix_cleanup_process_retained=true
active_sidecar_resume_initial_matrix_cleanup_process_retention_allowed=true
active_sidecar_resume_initial_matrix_cleanup_strength=failed
active_sidecar_resume_initial_matrix_cleanup_weak_reason=process_retained
active_sidecar_resume_initial_matrix_cleanup_process_retention_reason=run_related_session_retained
active_sidecar_resume_initial_matrix_cleanup_failure_code=fork_child_cleanup
active_sidecar_resume_initial_matrix_cleanup_related_sessions=4
active_sidecar_resume_initial_matrix_cleanup_related_sessions_retained=4
```

Noema error:

```text
active resume initial cleanup not clean: fork_child_cleanup: fork child 363b142b-b558-47bf-b5e2-6d86406804bc cleanup failed; fork_child_cleanup: fork child 4c706d9c-c716-4f44-a461-d3c599751099 cleanup failed; fork_child_cleanup: fork child 74b62fcb-92fe-462b-96f1-f14d2bb8a2cb cleanup failed
```

This is correctly rejected by Noema's production-safe cleanup policy.

## Matrix Runtime Evidence

Relevant runtime log lines:

```text
2026-05-04T14:32:25 route_started logical_session=da5bc810-00c6-4aa7-8349-85a7a06acc47
2026-05-04T14:33:24 route_started logical_session=363b142b-b558-47bf-b5e2-6d86406804bc
2026-05-04T14:33:30 route_started logical_session=74b62fcb-92fe-462b-96f1-f14d2bb8a2cb
2026-05-04T14:33:34 route_started logical_session=4c706d9c-c716-4f44-a461-d3c599751099
2026-05-04T14:33:42 updated stored acp session mapping logical_session=da5bc810-00c6-4aa7-8349-85a7a06acc47 agent_session=ses_20d063d87ffegeuNEKz2RGXpfE
2026-05-04T14:33:42 updated stored acp session mapping logical_session=363b142b-b558-47bf-b5e2-6d86406804bc agent_session=ses_20d0561dfffejo5CrrLulPzgfe
2026-05-04T14:33:42 updated stored acp session mapping logical_session=74b62fcb-92fe-462b-96f1-f14d2bb8a2cb agent_session=ses_20d054812ffe3uTUfDrYXEi1tg
2026-05-04T14:33:42 updated stored acp session mapping logical_session=4c706d9c-c716-4f44-a461-d3c599751099 agent_session=ses_20d053820ffev377LSXPr14yu9
2026-05-04T14:33:45 matrix session cleanup returned typed failure target=74b62fcb-92fe-462b-96f1-f14d2bb8a2cb failure_code=process_retained cleanup_strength=failed clean=false
2026-05-04T14:33:45 matrix session cleanup returned typed failure target=4c706d9c-c716-4f44-a461-d3c599751099 failure_code=process_retained cleanup_strength=failed clean=false
2026-05-04T14:33:45 matrix session cleanup returned typed failure target=363b142b-b558-47bf-b5e2-6d86406804bc failure_code=process_retained cleanup_strength=failed clean=false
2026-05-04T14:33:48 matrix session cleanup returned typed failure target=da5bc810-00c6-4aa7-8349-85a7a06acc47 failure_code=fork_child_cleanup cleanup_strength=failed clean=false
2026-05-04T14:33:52 matrix async run cancelled run_id=run-1af81c05-8b2a-4c12-8a6a-144f514898b2 cleanup_clean=false strong_cleanup=false cleanup_strength=failed failure_code=fork_child_cleanup
```

The exact child cleanup failures in the log:

```text
target=74b62fcb-92fe-462b-96f1-f14d2bb8a2cb remote_session_id=ses_20d054812ffe3uTUfDrYXEi1tg failure_code=process_retained clean=false strong_cleanup=false local_forgotten=true remote_deleted=false remote_closed=false remote_canceled=false process_reaped=false process_retained=true

target=4c706d9c-c716-4f44-a461-d3c599751099 remote_session_id=ses_20d053820ffev377LSXPr14yu9 failure_code=process_retained clean=false strong_cleanup=false local_forgotten=true remote_deleted=false remote_closed=false remote_canceled=false process_reaped=false process_retained=true

target=363b142b-b558-47bf-b5e2-6d86406804bc remote_session_id=ses_20d0561dfffejo5CrrLulPzgfe failure_code=process_retained clean=false strong_cleanup=false local_forgotten=true remote_deleted=false remote_closed=false remote_canceled=false process_reaped=false process_retained=true
```

Post-run process tree did not show a leftover `opencode acp` process:

```text
502285 /home/jose/.local/share/matrix/bin/matrix run
```

So the issue may be proof/ownership ordering rather than a long-lived child
process leak, but Noema has to fail closed because Matrix reports retained
related sessions and failed cleanup.

## Suspected Failure Mode

The previous issue fixed one fork-child cleanup proof shape: a fork child using
the parent agent client can be strong if remote cleanup is proven and the child
mirror is forgotten.

This live run appears different:

- Noema's warm run creates an initial active run session.
- The Matrix fork interpreter creates multiple fork child sessions during
  guidance rendering.
- Matrix kills/reaps the local OpenCode process for the initial run.
- Child cleanup then reports `failure_code=process_retained` with no remote
  delete/close/cancel proof.
- Parent/initial cleanup aggregates those child failures as
  `failure_code=fork_child_cleanup`.

My current hypothesis is that fork children created during an initial run are
being cleaned after the shared ACP client has already been killed or detached,
so child cleanup can no longer produce strong remote lifecycle proof. Matrix
then correctly fails closed, but the ordering prevents production-safe cleanup.

Alternative possibility: these children are not semantically retained, because
their underlying process was already reaped, but Matrix lacks a nested proof that
the retained process reference is only a dead/shared parent client. If that is
the intended interpretation, Matrix should emit non-retained child related
sessions with an explicit reason rather than `process_retained=true`.

## Expected Production Contract

For Noema to accept OpenCode `active_learned_resume` with `matrix_fork`, the
initial run cleanup must satisfy all of:

```text
strong_cleanup=true
cleanup_strength=strong
process_retained=false
failure_code=""
related_sessions has no retained=true entries
```

If Matrix cannot prove that, fail-closed is correct, but then Noema must treat
this capability as not production-ready and must not run benchmark claims on it.

## Requested Matrix Follow-Up

Please verify whether the initial active run cleanup should be able to clean
fork children before killing/detaching the shared ACP client, or whether child
cleanup should inherit a strong proof from the parent process reap when the child
session is only a logical mirror over the same already-killed client.

Useful acceptance criteria:

- A Noema `active_learned_resume` run with Matrix fork interpreter can complete
  its initial cleanup with `strong_cleanup=true`.
- No child fork cleanup reports `failure_code=process_retained` after the shared
  ACP process is already killed.
- If a child really is retained, Matrix includes nested child cleanup details in
  `related_sessions` or an equivalent structured field so Noema can report the
  exact retained owner.
- `failure_code=fork_child_cleanup` remains fail-closed when proof is missing.

## Matrix Maintainer Response

Accepted and fixed as a generic Matrix lifecycle bug, not as a Noema-specific
special case.

Root causes:

- Parent cleanup evaluated fork-child cleanup failures before the parent process
  proof was available, so children that only retained the shared parent ACP
  client could be reported as failed even when the parent process was later
  reaped.
- `/v1/runs` related-session accounting could issue a second cleanup for fork
  children already covered by the primary parent cleanup `fork_children` proof.
- Real OpenCode testing exposed a stricter edge: when a local stdio ACP client
  died and was replaced, Matrix could drop or ignore the old process tombstone,
  causing parent cleanup to miss `process_reaped=true` for older fork sessions.

Implemented contract:

- Fork children are cleaned first, but their final proof is reconciled after the
  parent cleanup has remote/process evidence.
- If a child has provider lifecycle proof, it is strong on its own.
- If child provider proof is unavailable but the child only retained the shared
  parent client and parent cleanup has `process_reaped=true`, Matrix projects
  that process proof into the child cleanup and reports the child as
  `strong_cleanup=true`.
- If neither child provider proof nor parent process proof exists, Matrix still
  fails closed with `fork_child_cleanup` / retained evidence.
- `/v1/runs` now treats fork children already present in a clean parent
  `fork_children` proof as `run_related_session_cleaned` and does not double
  clean them.
- The ACP router preserves and consumes session-bound tombstones for dead or
  replaced local stdio clients, including the case where a newer live client for
  the same workspace does not track the old remote session.

Validation:

- `go test ./internal/providers/agents ./internal/logic/session ./internal/providers/runapi -count=1`
- `go test ./...`
- `go run ./scripts/code_governance.go --config code-governance.toml`
- `bash scripts/deploy_local.sh`
- Local install and daemon restart completed.
- Real OpenCode ACP fork cascade smoke passed: one parent session, three real
  provider fork children, parent cleanup returned `clean=true`,
  `strong_cleanup=true`, `process_reaped=true`, `fork_children_cleaned=3`, and
  every child returned `clean=true`, `strong_cleanup=true`, no
  `process_retained`, no `failure_code`, and no retained related session.
