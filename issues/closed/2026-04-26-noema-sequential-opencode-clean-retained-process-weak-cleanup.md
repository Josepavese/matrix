# Noema sequential OpenCode runs: clean=true but retained process weak cleanup

## Summary

Noema experience-only Matrix/OpenCode sequential batches intermittently receive cleanup proof with `clean=true` but `strong_cleanup=false` because the OpenCode ACP process is retained:

`process_retention_reason=other local sessions still reference agent client`

This leaves Noema with a clean logical session but incomplete strong cleanup evidence for production-grade evaluation reports.

## Evidence

Repository: `/home/jose/hpdev/Libraries/noema`

Matrix base URL: `http://127.0.0.1:9091`

Batch command shape:

```bash
go run ./cmd/noema-eval run plan --config-dir ./configs --batch-plan ./examples/live/phase84-evidence-hardening-midrun-plan.json --agents opencode --arms active_cold,active_learned_resume --matrix --matrix-base-url http://127.0.0.1:9091 --parallelism 1
```

Observed records:

- `/home/jose/hpdev/Libraries/noema/programs/evaluation-platform/artifacts/phase91-experience-production-midlong-v2/runs/phase84-evidence-hardening-midrun-phase84-ledger-holdout-003-opencode-active-cold-seed-7/execution-record.json`
- `/home/jose/hpdev/Libraries/noema/programs/evaluation-platform/artifacts/phase91-experience-production-midlong-v3/runs/phase84-evidence-hardening-midrun-phase84-url-config-exploitation-002-opencode-active-cold-seed-7/execution-record.json`

Common cleanup shape:

```json
{
  "clean": true,
  "cleanup_strength": "retained",
  "local_forgotten": true,
  "process_reaped": false,
  "process_retained": true,
  "process_retention_allowed": true,
  "process_retention_reason": "other local sessions still reference agent client",
  "remote_canceled": false,
  "remote_closed": false,
  "remote_deleted": false,
  "strong_cleanup": false,
  "weak_cleanup_reason": "process_retained"
}
```

After the batch completes, no `opencode acp` process remains, so final process cleanup appears to happen eventually. The issue is that the per-run cleanup record cannot prove strong cleanup at the point Noema writes execution evidence.

## Expected

For `session_policy=new_ephemeral_delete_after_run` and `cleanup_policy=delete_remote_or_cancel_and_forget_local`, each completed sequential run should return one of:

- remote delete/close/cancel proof;
- process reap proof;
- or a distinct deferred-cleanup proof that can later be reconciled into strong batch-level evidence.

## Impact

Noema can validate task success and session cleanliness, but production-grade reports remain blocked from claiming `strong_cleanup_runs == observed_runs`.

## Matrix Maintainer Response

Status: accepted and fixed.

Root cause: `/v1/runs` correctly created an ephemeral policy session through
`session.policy.applied`, but final cleanup selected the post-run active session
snapshot. If a fork, judge, sidecar, or other workflow changed channel active
session before completion, Matrix could cleanup a different mirrored session.
That produced retained-process cleanup evidence for the wrong logical session
instead of strong cleanup evidence for the policy-owned run session.

Fix:

- `prepareSessionForRun` now returns the prepared policy session snapshot.
- `cleanupRunSession` is called with that prepared policy session when
  `session_policy=new_ephemeral_delete_after_run` is active.
- Active-session changes after route no longer move ephemeral cleanup to a
  different logical session.
- Documentation now states that ephemeral policy cleanup is pinned to the
  `session.policy.applied` logical session.

Verification:

- `go test ./internal/providers/runapi`
- `go test ./internal/providers/runapi -run TestHandleRuns_EphemeralCleanupTargetsPolicySessionWhenActiveChanges -v`
- `go run ./scripts/code_governance.go --config code-governance.toml`

The new regression test simulates a post-route active-session switch and proves
that cleanup still targets the original prepared policy session.
