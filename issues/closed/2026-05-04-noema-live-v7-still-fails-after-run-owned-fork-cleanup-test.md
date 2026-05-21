# Noema v7 still fails live active-resume cleanup after Matrix run-owned fork cleanup test

## Status

Closed.

## Reporter

Noema experience-layer evaluation.

## Summary

Matrix closed:

```text
issues/closed/2026-05-04-noema-active-resume-parent-owner-remediation-not-applied-in-live-run.md
```

with a new run-owned OpenCode fork cleanup integration test:

```text
tests/integration/opencode_run_owned_fork_cleanup_test.go
TestOpenCodeRunOwnedForkChildrenCleanupRemediatesParentOwnerLikeNoemaActiveResume
```

I verified that this test exists and that the Matrix-side targeted tests pass,
including the real OpenCode ACP smoke. However, the next real Noema canary
against the installed Matrix daemon still fails in the live
`active_learned_resume + matrix_fork + OpenCode` topology with the same product
blocker:

```text
standalone fork child cleanup retained its parent agent client
failure_code=run_related_session_retained
cleanup_strength=failed
process_retained=true
```

This means the Matrix test is still not isomorphic enough to the Noema live
path, or the daemon/runtime path differs from the in-process test path.

## Matrix Version

Installed binary:

```text
matrix 0.1.17-snapshot
commit be97bb41dabb98c5bfbbe299b25c69c40f0fd350
built 2026-05-04T15:59:34Z
```

Daemon observed during the run:

```text
704302 /home/jose/.local/share/matrix/bin/matrix run
```

## Verified Matrix Work

The closed issue documents this intended fix:

- `/v1/runs` ephemeral parent sessions persist `owner_run_id`.
- Run-owned fork-child cleanup accepts cleanup-owned children even when the
  caller did not set `ephemeral`.
- A fork child with strong provider proof plus `local_forgotten=true` should be
  promoted to `strong_cleanup=true`.
- The parent owner should appear as a non-retained related session with reason
  `fork_parent_agent_client_owner`.
- The parent remains restorable until final parent/run cleanup.

The new test does exercise a real OpenCode ACP process, but it uses an
in-process `session.Manager` with direct calls to `HandleSessionActionTyped`.
It does not reproduce the full daemon `/v1/runs` lifecycle and Noema live
timing.

Important shape difference observed:

- Matrix test: 2 fork artifact children, sequential, direct manager calls.
- Noema v7 live: initial active-resume parent plus multiple fork guidance
  children on the daemon channel, with overlapping route/cleanup activity.

## Matrix Tests Re-Verified

These passed before the Noema v7 canary:

```text
go test ./internal/logic/session ./internal/providers/runapi ./internal/providers/matrixapi -count=1
MATRIX_SMOKE_TEST=1 MATRIX_OPENCODE_ACP_STDIO=1 go test ./tests/integration -run TestOpenCodeRunOwnedForkChildrenCleanupRemediatesParentOwnerLikeNoemaActiveResume -count=1 -timeout 12m -v
```

The real smoke passed with OpenCode ACP and did not retain a new ACP process in
that test topology.

## Noema Canary Command

```bash
cd /home/jose/hpdev/Libraries/noema/programs/evaluation-platform
env NOEMA_SEMANTIC_PROVIDER=ollama \
  NOEMA_SEMANTIC_OLLAMA_MODEL=embeddinggemma:latest \
  NOEMA_OUTCOME_CRITIC_PROVIDER=auto \
  NOEMA_OUTCOME_CRITIC_MATRIX_URL=http://127.0.0.1:9091 \
  go run ./cmd/noema-eval run plan \
  --batch-plan examples/live/phase91-experience-web-research-matrix-fix-cold-resume-plan.json \
  --output-dir artifacts/phase91-experience-web-research-matrix-fix-cold-resume-v7 \
  --matrix \
  --active-interpreter matrix_fork \
  --require-llm-guidance \
  --parallelism 1
```

Batch artifact:

```text
/home/jose/hpdev/Libraries/noema/programs/evaluation-platform/artifacts/phase91-experience-web-research-matrix-fix-cold-resume-v7/batch-execution.json
```

## Noema v7 Result

Cold run:

```text
arm_id=active_cold
status=failed
stop_reason=noema_active_sidecar_wall_timeout
duration_ms=301886
matrix_run_id=run-010952d0-c5c9-48fa-a030-644a900de052
matrix_cleanup_clean=true
matrix_cleanup_strong=true
matrix_cleanup_process_retained=false
matrix_cleanup_strength=strong
outcome_critic_failure_scar=true
active_sidecar_failure_scars_learned=1
```

Warm run:

```text
arm_id=active_learned_resume
status=failed
duration_ms=199117
matrix_channel_id=noema-eval-channel-bea8a8ef3f1fa3c1318cac5e25faa2d3
error=active resume initial cleanup not clean: standalone fork child cleanup retained its parent agent client
```

Warm preflight selected the correct transport path:

```text
active_resume_preflight_patterns=1
active_resume_preflight_positive_routines=0
active_resume_preflight_failure_scars=1
active_resume_preflight_actionable_failure_scars=1
active_resume_preflight_decision=interrupt_resume
```

Warm cleanup proof consumed by Noema:

```text
active_sidecar_resume_initial_matrix_cleanup_proof=true
active_sidecar_resume_initial_matrix_cleanup_clean=false
active_sidecar_resume_initial_matrix_cleanup_strong=false
active_sidecar_resume_initial_matrix_cleanup_local_forgotten=true
active_sidecar_resume_initial_matrix_cleanup_remote_deleted=false
active_sidecar_resume_initial_matrix_cleanup_remote_closed=false
active_sidecar_resume_initial_matrix_cleanup_remote_canceled=true
active_sidecar_resume_initial_matrix_cleanup_process_reaped=false
active_sidecar_resume_initial_matrix_cleanup_process_retained=true
active_sidecar_resume_initial_matrix_cleanup_process_retention_allowed=true
active_sidecar_resume_initial_matrix_cleanup_strength=failed
active_sidecar_resume_initial_matrix_cleanup_weak_reason=process_retained
active_sidecar_resume_initial_matrix_cleanup_process_retention_reason=fork child uses parent agent client
active_sidecar_resume_initial_matrix_cleanup_failure_code=run_related_session_retained
active_sidecar_resume_initial_matrix_cleanup_related_sessions=1
active_sidecar_resume_initial_matrix_cleanup_related_sessions_retained=1
active_sidecar_resume_initial_matrix_cleanup_unsafe_reason=standalone fork child cleanup retained its parent agent client
```

## Runtime Evidence

Relevant Matrix runtime sequence from:

```text
/home/jose/.local/share/matrix/logs/matrix-runtime.jsonl
```

Cold run cleanup was strong:

```text
2026-05-04T18:09:50 matrix async run cancelled
run_id=run-010952d0-c5c9-48fa-a030-644a900de052
cleanup_clean=true strong_cleanup=true cleanup_strength=strong failure_code=""
```

Warm run started on channel:

```text
noema-eval-channel-bea8a8ef3f1fa3c1318cac5e25faa2d3
```

Initial active-resume logical session:

```text
a27eb164-27ab-4d07-94a3-22812ae0248c
```

Fork guidance child sessions observed:

```text
bf8c7455-c55e-4a78-ae2e-977e59cb1737
600de030-b111-494d-97e2-bcaf78995ed8
1a1b7896-5434-4e1d-9485-78ec8578bdd9
a2df7b91-b062-42c6-99c4-4d7d91df7985
7d73dfee-6b24-4540-b5dc-d00664e5a455
```

Each completed child cleanup still failed with the retained parent-client
diagnostic:

```text
target=bf8c7455-c55e-4a78-ae2e-977e59cb1737 failure_code=run_related_session_retained cleanup_strength=failed clean=false process_retained=true error="standalone fork child cleanup retained its parent agent client"
target=600de030-b111-494d-97e2-bcaf78995ed8 failure_code=run_related_session_retained cleanup_strength=failed clean=false process_retained=true error="standalone fork child cleanup retained its parent agent client"
target=1a1b7896-5434-4e1d-9485-78ec8578bdd9 failure_code=run_related_session_retained cleanup_strength=failed clean=false process_retained=true error="standalone fork child cleanup retained its parent agent client"
target=7d73dfee-6b24-4540-b5dc-d00664e5a455 failure_code=run_related_session_retained cleanup_strength=failed clean=false process_retained=true error="standalone fork child cleanup retained its parent agent client"
target=a2df7b91-b062-42c6-99c4-4d7d91df7985 failure_code=run_related_session_retained cleanup_strength=failed clean=false process_retained=true error="standalone fork child cleanup retained its parent agent client"
```

Final warm Matrix run failed closed:

```text
2026-05-04T18:12:59 matrix async run cancelled
run_id=run-69afa2e7-0749-47ad-b07d-9ba16f0a0b63
cleanup_clean=false strong_cleanup=false cleanup_strength=failed
warnings=[... run_related_session_retained run_related_session_cleanup_failed]
failure_code=run_related_session_retained
```

## Post-Run Process Evidence

After Noema v7 completed:

```text
704302 /home/jose/.local/share/matrix/bin/matrix run
712680 /home/jose/.local/bin/opencode acp
```

The retained `opencode acp` is under the Matrix daemon.

## Interpretation

Matrix has made progress: it no longer reports a misleading production-safe
cleanup when related sessions are retained, and it now has a real OpenCode ACP
regression test for one run-owned fork cleanup shape.

But the product blocker is not closed for Noema. The live daemon path still
returns `run_related_session_retained` for Noema's `matrix_fork` children and
retains an OpenCode ACP process. Noema is correctly failing closed.

Most likely causes to inspect:

- the integration test uses direct `session.Manager` calls instead of Matrix
  daemon HTTP `/v1/runs` plus session actions;
- the integration test performs two sequential fork artifact cleanups, while
  Noema creates more children with overlapping route and cleanup activity;
- the active-resume parent/child metadata differs in daemon runtime
  persistence, especially `owner_run_id`, parent linkage, `ephemeral`, or
  cleanup ownership;
- the child cleanup promotion logic may apply in `Action=fork` artifact cleanup
  but not in the standalone child cleanup path Noema receives;
- `SuppressForkParentOwnerCleanup` or equivalent guard may still be active for
  Noema's cleanup path;
- the test's final `router.Close()` may clean up state that the live daemon must
  prove during product cleanup, not after the fact.

## Required Follow-Up

Please add a Matrix-owned live-daemon regression test, not only an in-process
manager test.

The test should use the same public/runtime lifecycle that Noema exercises:

```text
1. Start/use a real Matrix daemon or the same HTTP server stack.
2. Start an async OpenCode `/v1/runs` run.
3. Create the active-resume parent session with the same metadata/persistence
   path used by `/v1/runs`.
4. Through the same Matrix API path Noema's matrix_fork interpreter uses, create
   at least five fork child sessions on the same channel/workspace.
5. Route child turns with overlapping lifetimes, not only sequentially.
6. Cleanup each child through the same standalone cleanup path returned to Noema.
7. Trigger the active-resume initial cleanup/cancel path.
8. Assert production-safe cleanup or fail closed without retained ACP process.
```

Passing criteria:

```text
each child cleanup:
  clean=true
  strong_cleanup=true
  cleanup_strength=strong
  process_retained=false
  failure_code=""
  no related_sessions[].retained=true

final run cleanup:
  clean=true
  strong_cleanup=true
  cleanup_strength=strong
  process_retained=false
  failure_code=""
  no run_related_session_retained
  no run_related_session_cleanup_failed

process tree:
  no new /home/jose/.local/bin/opencode acp retained under matrix run
```

Failing criteria:

```text
failure_code=run_related_session_retained
failure_code=fork_child_cleanup
cleanup_strength=failed
cleanup_strength=retained
process_retained=true
related_sessions[].retained=true
standalone fork child cleanup retained its parent agent client
retained opencode acp after run completion/failure
```

If this exact topology is unsupported, Matrix should expose it as an explicit
capability block instead of letting Noema discover it by live failure:

```text
agent=opencode
capability=interrupt_resume
interpreter=matrix_fork
status=blocked
reason=run_owned_fork_child_parent_owner_cleanup_not_production_safe
```

Noema should not proceed to performance benchmarking while this issue remains
open.

## Matrix Maintainer Resolution

Closed on 2026-05-04.

Matrix now separates non-destructive fork-child parent-owner proof from
destructive parent cleanup:

- A standalone fork child with a known local parent is promoted to
  `clean=true`, `strong_cleanup=true`, and `cleanup_strength=strong` when the
  child mirror is forgotten and provider lifecycle is proven by
  `remote_deleted`, `remote_closed`, `remote_canceled`, or the stdio ACP warning
  `remote_lifecycle_skipped_no_reusable_cached_agent_client`.
- The parent owner is emitted as a non-retained related session with reason
  `fork_parent_agent_client_owner`.
- Destructive fallback cleanup of the parent remains limited to run-owned
  ephemeral parent subtrees; ordinary live active-resume parents are not reaped
  during child cleanup.
- Orphan fork children with a missing local parent still fail closed with
  `failure_code=run_related_session_retained`.

Regression coverage added:

```text
go test ./internal/logic/session ./internal/providers/runapi ./internal/providers/matrixapi -count=1
go test ./... -count=1
MATRIX_SMOKE_TEST=1 MATRIX_OPENCODE_ACP_STDIO=1 go test ./tests/integration -run 'TestOpenCode.*Fork.*Cleanup' -count=1 -timeout 20m -v
```

The real OpenCode ACP smoke now covers:

- run-owned parent plus fork artifact cleanup;
- HTTP `/v1/session-actions` + `/v1/runs` parent, five standalone fork child
  route/cleanup cycles, parent restore, and final parent cleanup;
- real LLM responses for parent and child prompts;
- no new retained `opencode acp` process after cleanup.

Local deploy completed with `LOCAL_DEPLOY_OK`.
