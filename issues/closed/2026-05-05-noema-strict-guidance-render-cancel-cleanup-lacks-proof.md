# Noema strict-guidance render cancellation returns cleanup without remote/process proof

## Status

Closed.

## Reporter

Noema experience-layer evaluation.

## Summary

While validating the latest Matrix cleanup fixes from Noema, the main
digest-seeded `active_learned_resume + matrix_fork` diagnostic passed with
strong cleanup. However, a separate negative-path diagnostic exposed a cleanup
proof gap when Noema cancels an initial Matrix run after strict LLM guidance
rendering fails before fork guidance is accepted.

Matrix returned:

```text
clean=false
strong_cleanup=false
cleanup_strength=failed
failure_code=cleanup_clean_without_remote_or_process_proof
process_retained=false
process_retention_reason="no matching cached agent client"
remote_cancel_attempted=false
remote_deleted=false
remote_closed=false
remote_canceled=false
```

No `opencode acp` process was retained after the run, so this is not the same
bug as the earlier `run_related_session_retained` failure. The issue is that
Matrix cannot provide a production-safe cleanup proof for a run that Noema
asked to cancel after a strict guidance rendering failure.

## Matrix Version

Installed binary:

```text
matrix 0.1.17-snapshot
commit be97bb41dabb98c5bfbbe299b25c69c40f0fd350
built 2026-05-04T22:51:43Z
```

Daemon:

```text
/home/jose/.local/share/matrix/bin/matrix run
```

## Noema Diagnostic Command

```bash
cd /home/jose/hpdev/Libraries/noema/programs/evaluation-platform
env NOEMA_SEMANTIC_PROVIDER=ollama \
  NOEMA_SEMANTIC_OLLAMA_MODEL=embeddinggemma:latest \
  NOEMA_OUTCOME_CRITIC_PROVIDER=auto \
  NOEMA_OUTCOME_CRITIC_MATRIX_URL=http://127.0.0.1:9091 \
  go run ./cmd/noema-eval run plan \
  --batch-plan examples/live/archive/pre-non-interference/phase91-resume-embedding-diagnostic-after-matrix-fix-plan.json \
  --output-dir artifacts/phase91-resume-embedding-diagnostic-after-matrix-fix-v6 \
  --active-seed-records ./artifacts/phase91-experience-production-midlong-v5/runs/phase84-evidence-hardening-midrun-phase84-validation-backed-parser-warmup-001-opencode-active-cold-seed-7/execution-record.json \
  --matrix \
  --active-interpreter matrix_fork \
  --require-llm-guidance \
  --parallelism 1
```

Batch artifact:

```text
/home/jose/hpdev/Libraries/noema/programs/evaluation-platform/artifacts/phase91-resume-embedding-diagnostic-after-matrix-fix-v6/batch-execution.json
```

## Noema Result

The run correctly selected interrupt/resume:

```text
active_resume_preflight_patterns=1
active_resume_preflight_positive_routines=0
active_resume_preflight_failure_scars=1
active_resume_preflight_actionable_failure_scars=1
active_resume_preflight_decision=interrupt_resume
```

Then Noema rejected the Matrix fork guidance path because the seeded legacy scar
did not contain the newly required sanitized experience digest:

```text
active_interpreter_fork_attempts=1
active_interpreter_fork_accepted=0
active_interpreter_fork_rejected=1
active_interpreter_fork_reject_reason=matrix fork interpreter requires sanitized experience digest
active_sidecar_strict_guidance_render_failed_cancel_requested=true
active_sidecar_strict_guidance_render_failed_cancel_accepted=true
```

Final Matrix cleanup proof:

```text
matrix_cleanup_proof=true
matrix_cleanup_clean=false
matrix_cleanup_strong=false
matrix_cleanup_local_forgotten=true
matrix_cleanup_remote_deleted=false
matrix_cleanup_remote_closed=false
matrix_cleanup_remote_canceled=false
matrix_cleanup_process_reaped=false
matrix_cleanup_process_retained=false
matrix_cleanup_process_retention_allowed=false
matrix_cleanup_strength=failed
matrix_cleanup_process_retention_reason=no matching cached agent client
matrix_cleanup_failure_code=cleanup_clean_without_remote_or_process_proof
```

Structured cleanup JSON:

```json
{
  "agent_id": "opencode",
  "clean": false,
  "cleanup_policy": "delete_remote_or_cancel_and_forget_local",
  "cleanup_strength": "failed",
  "failure_code": "cleanup_clean_without_remote_or_process_proof",
  "local_forgotten": true,
  "logical_session_id": "4f0eb834-4a39-489c-8147-3b700aa1b70e",
  "process_reap_attempted": true,
  "process_reaped": false,
  "process_retention_reason": "no matching cached agent client",
  "remote_cancel_attempted": false,
  "remote_canceled": false,
  "remote_close_attempted": false,
  "remote_closed": false,
  "remote_delete_attempted": false,
  "remote_deleted": false,
  "strong_cleanup": false
}
```

## Post-Run Process Evidence

After this diagnostic and after the successful digest-seeded diagnostic, the
process tree contained only the Matrix daemon and no retained OpenCode ACP
client:

```text
/home/jose/.local/share/matrix/bin/matrix run
```

No `/home/jose/.local/bin/opencode acp` process remained.

## Control Diagnostic That Passed

A second diagnostic using a current judge-written sanitized `experience_digest`
seed exercised the intended full path and passed:

```text
status=succeeded
stop_reason=end_turn
active_resume_preflight_decision=interrupt_resume
active_interpreter_fork_attempts=1
active_interpreter_fork_accepted=1
active_sidecar_resume_intervention_proven=true
```

Initial cleanup:

```text
active_sidecar_resume_initial_matrix_cleanup_clean=true
active_sidecar_resume_initial_matrix_cleanup_strong=true
active_sidecar_resume_initial_matrix_cleanup_process_retained=false
active_sidecar_resume_initial_matrix_cleanup_related_sessions=2
active_sidecar_resume_initial_matrix_cleanup_related_sessions_retained=0
```

Final cleanup:

```text
matrix_cleanup_clean=true
matrix_cleanup_strong=true
matrix_cleanup_strength=strong
matrix_cleanup_process_retained=false
matrix_cleanup_failure_code=""
related_sessions=[]
```

Control artifact:

```text
/home/jose/hpdev/Libraries/noema/programs/evaluation-platform/artifacts/phase91-web-research-resume-digest-diagnostic-after-matrix-fix-v1/batch-execution.json
```

## Interpretation

The previous `run_related_session_retained` blocker appears fixed for the real
digest-backed Noema fork/resume path.

This issue is narrower:

- Noema starts an initial Matrix async run.
- Noema tries to render strict LLM guidance.
- The experience layer rejects the candidate before guidance is accepted,
  because the seeded scar lacks the required sanitized digest.
- Noema requests cancellation and Matrix reports cancellation accepted through
  the Noema path.
- The final Matrix cleanup proof lacks both remote cleanup proof and process
  reap proof, even though no process remains.

Possible Matrix-side causes to check:

- the run may not yet have a cached ACP client when cleanup runs;
- cancellation may be accepted at a higher run-control layer without producing
  a session-level remote cancel/delete/close proof;
- cleanup may lose the session/client association after `local_forgotten=true`;
- a run with no matching cached agent client may need an explicit
  `not_started_or_no_provider_process_observed` proof state instead of
  `cleanup_clean_without_remote_or_process_proof`.

## Requested Fix / Contract Clarification

For this cancellation-before-guidance-accepted path, Matrix should either:

```text
clean=true
strong_cleanup=true
cleanup_strength=strong
process_retained=false
failure_code=""
```

with explicit proof that no remote session/provider process existed or remained,
or it should keep failing closed but expose enough structured ownership/proof
data for Noema to distinguish:

```text
provider process never started
provider process already exited
provider client cache missing
remote session unknown
remote session cleanup not attempted
```

Passing criteria:

```text
no cleanup_clean_without_remote_or_process_proof for a cancellation path where
Matrix can prove no remote/provider process exists or remains
```

Failing criteria:

```text
clean=false
cleanup_strength=failed
failure_code=cleanup_clean_without_remote_or_process_proof
process_retained=false
remote_*_attempted=false
no related session or process proof explaining why cleanup is safe
```

## Maintainer Resolution

Accepted as a generic Matrix cleanup-proof gap, not as a Noema-specific
feature. The missing state was the cancellation-before-materialization path:
Matrix could know that no remote session id had been produced and that no exact
workspace-bound provider client was cached, but it only exposed the legacy
`process_retention_reason=no matching cached agent client` diagnostic. For an
ephemeral run, that correctly failed closed, but it gave integrators no strong
proof for the safe "not started / no provider process exists" case.

Implemented contract:

```text
process_absent=true
process_absence_reason="no matching cached agent client"
remote_session_id=""
clean=true
strong_cleanup=true
cleanup_strength=strong
failure_code=""
```

This proof is intentionally narrow. `process_absent=true` is strong only when no
remote session id was ever materialized. If Matrix knows a remote session id,
process absence remains diagnostic data and cleanup still requires remote
delete/close/cancel or process reap proof.

Files changed for this issue:

- `internal/middleware/link.go`: added structured process-absence proof fields
  to `SessionCleanupResult`.
- `internal/logic/sessioncleanup/cleanup.go`: made process absence a strong
  proof only for the no-remote-session path.
- `internal/logic/session/manager_cleanup_process.go`: records process absence
  explicitly when the reaper finds no matching cached agent client.
- `internal/logic/session/manager_cleanup_result.go`: feeds the new proof into
  cleanup strength evaluation.
- `internal/logic/sessioncleanup/cleanup_test.go`,
  `internal/logic/session/manager_test.go`, and
  `internal/providers/runapi/runs_test.go`: added unit and API trace coverage.
- `docs/matrix_agent_communication_run_trace.md`,
  `docs/matrix_protocol_neutral_runtime.md`,
  `docs/matrix_live_context_interrupt_policy.md`, and
  `docs/wiki/API-Reference.md`: documented the exact proof boundary.

Validation:

```text
go test ./internal/logic/sessioncleanup ./internal/logic/session ./internal/providers/runapi -count=1
go test ./... -count=1
go run ./scripts/code_governance.go --config code-governance.toml
```

All passed. The passing acceptance criterion is now covered by
`TestHandleRuns_RouteFailureAcceptsUnmaterializedProcessAbsenceProof`, which
verifies that the `/v1/runs` cleanup trace carries `process_absent=true` and
does not emit `cleanup_clean_without_remote_or_process_proof` for a
cancellation-before-provider-materialization path.
