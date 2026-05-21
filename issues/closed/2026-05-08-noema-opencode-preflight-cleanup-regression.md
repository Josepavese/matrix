# Noema/OpenCode Matrix preflight and cleanup regression during experience stress run

Date: 2026-05-08

## Status

Closed.

Matrix version:

```text
matrix 0.1.17-snapshot
commit be97bb41dabb98c5bfbbe299b25c69c40f0fd350
built 2026-05-05T08:16:27Z
```

Noema artifact:

```text
/home/jose/hpdev/Libraries/noema/programs/evaluation-platform/artifacts/phase91-experience-long-activities-stress-cold1-warm4-v3-current
```

Noema command:

```bash
env NOEMA_SEMANTIC_PROVIDER=ollama NOEMA_SEMANTIC_OLLAMA_MODEL=embeddinggemma:latest NOEMA_OUTCOME_CRITIC_PROVIDER=auto NOEMA_OUTCOME_CRITIC_MATRIX_URL=http://127.0.0.1:9091 go run ./cmd/noema-eval run plan --batch-plan examples/live/phase91-experience-long-activities-stress-cold1-warm4-plan.json --output-dir artifacts/phase91-experience-long-activities-stress-cold1-warm4-v3-current --matrix --active-interpreter matrix_fork --require-llm-guidance --parallelism 1
```

## What Happened

The run was experience-only and non-interference. It used OpenCode through Matrix with ephemeral sessions and cleanup policy `delete_remote_or_cancel_and_forget_local`.

The first cold run behaved as expected:

- Run: `phase91-long-product-strategy-001`, `active_cold`, seed `7`
- Matrix run id: `run-dfe7b664-b9b2-4b70-a5cf-1ea802f52779`
- Logical session: `9533fa29-c959-45a6-9a1b-0456dd0f7e57`
- Remote session: `ses_1fb6fb315ffeu7wk5iwwCiOwGq`
- Stop reason: `noema_active_sidecar_wall_timeout`
- Matrix cleanup: `clean=true`, `strong_cleanup=true`, `cleanup_strength=strong`, `process_reaped=true`, `process_retained=false`

Noema learned a failure scar from this run. The following warm run loaded the scar and generated one Matrix-fork LLM guidance item, then attempted interrupt/resume. The resume path failed:

- Run: `phase91-long-product-strategy-001`, `active_learned_resume`, seed `7`
- Record Matrix run id: `run-30829b63-4aac-4a63-b009-c855d814b982`
- Trace also contains the pre-resume run id: `run-d8c6b3df-db00-4a66-b8b7-830baa08824a`
- Logical session: `44992678-a4c7-457f-9795-523910772007`
- Noema active evidence: `patterns_available=1`, `suggestions_generated=1`, `suggestions_resumed=1`, `active_sidecar_resume_intervention_proven=true`
- Final status: `failed`
- Stop reason: `error`
- Matrix trace terminal events: `run.failed` and `provider.preflight.failed`
- `provider.preflight.failed` metadata:

```json
{
  "adapter": "opencode",
  "agent_id": "opencode",
  "code": "agent_preflight_failed",
  "command": "/home/jose/.local/bin/opencode",
  "phase": "session/new",
  "protocol": "acp",
  "transport": "stdio"
}
```

Matrix cleanup for this warm product run was still strong:

```json
{
  "agent_id": "opencode",
  "clean": true,
  "cleanup_policy": "delete_remote_or_cancel_and_forget_local",
  "cleanup_strength": "strong",
  "local_forgotten": true,
  "logical_session_id": "44992678-a4c7-457f-9795-523910772007",
  "process_reap_attempted": true,
  "process_reaped": true,
  "remote_cancel_attempted": false,
  "remote_canceled": false,
  "remote_close_attempted": false,
  "remote_closed": false,
  "remote_delete_attempted": false,
  "remote_deleted": false,
  "strong_cleanup": true
}
```

The next cold run exposed a second Matrix/provider problem on the post-task judge path:

- Run: `phase91-long-creative-campaign-001`, `active_cold`, seed `7`
- Matrix run id: `run-ad2d7330-e88e-4ffb-a078-c4358c2718a4`
- Logical session: `90176739-b32d-40ff-9cde-9fcece3f7ac6`
- Remote session: `ses_1fb6bd459ffe35ijbwRhHTf2CS`
- Stop reason: `noema_active_sidecar_wall_timeout`
- Matrix cleanup: `clean=true`, `strong_cleanup=true`, `cleanup_strength=strong`, `process_reaped=true`, `process_retained=false`
- Outcome critic failed closed because Matrix returned:

```text
matrix http status=502: code=agent_preflight_failed
```

Noema wrote the raw failure to:

```text
/home/jose/hpdev/Libraries/noema/programs/evaluation-platform/artifacts/phase91-experience-long-activities-stress-cold1-warm4-v3-current/runs/phase91-experience-long-activities-stress-cold1-warm4-phase91-long-creative-campaign-001-opencode-active-cold-seed-7/outcome-critic-primary-error.json
```

The final warm creative run then failed cleanup proof:

- Run: `phase91-long-creative-campaign-001`, `active_learned_resume`, seed `7`
- Matrix run id: `run-0b6eae1c-c11c-4709-a0b3-3e18f6a63c76`
- Logical session: `0ce0c930-3f65-4b7e-94ab-a20f2334de87`
- Remote session: `ses_1fb6a5ae6ffevylbmfEEHkwkGc`
- Noema preflight: `active_resume_preflight_decision=observe_only`, `active_resume_preflight_skip_reason=no_patterns`
- Noema error: `matrix cleanup proof not clean`
- Cleanup failure code: `cleanup_clean_without_remote_or_process_proof`
- Cleanup JSON:

```json
{
  "agent_id": "opencode",
  "clean": false,
  "cleanup_policy": "delete_remote_or_cancel_and_forget_local",
  "cleanup_strength": "failed",
  "failure_code": "cleanup_clean_without_remote_or_process_proof",
  "local_forgotten": true,
  "logical_session_id": "0ce0c930-3f65-4b7e-94ab-a20f2334de87",
  "process_reap_attempted": true,
  "process_reaped": false,
  "process_retention_reason": "no matching cached agent client",
  "protocol_kind": "acp",
  "remote_cancel_attempted": false,
  "remote_canceled": false,
  "remote_close_attempted": false,
  "remote_closed": false,
  "remote_delete_attempted": true,
  "remote_deleted": false,
  "remote_session_id": "ses_1fb6a5ae6ffevylbmfEEHkwkGc",
  "strong_cleanup": false
}
```

## Why This Looks Like a Matrix/Provider Issue

Noema did not use parallelism inside the experience batch (`--parallelism 1`). The product warm run reached the expected Noema state before failure: a prior scar was available, Matrix fork LLM guidance was generated, resume context was attached, and Noema recorded `active_sidecar_resume_intervention_proven=true`.

The failure happened after Matrix attempted to start the resumed OpenCode ACP session and reported `provider.preflight.failed` at `phase=session/new`. That suggests one of these Matrix-side possibilities:

- OpenCode ACP preflight failed because another OpenCode ACP process/session was still active or not isolated.
- Matrix considered an ACP client/session reusable or unavailable incorrectly after cancel/resume.
- Matrix did not propagate the underlying OpenCode stderr/exit reason, so Noema only sees a generic `502 agent_preflight_failed`.
- Matrix cleanup proof can still become non-production-safe after a later observe-only run even when no active resume guidance was attempted.

There was also a separate wiki-memory Matrix/OpenCode run active at the same time, launched by another process. If Matrix/OpenCode cannot safely support concurrent ACP sessions, Matrix should expose a deterministic capacity/lock error and still provide strong cleanup proof or explicit retained related-session details. Noema cannot treat generic `502 agent_preflight_failed` plus missing cleanup proof as production-safe.

Post-run process tree contained:

```text
1833455 /home/jose/.local/share/matrix/bin/matrix run
2755832 go run ./cmd/noema-eval run plan --batch-plan ./examples/live/phase96-wikimemory-live-research-closed-loop-plan.json --matrix --output-dir ./artifacts/phase96-wikimemory-live-research-closed-loop-v1
2755903 noema-eval run plan --batch-plan ./examples/live/phase96-wikimemory-live-research-closed-loop-plan.json --matrix --output-dir ./artifacts/phase96-wikimemory-live-research-closed-loop-v1
2760132 /home/jose/.local/bin/opencode acp
```

The remaining OpenCode process appears associated with the concurrent wiki-memory run, not the completed experience batch, but Matrix should confirm this through `related_sessions`.

## Expected Matrix Behavior

For the resume preflight failure:

- Return the underlying OpenCode ACP preflight stderr/exit details, not only `agent_preflight_failed`.
- Include whether failure is due to provider capacity, stale session, retained client, workspace affinity, channel conflict, missing auth, or process launch failure.
- Preserve the parent/resume run relationship in the returned trace.

For cleanup:

- Noema should consider cleanup production-safe only when `strong_cleanup=true`, `cleanup_strength="strong"`, `process_retained=false`, `failure_code=""`, and no `related_sessions[].retained=true`.
- If Matrix cannot prove remote deletion/close/cancel or process reaping, return `strong_cleanup=false` and a precise `failure_code`.
- Include `related_sessions` for the failed cleanup case, especially whether `ses_1fb6a5ae6ffevylbmfEEHkwkGc` was retained, reaped, unreferenced, or never associated with a cached process.

## Requested Matrix-Side Checks

1. Inspect Matrix logs for run ids `run-d8c6b3df-db00-4a66-b8b7-830baa08824a`, `run-30829b63-4aac-4a63-b009-c855d814b982`, `run-ad2d7330-e88e-4ffb-a078-c4358c2718a4`, and `run-0b6eae1c-c11c-4709-a0b3-3e18f6a63c76`.
2. Verify why OpenCode ACP preflight failed at `phase=session/new`.
3. Verify whether concurrent OpenCode ACP sessions are supported. If not, Matrix should serialize, reject with explicit capacity reason, or mark provider capability accordingly.
4. Verify cleanup proof for remote session `ses_1fb6a5ae6ffevylbmfEEHkwkGc`; return `related_sessions` and retained/reaped state.
5. Confirm whether `cleanup_clean_without_remote_or_process_proof` is expected for observe-only runs with no cached client, or whether Matrix should produce stronger evidence such as `run_unreferenced_agent_client_reaped`.

## Maintainer Resolution

Accepted as a generic Matrix provider-lifecycle and cleanup-proof regression,
not as a Noema-specific feature request.

Root cause:

- Matrix created cached stdio ACP clients with the current `/v1/runs` request
  context.
- When the run context was cancelled or ended during preflight/cleanup pressure,
  the cached OpenCode ACP process could be killed before cleanup/resume proof
  completed.
- A dead exact workspace client seen during remote lifecycle lookup was reported
  as `no reusable cached agent client`, but that lookup did not create a
  session-bound process tombstone. The later strict cleanup step therefore had
  a known `remote_session_id` but no remote delete/close/cancel proof and no
  process-reap proof, producing
  `cleanup_clean_without_remote_or_process_proof`.

Implemented Matrix-side fix:

- Cached provider clients are now router-lifetime resources, not request-lifetime
  resources. Per-turn prompt/delete/cancel calls still receive the turn context,
  but cancelling one run no longer cancels the reusable provider process itself.
- Remote lifecycle lookup now evicts and tombstones a dead exact workspace
  client before returning
  `remote_lifecycle_skipped_no_reusable_cached_agent_client`.
- Strict cleanup can consume that tombstone as `process_reaped=true` only for
  matching tracked `remote_session_id` values.
- Provider failures now expose `provider_error` and `failure_reason` in
  response/event diagnostics. The observed
  `ACP new session failed: client context cancelled` class is reported as
  `failure_reason=provider_client_context_cancelled`.

Concurrency interpretation:

- The logs show multiple OpenCode ACP stdio clients running in separate
  workspaces. Matrix's intended model supports this through `agent_id +
  workspace_path` client keys.
- The regression was not treated as proof that OpenCode has no concurrency
  support. It was a Matrix lifetime/proof bug: request cancellation could kill a
  cached client and cleanup could miss the resulting process proof.

Files changed:

- `internal/providers/agents/router.go`
- `internal/providers/agents/router_clients.go`
- `internal/providers/agents/router_tombstones.go`
- `internal/providers/agents/provider_failure.go`
- `internal/providers/agents/router_recovery_test.go`
- `internal/providers/agents/provider_failure_test.go`
- `docs/matrix_agent_communication_run_trace.md`
- `docs/matrix_protocol_neutral_runtime.md`
- `docs/matrix_live_context_interrupt_policy.md`
- `docs/wiki/API-Reference.md`
- `code-governance.toml`
- `docs/governance/code_debt_register.md`

Validation:

```text
go test ./internal/providers/agents ./internal/logic/session ./internal/providers/runapi -count=1
go test ./... -count=1
go run ./scripts/code_governance.go --config code-governance.toml
MATRIX_SMOKE_TEST=1 MATRIX_OPENCODE_ACP_STDIO=1 go test ./tests/integration -run 'TestOpenCodeHTTPFinalRunCleanupScopesReconcileLikeNoemaResume|TestSmoke_OpenCodeWS_ProjectsToolEvents' -count=1 -timeout 12m -v
```

All passed.
