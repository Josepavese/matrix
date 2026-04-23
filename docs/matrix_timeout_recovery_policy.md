# Matrix Timeout And Recovery Policy

Matrix does not use wall-clock timeouts as a normal control plane for autonomous agents.

Timeouts are allowed only around bounded infrastructure operations. Every timeout must have one of these explicit recovery positions:

- retry with bounded backoff;
- reconnect or restart;
- durable queue and later retry;
- safe user-visible failure with manual retry;
- graceful shutdown cap.

## Policy Table

| Area | Timeout | Recovery Policy |
| --- | --- | --- |
| `/v1/runs` agent turn | none by default | observe through events/SSE, use explicit `cancel`/`stop`, or opt into `emergency_kill_seconds` |
| `/v1/runs` emergency kill | caller-provided `emergency_kill_seconds` | run is marked `cancelled` with `emergency_kill_timeout`; caller can inspect trace and start a new run |
| `/v1/runs` cleanup after cancel | 30s cleanup context | cleanup uses a context detached from the canceled run context; trace records `session.cleanup` with `clean`, `warnings`, and `failure_code` |
| HTTP event sink delivery | 3s per POST | persistent delivery outbox, retry with exponential backoff, dead-letter after max attempts |
| Agent router keepalive | 30s scan interval | dead clients are evicted and pre-warmed on the next scan; failures retry on the next scan |
| Supervised network agents | watchdog loop | restart with backoff; crash-loop state after repeated fast crashes |
| ACP unix dial | 10s connect timeout | request fails safely; router/supervisor can retry by recreating or reconnecting the client |
| Network GET/JSON fetch | 30s HTTP client timeout | idempotent GETs retry transient transport, `429`, and `5xx` failures with bounded backoff |
| Network download | 5m download timeout | idempotent downloads retry transient transport, `429`, and `5xx` failures with bounded backoff |
| Network POST JSON | 30s HTTP client timeout | no automatic retry by default because caller-specific POSTs may not be idempotent |
| Bolt vault open | 1s lock wait | fail fast with explicit vault open error; caller should retry the command after releasing the competing process |
| Codex device-code discovery | 15s | child process is killed; onboarding remains in a state where the user can retry or select another auth path |
| OpenRouter OAuth exchange | 10s | safe user-visible failure; no automatic retry because auth-code exchange may not be idempotent |
| Public IP discovery during OAuth | 5s | optional best-effort enrichment; failure returns empty IP and flow continues |
| HTTP daemon shutdown | 5s | graceful shutdown cap; process exits after bounded cleanup attempt |

## Rules

- Do not add default run-turn timeouts.
- Do not retry non-idempotent POST operations unless the caller supplies an idempotency key or an explicit retry contract.
- Prefer `async` plus event observation for long-running agent work.
- Prefer durable queues for outbound integrations where losing an event is worse than delayed delivery.
- Treat timeout errors as operational events when they affect a run, sink, agent process, or user-visible workflow.
- Cleanup after explicit run cancel must never reuse the canceled run context.
  Remote delete/close/cancel and process reap need their own bounded cleanup
  context so interrupt/resume workflows can prove cleanup before starting the
  resume run.

## Current Gaps To Watch

- Vault lock timeout is intentionally fail-fast today. If multi-process usage becomes common, add a caller-level retry loop with clear lock diagnostics rather than increasing the bbolt timeout globally.
- ACP unix dial failure relies on router/supervisor recovery. If unix-socket ACP agents become common, add a bounded connect retry around client creation.
- POST retry remains opt-in only. Future OAuth or registry POST flows should add idempotency before enabling automatic retry.
