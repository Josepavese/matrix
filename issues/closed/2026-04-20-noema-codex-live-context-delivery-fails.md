# Noema diagnostic: Codex ACP live context attach is accepted but final delivery fails

Date: 2026-04-20

## Summary

Noema ran a short active-sidecar diagnostic through Matrix with Codex ACP.

Matrix accepted two `attach_context` actions during the active run, but both final delivery events failed shortly before run completion:

```text
live context delivery failed: ACP prompt failed: client context cancelled
```

This means Noema can observe Codex and learn from the run, but cannot yet claim Codex live in-run suggestion delivery.

## Noema Artifact

Repository:

```text
/home/jose/hpdev/Libraries/noema
```

Run artifact:

```text
programs/evaluation-platform/artifacts/phase73-active-sidecar-short-codex/runs/phase72-active-sidecar-short-phase72-active-short-slug-001-codex-active-learned-seed-7
```

Matrix run id:

```text
run-187cf2f7-b5ab-4202-8ff1-a080297fa3fb
```

## Observed Events

Accepted attach events:

- sequence `44`, `run.context.attached`, status `accepted`, delivery id `ctx-24cb006b-3217-4539-9d86-1c6cc130b377`
- sequence `45`, `run.context.attached`, status `accepted`, delivery id `ctx-6dde1e2a-13e1-48dd-bcb0-173c79695edf`

Failed delivery events:

- sequence `570`, `run.context.attached`, status `failed`, message `live context delivery failed: ACP prompt failed: client context cancelled`
- sequence `571`, `run.context.attached`, status `failed`, message `live context delivery failed: ACP prompt failed: client context cancelled`

Run completion:

- sequence `574`, `run.completed`, status `completed`

## Noema Counters

Codex `active_learned`:

- status: `succeeded`
- duration: `104.880s`
- timeline events: `584`
- patterns available: `1`
- patterns learned: `1`
- suggestions generated: `2`
- suggestions delivered: `0`
- suggestions received before completion: `0`
- suggestions blocked: `2`

## Request

Please inspect Codex ACP live context delivery semantics in Matrix:

- Is Codex ACP expected to support mid-run prompt/context delivery?
- Is Matrix racing delivery against run completion?
- Can Matrix emit a more specific provider capability/failure code than generic `client context cancelled`?
- Should Matrix mark this provider path as `unsupported`, `late`, or `failed` earlier so Noema can avoid repeated blocked suggestions?

This is not reported as a confirmed Matrix core bug. It is a provider/adapter interoperability gap surfaced by Noema active-sidecar evaluation.

## Follow-Up: Longer PolicyFlow Diagnostic Confirms The Same Failure

Noema also ran a longer Codex PolicyFlow pair:

```text
programs/evaluation-platform/artifacts/phase73-active-sidecar-longrun-codex-active-pair/runs/phase71-active-sidecar-longrun-phase71-policyflow-active-warmup-001-codex-active-learned-seed-7
```

Matrix run id:

```text
run-14cc0ee9-fbca-45bd-b574-74601c03881d
```

Result:

- `active_cold`: succeeded, `309.728s`, `645` Matrix events, `1` pattern learned
- `active_learned`: succeeded, `299.238s`, `709` Matrix events, `2` suggestions generated, `0` delivered, `2` blocked

Accepted attach events:

- sequence `50`, delivery id `ctx-16740c5f-1831-4c88-a58e-ba3548cb091b`
- sequence `51`, delivery id `ctx-352b8e10-3a68-4889-bb61-dcd3faeda3a7`

Failed delivery events:

- sequence `681`, `live context delivery failed: ACP prompt failed: client context cancelled`
- sequence `682`, `live context delivery failed: ACP prompt failed: client context cancelled`
- sequence `685`, `run.completed`

This rules out the simplest explanation that the short task ended too quickly. In the long pair, Matrix accepted the attach requests roughly five minutes before completion, but final Codex ACP delivery still failed only near completion.

## Follow-Up: Interrupt/Resume Fallback Needs Clean Cancel Cleanup

Noema implemented a provider fallback arm named `active_learned_resume` for agents that do not reliably consume live context.

The fallback flow is:

1. start the first Matrix run asynchronously
2. observe structural events until Noema has learned suggestions
3. call Matrix run action `cancel` with reason `noema_active_sidecar_resume`
4. wait for a clean Matrix cleanup proof for the interrupted run
5. start a second Matrix run with the same workspace plus a visible `<noema type="interrupt-resume-context">...</noema>` capsule

This works conceptually and previously succeeded when Noema allowed weak cleanup proof, but Noema now correctly gates the flow on clean cleanup before starting the resume run.

Latest strict run:

```text
/home/jose/hpdev/Libraries/noema/programs/evaluation-platform/artifacts/phase73-active-sidecar-short-codex-resume-v2
/home/jose/hpdev/Libraries/noema/programs/evaluation-platform/artifacts/phase73-active-sidecar-short-codex-resume-v3
```

Result:

- `v2 active_cold`: succeeded in `82.701s`
- `v2 active_learned_resume`: failed before resume because initial interrupted run cleanup was not clean
- `v3 active_cold`: succeeded in `129.834s`
- `v3 active_learned_resume`: failed in `59.574s` with the same cleanup failure, confirming reproducibility after Noema fixed failed-record duration accounting

Failure:

```text
active resume initial cleanup not clean: remote_delete: failed to start agent /home/jose/.nvm/versions/node/v22.12.0/bin/codex-acp: context canceled; remote_cancel: failed to start agent /home/jose/.nvm/versions/node/v22.12.0/bin/codex-acp: context canceled
```

Cleanup proof exposed to Noema:

```text
active_sidecar_resume_initial_matrix_cleanup_proof=true
active_sidecar_resume_initial_matrix_cleanup_clean=false
active_sidecar_resume_initial_matrix_cleanup_local_forgotten=true
active_sidecar_resume_initial_matrix_cleanup_remote_deleted=false
active_sidecar_resume_initial_matrix_cleanup_remote_closed=false
active_sidecar_resume_initial_matrix_cleanup_remote_canceled=false
active_sidecar_resume_initial_matrix_cleanup_process_reaped=false
```

Noema-side interpretation:

- This should not be hidden by Noema; production evaluation must fail if interrupted agent sessions cannot be proven clean.
- For Codex, live attach appears unreliable, so interrupt/resume is the practical fallback.
- Interrupt/resume depends on Matrix being able to cancel/close/reap the interrupted run without trying to start a fresh ACP agent under an already-canceled context.

Requested Matrix behavior:

- Provide a clean cleanup path for `run action=cancel` on async runs even when the provider is Codex ACP.
- If remote session deletion/cancel requires an agent process, use a cleanup context that is independent from the canceled run context.
- Emit a stable machine-readable cleanup failure class, for example `agent_start_context_cancelled_during_cleanup`, so Noema can separate Matrix cleanup gaps from provider capability gaps.
- Preserve local-forgotten evidence but do not mark cleanup clean unless the provider process/session is deleted, closed, canceled, or otherwise proven unreachable without leak.

This is now a blocker for treating Codex as production-grade in active Noema sidecar mode.

Related generic cleanup blocker, also reproduced with OpenCode:

```text
/home/jose/hpdev/Libraries/matrix/issues/2026-04-20-noema-interrupt-resume-cleanup-context-cancelled.md
```

## Matrix Maintainer Response

Accepted as two separate findings.

### Codex ACP Live Context

Matrix does not treat Codex ACP as proven mid-turn interruptible.

ACP standardizes `session/cancel`, not guaranteed injection of a second prompt
into an already running turn. Matrix real-agent probes showed:

- OpenCode ACP consumed live context before run completion.
- Codex through `codex-acp` accepted live context requests but did not consume
  them before completion.
- Gemini CLI ACP showed the same late/non-consumed behavior.

Matrix product semantics now document this as provider capability, not protocol
guarantee:

- `accepted` means Matrix accepted/queued delivery.
- `delivered` before `run.completed` is in-run delivery proof.
- `late` means provider did not consume the context before run terminal state.
- Codex active sidecar should use interrupt/resume, not live attach, until
  `codex-acp` proves mid-turn live context consumption.

### Cleanup Blocker

The generic interrupt/resume cleanup blocker was confirmed and fixed under:

```text
issues/closed/2026-04-20-noema-interrupt-resume-cleanup-context-cancelled.md
```

Implemented cleanup changes:

- cleanup after async run cancel uses a bounded context detached from the
  canceled run context;
- context-canceled execution maps to `run.cancelled`;
- cleanup proof includes optional `failure_code`;
- stable failure code added:
  `agent_start_context_cancelled_during_cleanup`.

Validation:

```text
go test -race ./internal/providers/runapi ./internal/logic/sessioncleanup ./internal/logic/session
```

Result: passed.

Installed-runtime Codex ACP cleanup smoke:

```text
run_id=run-870360ce-b2dd-4adf-b361-0371ace5603e
status=cancelled
session.cleanup clean=true
remote_canceled=true
process_reaped=true
failure_code=""
```

Status: closed as Matrix-side action complete. Noema should rerun Codex
`active_learned_resume`; live Codex attach remains provider capability gap, not
Matrix core bug.
