# Noema verification still hits OpenCode `agent_preflight_failed` after provider lifetime fix

Date: 2026-05-08

Status: closed

Previous issue:

```text
issues/closed/2026-05-08-noema-opencode-preflight-cleanup-regression.md
```

Matrix version under test:

```text
matrix 0.1.17-snapshot
commit be97bb41dabb98c5bfbbe299b25c69c40f0fd350
built 2026-05-08T08:02:49Z
```

The daemon was explicitly restarted after the memory batch ended:

```text
PID 2961892 started Fri May 8 10:48:55 2026
/home/jose/.local/share/matrix/bin/matrix run
```

No concurrent Noema wiki-memory/OpenCode run was active when this verification started.

## Verification Command

```bash
env NOEMA_SEMANTIC_PROVIDER=ollama \
  NOEMA_SEMANTIC_OLLAMA_MODEL=embeddinggemma:latest \
  NOEMA_OUTCOME_CRITIC_PROVIDER=auto \
  NOEMA_OUTCOME_CRITIC_MATRIX_URL=http://127.0.0.1:9091 \
  go run ./cmd/noema-eval run plan \
  --batch-plan examples/live/phase91-experience-long-activities-stress-cold1-warm4-plan.json \
  --output-dir artifacts/phase91-experience-long-activities-stress-cold1-warm4-v4-matrix-fix-verify \
  --matrix \
  --active-interpreter matrix_fork \
  --require-llm-guidance \
  --parallelism 1
```

Noema artifact:

```text
/home/jose/hpdev/Libraries/noema/programs/evaluation-platform/artifacts/phase91-experience-long-activities-stress-cold1-warm4-v4-matrix-fix-verify
```

## What Still Fails

The first run used OpenCode through Matrix, hit the intended Noema wall timeout, and cleanup was strong:

```json
{
  "run_id": "phase91-experience-long-activities-stress-cold1-warm4-phase91-long-product-strategy-001-opencode-active-cold-seed-7",
  "arm_id": "active_cold",
  "family_id": "long-product-strategy",
  "status": "failed",
  "stop_reason": "noema_active_sidecar_wall_timeout",
  "matrix_run_id": "run-47bc61fc-7619-4742-bca1-67a7c341bdbe",
  "logical_session_id": "1eb97328-4515-4806-aeff-34e0883ebdc3",
  "remote_session_id": "ses_1f938dc0bffehrP2w15IXC91CL",
  "cleanup": {
    "clean": true,
    "strong_cleanup": true,
    "cleanup_strength": "strong",
    "failure_code": "",
    "process_retained": false,
    "related_sessions": []
  }
}
```

Immediately after this, Noema ran the Matrix-backed post-task outcome critic. That critic failed closed:

```json
{
  "schema": "noema.outcome_critic.primary_error.v1",
  "error": "matrix http status=502: code=agent_preflight_failed"
}
```

Noema execution record notes:

```text
outcome_critic_primary_error=matrix http status=502: code=agent_preflight_failed
outcome_critic_failed_closed=true
outcome_critic_error=matrix-backed outcome critic failed closed: matrix http status=502: code=agent_preflight_failed
```

The later warm product run also failed before useful Matrix/Noema work:

```json
{
  "run_id": "phase91-experience-long-activities-stress-cold1-warm4-phase91-long-product-strategy-001-opencode-active-learned-resume-seed-7",
  "arm_id": "active_learned_resume",
  "family_id": "long-product-strategy",
  "status": "failed",
  "stop_reason": "error",
  "matrix_run_id": "run-66179edc-abdb-43cb-a054-ccac6976fa5a",
  "logical_session_id": "ac0ada90-27f8-4347-abd3-dffaa41cd6d0",
  "active_mode": "observe_only",
  "outcome_critic_primary_error": "matrix http status=502: code=agent_preflight_failed",
  "cleanup": {
    "clean": true,
    "cleanup_strength": "retained",
    "process_retained": true,
    "related_sessions": []
  }
}
```

The full batch completed and exposed one more remaining cleanup failure:

```json
{
  "run_id": "phase91-experience-long-activities-stress-cold1-warm4-phase91-long-creative-campaign-001-opencode-active-learned-resume-seed-7",
  "arm_id": "active_learned_resume",
  "family_id": "long-creative-campaign",
  "status": "failed",
  "matrix_run_id": "run-716fbfab-e2e7-4053-b7cb-09b5cac83f27",
  "logical_session_id": "fd129b3c-1a52-4906-87aa-8c1d2e96f72d",
  "remote_session_id": "ses_1f932d47fffeFO0YCi9o8ZpizC",
  "initial_resume_run_id": "run-24148782-47f1-4655-8f64-83cb148ec2fb",
  "initial_resume_cleanup": {
    "clean": true,
    "strong_cleanup": true,
    "cleanup_strength": "strong",
    "related_sessions": 2,
    "related_sessions_retained": 0
  },
  "fork_interpreter": {
    "attempts": 1,
    "accepted": 1,
    "rejected": 0
  },
  "final_cleanup": {
    "clean": false,
    "strong_cleanup": false,
    "cleanup_strength": "failed",
    "failure_code": "cleanup_clean_without_remote_or_process_proof",
    "process_reaped": false,
    "process_retention_reason": "no matching cached agent client"
  }
}
```

Matrix runtime log for the same verification class still shows the underlying provider error:

```text
[agent_preflight_failed] agent provider preflight failed agent=opencode protocol=acp phase=session/prompt: ACP prompt failed: context canceled
```

This means the previous fix improved diagnostics and some cleanup cases, but Noema still cannot accept the issue as resolved. A normal Matrix-backed OpenCode request immediately after a timed/cancelled OpenCode run can still fail with provider preflight/context cancellation.

## Exact Matrix Test Needed

Please add a Matrix-side regression test that reproduces the Noema lifecycle without depending on Noema.

Suggested test name:

```text
TestOpenCode_RunTimeoutCleanupThenImmediateJudgeRun_DoesNotPreflightFail
```

Suggested integration command:

```bash
MATRIX_SMOKE_TEST=1 MATRIX_OPENCODE_ACP_STDIO=1 \
  go test ./tests/integration \
  -run TestOpenCode_RunTimeoutCleanupThenImmediateJudgeRun_DoesNotPreflightFail \
  -count=1 -timeout 12m -v
```

Required test shape:

1. Start Matrix with real OpenCode ACP stdio, using the same installed `/home/jose/.local/bin/opencode`.
2. Create workspace `A`.
3. Start an async OpenCode run in workspace `A` with:
   - `session_policy=new_ephemeral_delete_after_run`
   - `cleanup_policy=delete_remote_or_cancel_and_forget_local`
   - a prompt that causes active work for long enough that the test can cancel while the provider is inside `session/prompt`.
4. Cancel the run through the same path Noema uses for wall timeout.
5. Wait for cleanup result.
6. Assert cleanup contract:
   - `clean=true`
   - `strong_cleanup=true`
   - `cleanup_strength="strong"`
   - `process_retained=false`
   - `failure_code=""`
   - no `related_sessions[].retained=true`
7. Immediately create workspace `B`.
8. Start a fresh Matrix OpenCode request in workspace `B` that simulates the post-task judge:
   - same `agent_id=opencode`
   - short prompt
   - no resume
   - no fork
   - no concurrent run
9. Assert the judge-like request does not return HTTP `502`.
10. Assert the trace does not contain:
   - `provider.preflight.failed`
   - `agent_preflight_failed`
   - `provider_client_context_cancelled`
   - `ACP prompt failed: context canceled`
11. Assert the request reaches at least:
   - `session.created`
   - `agent.prompt.sent`
   - terminal `run.completed` or normal agent final message
12. Repeat the same sequence with workspace `B == A`, because Noema can run post-task critic/fork in a related workspace immediately after the original run.

The important invariant is:

```text
Cancelling a turn context must never poison the router-lifetime provider client or the next client created for the same agent/workspace key.
```

## Unit-Level Test Also Needed

Please add a cheaper router/provider test using a fake ACP client so the bug is caught without real OpenCode.

Suggested test name:

```text
TestRouter_PromptContextCancellationDoesNotPoisonNextClient
```

Required fake behavior:

- Fake factory records the context passed to `NewClient`.
- Fake client `Prompt(ctx, ...)` blocks until `ctx.Done()` and returns `context.Canceled`.
- First request uses a cancellable turn context and is cancelled while inside `Prompt`.
- Cleanup runs.
- Second request uses a fresh non-cancelled context for the same agent/workspace.

Assertions:

- The context passed to `NewClient` must not be the cancelled turn context.
- The second request must not fail with `context.Canceled`, `client context cancelled`, or `agent_preflight_failed`.
- If the old client is evicted, Matrix must tombstone/reap it and the second request must create a fresh client with a live context.
- Cleanup proof must remain strong or fail closed with an explicit retained related session.

Pseudocode:

```go
func TestRouter_PromptContextCancellationDoesNotPoisonNextClient(t *testing.T) {
    router := newRouterWithFakeACPClient()

    ctx1, cancel1 := context.WithCancel(context.Background())
    run1 := startAsyncPrompt(ctx1, router, agentID("opencode"), workspace("A"), promptThatBlocks())
    waitUntilFakeClientEnteredPrompt()
    cancel1()

    cleanup1 := cleanupRun(run1)
    require.True(t, cleanup1.StrongCleanup)
    require.False(t, cleanup1.ProcessRetained)

    ctx2 := context.Background()
    run2 := startAsyncPrompt(ctx2, router, agentID("opencode"), workspace("B"), "judge this completed task")

    require.NoError(t, run2.Err)
    require.NotContains(t, run2.Events, "provider.preflight.failed")
    require.NotContains(t, run2.ErrorText, "context canceled")
    require.NotContains(t, run2.ErrorText, "agent_preflight_failed")
}
```

## Diagnostic Contract Expected On Failure

If Matrix still must return `agent_preflight_failed`, the HTTP response and trace should include enough fields for Noema to make a production-safe decision:

```json
{
  "code": "agent_preflight_failed",
  "failure_reason": "provider_client_context_cancelled",
  "provider_error": "ACP prompt failed: context canceled",
  "phase": "session/prompt",
  "agent_id": "opencode",
  "protocol": "acp",
  "transport": "stdio",
  "workspace_path": "...",
  "client_key": "opencode:<workspace>",
  "run_id": "...",
  "remote_session_id": "...",
  "cleanup": {
    "strong_cleanup": false,
    "cleanup_strength": "failed",
    "process_retained": true,
    "failure_code": "run_related_session_retained",
    "related_sessions": [
      {
        "remote_session_id": "...",
        "retained": true,
        "reason": "..."
      }
    ]
  }
}
```

Noema should not receive only:

```text
matrix http status=502: code=agent_preflight_failed
```

## Second Exact Matrix Test Needed

Please also add a Matrix-side regression test for the final cleanup failure after run-owned fork/resume.

Suggested test name:

```text
TestOpenCode_RunOwnedForkResume_FinalCleanupStrongAfterInitialCleanup
```

Suggested integration command:

```bash
MATRIX_SMOKE_TEST=1 MATRIX_OPENCODE_ACP_STDIO=1 \
  go test ./tests/integration \
  -run TestOpenCode_RunOwnedForkResume_FinalCleanupStrongAfterInitialCleanup \
  -count=1 -timeout 12m -v
```

Required test shape:

1. Start a normal OpenCode async run in workspace `A`.
2. Use the same resume/fork path that Noema uses for `active_learned_resume`.
3. Force/cancel the initial run so Matrix performs initial cleanup.
4. Assert initial cleanup is strong and any `related_sessions` are explicit:
   - `strong_cleanup=true`
   - `cleanup_strength="strong"`
   - `related_sessions_retained=0`
5. Start the resumed/final run produced by that fork/resume path.
6. Let it complete or cancel it through the normal Noema cleanup policy.
7. Assert final cleanup is production-safe:
   - `clean=true`
   - `strong_cleanup=true`
   - `cleanup_strength="strong"`
   - `process_retained=false`
   - `failure_code=""`
   - no retained related sessions
8. Fail the test if final cleanup returns:
   - `cleanup_clean_without_remote_or_process_proof`
   - `process_retention_reason="no matching cached agent client"`
   - `remote_session_id` present but no remote/process proof

This test should specifically cover the case where the initial resume cleanup has related-session evidence and no retained related sessions, but the final run later loses the cached-client/process proof for its own `remote_session_id`.

## Acceptance Criteria

This issue is acceptable from Noema only when:

- The integration test above passes with real OpenCode ACP stdio.
- The fake-client router test above passes.
- The run-owned fork/resume final-cleanup integration test above passes.
- A fresh Noema rerun of `phase91-experience-long-activities-stress-cold1-warm4-plan.json` on a restarted Matrix daemon has no `outcome_critic_primary_error=matrix http status=502: code=agent_preflight_failed`.
- Any failed run returns production-safe cleanup proof:
  - `strong_cleanup=true`
  - `cleanup_strength="strong"`
  - `process_retained=false`
  - `failure_code=""`
  - no retained related sessions

## Maintainer Resolution

Matrix accepted this as a real regression. The previous router-lifetime fix
kept provider clients from inheriting the request context, but it did not evict
an already-poisoned workspace client after a turn was cancelled while
`session/prompt` was in flight. A following judge/follow-up run could therefore
reuse a client whose ACP read loop had already closed and fail with
`client context cancelled`.

Implemented fix:

- `Router.Route` now evicts the exact `agent_id + workspace_path` client after
  cancellable prompt failures (`context canceled`, `context deadline exceeded`,
  or provider diagnostics such as `provider_client_context_cancelled`).
- Eviction closes the old stdio ACP process and preserves a remote-session
  tombstone, including the remote id returned by the failed turn.
- Cleanup can consume that tombstone as process proof, so cancelled ephemeral
  runs remain `clean=true`, `strong_cleanup=true`,
  `cleanup_strength=strong`, `process_retained=false`, and
  `failure_code=""`.
- The next same-agent request for the same or a different workspace creates a
  fresh provider client and must not inherit `client context cancelled`.
- Documentation now records this invariant in the run trace, live context, API,
  and protocol-neutral runtime docs.

Regression tests added:

- `TestRouter_PromptContextCancellationDoesNotPoisonNextClient`
- `TestOpenCode_RunTimeoutCleanupThenImmediateJudgeRun_DoesNotPreflightFail`
- `TestOpenCode_RunOwnedForkResume_FinalCleanupStrongAfterInitialCleanup`

Real OpenCode ACP evidence:

```bash
MATRIX_SMOKE_TEST=1 MATRIX_OPENCODE_ACP_STDIO=1 \
  go test ./tests/integration \
  -run TestOpenCode_RunTimeoutCleanupThenImmediateJudgeRun_DoesNotPreflightFail \
  -count=1 -timeout 12m -v
```

Passed. The run cancelled inside OpenCode ACP `session/prompt`, Matrix evicted
the poisoned client, cleanup was strong, and immediate judge-like requests in
both a different workspace and the same workspace returned `MATRIX_JUDGE_OK`
without HTTP 502 or provider preflight poison.

```bash
MATRIX_SMOKE_TEST=1 MATRIX_OPENCODE_ACP_STDIO=1 \
  go test ./tests/integration \
  -run TestOpenCode_RunOwnedForkResume_FinalCleanupStrongAfterInitialCleanup \
  -count=1 -timeout 12m -v
```

Passed. The final ephemeral run cleanup stayed strong and non-retained after the
fork/resume path.
