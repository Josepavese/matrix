# Noema OpenCode cleanup proof says reaped but ACP process remains alive

## Status

Closed.

## Reporter

Noema experience-layer evaluation.

## Summary

During a Noema experience-only non-interference run, Matrix returned strong
cleanup proof for two OpenCode ACP runs, including `clean=true`,
`strong_cleanup=true`, and `process_reaped=true`.

After the batch completed, one `opencode acp` process from the first run was
still alive as a child of `matrix run`. This makes the cleanup proof externally
inconsistent even though the run completed and Noema wrote execution records.

## Reproduction Context

Matrix binary:

```text
matrix 0.1.16-snapshot
commit fcd679a5225f0a4cfa459709cdd55092b44730e1
```

Repository:

```text
/home/jose/hpdev/Libraries/noema
```

Command:

```bash
env NOEMA_SEMANTIC_PROVIDER=ollama \
  NOEMA_SEMANTIC_OLLAMA_MODEL=embeddinggemma:latest \
  NOEMA_OUTCOME_CRITIC_PROVIDER=auto \
  NOEMA_OUTCOME_CRITIC_MATRIX_URL=http://127.0.0.1:9091 \
  go run ./cmd/noema-eval run plan \
  --batch-plan examples/live/phase91-experience-nolegacy-cold-resume-plan.json \
  --output-dir artifacts/phase91-experience-nolegacy-cold-resume-after-renewal-v1 \
  --matrix \
  --active-interpreter matrix_fork \
  --require-llm-guidance \
  --parallelism 1
```

Batch artifact:

```text
/home/jose/hpdev/Libraries/noema/programs/evaluation-platform/artifacts/phase91-experience-nolegacy-cold-resume-after-renewal-v1/batch-execution.json
```

## Observed Cleanup Proof

Both records report strong cleanup:

```text
active_cold             clean=true strong_cleanup=true remote_canceled=true process_reaped=true
active_learned_resume   clean=true strong_cleanup=true remote_canceled=true process_reaped=true
```

The first run record also reports:

```json
{
  "matrix_cleanup": {
    "agent_id": "opencode",
    "clean": true,
    "cleanup_strength": "strong",
    "process_reap_attempted": true,
    "process_reaped": true,
    "remote_cancel_attempted": true,
    "remote_canceled": true,
    "strong_cleanup": true
  }
}
```

## Actual Process State After Batch Completion

Observed after the batch completed:

```text
PID       PPID  STAT  ELAPSED  TIME      CMD
71323     5923  Sl    ...      ...       matrix run
186382   71323  Sl    05:37    00:00:06  /home/jose/.local/bin/opencode acp
```

The `opencode acp` process start time matches the first run start:

```text
PID 186382 started Mon May 4 09:25:58 2026
```

The corresponding Noema record started at:

```text
2026-05-04T07:25:58.994400141Z
```

## Expected

One of:

- If the ACP process is still alive after the run, Matrix reports retained or
  failed cleanup instead of `process_reaped=true`.
- If the process is intentionally retained for safe provider reuse, Matrix
  exposes structured safe-retention proof that lets clients distinguish reuse
  from leaked active work.
- If cleanup proof says `process_reaped=true`, no process from that run remains
  alive after batch completion.

## Actual

- Noema receives strong cleanup proof.
- Execution records are written.
- One OpenCode ACP process from the completed run remains alive under Matrix.

## Why This Matters

Noema treats Matrix as the lifecycle authority in non-interference experience
proofs. If `process_reaped=true` can coexist with a still-running ACP process,
Noema cannot safely use Matrix cleanup proof as production-grade evidence.

This does not appear to block task execution in the observed run, but it weakens
cleanup truth and long-run reliability.

## Requested Fix

- Make `process_reaped` reflect externally observable process state.
- If Matrix uses shared or retained ACP processes, expose explicit structured
  retention proof and do not call it reaped.
- Add a regression test where clients verify cleanup proof against a child
  process that remains alive after `/v1/runs` completion.

## Maintainer Resolution

Accepted. The observed remaining `opencode acp` process was not the
workspace-bound run client that Matrix reaped. It was a provider discovery/control
client spawned by run-internal session listing before the evaluation turn. That
made the batch-level cleanup evidence too weak for non-interference consumers:
the run client proof was true, but Matrix still had an unreferenced local ACP
child alive under `matrix run`.

Implemented fixes:

- `/v1/runs` internal before/after session snapshots now use local-only session
  lists and do not trigger provider remote discovery.
- ACP clients now track newly created remote session ids immediately, so
  `ReapAgentSessionClient` can only claim ownership for the matching remote
  session.
- Ephemeral run cleanup now performs a post-cleanup provider-client reconcile.
  Unreferenced clients closed by that pass are reported as related cleanup
  evidence with reason `run_unreferenced_agent_client_reaped`.
- Reconcile failure now fails the cleanup proof explicitly with
  `failure_code=run_agent_client_reconcile_failed` instead of allowing silent
  process retention.
- Regression coverage added for local-only run snapshots, ACP remote-session
  ownership tracking, and run cleanup reconcile evidence.

Documentation updated:

- `docs/matrix_agent_communication_run_trace.md`
- `docs/matrix_protocol_neutral_runtime.md`
- `docs/wiki/API-Reference.md`
- `docs/matrix_timeout_recovery_policy.md`
