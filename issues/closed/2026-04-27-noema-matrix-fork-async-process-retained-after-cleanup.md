# Matrix fork async process retained after Noema cleanup

## Summary

Noema completed a Matrix/OpenCode `active_learned` run with Matrix cleanup reported as strong/clean, but one `opencode acp` process launched by Matrix remained alive under `matrix run` after the batch had finished.

Noema did not modify Matrix. The retained process was killed manually after evidence collection to avoid contaminating later runs.

## Evidence

- Noema artifact:
  - `/home/jose/hpdev/Libraries/noema/programs/evaluation-platform/artifacts/phase91-experience-web-research-noninterference-v1`
- Run:
  - `phase91-experience-web-research-noninterference-001-opencode-active-learned-seed-7`
- Matrix cleanup in execution record:
  - `matrix_cleanup.clean=true`
  - `matrix_cleanup.cleanup_strength=strong`
  - `process_reaped=true`
  - `remote_canceled=true`
  - main logical session id: `896aefc7-0f2f-4882-9e6d-57ad9e56419e`
- Active interpreter:
  - `active_interpreter_effective=matrix_fork`
  - `active_interpreter_fork_attempts=1`
  - `active_interpreter_fork_accepted=1`
  - `active_interpreter_parent_session_id=a74c2cb3-c441-4a3d-bb76-f9bae8a72edd`

Process snapshot after batch completion showed:

```text
14389 ... matrix run
83079 ... /home/jose/.local/bin/opencode acp
```

The retained process was not the main run process after Noema completed; it appeared to be associated with Matrix fork interpreter activity.

## Expected

When a Matrix fork action is created as ephemeral with cleanup policy `delete_remote_or_cancel_and_forget_local`, Matrix should close/cancel/reap the fork child process as part of fork completion or as part of the parent/session cleanup path.

If Matrix intentionally keeps fork parent/session processes alive, the cleanup proof should not report strong/clean without also accounting for retained fork children.

## Impact

Noema can report clean Matrix cleanup while a fork-related ACP process remains alive. This can contaminate later evaluation runs and makes cleanup evidence too optimistic for `matrix_fork` active interpreter runs.

## Requested check

Please verify Matrix cleanup accounting for async `session_action=fork` jobs:

- whether fork child sessions/processes are included in cleanup proof
- whether completed async fork jobs are reaped automatically
- whether parent cleanup should cascade to fork children
- whether cleanup proof should include retained fork process evidence when any fork child remains alive

## Matrix maintainer response

Accepted.

The issue is valid as a cleanup-accounting gap. A single-session cleanup proof
was too narrow for fork-based workflows: it could prove cleanup for the target
session while a fork-related child/parent process remained outside that proof.

Implemented policy:

- `delete` / `cleanup` is now fork-subtree aware.
- Before cleaning a target session, Matrix cleans mirrored fork children that
  reference the target logical session or remote parent session.
- Parent cleanup proof now includes `fork_children_cleaned` and nested
  `fork_children` cleanup records.
- Child cleanup may temporarily report `process_retained=true` while the parent
  mirror still exists, but the final parent cleanup must account for the shared
  `agent_id + workspace_path` client and reap it when no Matrix session still
  references it.
- Async fork jobs are idempotent with parent teardown: if the parent cleanup
  already removed the child, the job records
  `fork_child_cleanup_already_missing` instead of turning cleanup accounting
  into a false failure.

Verification added:

```text
TestSessionManager_EphemeralParentCleanupCascadesToForkChild
TestSessionManager_ParentCleanupCascadesToRunningAsyncForkChild
```

Targeted test command:

```text
go test ./internal/logic/session -run 'TestSessionManager_(EphemeralParentCleanupCascadesToForkChild|ParentCleanupCascadesToRunningAsyncForkChild|ForkAsyncReturnsBeforeChildTurnCompletes|ForkInputCanCleanupChildAndRestoreParent)' -count=1 -v
```

Result: passed.
