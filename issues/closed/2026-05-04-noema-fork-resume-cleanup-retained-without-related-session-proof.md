# Noema fork/resume cleanup remains retained without related-session proof

## Status

Closed.

Resolved in Matrix by making post-run reconcile retained clients part of the
run cleanup proof. `runreconcile.Apply` now projects
`AgentClientReconcileResult.Retained` into `related_sessions` and fails the
ephemeral run cleanup with `failure_code=run_related_session_retained` instead
of leaving `cleanup_strength=retained` without structured evidence.

Regression coverage added:

- `internal/logic/runreconcile`: retained reconcile clients fail run cleanup;
  reaped reconcile clients remain positive cleanup evidence.
- `internal/providers/runapi`: `/v1/runs` ephemeral cleanup returns a failed
  cleanup proof and failed `session.cleanup` trace when reconcile reports a
  retained provider client.

Validation:

```text
go test ./internal/logic/runreconcile ./internal/providers/runapi -run 'TestApply|TestHandleRuns_EphemeralCleanup|TestHandleRuns_NewEphemeralDeleteAfterRunCreatesCleansAndTraces' -count=1 -v
go test ./...
git diff --check
```

## Reporter

Noema experience-layer evaluation.

## Summary

Noema reran the Matrix fork/resume cleanup canary after the Matrix `0.1.17`
cleanup reconciliation fix.

The simple cold run cleanup is now production-safe, but the actual
`active_learned_resume` + `matrix_fork` path is still not production-safe:
Matrix returns `clean=true` with `cleanup_strength="retained"`,
`strong_cleanup=false`, `process_retained=true`, and
`process_retention_reason="fork child uses parent agent client"`.

Noema now correctly fails closed under the stricter cleanup contract, so the
LLM guidance rendering is rejected and no benchmark evidence can be accepted.

The remaining issue is that Matrix does not expose the new structured failure
shape expected by the contract:

- `failure_code` is empty/omitted instead of `run_related_session_retained`.
- `related_sessions` is absent/null.
- A Matrix child `opencode acp` process remains alive after the failed run.

## Matrix Version

```text
matrix 0.1.17
commit 6864d59ec7965594a23b94f2800b1320c2630df8
built 2026-05-04T09:36:51Z
```

Matrix daemon during the run:

```text
376459 /home/jose/.local/share/matrix/bin/matrix run
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
  --output-dir artifacts/phase91-experience-web-research-matrix-fix-cold-resume-v1 \
  --matrix \
  --active-interpreter matrix_fork \
  --require-llm-guidance \
  --parallelism 1
```

Batch artifact:

```text
/home/jose/hpdev/Libraries/noema/programs/evaluation-platform/artifacts/phase91-experience-web-research-matrix-fix-cold-resume-v1/batch-execution.json
```

## Relevant Matrix Run IDs

Cold control run:

```text
run-d1ca22ae-5912-46e8-ae4d-d461c2740ea8
```

Warm fork/resume run:

```text
run-369fc872-6e72-485e-8ce5-624d1d6aee7a
```

## Cold Control Cleanup

The cold run cleanup is accepted as production-safe:

```json
{
  "agent_id": "opencode",
  "clean": true,
  "cleanup_policy": "delete_remote_or_cancel_and_forget_local",
  "cleanup_strength": "strong",
  "local_forgotten": true,
  "logical_session_id": "51a7c903-7647-4f03-bf79-4b6f09d61320",
  "process_reap_attempted": true,
  "process_reaped": true,
  "protocol_kind": "acp",
  "remote_cancel_attempted": false,
  "remote_canceled": false,
  "remote_close_attempted": false,
  "remote_closed": false,
  "remote_delete_attempted": true,
  "remote_deleted": false,
  "remote_session_id": "ses_20d97220fffebtoRyj2Q60EA2t",
  "strong_cleanup": true
}
```

## Failing Fork/Resume Cleanup

Warm run facts:

```text
arm_id=active_learned_resume
status=failed
stop_reason=noema_active_sidecar_strict_guidance_render_failed
error=strict LLM guidance required but active suggestion rendering failed
patterns_available=1
failure_scars_learned=1
active_resume_preflight_decision=interrupt_resume
active_interpreter_fork_attempts=1
active_interpreter_fork_accepted=0
active_interpreter_fork_rejected=1
active_interpreter_fork_reject_reason=matrix fork cleanup proof insufficient
```

Full cleanup JSON from Noema execution record:

```json
{
  "agent_id": "opencode",
  "clean": true,
  "cleanup_policy": "delete_remote_or_cancel_and_forget_local",
  "cleanup_strength": "retained",
  "local_forgotten": true,
  "logical_session_id": "0becda02-56c8-47fb-980d-76238d528cd3",
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
  "remote_session_id": "ses_20d8fe625ffe72fH9Tra8FRTrP",
  "strong_cleanup": false,
  "weak_cleanup_reason": "process_retained"
}
```

Related sessions observed by Noema:

```json
[]
```

The Matrix trace records `related_sessions: null` for this cleanup event.

## Post-Run Process Tree

Immediately after the Noema run completed:

```text
376459 /home/jose/.local/share/matrix/bin/matrix run
398272 /home/jose/.local/bin/opencode acp
```

Detailed process state:

```text
PID     PPID    PGID    SID     STAT ELAPSED CMD
376459  5923    376459  376459  Ssl  22:43   /home/jose/.local/share/matrix/bin/matrix run
398272  376459  398272  376459  Sl   02:14   /home/jose/.local/bin/opencode acp
```

## Expected

For fork/resume runs, Matrix should make one of these states explicit:

- Production-safe cleanup: `strong_cleanup=true`,
  `cleanup_strength="strong"`, `process_retained=false`,
  empty `failure_code`, and no `related_sessions[].retained=true`.
- Fail-closed cleanup: non-empty `failure_code`, preferably
  `run_related_session_retained`, plus `related_sessions` explaining the retained
  parent/fork ACP client.
- Positive reconcile evidence: `related_sessions` may include
  `reason=run_unreferenced_agent_client_reaped` only when `retained=false`.

## Actual

- Cleanup is retained and therefore correctly rejected by Noema.
- `failure_code` is empty/omitted.
- `related_sessions` is absent/null.
- A Matrix child `opencode acp` process remains alive after the failed run.

## Detailed Analysis

This does not look like a normal model/provider latency problem and does not
look like a Noema prompt problem. The sequence appears to be a lifecycle-accounting
gap around a fork child that shares the parent OpenCode ACP client.

The important distinction is that the test produced two different cleanup
surfaces:

- The cold `/v1/runs` cleanup surface is healthy. It reaches
  `cleanup_strength=strong`, `strong_cleanup=true`, and leaves no ACP process.
- The warm `active_learned_resume` surface reaches the fork interpreter path.
  That path creates a fork child and then tries to clean it. The cleanup proof is
  retained because the child uses the parent ACP client.

The warm run therefore fails before guidance can be delivered. Noema required
real LLM guidance (`--require-llm-guidance`) and rejected the suggestion because
Matrix returned a cleanup proof that was not production-safe. This is the
correct Noema behavior under the new contract.

The remaining problem is inside Matrix cleanup proof/accounting. The observed
state is internally incomplete:

- Matrix knows a process is retained: `process_retained=true`.
- Matrix knows why: `process_retention_reason="fork child uses parent agent client"`.
- Matrix still returns `clean=true`, not `clean=false`.
- Matrix does not emit `failure_code=run_related_session_retained`.
- Matrix does not populate `related_sessions`.
- The OS process tree confirms a retained Matrix child ACP process.

That combination means the cleanup is no longer falsely reported as strong, but
it is still not expressed in the structured fail-closed shape that Noema and the
Matrix docs expect for run-level fork/resume cleanup.

## What I Think Happened

1. Noema launched the cold run through Matrix with an ephemeral session policy.
   The web-research task timed out under the short budget, then the post-task
   judge produced a valid failure scar.

2. Noema launched the warm run through Matrix with the same normal product path.
   The experience layer retrieved the failure scar via semantic route and
   decided `active_resume_preflight_decision=interrupt_resume`.

3. Because `--active-interpreter matrix_fork` was enabled, Noema asked Matrix to
   use a forked child LLM turn to verbalize the prior into agent-facing guidance.

4. Matrix created or used a fork child session for the interpreter. That child
   appears to share the same OpenCode ACP provider process/client as the parent
   session. This is consistent with the cleanup reason
   `fork child uses parent agent client`.

5. Matrix attempted to clean the fork child. The child cleanup was allowed to
   retain the process because reaping it directly could kill the parent client.
   This is consistent with `internal/logic/session/manager_cleanup_process.go`,
   where `allowProcessRetention` sets `ProcessRetained=true`,
   `ProcessRetentionAllowed=true`, and reason
   `ForkChildUsesParentAgentClient` when the metadata has a parent session.

6. Noema rejected the fork interpreter result because the cleanup proof was not
   production-safe. Since guidance was mandatory, Noema cancelled the active run
   and Matrix performed run cleanup.

7. During final run cleanup, Matrix still had at least one OpenCode ACP client
   alive under the daemon. The process tree confirms that this was not merely a
   stale Noema record.

8. The process was not represented to Noema as a related retained session. It
   either was not visible in the run-level local-only session snapshots, or it
   was visible only as an active agent-client reference during reconcile.

9. The final proof therefore fell into an ambiguous middle state: top-level
   `process_retained=true` and `cleanup_strength=retained`, but no
   `related_sessions` and no `failure_code`. That is the contract gap.

## Why This Is Not Accepted as Production-Safe

Noema now accepts Matrix cleanup only when all of these are true:

- `strong_cleanup=true`
- `cleanup_strength="strong"`
- `process_retained=false`
- empty `failure_code`
- no `related_sessions[].retained=true`

The warm run fails at least three of those gates:

- `strong_cleanup=false`
- `cleanup_strength="retained"`
- `process_retained=true`

Noema therefore did the right thing by failing closed. Accepting this run as
valid guidance would reintroduce the exact class of false cleanup proof that the
previous issue tried to eliminate.

## Why I Think the Error Is in Matrix

The retained process is a child of the Matrix daemon:

```text
398272  376459  398272  376459  Sl  /home/jose/.local/bin/opencode acp
```

Noema does not own that ACP process. Noema only calls Matrix APIs and reads the
Matrix response. The process remained after the Noema runner exited, which means
the lifecycle boundary is inside Matrix's session/client management.

Also, Matrix already has the concepts needed to report this cleanly:

- `SessionCleanupResult.RelatedSessions`
- `SessionCleanupResult.FailureCode`
- `AgentClientReconcileResult.Reaped`
- `AgentClientReconcileResult.Retained`
- `sessioncleanup.FailureRunRelatedSessionRetained`
- `sessioncleanup.ReasonRunUnreferencedAgentClientReaped`

The result simply does not project the retained client/session into the final
run-level proof.

## Most Likely Code Locations

### 1. `internal/logic/runreconcile/reconcile.go`

This is the strongest suspect.

`runreconcile.Apply` currently appends only `result.Reconcile.Reaped` to
`cleanup.RelatedSessions`:

```go
for _, ref := range result.Reconcile.Reaped {
    req.Cleanup.RelatedSessions = append(req.Cleanup.RelatedSessions, ...)
}
```

But `middleware.AgentClientReconcileResult` also has:

```go
Retained []AgentClientRef
```

The router implementation in `internal/providers/agents/router_reconcile.go`
does populate `Retained` for active cached clients:

```go
if _, ok := activeKeys[key]; ok {
    result.Retained = append(result.Retained, ref)
    continue
}
```

If the retained OpenCode ACP client appears in `Reconcile.Retained`, Matrix
currently ignores that evidence at the run cleanup layer. That would explain why
Noema sees no `related_sessions`, no `failure_code`, and yet an ACP process is
still alive.

Expected behavior for a run-level production cleanup:

- If a retained reconciled client is acceptable only because it still has a
  Matrix session reference, the cleanup proof must identify that session in
  `related_sessions`.
- If the retained client is related to the just-finished ephemeral run, the
  cleanup proof must fail closed with `failure_code=run_related_session_retained`.
- If the retained client is unrelated/pre-existing, Matrix must still make that
  explicit so Noema can distinguish "safe pre-existing daemon state" from
  "leaked fork/resume client".

### 2. `internal/providers/runapi/session_cleanup.go`

`cleanupRunSessions` performs these steps:

```go
cleanup, err := s.cleanupRunSession(ctx, scope.exec, target)
accountRunRelatedSessions(...)
runreconcile.Apply(...)
appendCleanupEvent(...)
```

The order is reasonable, but the problem is what gets counted.

`accountRunRelatedSessions` only sees `sessionSnapshot` records from
`scope.after` and `scope.afterList`. If the fork child has already been locally
forgotten by the time `afterList` is collected, or if the retained process is
only represented as a cached agent client and not as a `SessionEntry`, the
related-session path cannot fire.

That makes `runreconcile.Apply` the last chance to account for the retained
process. Because retained reconcile refs are ignored, the final cleanup proof is
under-specified.

### 3. `internal/providers/runapi/session_cleanup_related.go`

This file has the correct fail-closed helper:

```go
markRunRelatedSessionRetained(...)
```

That helper:

- appends a `related_sessions` entry,
- sets `ProcessRetained=true`,
- sets `FailureCode=run_related_session_retained`,
- sets `Error="run cleanup retained a related session"`,
- downgrades clean/strong state when needed.

The fact that Noema received no related sessions means this helper was probably
not reached for the retained fork/parent client. Either the retained object was
not in `afterList`, or it was not represented as a `sessionSnapshot`.

### 4. `internal/logic/session/manager_cleanup_process.go`

This file explains the exact string observed in the proof:

```go
if strings.TrimSpace(meta.ParentSessionID) != "" ||
   strings.TrimSpace(meta.ParentRemoteID) != "" {
    result.ProcessRetained = true
    result.ProcessRetentionAllowed = true
    result.ProcessRetentionReason = sessioncleanup.ForkChildUsesParentAgentClient
    return true
}
```

That behavior can be legitimate for action-level fork-child cleanup: the child
must not reap the shared parent client. It is not enough as run-level proof for a
Noema production benchmark unless the parent/run cleanup subsequently reaps the
client or explicitly records why it is retained.

### 5. `internal/logic/session/manager_cleanup_result.go`

This file allows `Clean=true` with `cleanup_strength=retained` when retention is
allowed:

```go
ProcessCleanupSatisfied:
    input.ProcessRetained && input.ProcessRetentionAllowed
```

That is useful for shared non-ephemeral cleanup, but Noema cannot treat it as
production-safe for fork/resume evidence. For `/v1/runs` ephemeral cleanup,
Matrix should add run-level policy enforcement after normal session cleanup so a
retained process becomes structured run failure unless explicitly reconciled as
safe and unrelated.

## How to Verify

### Reproduction

Run the exact command above from Noema and inspect:

```bash
jq -r '.records[] | [
  .arm_id,
  .status,
  .stop_reason,
  (.active_sidecar.patterns_available // 0),
  (.active_sidecar.fork_interpreter_attempts // 0),
  (.active_sidecar.fork_interpreter_accepted // 0),
  (.active_sidecar.fork_interpreter_rejected // 0),
  (.active_sidecar.fork_interpreter_reject_reason // ""),
  (.matrix_cleanup.clean // false),
  (.matrix_cleanup.strong_cleanup // false),
  (.matrix_cleanup.cleanup_strength // ""),
  (.matrix_cleanup.process_retained // false),
  ((.matrix_cleanup.related_sessions // []) | length),
  (.matrix_cleanup.failure_code // "")
] | @tsv' \
  /home/jose/hpdev/Libraries/noema/programs/evaluation-platform/artifacts/phase91-experience-web-research-matrix-fix-cold-resume-v1/batch-execution.json
```

Expected current failing signature:

```text
active_cold             failed  noema_active_sidecar_wall_timeout                 ... clean=true strong_cleanup=true  cleanup_strength=strong   process_retained=false related_sessions=0 failure_code=""
active_learned_resume   failed  noema_active_sidecar_strict_guidance_render_failed ... clean=true strong_cleanup=false cleanup_strength=retained process_retained=true  related_sessions=0 failure_code=""
```

Then verify process state:

```bash
ps -eo pid,ppid,pgid,sid,stat,etime,cmd | rg 'matrix run|opencode acp'
```

Expected current failing signature:

```text
matrix run
opencode acp
```

### Trace Inspection

Inspect the run cleanup event directly:

```bash
jq '.events[]? | select(.kind=="session.cleanup") | {
  status,
  metadata: {
    clean: .metadata.clean,
    strong_cleanup: .metadata.strong_cleanup,
    cleanup_strength: .metadata.cleanup_strength,
    process_retained: .metadata.process_retained,
    process_retention_reason: .metadata.process_retention_reason,
    related_sessions: .metadata.related_sessions,
    failure_code: .metadata.failure_code,
    error: .metadata.error
  }
}' \
  /home/jose/hpdev/Libraries/noema/programs/evaluation-platform/artifacts/phase91-experience-web-research-matrix-fix-cold-resume-v1/runs/phase91-experience-web-research-matrix-fix-cold-resume-phase91-web-research-tight-budget-001-opencode-active-learned-resume-seed-7/matrix-trace.json
```

If the hypothesis is correct, the event will show retained process metadata but
`related_sessions=null` or empty and no failure code.

### Add Temporary Matrix Instrumentation

Add debug logs around these points:

- `prepareRunSessionContext`: dump `before`, `beforeList`, and `prepared`.
- `cleanupRunSessionContext`: dump `after` and `afterList`.
- `accountRunRelatedSessions`: dump each snapshot key, whether it existed
  before, whether it is owned by the run, and whether it calls
  `markRunRelatedSessionRetained`.
- `runreconcile.Apply`: dump both `result.Reconcile.Reaped` and
  `result.Reconcile.Retained`.
- `Manager.allowProcessRetention`: dump `meta.ID`, `meta.ParentSessionID`,
  `meta.ParentRemoteID`, `meta.AgentSessionID`, `meta.Ephemeral`, and the final
  retention reason.

The decisive observation is whether the retained `opencode acp` appears in
`Reconcile.Retained`. If it does, `runreconcile.Apply` is dropping or
under-projecting the evidence into the run cleanup proof.

### Unit/Regression Tests to Add

Add or extend Matrix tests in `internal/providers/runapi/runs_test.go`.

Test A: retained reconcile client fails run cleanup proof.

- Router returns a successful ephemeral run cleanup for the target session.
- Router's reconcile result has `Retained=[{AgentID:"opencode", WorkspacePath:"/tmp/eval-ws"}]`.
- Expected response cleanup:
  - `clean=false`
  - `cleanup_strength=failed`
  - `process_retained=true`
  - `failure_code=run_related_session_retained`
  - `related_sessions[0].retained=true`

Test B: reaped reconcile client remains positive evidence.

- Router's reconcile result has only `Reaped`.
- Expected response cleanup:
  - run may remain strong if target cleanup was strong
  - `related_sessions[0].reason=run_unreferenced_agent_client_reaped`
  - `related_sessions[0].retained=false`

Test C: fork child cleanup retained must not be silently accepted as run-level
production cleanup.

- Simulate a fork child with `ParentSessionID` or `ParentRemoteID`.
- Its action-level cleanup may be `cleanup_strength=retained`.
- The enclosing `/v1/runs` cleanup must either later reap the parent client or
  expose retained related-session failure.

## Suggested Fix Direction

Do not remove the fork-child retention behavior blindly. It exists for a reason:
a fork child can share the parent ACP client, so child cleanup should not kill
the parent client directly.

The fix should be at the run-level accounting boundary:

1. Preserve action-level retained cleanup as a valid local signal only when it is
   explicitly scoped to the fork child.

2. At `/v1/runs` cleanup completion, require a production-safe aggregate proof:
   no retained run-created/session-related clients may remain unreported.

3. In `runreconcile.Apply`, handle `result.Reconcile.Retained`, not just
   `Reaped`.

4. If a retained reconcile ref matches the run agent/workspace and the run used
   ephemeral cleanup, mark the cleanup as failed with
   `failure_code=run_related_session_retained` and append a retained
   `related_sessions` entry.

5. If Matrix can prove the retained client predates the run and is unrelated,
   expose that explicitly in `related_sessions` or a separate audit field so
   callers do not infer safety from missing evidence.

6. Ensure `session.cleanup` trace metadata and HTTP response cleanup are
   identical after the final aggregate proof is computed.

## Acceptance Criteria

Noema can accept the Matrix fix only when rerunning the same canary produces one
of these outcomes:

- Guidance succeeds and all Matrix cleanup proofs involved in fork/resume are
  production-safe:
  `strong_cleanup=true`, `cleanup_strength=strong`,
  `process_retained=false`, no `failure_code`, no retained related sessions, and
  no post-run `opencode acp` process.
- Guidance fails closed with structured proof:
  `clean=false`, `failure_code=run_related_session_retained`, and at least one
  `related_sessions[].retained=true` explaining the retained fork/parent client.

The following outcome is still not acceptable:

- `cleanup_strength=retained`, `process_retained=true`,
  `failure_code=""`, `related_sessions=null/[]`, and a live `opencode acp`
  process after the run.

## Impact

Noema can accept simple-run cleanup, but cannot accept Matrix fork/resume as
production-safe. Experience benchmarks that require `matrix_fork` guidance are
blocked because strict LLM guidance fails closed before any suggestion can be
delivered.

## Requested Fix

- Reconcile or reap the retained parent/fork ACP client when possible.
- If retention remains, report `failure_code=run_related_session_retained`.
- Populate `related_sessions` with the retained session/client evidence.
- Add a regression test for a fork child using a parent agent client where final
  cleanup must not be ambiguous and must not leave an unreported ACP process.
