# Noema request: OpenCode cleanup reports clean without remote/process proof

## Context

Noema Phase 77 reran the PolicyFlow holdout through Matrix/OpenCode using `active_learned_resume`.

Artifact:

`/home/jose/hpdev/Libraries/noema/programs/evaluation-platform/artifacts/phase77-active-sidecar-structural-opencode-holdout`

The run failed before resume:

```text
active resume initial cleanup not clean
```

Noema intentionally blocks resume unless the interrupted run has cleanup proof stronger than local forgetting.

## Observed cleanup notes

```text
active_sidecar_resume_initial_matrix_cleanup_proof=true
active_sidecar_resume_initial_matrix_cleanup_clean=true
active_sidecar_resume_initial_matrix_cleanup_local_forgotten=true
active_sidecar_resume_initial_matrix_cleanup_remote_deleted=false
active_sidecar_resume_initial_matrix_cleanup_remote_closed=false
active_sidecar_resume_initial_matrix_cleanup_remote_canceled=false
active_sidecar_resume_initial_matrix_cleanup_process_reaped=false
```

This is contradictory for Noema's resume safety contract:

- Matrix says `clean=true`
- but there is no remote delete/close/cancel proof
- and no process reap proof

Noema cannot safely resume from this because local forgetting alone can leave the interrupted provider process/session alive.

Follow-up note:

A second OpenCode rerun later reached resume successfully and produced strong cleanup proof (`remote_canceled=true`, `process_reaped=true`). This means the issue appears intermittent or path-dependent, not a constant OpenCode capability absence. The request still stands because Matrix should not emit `clean=true` on any path that lacks remote/process/equivalent proof for an interrupt/resume workflow.

## Request

Please clarify Matrix cleanup semantics for ACP/OpenCode:

- If only local forgetting happened, `clean` should probably be `false` or accompanied by a failure/weak-cleanup code.
- If Matrix knows the remote/process is actually stopped, expose one of:
  - `remote_deleted=true`
  - `remote_closed=true`
  - `remote_canceled=true`
  - `process_reaped=true`
- If OpenCode has a provider-specific safe cleanup state, expose it as explicit structural proof rather than relying on `clean=true`.

## Desired invariant

For interrupt/resume-capable runs, this should hold:

```text
clean=true => remote_deleted || remote_closed || remote_canceled || process_reaped || equivalent_explicit_provider_proof
```

If Matrix cannot prove that invariant, Noema will continue to block resume and mark the run diagnostic.

## Noema-side action already taken

Noema now reports the specific unsafe reason:

```text
cleanup_clean_without_remote_or_process_proof
```

This keeps the product gate strict while making the integration failure easier to diagnose.

## Matrix maintainer response

Status: accepted and implemented.

Matrix cleanup now distinguishes operational cleanup from strong cleanup proof:

- `clean`
- `strong_cleanup`
- `cleanup_strength`
- `weak_cleanup_reason`

For ephemeral interrupt/resume flows, `clean=true` now requires at least one
strong proof:

- `remote_deleted=true`
- `remote_closed=true`
- `remote_canceled=true`
- `process_reaped=true`

If Matrix only forgets local state for an ephemeral run, cleanup fails with
`failure_code=cleanup_clean_without_remote_or_process_proof`.

For shared non-ephemeral sessions, retained provider clients remain possible,
but they are no longer ambiguous: Matrix reports
`cleanup_strength=retained`, `process_retained=true`, and
`weak_cleanup_reason=process_retained`.

The final `error` field now represents terminal cleanup failure only. Fallback
attempts such as unsupported `session/delete` followed by successful
`session/close` or `session/cancel` stay visible through structured flags but do
not pollute successful cleanup as an error.

Validation:

- Unit tests cover ephemeral local-only failure, strong remote-cancel proof, and
  retained-process weak cleanup.
- Real-agent smoke passed on installed Matrix with `opencode`, `codex-acp`, and
  `gemini`.
- Observed proofs: OpenCode/Gemini use `remote_canceled=true` plus
  `process_reaped=true`; Codex uses `remote_closed=true` plus
  `process_reaped=true`.
