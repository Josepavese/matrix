# Matrix fork async process retained after cleanup, still reproduces on 0.1.16-snapshot

## Summary

Noema completed a Matrix/OpenCode `active_learned` run with Matrix cleanup reported as strong/clean, but one `opencode acp` process launched by Matrix remained alive under `matrix run` after the batch had finished.

Noema did not modify Matrix code. The retained process was killed manually after evidence collection to avoid contaminating later runs.

## Matrix Version

```text
matrix 0.1.16-snapshot
commit fcd679a5225f0a4cfa459709cdd55092b44730e1
built 2026-04-27T15:23:50Z
```

## Noema Evidence

Artifact:

```text
/home/jose/hpdev/Libraries/noema/programs/evaluation-platform/artifacts/phase91-experience-web-research-noninterference-v2
```

Run:

```text
phase91-experience-web-research-noninterference-001-opencode-active-learned-seed-7
```

Relevant Noema record fields:

```text
active_interpreter_effective=matrix_fork
active_interpreter_fork_attempts=1
active_interpreter_fork_accepted=1
active_interpreter_parent_session_id=576419c1-f4c4-4e97-9474-a53ea28b4ae1
matrix_cleanup.clean=true
matrix_cleanup.cleanup_strength=strong
matrix_cleanup.process_reaped=true
matrix_cleanup.remote_canceled=true
```

Post-run process evidence after waiting 30 seconds:

```text
166339 ... matrix run
174174 ... /home/jose/.local/bin/opencode acp
```

## Expected

When a Matrix fork action is created as ephemeral with cleanup policy `delete_remote_or_cancel_and_forget_local`, Matrix should close/cancel/reap the fork child process as part of fork completion or parent/session cleanup.

If Matrix intentionally keeps fork parent/session processes alive, the cleanup proof must not report strong/clean without accounting for retained fork children.

## Impact

Noema can report clean Matrix cleanup while a fork-related ACP process remains alive. This contaminates later evaluation runs and makes cleanup evidence too optimistic for `matrix_fork` active interpreter runs.

## Requested Check

Please verify Matrix cleanup accounting for async `session_action=fork` jobs:

- whether fork child sessions/processes are included in cleanup proof
- whether completed async fork jobs are reaped automatically
- whether parent cleanup cascades to fork children
- whether cleanup proof reports retained fork process evidence when any fork child remains alive

## Matrix Maintainer Response

Accepted and fixed.

The reproduced trace showed two different session identities:

- The ephemeral run policy session was `89a7b6aa-0693-48d1-afe0-8d3511a05d10`; Matrix cleaned this session and reported `cleanup_strength=strong`.
- The run trace later resumed/attached context on `576419c1-f4c4-4e97-9474-a53ea28b4ae1`; that related session was not part of the primary cleanup proof.

The first fork-subtree fix was therefore incomplete: it handled children of the
cleanup target, but it did not account for run-related sessions outside that
target. That allowed a primary cleanup proof to look strong while a related
workspace agent process remained alive.

Implemented changes:

- `/v1/runs` ephemeral cleanup now snapshots session state before and after the run.
- Cleanup still targets the prepared policy session first.
- New owned related sessions discovered after the run are cleaned as supplemental cleanup targets.
- Pre-existing/shared related sessions are explicitly reported in `related_sessions` and downgrade the run cleanup proof to `cleanup_strength=retained` with `process_retained=true`.
- Cleanup events now use the shared cleanup metadata projection, so run traces include `fork_children`, `fork_children_cleaned`, and `related_sessions`.
- Fork child cleanup no longer reaps the workspace agent client directly; fork children retain the shared parent workspace client with reason `fork child uses parent agent client`, leaving parent/run cleanup responsible for final process reap.
- Documentation and governance were updated for the new cleanup proof semantics.

Verification:

- `go test ./internal/providers/runapi ./internal/logic/session ./internal/logic/sessioncleanup ./internal/providers/matrixapi ./internal/providers/agents ./pkg/zedacp -count=1`
- `go test ./internal/logic/session -count=10`
- `go run ./scripts/code_governance.go --config code-governance.toml`
- `bash scripts/deploy_preflight.sh`

Status: closed.
