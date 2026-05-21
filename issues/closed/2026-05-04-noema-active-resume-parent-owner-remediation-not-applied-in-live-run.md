# Noema active resume still retains fork children after parent-owner remediation fix

## Status

Open.

## Reporter

Noema experience-layer evaluation.

## Summary

Matrix closed `2026-05-04-noema-active-resume-fails-closed-but-retains-opencode-client.md`
with a parent-owner remediation fix:

- forced cleanup of a run-owned ephemeral fork child should remediate
  `fork child uses parent agent client` by cleaning the ephemeral parent owner
  as a subtree;
- after parent owner cleanup proves the shared client is closed/reaped/gone, the
  child cleanup should be promoted to strong cleanup;
- a real installed-runtime OpenCode smoke reportedly passed with no remaining
  `opencode acp` process.

The same Noema canary was rerun against the newly installed daemon, but the live
path still fails exactly like the previous canary:

- `active_learned_resume` finds the scar and chooses `interrupt_resume`;
- Matrix fork guidance child sessions are created and route to completion;
- child cleanup still fails as standalone retained parent-client cleanup;
- Noema fails before guidance can be resumed;
- an `opencode acp` process remains alive under `matrix run`.

This suggests the remediation path does not apply to Noema's actual
`active_learned_resume` fork-cleanup sequence, or one of its preconditions does
not match the live Noema sessions.

## Matrix Version

```text
matrix 0.1.17-snapshot
commit be97bb41dabb98c5bfbbe299b25c69c40f0fd350
built 2026-05-04T15:16:39Z
```

Matrix daemon used by the run:

```text
648613 Mon May  4 17:16:51 2026 /home/jose/.local/share/matrix/bin/matrix run
```

No `opencode acp` process existed before the canary.

Targeted tests passed before the run:

```text
go test ./internal/logic/session ./internal/providers/runapi -count=1
```

Noema-side tests also passed:

```text
go test ./internal/matrixbridge ./internal/metacore/outcomecritic ./internal/layers/experience/interpreter/matrixfork
go test ./pkg/evalplatform
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
  --output-dir artifacts/phase91-experience-web-research-matrix-fix-cold-resume-v6 \
  --matrix \
  --active-interpreter matrix_fork \
  --require-llm-guidance \
  --parallelism 1
```

Batch artifact:

```text
/home/jose/hpdev/Libraries/noema/programs/evaluation-platform/artifacts/phase91-experience-web-research-matrix-fix-cold-resume-v6/batch-execution.json
```

## Relevant IDs

Noema warm run:

```text
phase91-experience-web-research-matrix-fix-cold-resume-phase91-web-research-tight-budget-001-opencode-active-learned-resume-seed-7
```

Noema Matrix channel:

```text
noema-eval-channel-9de50f095b1560b122389ee55dbf7175
```

Matrix run id from runtime log:

```text
run-f551083e-716c-4b15-8b48-d8b2685c1494
```

Initial active run logical session:

```text
df49767f-f55b-4829-85d4-b384517870ee
```

Fork child logical sessions observed during warm guidance rendering:

```text
2bf9cf69-d825-439f-9387-8f83129534e8
29b5830b-399e-490e-b176-cb8f3c2bc82e
783a360a-9833-4b1c-8aca-8e257aedd207
7d7c9ca5-69f9-491e-8aa6-9e4b7211e22a
```

## Noema Result

Cold run:

```text
active_cold status=failed
stop_reason=noema_active_sidecar_wall_timeout
matrix_cleanup_clean=true
matrix_cleanup_strong=true
matrix_cleanup_process_retained=false
matrix_cleanup_strength=strong
outcome_critic_failure_scar=true
active_sidecar_failure_scars_learned=1
```

Warm run:

```text
active_learned_resume status=failed
error=active resume initial cleanup not clean: standalone fork child cleanup retained its parent agent client
duration_ms=175118
active_resume_preflight_patterns=1
active_resume_preflight_failure_scars=1
active_resume_preflight_actionable_failure_scars=1
active_resume_preflight_decision=interrupt_resume
```

Warm cleanup proof consumed by Noema:

```text
active_sidecar_resume_initial_matrix_cleanup_clean=false
active_sidecar_resume_initial_matrix_cleanup_strong=false
active_sidecar_resume_initial_matrix_cleanup_local_forgotten=true
active_sidecar_resume_initial_matrix_cleanup_remote_canceled=true
active_sidecar_resume_initial_matrix_cleanup_process_reaped=false
active_sidecar_resume_initial_matrix_cleanup_process_retained=true
active_sidecar_resume_initial_matrix_cleanup_process_retention_allowed=true
active_sidecar_resume_initial_matrix_cleanup_strength=failed
active_sidecar_resume_initial_matrix_cleanup_process_retention_reason=fork child uses parent agent client
active_sidecar_resume_initial_matrix_cleanup_failure_code=run_related_session_retained
active_sidecar_resume_initial_matrix_cleanup_related_sessions=1
active_sidecar_resume_initial_matrix_cleanup_related_sessions_retained=1
active_sidecar_resume_initial_matrix_cleanup_unsafe_reason=standalone fork child cleanup retained its parent agent client
```

## Matrix Runtime Evidence

Relevant runtime log lines:

```text
2026-05-04T17:23:24 matrix async run cancelled run_id=run-36feda77-92fa-4c8e-bf19-bf97496e8811 cleanup_clean=true strong_cleanup=true cleanup_strength=strong failure_code=""
2026-05-04T17:24:35 route_started logical_session=df49767f-f55b-4829-85d4-b384517870ee channel=noema-eval-channel-9de50f095b1560b122389ee55dbf7175
2026-05-04T17:25:08 route_started logical_session=2bf9cf69-d825-439f-9387-8f83129534e8
2026-05-04T17:25:40 route_started logical_session=29b5830b-399e-490e-b176-cb8f3c2bc82e
2026-05-04T17:25:41 route_started logical_session=783a360a-9833-4b1c-8aca-8e257aedd207
2026-05-04T17:25:41 route_started logical_session=7d7c9ca5-69f9-491e-8aa6-9e4b7211e22a
2026-05-04T17:25:59 route_completed logical_session=7d7c9ca5-69f9-491e-8aa6-9e4b7211e22a
2026-05-04T17:26:00 cleanup failed target=7d7c9ca5-69f9-491e-8aa6-9e4b7211e22a failure_code=run_related_session_retained cleanup_strength=failed clean=false process_retained=true
2026-05-04T17:26:01 route_completed logical_session=783a360a-9833-4b1c-8aca-8e257aedd207
2026-05-04T17:26:01 cleanup failed target=783a360a-9833-4b1c-8aca-8e257aedd207 failure_code=run_related_session_retained cleanup_strength=failed clean=false process_retained=true
2026-05-04T17:26:02 route_completed logical_session=29b5830b-399e-490e-b176-cb8f3c2bc82e
2026-05-04T17:26:03 cleanup failed target=29b5830b-399e-490e-b176-cb8f3c2bc82e failure_code=run_related_session_retained cleanup_strength=failed clean=false process_retained=true
2026-05-04T17:26:04 route_completed logical_session=2bf9cf69-d825-439f-9387-8f83129534e8
2026-05-04T17:26:05 cleanup failed target=2bf9cf69-d825-439f-9387-8f83129534e8 failure_code=run_related_session_retained cleanup_strength=failed clean=false process_retained=true
2026-05-04T17:26:06 matrix async run cancelled run_id=run-f551083e-716c-4b15-8b48-d8b2685c1494 cleanup_clean=false strong_cleanup=false cleanup_strength=failed failure_code=run_related_session_retained
```

The error is still:

```text
standalone fork child cleanup retained its parent agent client
```

## Post-Run Process Tree

Immediately after Noema completed:

```text
648613 /home/jose/.local/share/matrix/bin/matrix run
655508 /home/jose/.local/bin/opencode acp
```

Detailed tree:

```text
matrix,648613 run
  |-opencode,655508 acp
  |   |-{opencode},655511
  |   |-{opencode},655512
  |   |-{opencode},655513
  |   |-{opencode},655514
  |   |-{opencode},655515
  |   |-{opencode},655516
  |   |-{opencode},655517
  |   |-{opencode},655518
  |   |-{opencode},655523
  |   |-{opencode},655524
  |   |-{opencode},655525
  |   |-{opencode},655543
  |   `-{opencode},655544
```

## Interpretation

The parent-owner remediation does not appear to run in this live Noema path.

Likely causes to verify:

- one or more remediation preconditions are false in Noema-created sessions
  (`child.Ephemeral`, `parent.Ephemeral`, parent cleanup policy, workspace path,
  agent id, or `ForceForgetLocal`);
- Noema's Matrix fork guidance cleanup is still using a path where
  `SuppressForkParentOwnerCleanup=true` unintentionally applies to a standalone
  run-owned child cleanup;
- parent lookup via `ParentSessionID` fails for these Matrix fork children;
- the smoke test exercises a simpler parent-child topology than the actual
  `active_learned_resume` sequence, which creates multiple concurrent fork
  guidance children.

## Requested Matrix Follow-Up

Please add diagnostics or tests that distinguish why
`cleanupRunOwnedForkParentOwnerFromStandaloneChild` did not remediate the
Noema live run.

Acceptance criteria for Noema remain:

- warm `active_learned_resume` either reaches production-safe initial cleanup and
  can resume guidance, or fails closed without retaining `opencode acp`;
- no benchmark should proceed while the live run still leaves retained ACP
  clients;
- if this topology is unsupported, Matrix should expose that as capability state
  rather than requiring Noema to discover it by failed canaries.

## Required Matrix-Only Acceptance Test

Please add a Matrix-owned regression/integration test that is isomorphic to the
Noema `active_learned_resume + matrix_fork + OpenCode` path, without importing
or invoking Noema.

The goal is to stop the current loop where Matrix passes a narrower smoke test
but fails in the real Noema topology. The test should reproduce the lifecycle
shape, not Noema's evaluation logic.

Suggested test name:

```text
TestOpenCodeRunOwnedForkChildrenCleanupRemediatesParentOwnerLikeNoemaActiveResume
```

Required topology:

```text
1. Start a normal Matrix async run with OpenCode ACP.
2. Use an ephemeral run-owned parent session with cleanup_policy=delete_remote_or_cancel_and_forget_local.
3. While the parent session is active, create multiple fork child sessions from it.
4. Route artifact turns through those children, preferably concurrently or with overlapping lifetimes.
5. Let each child reach route_completed.
6. Cleanup the child sessions through the same path used by Noema's matrix_fork interpreter.
7. Trigger the active-resume-style initial cleanup/cancel path used before Noema resumes guidance.
8. Verify Matrix either remediates the parent owner and returns strong cleanup, or fails closed without retaining any ACP process.
```

The fixture should not simplify away the important parts:

```text
- more than one fork child;
- real OpenCode ACP stdio provider, not only a mock;
- ephemeral parent and ephemeral children;
- same workspace path for parent/children;
- parent-child metadata persisted exactly as the runtime does;
- cleanup_policy=delete_remote_or_cancel_and_forget_local;
- ForceForgetLocal=true on cleanup;
- the cleanup call path must be the same one exercised by Noema's Matrix fork interpreter / active resume sequence;
- post-run process check against /home/jose/.local/bin/opencode acp or equivalent configured OpenCode binary.
```

Passing assertions:

```text
parent cleanup:
  clean=true
  strong_cleanup=true
  cleanup_strength=strong
  process_retained=false
  failure_code=""

each fork child cleanup:
  clean=true
  strong_cleanup=true
  cleanup_strength=strong
  process_retained=false
  failure_code=""

related_sessions:
  parent owner may appear with reason=fork_parent_agent_client_owner
  no related_sessions[].retained=true

runtime:
  /v1/runs final cleanup agrees with returned cleanup proof
  no run_related_session_retained
  no run_related_session_cleanup_failed
  no fork_child_cleanup
  no standalone fork child cleanup retained its parent agent client

process tree:
  no OpenCode ACP process remains after the run/test
```

Failing assertions:

```text
- any cleanup proof with clean=true and process_retained=true;
- cleanup_strength=retained on a product cleanup path;
- failure_code=run_related_session_retained;
- failure_code=fork_child_cleanup;
- a retained parent owner related session;
- a surviving opencode acp process after the run completes or fails.
```

If this test cannot be made to pass because the topology is unsupported, Matrix
should expose a capability result that lets Noema downgrade this transport
combination instead of discovering the limitation through repeated canaries:

```text
agent=opencode
capability=interrupt_resume
interpreter=matrix_fork
status=blocked
reason=run_owned_fork_child_parent_owner_cleanup_not_production_safe
```

Only after this Matrix-only test passes should Noema rerun the full
`phase91-experience-web-research-matrix-fix-cold-resume` canary.

## Matrix Resolution

Status: closed on 2026-05-04.

Root cause: the previous remediation was too narrow for the live
active-resume topology. It required child `ephemeral=true` and, on the fork
artifact cleanup path, could still suppress parent-owner remediation. It also
over-corrected by trying to clean the parent owner from each child cleanup,
which is not the right lifecycle for multiple fork children because the parent
must remain restorable until final run cleanup.

Fix implemented:

- `/v1/runs` ephemeral parent sessions now persist an explicit `owner_run_id`.
- Run-owned fork-child cleanup accepts children that are cleanup-owned by
  `cleanup_policy`, even when `ephemeral` is not set by the caller.
- A fork child with strong provider proof (`remote_deleted`, `remote_closed`, or
  `remote_canceled`) plus `local_forgotten=true` is promoted to
  `strong_cleanup=true` without treating the shared parent workspace process as
  retained by the child.
- The parent owner is returned as a non-retained `related_sessions` entry with
  reason `fork_parent_agent_client_owner`.
- The parent owner remains available for active-resume restore and is closed or
  reaped by final parent/run cleanup.
- If child provider proof is missing, Matrix can still fall back to parent-owner
  subtree cleanup and only promotes the child if that parent cleanup proves the
  process is gone.
- Fork restore now refuses to restore a parent mirror that was already deleted
  by cleanup fallback.

Acceptance evidence:

```text
go test ./internal/logic/session ./internal/providers/runapi -count=1
ok github.com/Josepavese/matrix/internal/logic/session
ok github.com/Josepavese/matrix/internal/providers/runapi

go test ./tests/integration -run TestOpenCodeRunOwnedForkChildrenCleanupRemediatesParentOwnerLikeNoemaActiveResume -count=1
ok github.com/Josepavese/matrix/tests/integration

MATRIX_SMOKE_TEST=1 MATRIX_OPENCODE_ACP_STDIO=1 go test ./tests/integration -run TestOpenCodeRunOwnedForkChildrenCleanupRemediatesParentOwnerLikeNoemaActiveResume -count=1 -timeout 12m -v
PASS: real OpenCode ACP parent session, two real fork artifact child sessions,
strong child cleanup proofs, strong final parent cleanup, no new retained
opencode acp process after router close.
```
