# Noema OpenCode active-resume initial cleanup retained after preflight fix

Date: 2026-05-08

Status: closed

Previous related issue:

```text
issues/closed/2026-05-08-noema-opencode-preflight-regression-after-lifetime-fix.md
```

## Summary

The latest Matrix fix appears to have resolved the previous `agent_preflight_failed`
regression for post-timeout judge requests and for at least one active-resume
sequence.

Noema can now run the Matrix-backed post-task judge after timeout/cancel without
getting `502 agent_preflight_failed`, and the first warm active-resume path in the
verification batch produced strong cleanup.

However, the same non-interference Noema batch still exposes a residual Matrix
cleanup blocker on a later active-resume sequence:

```text
active resume initial cleanup not clean: process_retained
```

The failure is now narrower than the previous issue. It is not a generic judge
preflight failure anymore. It is a retained-process proof during the initial
cleanup step of a second `active_learned_resume` family in the same serial batch.

## Matrix Version Under Test

```text
matrix 0.1.17-snapshot
commit be97bb41dabb98c5bfbbe299b25c69c40f0fd350
built 2026-05-08T09:38:54Z
```

Daemon:

```text
3063412 /home/jose/.local/share/matrix/bin/matrix run
```

No concurrent OpenCode process was visible before the verification run. After the
run, `pgrep -af opencode` returned no standalone OpenCode ACP process.

## Noema Verification Command

Run from:

```text
/home/jose/hpdev/Libraries/noema/programs/evaluation-platform
```

Command:

```bash
env NOEMA_SEMANTIC_PROVIDER=ollama \
  NOEMA_SEMANTIC_OLLAMA_MODEL=embeddinggemma:latest \
  NOEMA_OUTCOME_CRITIC_PROVIDER=auto \
  NOEMA_OUTCOME_CRITIC_MATRIX_URL=http://127.0.0.1:9091 \
  go run ./cmd/noema-eval run plan \
  --batch-plan examples/live/phase91-experience-long-activities-stress-cold1-warm4-plan.json \
  --output-dir artifacts/phase91-experience-long-activities-stress-cold1-warm4-v5-matrix-fix-accepted \
  --matrix \
  --active-interpreter matrix_fork \
  --require-llm-guidance \
  --parallelism 1
```

Artifact:

```text
/home/jose/hpdev/Libraries/noema/programs/evaluation-platform/artifacts/phase91-experience-long-activities-stress-cold1-warm4-v5-matrix-fix-accepted
```

The generated Noema experience report validates with current source:

```bash
go run ./cmd/noema-eval validate \
  --config-dir configs \
  --batch-execution artifacts/phase91-experience-long-activities-stress-cold1-warm4-v5-matrix-fix-accepted/batch-execution.json \
  --report artifacts/phase91-experience-long-activities-stress-cold1-warm4-v5-matrix-fix-accepted/experience-proof-report.json
```

Result:

```text
validated=2
```

## What Improved

The previous `agent_preflight_failed` class did not reproduce in this verification.

The two cold timeout runs both reached Matrix judge:

```json
{
  "product_cold": {
    "matrix_run_id": "run-e7cf9a0e-e2b9-44aa-a101-4d07402b2c14",
    "stop_reason": "noema_active_sidecar_wall_timeout",
    "judge_provider": "noema.outcomecritic.matrix_judge",
    "judge_intent_satisfied": "no",
    "judge_risk": "high",
    "cleanup_strength": "strong"
  },
  "creative_cold": {
    "matrix_run_id": "run-b5fb0167-4da2-46ce-bc5c-3124c2ad93de",
    "stop_reason": "noema_active_sidecar_wall_timeout",
    "judge_provider": "noema.outcomecritic.matrix_judge",
    "judge_intent_satisfied": "no",
    "judge_risk": "high",
    "cleanup_strength": "strong"
  }
}
```

The first warm active-resume sequence also completed the Matrix fork/resume path
with strong cleanup:

```json
{
  "run_id": "phase91-experience-long-activities-stress-cold1-warm4-phase91-long-product-strategy-001-opencode-active-learned-resume-seed-7",
  "matrix_run_id": "run-50f5b373-d4cb-4e71-a34c-ff87f28e820d",
  "status": "failed",
  "stop_reason": "noema_active_sidecar_wall_timeout",
  "judge_intent_satisfied": "yes",
  "judge_risk": "low",
  "patterns_available": 1,
  "suggestions_generated": 1,
  "suggestions_resumed": 1,
  "active_sidecar_resume_intervention_proven": true,
  "initial_cleanup": {
    "clean": true,
    "strong": true,
    "cleanup_strength": "strong",
    "process_retained": false,
    "related_sessions": 2,
    "related_sessions_retained": 0
  },
  "final_cleanup": {
    "clean": true,
    "strong_cleanup": true,
    "cleanup_strength": "strong"
  }
}
```

This is real progress versus the prior issue.

## Remaining Failure

The second warm active-resume sequence, for the creative-campaign family, fails
before receiving a Matrix run id:

```json
{
  "run_id": "phase91-experience-long-activities-stress-cold1-warm4-phase91-long-creative-campaign-001-opencode-active-learned-resume-seed-7",
  "arm_id": "active_learned_resume",
  "family_id": "long-creative-campaign",
  "status": "failed",
  "matrix_run_id": null,
  "error": "active resume initial cleanup not clean: process_retained"
}
```

Noema preflight had a legitimate actionable scar and selected interrupt/resume:

```text
active_resume_preflight_patterns=1
active_resume_preflight_positive_routines=0
active_resume_preflight_failure_scars=1
active_resume_preflight_actionable_failure_scars=1
active_resume_preflight_decision=interrupt_resume
```

Initial cleanup proof:

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
active_sidecar_resume_initial_matrix_cleanup_process_retention_reason=other local sessions still reference agent client
active_sidecar_resume_initial_matrix_cleanup_unsafe_reason=process_retained
```

Noema must fail closed here. Under the current Noema/Matrix contract, cleanup is
production-safe only when all of these are true:

```text
strong_cleanup=true
cleanup_strength="strong"
process_retained=false
failure_code=""
no related_sessions[].retained=true
```

This run does not meet that contract.

## Why This Looks Matrix-Side

The batch was serial:

```text
--parallelism 1
```

Noema did not inject prebuilt guidance or manually seed a scar. The cold runs
created post-task Matrix-judged scars, and the warm runs attempted normal
active-resume behavior.

The first active-resume sequence succeeded at the Matrix lifecycle level. The
second sequence failed in the initial cleanup proof before Noema received a new
Matrix run id. That suggests Matrix still has a stale related local session/client
reference after a prior timeout/judge/fork/resume sequence, or that the cleanup
proof is counting a related client as retained when it should be tombstoned,
reaped, or explicitly detached from the new active-resume attempt.

The key diagnostic phrase is:

```text
other local sessions still reference agent client
```

That phrase is useful, but the current proof is still not actionable enough for
Noema to accept as production-safe. If the retained client is legitimately shared,
Matrix needs to expose which related sessions own it and whether they are live,
terminal, tombstoned, or unreferenced. If it is not legitimately shared, Matrix
should reap/tombstone it and return strong cleanup.

## Exact Matrix Test Needed

Please add a Matrix-side regression test that reproduces this exact lifecycle,
without depending on Noema.

Suggested integration test name:

```text
TestOpenCode_SequentialRunOwnedForkResume_SecondInitialCleanupDoesNotRetainProcess
```

Suggested command:

```bash
MATRIX_SMOKE_TEST=1 MATRIX_OPENCODE_ACP_STDIO=1 \
  go test ./tests/integration \
  -run TestOpenCode_SequentialRunOwnedForkResume_SecondInitialCleanupDoesNotRetainProcess \
  -count=1 -timeout 20m -v
```

Required test shape:

1. Start Matrix with real OpenCode ACP stdio using the installed OpenCode binary.
2. Use `session_policy=new_ephemeral_delete_after_run`.
3. Use `cleanup_policy=delete_remote_or_cancel_and_forget_local`.
4. Run sequence A in workspace/family A:
   - start async OpenCode run
   - let it enter prompt/work
   - cancel through the same path used by Noema wall timeout
   - run a short judge-like OpenCode request immediately after cancellation
   - run a fork/interpreter request
   - resume with fork output
   - assert final cleanup is strong
5. Run sequence B in a different workspace/family B in the same Matrix daemon:
   - start async OpenCode run
   - let it enter prompt/work
   - cancel through the same path
   - before resume, perform the same initial cleanup step Matrix exposes to Noema
   - assert the initial cleanup is strong
6. Fail the test if sequence B returns:
   - `process_retained=true`
   - `cleanup_strength="retained"`
   - `weak_reason=process_retained`
   - `process_retention_reason="other local sessions still reference agent client"`
7. Assert any `related_sessions` evidence is explicit:
   - no `related_sessions[].retained=true`
   - if a related client was reaped, include a positive reason such as `run_unreferenced_agent_client_reaped`
   - if a related client is live, include the owning run/session id and terminal state

The invariant:

```text
A completed or cancelled sequence A must not leave a local ACP client reference
that makes sequence B's initial active-resume cleanup retained.
```

## Cheaper Unit-Level Test Needed

Suggested unit test name:

```text
TestRouter_SequentialWorkspaceResumeCleanupDoesNotRetainPriorClientReference
```

Required fake behavior:

- fake ACP client tracks workspace key, logical session id, remote session id,
  prompt lifecycle, cancel lifecycle, and close/delete lifecycle
- sequence A creates a client, cancels during prompt, performs judge-like prompt,
  performs fork-like prompt, resumes, then cleanup tombstones/reaps it
- sequence B creates a separate workspace client after sequence A
- initial cleanup for sequence B must not see sequence A as a retained related
  local session

Assertions:

```text
cleanup.clean == true
cleanup.strong_cleanup == true
cleanup.cleanup_strength == "strong"
cleanup.process_retained == false
cleanup.failure_code == ""
related_sessions_retained == 0
```

If Matrix intentionally permits a shared router-lifetime OpenCode client across
workspaces, the test should assert that cleanup proof distinguishes:

```text
shared live provider process owned by active run
```

from:

```text
stale retained local session that blocks Noema production cleanup
```

Noema can accept the former only if Matrix proves it is not tied to the cleaned
run/session and exposes the owning active run. Noema cannot accept the latter.

## Acceptance Criteria

Noema can accept this resolved when a fresh rerun of the same artifact shape
shows:

```text
0 agent_preflight_failed
0 process_retained=true cleanup proofs
all active-resume initial cleanup proofs strong
all final cleanup proofs strong when a Matrix run id exists
no related_sessions retained
```

A minimum Noema verification command after the Matrix fix:

```bash
env NOEMA_SEMANTIC_PROVIDER=ollama \
  NOEMA_SEMANTIC_OLLAMA_MODEL=embeddinggemma:latest \
  NOEMA_OUTCOME_CRITIC_PROVIDER=auto \
  NOEMA_OUTCOME_CRITIC_MATRIX_URL=http://127.0.0.1:9091 \
  go run ./cmd/noema-eval run plan \
  --batch-plan examples/live/phase91-experience-long-activities-stress-cold1-warm4-plan.json \
  --output-dir artifacts/phase91-experience-long-activities-stress-cold1-warm4-v6-matrix-initial-cleanup-fix \
  --matrix \
  --active-interpreter matrix_fork \
  --require-llm-guidance \
  --parallelism 1
```

## Matrix Maintainer Resolution

Accepted as a general Matrix cleanup-proof issue, not as a project-specific
Noema feature.

Implemented fixes:

- ACP router tombstones are now remote-session scoped. A workspace-only reap can
  no longer consume a tombstone that proves a specific remote session was
  closed/reaped.
- Session cleanup now distinguishes target cleanup from unrelated shared
  workspace ownership. If the target remote session is deleted, closed, or
  canceled and another local session still owns the same provider client, Matrix
  returns strong target cleanup and exposes the other owner as non-retained
  `related_sessions[]` evidence with reason `shared_agent_client_owner`.
- `process_retained=true` remains unsafe. Matrix only avoids it when there is
  explicit strong target proof and explicit owner evidence.

Matrix-side tests added or updated:

- `TestRouter_ReapAgentClientDoesNotConsumeExplicitRemoteTombstone`
- `TestSessionManager_DeleteRecordsSharedAgentClientOwnerWithoutRetainingTarget`
- `TestOpenCode_SequentialRunOwnedForkResume_SecondInitialCleanupDoesNotRetainProcess`

Verification performed:

```bash
go test ./internal/providers/agents \
  -run 'TestRouter_ReapAgent(ClientDoesNotConsumeExplicitRemoteTombstone|SessionClientUsesRecentDeadClientTombstone|SessionClientPreservesSiblingRemoteTombstones|SessionClientUsesTombstoneWhenCurrentClientDoesNotTrackSession)' \
  -count=1 -v

go test ./internal/logic/session \
  -run 'TestSessionManager_DeleteRecordsSharedAgentClientOwnerWithoutRetainingTarget|TestSessionManager_RunOwnedForkInputCleanupRemediatesParentOwnerLikeActiveResume|TestSessionManager_RunOwnedStandaloneForkChildCleanupPromotesParentOwnerProof|TestSessionManager_EphemeralParentCleanupCascadesToForkChild' \
  -count=1 -v

MATRIX_SMOKE_TEST=1 MATRIX_OPENCODE_ACP_STDIO=1 \
  go test ./tests/integration \
  -run TestOpenCode_SequentialRunOwnedForkResume_SecondInitialCleanupDoesNotRetainProcess \
  -count=1 -timeout 20m -v

go test ./...

./scripts/deploy_preflight.sh
```

All listed checks passed. The Noema batch itself was not rerun from this Matrix
session because Matrix must remain project-agnostic; the Matrix-only OpenCode ACP
acceptance test covers the lifecycle invariant from this issue.
