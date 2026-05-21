# Noema v6: timeout cleanup loses selected session after Matrix initial-cleanup fix

Date: 2026-05-08

Status: closed

Previous related issue:

```text
issues/closed/2026-05-08-noema-active-resume-initial-cleanup-retained-after-preflight-fix.md
```

## Summary

Noema reran the exact requested acceptance batch after the Matrix fix:

```text
phase91-experience-long-activities-stress-cold1-warm4-v6-matrix-initial-cleanup-fix
```

The fix made real progress on the prior specific failure:

- the second `active_learned_resume` sequence now reaches interrupt/resume
- its initial resume cleanup is strong
- its final cleanup is strong
- `related_sessions_retained=0`

However, the batch still cannot be accepted because the first cold timeout run now
fails Matrix cleanup proof and breaks the immediate Matrix judge:

```text
cleanup_strength=retained
process_retained=true
process_retention_reason=other local sessions still reference agent client
outcome_critic_primary_error=matrix http status=502: code=agent_preflight_failed
```

This looks like a race where Noema cancels a run during session creation. Matrix
then creates/selects a real remote session after `run.cancelled`, but cleanup is
performed against a different logical session id with no `remote_session_id`.

## Matrix Version Under Test

```text
matrix 0.1.17-snapshot
commit be97bb41dabb98c5bfbbe299b25c69c40f0fd350
built 2026-05-08T10:21:26Z
```

Daemon:

```text
PID 3165199
started Fri May 8 12:21:48 2026
/home/jose/.local/share/matrix/bin/matrix run
```

No concurrent OpenCode or Noema-eval process was active before launching the
verification.

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
  --output-dir artifacts/phase91-experience-long-activities-stress-cold1-warm4-v6-matrix-initial-cleanup-fix \
  --matrix \
  --active-interpreter matrix_fork \
  --require-llm-guidance \
  --parallelism 1
```

Noema artifact:

```text
/home/jose/hpdev/Libraries/noema/programs/evaluation-platform/artifacts/phase91-experience-long-activities-stress-cold1-warm4-v6-matrix-initial-cleanup-fix
```

The generated report is schema-valid:

```bash
go run ./cmd/noema-eval validate \
  --config-dir configs \
  --batch-execution artifacts/phase91-experience-long-activities-stress-cold1-warm4-v6-matrix-initial-cleanup-fix/batch-execution.json \
  --report artifacts/phase91-experience-long-activities-stress-cold1-warm4-v6-matrix-initial-cleanup-fix/experience-proof-report.json
```

Result:

```text
validated=2
```

That only validates schema. It does not mean cleanup is acceptable.

## Batch Outcome

High-level record summary:

```json
[
  {
    "family_id": "long-product-strategy",
    "arm_id": "active_cold",
    "status": "failed",
    "stop_reason": "noema_active_sidecar_wall_timeout",
    "matrix_run_id": "run-c86840c7-2037-4281-b626-cbcafb651036",
    "cleanup_strength": "retained",
    "process_retained": true,
    "outcome_critic_primary_error": "matrix http status=502: code=agent_preflight_failed"
  },
  {
    "family_id": "long-product-strategy",
    "arm_id": "active_learned_resume",
    "status": "succeeded",
    "stop_reason": "end_turn",
    "matrix_run_id": "run-936a9522-731a-4d45-bdd7-940ff67ad5df",
    "cleanup_strength": "strong",
    "process_retained": false
  },
  {
    "family_id": "long-creative-campaign",
    "arm_id": "active_cold",
    "status": "failed",
    "stop_reason": "noema_active_sidecar_wall_timeout",
    "matrix_run_id": "run-ac1a8016-d983-4c17-a8c6-b2f33abba7da",
    "cleanup_strength": "strong",
    "process_retained": false
  },
  {
    "family_id": "long-creative-campaign",
    "arm_id": "active_learned_resume",
    "status": "succeeded",
    "stop_reason": "end_turn",
    "matrix_run_id": "run-5a2ec0a8-4062-486b-9670-316177f2a4ac",
    "cleanup_strength": "strong",
    "process_retained": false,
    "initial_resume_cleanup_strength": "strong",
    "initial_resume_related_sessions_retained": 0
  }
]
```

So the previous issue is partly resolved, but Matrix still fails the acceptance
criteria:

```text
0 agent_preflight_failed
0 process_retained=true cleanup proofs
all cleanup proofs strong
```

## Critical Evidence

For the first cold product run, Noema record:

```json
{
  "run_id": "phase91-experience-long-activities-stress-cold1-warm4-phase91-long-product-strategy-001-opencode-active-cold-seed-7",
  "session_isolation_id": "0da8c154-8eae-4ee9-91c2-1fea7d6c4213",
  "matrix_run_id": "run-c86840c7-2037-4281-b626-cbcafb651036",
  "matrix_cleanup": {
    "clean": true,
    "strong_cleanup": false,
    "cleanup_strength": "retained",
    "logical_session_id": "0da8c154-8eae-4ee9-91c2-1fea7d6c4213",
    "process_retained": true,
    "process_retention_allowed": true,
    "process_retention_reason": "other local sessions still reference agent client",
    "remote_delete_attempted": false,
    "remote_deleted": false,
    "remote_close_attempted": false,
    "remote_closed": false,
    "remote_cancel_attempted": false,
    "remote_canceled": false
  }
}
```

The Matrix trace for the same run shows a different selected logical session and
a real remote session:

```json
[
  {
    "kind": "run.cancelled",
    "timestamp": "2026-05-08T11:47:43.80212066Z"
  },
  {
    "kind": "session.created",
    "timestamp": "2026-05-08T11:47:43.826064889Z",
    "metadata": {
      "logical_session_id": "bc390278-8629-40de-9bdd-c04252b8c4c3",
      "remote_session_id": "ses_1f896a4a9ffemWnmpUlGXLRSGT",
      "status": "active"
    }
  },
  {
    "kind": "session.cleanup",
    "timestamp": "2026-05-08T11:47:46.302341583Z",
    "metadata": {
      "logical_session_id": "0da8c154-8eae-4ee9-91c2-1fea7d6c4213",
      "remote_session_id": "",
      "cleanup_strength": "retained",
      "process_retained": true,
      "process_retention_reason": "other local sessions still reference agent client",
      "related_sessions": null,
      "strong_cleanup": false
    }
  }
]
```

The immediate post-task Matrix judge then fails closed:

```json
{
  "schema": "noema.outcome_critic.primary_error.v1",
  "error": "matrix http status=502: code=agent_preflight_failed"
}
```

Path:

```text
/home/jose/hpdev/Libraries/noema/programs/evaluation-platform/artifacts/phase91-experience-long-activities-stress-cold1-warm4-v6-matrix-initial-cleanup-fix/runs/phase91-experience-long-activities-stress-cold1-warm4-phase91-long-product-strategy-001-opencode-active-cold-seed-7/outcome-critic-primary-error.json
```

## Why This Looks Like The Remaining Bug

The ordering is the key:

```text
run.cancelled at 11:47:43.802
session.created at 11:47:43.826
session.cleanup at 11:47:46.302
```

Matrix appears to create/select a real ACP remote session after the cancellation
request has already been accepted. Cleanup then uses Noema's session isolation id,
not the Matrix-selected logical session id, and therefore has no remote session id
to delete/close/cancel/reap.

This produces the unsafe proof:

```text
clean=true
strong_cleanup=false
cleanup_strength=retained
process_retained=true
remote_session_id=""
related_sessions=null
```

Noema must fail closed here. It cannot infer that
`ses_1f896a4a9ffemWnmpUlGXLRSGT` was cleaned, because Matrix did not attach it to
the cleanup proof.

The next Matrix/OpenCode request, the outcome critic, then fails with
`agent_preflight_failed`, which is exactly the class the prior fixes were meant to
eliminate.

## Exact Matrix Test Needed

Please add a Matrix-side regression test that targets the cancel/session-create
race directly.

Suggested integration test name:

```text
TestOpenCode_CancelDuringSessionCreate_CleanupUsesSelectedSessionAndJudgeDoesNotPreflightFail
```

Required shape:

1. Start a real OpenCode ACP async run with:
   - `session_policy=new_ephemeral_delete_after_run`
   - `cleanup_policy=delete_remote_or_cancel_and_forget_local`
2. Use a prompt or fake timing hook that allows cancellation while session
   creation or first prompt is still in-flight.
3. Trigger cancel through the same API path used by Noema wall timeout.
4. Allow Matrix to finish any late `session.created` path if it races after
   `run.cancelled`.
5. Cleanup must bind to the actual selected Matrix logical session and remote
   session, not only the caller/request session id.
6. Assert cleanup proof:
   - `clean=true`
   - `strong_cleanup=true`
   - `cleanup_strength="strong"`
   - `process_retained=false`
   - `failure_code=""`
   - `remote_session_id` is the selected remote session if one was created
   - no retained related sessions
7. Immediately run a fresh short OpenCode request simulating Noema's Matrix judge.
8. Assert no:
   - `agent_preflight_failed`
   - `provider.preflight.failed`
   - `client context cancelled`
   - `ACP prompt failed: context canceled`

The invariant:

```text
If Matrix creates/selects a remote session after a cancellation request, cleanup
must still clean that selected session and must not leave the provider client in a
state that poisons the next OpenCode request.
```

## Cheaper Unit-Level Test Needed

Suggested unit test name:

```text
TestSessionManager_CancelRaceCleanupTracksLateSelectedRemoteSession
```

Fake behavior:

- request logical session id is `requested-session`
- router/session manager later selects `selected-session`
- ACP client creates `remote-session`
- cancel is accepted before the selected session is fully recorded
- cleanup is requested for the run/request

Assertions:

```text
cleanup.logical_session_id == "selected-session"
cleanup.remote_session_id == "remote-session"
cleanup.strong_cleanup == true
cleanup.cleanup_strength == "strong"
cleanup.process_retained == false
cleanup.related_sessions_retained == 0
```

If Matrix intentionally reports the caller/request id in cleanup, then it still
must include the selected logical/remote session in explicit fields so Noema can
prove target cleanup. The current `remote_session_id=""` proof is not acceptable.

## Acceptance Criteria

Noema can accept Matrix only when a fresh rerun of the same command shows:

```text
0 agent_preflight_failed
0 process_retained=true cleanup proofs
4/4 cleanup proofs strong, or every missing run id has explicit fail-closed cause
active-resume initial cleanup strong
final cleanup strong for every Matrix run id
no retained related sessions
```

The prior Matrix fix should be kept: v6 shows that the second active-resume path
now works. The remaining blocker is the earlier cancel/session-create race.

## Matrix Maintainer Resolution

Accepted as a general Matrix `/v1/runs` lifecycle bug.

Root cause:

- `session_policy=new_ephemeral_delete_after_run` created a prepared ephemeral
  logical session, but `/v1/runs` did not pass that logical id into the
  conversation route request. Under cancel/provider-startup races, the session
  manager could select or create a different active session.
- Cleanup target selection then preferred the prepared logical session even when
  Matrix observed a late-selected remote session with a real provider id. That
  allowed stale cleanup proofs such as `remote_session_id=""` and
  `process_retained=true`.

Implemented fix:

- `/v1/runs` now binds routing to the prepared ephemeral logical session.
- If cancellation still races with provider startup and Matrix observes a
  late-selected logical/remote session, cleanup targets that selected session
  instead of the stale prepared id.
- Existing cancellation cleanup remains detached from the cancelled request
  context, so remote/provider cleanup can complete after run cancel.

Tests added or updated:

- `TestHandleRunActionsCancelRaceCleanupTracksLateSelectedRemoteSession`
- `TestOpenCode_CancelDuringSessionCreate_CleanupUsesSelectedSessionAndJudgeDoesNotPreflightFail`

Verification performed:

```bash
go test ./internal/providers/runapi \
  -run 'TestHandleRunActionsCancel(RaceCleanupTracksLateSelectedRemoteSession|CleansEphemeralRunWithDetachedContext)|TestHandleRuns_EphemeralCleanup(CleansRunCreatedActiveSessionWhenActiveChanges|FailsPreExistingActiveSessionWhenActiveChanges)' \
  -count=1 -v

MATRIX_SMOKE_TEST=1 MATRIX_OPENCODE_ACP_STDIO=1 \
  go test ./tests/integration \
  -run TestOpenCode_CancelDuringSessionCreate_CleanupUsesSelectedSessionAndJudgeDoesNotPreflightFail \
  -count=1 -timeout 20m -v

go test ./...
```

All checks passed. The OpenCode smoke verified the real cancellation path and a
fresh immediate judge request without `agent_preflight_failed`.
