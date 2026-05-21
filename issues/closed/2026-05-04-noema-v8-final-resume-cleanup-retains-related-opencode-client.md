# Noema v8 passes initial fork cleanup but final resume cleanup retains related OpenCode client

## Status

Open.

## Reporter

Noema experience-layer evaluation.

## Summary

Matrix fixed the previous Noema blocker where `matrix_fork` child cleanup failed
with:

```text
standalone fork child cleanup retained its parent agent client
```

The latest Noema canary confirms that progress: the warm
`active_learned_resume` path now reaches resume, and the initial resume cleanup
reported to Noema is production-safe:

```text
active_sidecar_resume_initial_matrix_cleanup_clean=true
active_sidecar_resume_initial_matrix_cleanup_strong=true
active_sidecar_resume_initial_matrix_cleanup_process_retained=false
active_sidecar_resume_initial_matrix_cleanup_related_sessions=1
active_sidecar_resume_initial_matrix_cleanup_related_sessions_retained=0
```

However, the final resumed run cleanup still fails closed and leaves an OpenCode
ACP process under `matrix run`:

```text
matrix_cleanup_clean=false
matrix_cleanup_strong=false
matrix_cleanup_strength=failed
matrix_cleanup_failure_code=run_related_session_retained
matrix_cleanup_process_retained=true
matrix_cleanup_related_sessions=1
matrix_cleanup_related_sessions_retained=1
```

This means Matrix made real progress, but the full Noema
`active_learned_resume + matrix_fork + resumed run` topology is still not
production-safe.

## Matrix Version

Installed binary:

```text
matrix 0.1.17-snapshot
commit be97bb41dabb98c5bfbbe299b25c69c40f0fd350
built 2026-05-04T16:43:02Z
```

Daemon:

```text
751746 /home/jose/.local/share/matrix/bin/matrix run
started Mon May 4 18:43:50 2026
```

## Matrix Verification Before Canary

The newly closed issue:

```text
issues/closed/2026-05-04-noema-live-v7-still-fails-after-run-owned-fork-cleanup-test.md
```

claims the new contract:

- standalone fork child cleanup can be promoted to strong when child lifecycle
  proof is sufficient;
- parent owner is emitted as a non-retained related session with reason
  `fork_parent_agent_client_owner`;
- ordinary live active-resume parents are not destructively reaped during child
  cleanup;
- orphan fork children still fail closed.

I re-ran these Matrix tests locally:

```text
go test ./internal/logic/session ./internal/providers/runapi ./internal/providers/matrixapi -count=1
MATRIX_SMOKE_TEST=1 MATRIX_OPENCODE_ACP_STDIO=1 go test ./tests/integration -run 'TestOpenCode.*Fork.*Cleanup' -count=1 -timeout 20m -v
```

Both passed. The smoke included:

- `TestOpenCodeRunOwnedForkChildrenCleanupRemediatesParentOwnerLikeNoemaActiveResume`
- `TestOpenCodeHTTPStandaloneForkCleanupPromotesLiveParentOwnerLikeNoemaActiveResume`

No OpenCode ACP process remained after those tests.

## Noema Command

```bash
cd /home/jose/hpdev/Libraries/noema/programs/evaluation-platform
env NOEMA_SEMANTIC_PROVIDER=ollama \
  NOEMA_SEMANTIC_OLLAMA_MODEL=embeddinggemma:latest \
  NOEMA_OUTCOME_CRITIC_PROVIDER=auto \
  NOEMA_OUTCOME_CRITIC_MATRIX_URL=http://127.0.0.1:9091 \
  go run ./cmd/noema-eval run plan \
  --batch-plan examples/live/phase91-experience-web-research-matrix-fix-cold-resume-plan.json \
  --output-dir artifacts/phase91-experience-web-research-matrix-fix-cold-resume-v8 \
  --matrix \
  --active-interpreter matrix_fork \
  --require-llm-guidance \
  --parallelism 1
```

Batch artifact:

```text
/home/jose/hpdev/Libraries/noema/programs/evaluation-platform/artifacts/phase91-experience-web-research-matrix-fix-cold-resume-v8/batch-execution.json
```

## Noema Result

Cold arm:

```text
arm_id=active_cold
status=failed
stop_reason=noema_active_sidecar_wall_timeout
duration_ms=301826
matrix_run_id=run-24e8c5e1-08b5-49f2-9f02-1c42e92b96a4
matrix_cleanup_clean=true
matrix_cleanup_strong=true
matrix_cleanup_process_retained=false
matrix_cleanup_strength=strong
outcome_critic_failure_scar=true
```

Warm arm:

```text
arm_id=active_learned_resume
status=failed
duration_ms=535466
matrix_channel_id=noema-eval-channel-4f1dddd565a61f1d08391e561a96771b
matrix_run_id=run-af9e3efc-f97f-44a5-9ac6-0f70c9fcfba3
error=matrix cleanup proof not clean: run cleanup retained a related agent client
```

Warm preflight selected the expected path:

```text
active_resume_preflight_patterns=1
active_resume_preflight_positive_routines=0
active_resume_preflight_failure_scars=1
active_resume_preflight_actionable_failure_scars=1
active_resume_preflight_decision=interrupt_resume
```

Initial resume cleanup reported to Noema:

```text
active_sidecar_resume_initial_run_id=run-99818c88-3839-4c72-9b5b-9d78c25f7263
active_sidecar_resume_initial_matrix_cleanup_proof=true
active_sidecar_resume_initial_matrix_cleanup_clean=true
active_sidecar_resume_initial_matrix_cleanup_strong=true
active_sidecar_resume_initial_matrix_cleanup_local_forgotten=true
active_sidecar_resume_initial_matrix_cleanup_remote_canceled=true
active_sidecar_resume_initial_matrix_cleanup_process_reaped=false
active_sidecar_resume_initial_matrix_cleanup_process_retained=false
active_sidecar_resume_initial_matrix_cleanup_process_retention_allowed=false
active_sidecar_resume_initial_matrix_cleanup_strength=strong
active_sidecar_resume_initial_matrix_cleanup_related_sessions=1
active_sidecar_resume_initial_matrix_cleanup_related_sessions_retained=0
```

Interpreter/fork guidance succeeded enough to reach resume:

```text
active_interpreter_requested=matrix_fork
active_interpreter_effective=matrix_fork
active_interpreter_require_llm=true
active_interpreter_fork_attempts=1
active_interpreter_fork_accepted=1
active_interpreter_fork_rejected=0
active_interpreter_parent_session_id=2e5c2f9c-05d0-4bf8-b96d-98cfd15a7993
```

Final resumed run cleanup failed:

```text
matrix_cleanup_proof=true
matrix_cleanup_clean=false
matrix_cleanup_strong=false
matrix_cleanup_local_forgotten=true
matrix_cleanup_remote_deleted=false
matrix_cleanup_remote_closed=false
matrix_cleanup_remote_canceled=true
matrix_cleanup_process_reaped=true
matrix_cleanup_process_retained=true
matrix_cleanup_process_retention_allowed=true
matrix_cleanup_strength=failed
matrix_cleanup_weak_reason=process_retained
matrix_cleanup_process_retention_reason=run_related_session_retained
matrix_cleanup_failure_code=run_related_session_retained
matrix_cleanup_related_sessions=1
matrix_cleanup_related_sessions_retained=1
```

Final cleanup JSON from `batch-execution.json`:

```json
{
  "agent_id": "opencode",
  "clean": false,
  "cleanup_policy": "delete_remote_or_cancel_and_forget_local",
  "cleanup_strength": "failed",
  "error": "run cleanup retained a related agent client",
  "failure_code": "run_related_session_retained",
  "local_forgotten": true,
  "logical_session_id": "8a3c1480-195b-4a16-b676-34e4d408289f",
  "process_reap_attempted": true,
  "process_reaped": true,
  "process_retained": true,
  "process_retention_allowed": true,
  "process_retention_reason": "run_related_session_retained",
  "protocol_kind": "acp",
  "related_sessions": [
    {
      "agent_id": "opencode",
      "reason": "run_related_session_retained",
      "retained": true
    }
  ],
  "remote_cancel_attempted": true,
  "remote_canceled": true,
  "remote_close_attempted": true,
  "remote_close_unsupported": true,
  "remote_closed": false,
  "remote_delete_attempted": true,
  "remote_delete_unsupported": true,
  "remote_deleted": false,
  "remote_session_id": "ses_20be4e189ffe5FiVNHuDGzmUEq",
  "strong_cleanup": false,
  "weak_cleanup_reason": "process_retained"
}
```

The retained related session entry is under-specified: it does not include a
logical session id, remote session id, workspace, parent id, or reason detail
that tells Noema which Matrix session/client was retained.

## Matrix Runtime Evidence

Relevant runtime lines:

```text
2026-05-04T19:45:04 run_id=run-24e8c5e1-08b5-49f2-9f02-1c42e92b96a4 cleanup_clean=true strong_cleanup=true cleanup_strength=strong failure_code=""
2026-05-04T19:46:38 route_started logical_session=337c54c1-c32a-4881-8d63-8c17f420a31e channel=noema-eval-channel-4f1dddd565a61f1d08391e561a96771b
2026-05-04T19:47:11 route_started logical_session=ad2a844a-eb53-42f7-91ef-f6c2c598ae93 channel=noema-eval-channel-4f1dddd565a61f1d08391e561a96771b
2026-05-04T19:47:46 route_started logical_session=d979ca25-814b-40c7-88fe-8a2f8559d769 channel=noema-eval-channel-4f1dddd565a61f1d08391e561a96771b
2026-05-04T19:48:04 route_completed logical_session=ad2a844a-eb53-42f7-91ef-f6c2c598ae93
2026-05-04T19:48:10 route_completed logical_session=d979ca25-814b-40c7-88fe-8a2f8559d769
2026-05-04T19:48:27 run_id=run-99818c88-3839-4c72-9b5b-9d78c25f7263 cleanup_clean=false strong_cleanup=false cleanup_strength=failed failure_code=run_related_session_retained
2026-05-04T19:48:27 route_started logical_session=8a3c1480-195b-4a16-b676-34e4d408289f channel=noema-eval-resume-channel-c27810fe7263bfbcb25398acedb00cd6
2026-05-04T19:55:32 route_completed logical_session=8a3c1480-195b-4a16-b676-34e4d408289f
2026-05-04T19:55:33 matrix async run bridge failed run_id=run-af9e3efc-f97f-44a5-9ac6-0f70c9fcfba3 cleanup_clean=false strong_cleanup=false cleanup_strength=failed failure_code=run_related_session_retained
```

Important inconsistency to check:

```text
Noema notes for active_sidecar_resume_initial_run_id=run-99818c88-3839-4c72-9b5b-9d78c25f7263 say cleanup_clean=true.
Matrix runtime log for run-99818c88-3839-4c72-9b5b-9d78c25f7263 says cleanup_clean=false and failure_code=run_related_session_retained.
```

This may mean Noema receives a strong session-action cleanup proof while the
Matrix async-run cleanup/reconcile for the same run id later fails. If so,
Matrix needs to make those two cleanup proofs consistent or expose which proof
is authoritative.

## Post-Run Process Evidence

After the Noema batch completed:

```text
751746 /home/jose/.local/share/matrix/bin/matrix run
828647 /home/jose/.local/bin/opencode acp
```

The retained ACP process started during warm fork/resume:

```text
PID 828647
PPID 751746
STARTED Mon May 4 19:47:10 2026
CMD /home/jose/.local/bin/opencode acp
```

That start time matches the warm initial/fork phase, not the cold phase.

## Additional Diagnostic

Attempting to inspect the relevant sessions through the installed Matrix CLI
while the daemon was active failed:

```text
matrix session inspect 2e5c2f9c-05d0-4bf8-b96d-98cfd15a7993
matrix session inspect 8a3c1480-195b-4a16-b676-34e4d408289f
matrix session inspect 337c54c1-c32a-4881-8d63-8c17f420a31e

failed to open vault: vault error: [ERR_VAULT_OPEN] Failed to open bbolt database: timeout (op: bolt.NewProvider)
```

If the daemon owns the vault lock, Matrix may need a daemon-mediated inspect
path for live cleanup diagnostics.

## Interpretation

Progress is real:

- Matrix no longer fails the initial Noema fork cleanup with the old
  `standalone fork child cleanup retained its parent agent client` error.
- Noema can now reach the resumed run.
- Matrix correctly fails closed instead of claiming production-safe cleanup.

Remaining blocker:

- final resumed run cleanup still cannot clean or account for the related ACP
  client;
- the retained related session proof is too sparse to identify what remained;
- a real `opencode acp` process remains under `matrix run`;
- Noema must reject the run as non-production-safe.

## Requested Fix

Please extend the Matrix regression from the previous issue to include the full
final resumed run lifecycle:

```text
1. Start initial active-resume OpenCode run.
2. Create Matrix fork guidance artifacts.
3. Cancel/cleanup the initial run.
4. Start the resumed OpenCode run on the resume channel.
5. Let the resumed run complete or cancel.
6. Final cleanup/reconcile must account for all parent/fork/resume ACP clients.
```

Passing criteria:

```text
initial cleanup:
  clean=true
  strong_cleanup=true
  cleanup_strength=strong
  process_retained=false
  related_sessions_retained=0

final resumed run cleanup:
  clean=true
  strong_cleanup=true
  cleanup_strength=strong
  process_retained=false
  failure_code=""
  no related_sessions[].retained=true

process tree:
  no new /home/jose/.local/bin/opencode acp remains under matrix run
```

Failing criteria:

```text
failure_code=run_related_session_retained
cleanup_strength=failed
process_retained=true
related_sessions[].retained=true
related session without logical/remote/session ownership details
retained opencode acp after the resumed run cleanup
```

Until this is fixed, Noema should keep `active_learned_resume + matrix_fork +
OpenCode` blocked for performance claims.

## Resolution

Implemented in Matrix local source.

Changes made:

- run cleanup snapshots now use local-only session lists and no longer call
  mutating `status`, avoiding ghost sessions created during cleanup accounting;
- run-related same-agent sessions created in the run channel are treated as
  cleanup-owned even when they were not explicitly marked ephemeral;
- run reconcile is scoped by `agent_id + workspace_path`, so unrelated retained
  clients outside the final run workspace no longer fail the final cleanup proof;
- retained/reaped client proof now carries logical session id, remote session id,
  protocol, agent, workspace id, and workspace path;
- stdio ACP lifecycle cleanup no longer spawns a fresh provider process only to
  cancel/delete a session owned by an already reaped client;
- shared-client tombstones preserve proof per remote session, so parent/child
  cleanup can still prove process reap after one sibling consumed its proof;
- `pkg/zedacp` keeps prompt/load observers registered through a short
  post-response idle drain, preventing real providers from losing a trailing
  `agent_message_chunk` emitted immediately after `session/prompt` returns.

Verification:

```text
go test ./pkg/zedacp ./internal/providers/agents -count=1
PASS

go test ./... -count=1
PASS

MATRIX_SMOKE_TEST=1 MATRIX_OPENCODE_ACP_STDIO=1 \
  go test ./tests/integration -run 'TestOpenCode.*(Fork.*Cleanup|FinalRunCleanup)' -count=1 -timeout 25m -v
PASS
ok github.com/Josepavese/matrix/tests/integration 572.299s
```

Real OpenCode ACP evidence:

- `MATRIX_PARENT_READY`, `MATRIX_CHILD_1_OK`, and `MATRIX_CHILD_2_OK` were
  emitted and captured.
- `MATRIX_HTTP_PARENT_READY` and `MATRIX_HTTP_CHILD_1_OK` through
  `MATRIX_HTTP_CHILD_5_OK` were emitted and captured.
- `MATRIX_SCOPE_PARENT_READY`, `MATRIX_SCOPE_CHILD_READY`, and
  `MATRIX_SCOPE_FINAL_READY` were emitted and captured.
- The final Noema-like run completed with captured final output instead of
  `response_len=0`.
- The test process verified no new `opencode acp` child process remained after
  cleanup.

Status: closed.
