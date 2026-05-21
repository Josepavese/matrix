# Noema active resume now fails closed but still retains OpenCode ACP client

## Status

Closed.

## Reporter

Noema experience-layer evaluation.

## Summary

Matrix fixed the previous unsafe cleanup contract: Noema no longer receives
`clean=true` when a standalone fork-child cleanup retains the parent ACP client.
The latest Noema canary now fails closed with structured retained-session
evidence.

That is a real improvement and satisfies the most important safety requirement.

However, the production path is still blocked:

- `active_learned_resume` reaches the correct Noema decision to use
  `interrupt_resume`;
- Matrix fork guidance children are started and complete several artifact turns;
- Matrix returns fail-closed cleanup for standalone fork children;
- the warm run fails before guidance can be resumed into the agent;
- after the run, one `opencode acp` process remains alive under `matrix run`.

This means the cleanup proof is now honest, but the live `active_learned_resume`
path is still not production-usable for Noema with `matrix_fork`.

## Matrix Version

```text
matrix 0.1.17-snapshot
commit be97bb41dabb98c5bfbbe299b25c69c40f0fd350
built 2026-05-04T14:44:28Z
```

Matrix daemon used by the run:

```text
625800 Mon May  4 16:44:38 2026 /home/jose/.local/share/matrix/bin/matrix run
```

Targeted tests passed before the run:

```text
go test ./internal/logic/session ./internal/providers/matrixapi ./internal/providers/runapi ./internal/providers/agents -count=1
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
  --output-dir artifacts/phase91-experience-web-research-matrix-fix-cold-resume-v5 \
  --matrix \
  --active-interpreter matrix_fork \
  --require-llm-guidance \
  --parallelism 1
```

Batch artifact:

```text
/home/jose/hpdev/Libraries/noema/programs/evaluation-platform/artifacts/phase91-experience-web-research-matrix-fix-cold-resume-v5/batch-execution.json
```

## Relevant IDs

Noema warm run:

```text
phase91-experience-web-research-matrix-fix-cold-resume-phase91-web-research-tight-budget-001-opencode-active-learned-resume-seed-7
```

Noema Matrix channel:

```text
noema-eval-channel-084a980109a9c75c74aeecf04e1c8a51
```

Matrix run id from runtime log:

```text
run-de7ca05c-66ad-4798-8857-335604b74b8c
```

Initial active run logical session:

```text
807448cb-4c42-4935-b45a-fa528dd8cfbf
```

Fork child logical sessions observed during warm guidance rendering:

```text
01e5bca5-ea1d-4d14-b671-8050c7904acb
f8c03133-0341-4b9a-8abd-f809121ffe77
49c141ef-f877-48a3-ada0-15600306a075
d4245e23-00f6-41c1-8bb3-b1f41be219d4
a207cf99-dd8d-438a-99f2-c32921c8bf65
d1d781e7-ff81-4811-9159-561dd768eb17
```

## What Improved

The previous issue asked Matrix not to expose `clean=true` for retained cleanup.
That is now fixed in the live canary.

Noema received:

```text
active_sidecar_resume_initial_matrix_cleanup_clean=false
active_sidecar_resume_initial_matrix_cleanup_strong=false
active_sidecar_resume_initial_matrix_cleanup_strength=failed
active_sidecar_resume_initial_matrix_cleanup_failure_code=run_related_session_retained
active_sidecar_resume_initial_matrix_cleanup_related_sessions=1
active_sidecar_resume_initial_matrix_cleanup_related_sessions_retained=1
```

Noema rejected it with:

```text
active resume initial cleanup not clean: standalone fork child cleanup retained its parent agent client
```

This is correct fail-closed behavior.

## Still Blocked

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
active_resume_preflight_patterns=1
active_resume_preflight_positive_routines=0
active_resume_preflight_failure_scars=1
active_resume_preflight_actionable_failure_scars=1
active_resume_preflight_decision=interrupt_resume
status=failed
error=active resume initial cleanup not clean: standalone fork child cleanup retained its parent agent client
```

So Noema does find and select the failure scar, but cannot proceed because
Matrix cannot produce production-safe initial cleanup for this live fork/resume
path.

## Matrix Runtime Evidence

Relevant runtime log lines:

```text
2026-05-04T16:57:54 matrix async run cancelled run_id=run-76432f66-1475-4e44-b1cf-a9a8c206ede8 cleanup_clean=true strong_cleanup=true cleanup_strength=strong failure_code=""
2026-05-04T16:59:27 route_started logical_session=807448cb-4c42-4935-b45a-fa528dd8cfbf channel=noema-eval-channel-084a980109a9c75c74aeecf04e1c8a51
2026-05-04T17:00:01 route_started logical_session=01e5bca5-ea1d-4d14-b671-8050c7904acb
2026-05-04T17:00:33 route_started logical_session=f8c03133-0341-4b9a-8abd-f809121ffe77
2026-05-04T17:00:47 route_started logical_session=49c141ef-f877-48a3-ada0-15600306a075
2026-05-04T17:00:47 route_started logical_session=d4245e23-00f6-41c1-8bb3-b1f41be219d4
2026-05-04T17:01:07 cleanup failed target=d4245e23-00f6-41c1-8bb3-b1f41be219d4 failure_code=run_related_session_retained clean=false process_retained=true
2026-05-04T17:01:08 cleanup failed target=01e5bca5-ea1d-4d14-b671-8050c7904acb failure_code=run_related_session_retained clean=false process_retained=true
2026-05-04T17:01:12 cleanup failed target=49c141ef-f877-48a3-ada0-15600306a075 failure_code=run_related_session_retained clean=false process_retained=true
2026-05-04T17:01:27 cleanup failed target=f8c03133-0341-4b9a-8abd-f809121ffe77 failure_code=run_related_session_retained clean=false process_retained=true
2026-05-04T17:01:45 cleanup failed target=d1d781e7-ff81-4811-9159-561dd768eb17 failure_code=run_related_session_retained clean=false process_retained=true
2026-05-04T17:02:05 cleanup failed target=a207cf99-dd8d-438a-99f2-c32921c8bf65 failure_code=run_related_session_retained clean=false process_retained=true
2026-05-04T17:02:05 matrix async run cancelled run_id=run-de7ca05c-66ad-4798-8857-335604b74b8c cleanup_clean=false strong_cleanup=false cleanup_strength=failed failure_code=run_related_session_retained
```

## Post-Run Process Tree

Immediately after the Noema run completed:

```text
625800 /home/jose/.local/share/matrix/bin/matrix run
635320 /home/jose/.local/bin/opencode acp
```

Detailed tree:

```text
matrix,625800 run
  |-opencode,635320 acp
  |   |-{opencode},635321
  |   |-{opencode},635322
  |   |-{opencode},635323
  |   |-{opencode},635324
  |   |-{opencode},635325
  |   |-{opencode},635326
  |   |-{opencode},635327
  |   |-{opencode},635328
  |   |-{opencode},635332
  |   |-{opencode},635333
  |   |-{opencode},635343
  |   |-{opencode},635354
  |   `-{opencode},635355
```

## Interpretation

This is no longer the old bug where Matrix returned unsafe proof as clean. The
new behavior is safer and more truthful.

The remaining issue is that the Matrix fork/resume transport cannot yet execute
Noema's active learned resume path without accumulating standalone fork-child
retention. Matrix either needs:

- a lifecycle path where fork guidance children are cleaned as part of the
  parent subtree with parent process proof, not as standalone retained children;
- a stronger cleanup/reap mechanism for the shared parent ACP client after the
  active-resume preflight failure;
- or a capability-level answer that this `matrix_fork` plus OpenCode
  `active_learned_resume` path is not currently production-safe.

Until this is resolved, Noema should keep blocking benchmark claims for this
capability.

## Requested Matrix Follow-Up

Please verify whether the retained `opencode acp` client after this failed
active-resume run is expected or a lifecycle leak.

Acceptance criteria for Noema:

- if Matrix returns failure, no related OpenCode ACP client should remain alive
  unless explicitly documented as retained with operator cleanup requirements;
- a successful active-resume path must return strong cleanup before Noema
  resumes guidance into the agent;
- if this transport shape cannot support that, Matrix should expose capability
  status so Noema can downgrade `matrix_fork`/OpenCode `interrupt_resume`.

## Matrix Maintainer Response

Accepted as a generic Matrix lifecycle issue.

The previous fix made the proof safe but still left the run-owned fork subtree in
a non-productive state: standalone fork-child cleanup could fail closed while the
shared parent ACP client stayed alive. That is safe from a proof standpoint, but
not sufficient for production cleanup.

Implemented behavior:

- forced cleanup of a run-owned ephemeral fork child can now remediate
  `fork child uses parent agent client` retention by cleaning the ephemeral
  parent owner as a subtree;
- when parent owner cleanup proves the shared client is closed/reaped/gone, the
  child cleanup is promoted to `clean=true`, `strong_cleanup=true`,
  `cleanup_strength=strong`;
- the parent owner appears as a non-retained related session with reason
  `fork_parent_agent_client_owner`;
- inline fork artifact cleanup suppresses this reverse parent cleanup so a
  temporary child cleanup cannot delete a parent session that must be restored;
- `/v1/runs` related-session accounting now recognizes related parent proof
  already covered by child cleanup and does not double-clean it.

Validation:

```text
go test ./internal/logic/session ./internal/providers/runapi -count=1
go test ./...
git diff --check
go run ./scripts/code_governance.go --config code-governance.toml
bash scripts/deploy_local.sh
```

Installed runtime:

```text
matrix 0.1.17-snapshot
commit be97bb41dabb98c5bfbbe299b25c69c40f0fd350
built 2026-05-04T15:13:27Z

matrix run PID 645588
```

Real `opencode` ACP smoke passed against the installed daemon:

```text
HTTP status: 201
clean: true
strong_cleanup: true
cleanup_strength: strong
process_reaped: true
process_retained: false
failure_code: empty
parent related sessions: 1
retained related sessions: 0

MATRIX_REAL_RUN_OWNED_FORK_CHILD_PARENT_OWNER_REMEDIATION_OK
```

After the smoke, exact process checks found no remaining
`/home/jose/.local/bin/opencode acp` process.
