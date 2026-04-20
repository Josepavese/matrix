# Noema blocker: async run cancel cleanup uses canceled context and prevents interrupt/resume

Date: 2026-04-20

## Summary

Noema added a provider fallback arm named `active_learned_resume`.

This arm is required for agents that do not reliably consume live `attach_context` while a run is active. Instead of relying on mid-run prompt delivery, Noema:

1. starts a Matrix run asynchronously
2. watches live events until learned sidecar suggestions become available
3. requests `run action=cancel` with reason `noema_active_sidecar_resume`
4. waits for a clean cleanup proof for the interrupted run
5. starts a fresh Matrix run in the same workspace with a visible `<noema type="interrupt-resume-context">...</noema>` capsule

The current Matrix cleanup result after action cancel is not clean. It appears Matrix tries to start the provider process during remote delete/cancel while using a context that has already been canceled.

This blocks Noema from treating interrupt/resume as production-safe.

## Affected Providers

Observed with both Codex ACP and OpenCode:

```text
remote_delete: failed to start agent /home/jose/.nvm/versions/node/v22.12.0/bin/codex-acp: context canceled
remote_cancel: failed to start agent /home/jose/.nvm/versions/node/v22.12.0/bin/codex-acp: context canceled
```

```text
remote_delete: failed to start agent /home/jose/.local/bin/opencode: context canceled
remote_cancel: failed to start agent /home/jose/.local/bin/opencode: context canceled
```

## Noema Evidence

Repository:

```text
/home/jose/hpdev/Libraries/noema
```

Codex strict resume run:

```text
programs/evaluation-platform/artifacts/phase73-active-sidecar-short-codex-resume-v3
```

Result:

- `active_cold`: succeeded in `129.834s`
- `active_learned_resume`: failed in `59.574s`
- cleanup proof existed but was not clean

OpenCode strict resume run:

```text
programs/evaluation-platform/artifacts/phase74-active-sidecar-short-opencode-resume
```

Result:

- `active_cold`: succeeded in `82.177s`
- `active_learned_resume`: failed in `68.594s`
- cleanup proof existed but was not clean

Shared cleanup flags:

```text
matrix_cleanup_proof=true
matrix_cleanup_clean=false
matrix_cleanup_local_forgotten=true
matrix_cleanup_remote_deleted=false
matrix_cleanup_remote_closed=false
matrix_cleanup_remote_canceled=false
matrix_cleanup_process_reaped=false
```

## Why This Matters

Noema cannot safely start the resume run unless the interrupted run is proven cleaned up.

Allowing resume after weak cleanup would risk:

- leaked provider processes
- hidden active sessions inside Codex/OpenCode
- contaminated evaluation evidence
- false claims that interrupt/resume is production-grade

Noema now correctly fails this arm when cleanup is not clean.

## Requested Matrix Behavior

Please make async `run action=cancel` produce a clean cleanup proof when cleanup is possible:

- use a cleanup context that survives the canceled run context
- do not start the provider under a context that is already canceled
- if provider startup is required for remote delete/cancel, run it under a bounded cleanup-specific context
- return a stable machine-readable failure class when cleanup cannot be completed
- keep `local_forgotten=true` separate from `clean=true`; local forgetting alone is not enough for Noema production evidence

Suggested failure code:

```text
agent_start_context_cancelled_during_cleanup
```

## Acceptance Criteria

- Noema can call Matrix action `cancel` on an async run.
- Matrix returns or later exposes cleanup proof with one of:
  - `remote_deleted=true`
  - `remote_closed=true`
  - `remote_canceled=true`
  - `process_reaped=true`
- `clean=true` is only set when the interrupted session/process is actually cleaned up or proven unreachable without leak.
- The behavior works for both Codex ACP and OpenCode where provider support allows it.

This is a Matrix integration blocker for Noema `active_learned_resume`.

## Matrix Maintainer Response

Accepted.

Root cause confirmed: async run cancellation canceled the run execution context,
then cleanup reused that same canceled context. Provider cleanup could therefore
fail while trying to start/reuse the ACP process for remote delete/cancel.

Implemented changes:

- `/v1/runs/{run_id}/actions` `cancel` now allows the execution path to map
  `context.Canceled` to `run.cancelled`, not `run.failed`.
- Ephemeral run cleanup now uses a bounded cleanup context detached from the
  canceled run context.
- Post-run session enrichment after cancellation also uses a detached bounded
  context.
- `SessionCleanupResult` now exposes optional `failure_code`.
- Cleanup metadata and `session.cleanup` trace events include `failure_code`.
- Stable failure code added:
  `agent_start_context_cancelled_during_cleanup`.
- Regression test added for async cancel cleanup proving cleanup context is not
  canceled and produces clean proof.

Validation:

```text
go test -race ./internal/providers/runapi ./internal/logic/sessioncleanup ./internal/logic/session
```

Result: passed.

Installed-runtime validation:

```text
matrix readiness --expect-runtime-up
```

Result: passed.

OpenCode ACP async cancel smoke:

```text
run_id=run-4f69acc2-dce8-4e88-9ddb-4d24b294c7bc
status=cancelled
session.cleanup clean=true
local_forgotten=true
process_reaped=true
failure_code=""
```

Codex ACP async cancel smoke:

```text
run_id=run-870360ce-b2dd-4adf-b361-0371ace5603e
status=cancelled
session.cleanup clean=true
remote_canceled=true
process_reaped=true
remote_delete_unsupported=true
failure_code=""
```

Status: closed in Matrix. Noema should rerun `active_learned_resume` against the
next installed Matrix build/release.
