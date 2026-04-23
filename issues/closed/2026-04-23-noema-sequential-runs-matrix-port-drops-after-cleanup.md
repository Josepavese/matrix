# Noema Phase 87: sequential Matrix evaluation runs lose Matrix HTTP availability after cleanup

## Context

Noema reran Phase 87 after Matrix closed:

```text
issues/closed/2026-04-23-noema-fork-interpreter-live-attach-session-mismatch.md
```

The previous live-attach composition issue may be fixed, but Noema cannot verify it yet because sequential Matrix runs now intermittently lose Matrix HTTP availability between or during runs.

No Matrix source was modified by Noema. This issue records observed behavior from the installed Matrix PAL binary:

```text
/home/jose/.local/share/matrix/bin/matrix
```

## Command

```bash
cd /home/jose/hpdev/Libraries/noema/programs/evaluation-platform
go run ./cmd/noema-eval run plan \
  --config-dir ./configs \
  --batch-plan ./examples/live/phase72-active-sidecar-short-plan.json \
  --agents opencode \
  --arms active_cold,active_learned \
  --matrix \
  --active-interpreter matrix_fork \
  --output-dir ./artifacts/phase87-active-interpreter-matrix-fork-opencode-short-v4
```

The same symptom also appeared in:

```text
/home/jose/hpdev/Libraries/noema/programs/evaluation-platform/artifacts/phase87-active-interpreter-matrix-fork-opencode-short-v3
```

## v4 Evidence

Batch artifact:

```text
/home/jose/hpdev/Libraries/noema/programs/evaluation-platform/artifacts/phase87-active-interpreter-matrix-fork-opencode-short-v4/batch-execution.json
```

The first run succeeded:

```json
{
  "arm_id": "active_cold",
  "status": "succeeded",
  "duration_ms": 101873,
  "matrix_run_id": "run-38f4de5d-9ee6-4d25-9e2b-9c9d2b09e8b8",
  "notes": [
    "matrix_autostart_up=true",
    "matrix_autostart_started=true",
    "matrix_cleanup_proof=true",
    "matrix_cleanup_clean=true",
    "matrix_cleanup_process_reaped=true",
    "matrix_cleanup_process_retained=false"
  ]
}
```

The next sequential run failed because Matrix HTTP became unavailable:

```json
{
  "arm_id": "active_learned",
  "status": "failed",
  "duration_ms": 134487,
  "matrix_run_id": "run-27d277bc-12f6-49c7-b97d-6549eb84fecd",
  "error": "Get \"http://127.0.0.1:9091/v1/runs/run-27d277bc-12f6-49c7-b97d-6549eb84fecd/trace\": dial tcp 127.0.0.1:9091: connect: connection refused",
  "notes": [
    "matrix_autostart_up=true",
    "matrix_autostart_started=false",
    "active_sidecar_monitor_error=Get \"http://127.0.0.1:9091/v1/runs/run-27d277bc-12f6-49c7-b97d-6549eb84fecd/events?after=evt-d75cf708-5beb-4a45-b08f-6345dea91ba9\": dial tcp 127.0.0.1:9091: connect: connection refused",
    "matrix_cleanup_proof=false"
  ]
}
```

After the failed batch, no Matrix process remained:

```text
pgrep -af 'matrix run|opencode acp|noema-eval'
# only the pgrep command itself
```

## v3 Evidence

The inverse sequence happened in `v3`:

- `active_cold` failed with `connection refused`
- `active_learned` later autostarted Matrix and succeeded, but had `patterns_available=0` and `suggestions_generated=0`, so it did not verify the fork/live-attach fix

Relevant artifact:

```text
/home/jose/hpdev/Libraries/noema/programs/evaluation-platform/artifacts/phase87-active-interpreter-matrix-fork-opencode-short-v3/batch-execution.json
```

Cold failure in v3:

```json
{
  "arm_id": "active_cold",
  "status": "failed",
  "matrix_run_id": "run-a6cb1e7a-63d7-4bc7-92a7-9d0883f96552",
  "error": "Get \"http://127.0.0.1:9091/v1/runs/run-a6cb1e7a-63d7-4bc7-92a7-9d0883f96552/trace\": dial tcp 127.0.0.1:9091: connect: connection refused",
  "notes": [
    "matrix_autostart_up=true",
    "matrix_autostart_started=false",
    "active_sidecar_monitor_error=Get \"http://127.0.0.1:9091/v1/runs/run-a6cb1e7a-63d7-4bc7-92a7-9d0883f96552/events?after=evt-faff429a-cd1a-45b5-a9bb-014ae58c60d8\": dial tcp 127.0.0.1:9091: connect: connection refused",
    "matrix_cleanup_proof=false"
  ]
}
```

## Why This Blocks Noema

Noema needs a stable sequential active evaluation batch:

```text
active_cold -> learn pattern -> active_learned -> fork interpreter -> live attach -> validation/cleanup proof
```

If Matrix HTTP disappears after cleanup or while the monitor is polling, Noema cannot verify:

- fork artifact acceptance
- live context delivery
- `sidecar.capsule.delivered`
- cleanup proof
- active learned-vs-cold evidence

This also prevents moving to mid/long runs safely.

## Expected Contract

For `matrix run` used as Noema's local PAL service:

- Matrix HTTP must remain reachable across sequential async runs unless explicitly stopped.
- Reaping an ACP agent process during run cleanup must not terminate the Matrix HTTP service.
- If the Matrix service intentionally exits after an idle/no-session condition, Noema needs typed lifecycle evidence or a documented health contract so it can restart safely before the next run.
- If Matrix detects an internal fatal condition, it should log typed fatal evidence in `matrix-runtime.jsonl`/stdout before exit.

## Request

Please check whether recent cleanup/process retention changes can cause the Matrix PAL service process to exit when an ACP child is reaped, or whether the health check can return `up=true` shortly before the HTTP server exits.

The desired behavior for Noema evaluation is a durable Matrix service for the whole batch, or a clearly documented restart boundary with deterministic health failure before a run starts.

Until this is fixed, Noema will avoid treating the Phase 87 Matrix fork interpreter path as verified and should not start mid/long runs on this lane.

---

## Matrix Maintainer Triage

Accepted for runtime lifecycle hardening, but not yet accepted as proven cleanup
bug.

Observed Matrix PAL logs around the reported v4 window show graceful daemon
shutdown, not panic/fatal exit:

- `daemon context cancelled, closing listener`
- `shutdown_started`
- `daemon_stopped`
- `agent router keepalive stopped`
- `daemon_exited`

Current code review:

- `matrix run` is driven by the top-level signal context.
- The daemon exits cleanly when the process receives `SIGINT`/`SIGTERM`, or
  when the JSON-RPC daemon returns an error.
- Session cleanup uses an isolated timeout context for cleanup work.
- Cleanup can close/reap ACP child clients through the router, but it does not
  cancel the top-level Matrix runtime context.
- No idle shutdown path was found.

Local smoke after the report:

- started installed Matrix `v0.1.10`
- verified `/_matrix/runtime`
- verified `/v1/session-actions` capabilities for `opencode`
- waited through keepalive and ACP child rewarm
- Matrix HTTP stayed reachable on `127.0.0.1:9090` and `127.0.0.1:9091`

Implemented hardening:

- Matrix now logs explicit `runtime_signal_received` evidence with the exact
  signal before cancelling the runtime context.
- This makes future `connection refused` reports classifiable as external
  signal, daemon error, crash, or startup failure.
- Release/runbook documentation now describes this shutdown evidence.

Next verification required:

- rerun the Noema sequential batch without Matrix deploy/reinstall/kill work in
  parallel;
- if Matrix HTTP disappears again, attach the surrounding
  `runtime_signal_received`, `shutdown_started`, `daemon_exited`, `ERROR`, or
  panic/fatal log lines;
- if no signal/fatal evidence appears, treat it as a Matrix durability bug and
  investigate process parent/session handling.

---

## Matrix Maintainer Resolution

Closed as runtime observability/hardening complete.

Outcome:

- no cleanup path was found that cancels the top-level Matrix daemon context;
- installed-runtime evidence showed graceful shutdown, not a cleanup-induced
  fatal exit;
- Matrix now logs `runtime_signal_received` before runtime context
  cancellation, followed by shutdown lifecycle evidence;
- release and production-readiness docs define the expected shutdown evidence;
- local smoke and full test suite passed after freeing the port from a residual
  Noema batch process.

Verification:

- signal smoke produced `runtime_signal_received`, `shutdown_started`, and
  `daemon_exited`;
- `go test ./...`;
- real OpenCode ACP cancel/resume cleanup test kept Matrix reachable until
  explicit runtime stop.

Status: closed. Reopen only with a fresh Matrix log window showing HTTP
disappearance without `runtime_signal_received`, daemon stop evidence, or an
external supervisor/process kill.

---

## 2026-04-24 Phase 84 Experience-Only Reproduction

Noema attempted a longer OpenCode experience-only Phase 84 batch:

```bash
cd /home/jose/hpdev/Libraries/noema/programs/evaluation-platform
/tmp/noema-eval run plan \
  --config-dir ./configs \
  --batch-plan ./examples/live/phase84-evidence-hardening-midrun-plan.json \
  --agents opencode \
  --arms active_cold,active_learned_resume \
  --max-runs 4 \
  --matrix \
  --output-dir ./artifacts/phase84-experience-only-matrix-midrun-opencode-4run-v1
```

The first `active_cold` run completed and produced the expected failure scar:

```json
{
  "arm_id": "active_cold",
  "task_id": "phase84-validation-backed-parser-warmup-001",
  "status": "failed",
  "duration_ms": 66834,
  "failure_scars_learned": 1,
  "matrix_cleanup_proof": true,
  "matrix_cleanup_clean": true
}
```

The next `active_learned_resume` run failed because Matrix HTTP disappeared
while Noema was polling the resumed run:

```json
{
  "arm_id": "active_learned_resume",
  "task_id": "phase84-validation-backed-parser-warmup-001",
  "status": "failed",
  "duration_ms": 102843,
  "matrix_run_id": "run-1e016c04-6904-4eac-acc4-52466ea4dd68",
  "error": "Get \"http://127.0.0.1:9091/v1/runs/run-1e016c04-6904-4eac-acc4-52466ea4dd68/trace\": dial tcp 127.0.0.1:9091: connect: connection refused",
  "notes": [
    "active_sidecar_resume_monitor_error=Get \"http://127.0.0.1:9091/v1/runs/run-1e016c04-6904-4eac-acc4-52466ea4dd68/events?after=evt-097ca840-55cb-4904-9952-16e801339969\": dial tcp 127.0.0.1:9091: connect: connection refused",
    "matrix_cleanup_proof=false"
  ]
}
```

Matrix runtime log around the failure now contains typed shutdown evidence:

```json
{"time":"2026-04-24T00:22:31.768988965+02:00","level":"INFO","msg":"runtime signal received","component":"runtime","event":"runtime_signal_received","signal":"terminated"}
{"time":"2026-04-24T00:22:31.769052073+02:00","level":"INFO","msg":"daemon context cancelled, closing listener","component":"daemon","event":"daemon_shutdown"}
{"time":"2026-04-24T00:22:31.769063926+02:00","level":"INFO","msg":"shutting down...","component":"runtime","event":"shutdown_started"}
{"time":"2026-04-24T00:22:31.769100566+02:00","level":"INFO","msg":"daemon stopped gracefully","component":"daemon","event":"daemon_stopped"}
{"time":"2026-04-24T00:22:31.769110279+02:00","level":"INFO","msg":"matrix daemon exited cleanly","component":"runtime","event":"daemon_exited"}
```

The Noema runner then exited with signal status:

```text
Fri Apr 24 00:19:30 CEST 2026
Terminated
Fri Apr 24 00:23:02 CEST 2026
EXIT_CODE=143
```

This confirms the HTTP disappearance is caused by Matrix receiving `SIGTERM`,
not by an untyped crash. The remaining blocker is to identify who sends the
signal during sequential Noema eval batches and whether Matrix/Noema should
isolate the PAL service from per-run cleanup signalling.
