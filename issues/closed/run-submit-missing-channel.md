# CLI run submit omits the required run channel

Status: closed — corrected for v0.1.54; discovered while verifying README/Wiki examples on 2026-10-09.
Scope: `matrix run submit`; HTTP submission with explicit channel works through
its published contract. No provider task was started by this probe.

## Reproduction

With the v0.1.53 local daemon running:

```bash
matrix run submit --agent docs-unregistered-probe --prompt 'Documentation contract probe' --json
```

Observed exit 1:

```text
Error: submit refused: HTTP 400: Bad Request: channel_id and input are required
```

## Cause

`internal/logic/runclient/submit.go` serializes only `agent_id` and `input`.
`internal/providers/runapi/run_request.go` requires `channel_id` before dispatch.
The client also omits `execution_mode`, which selects synchronous execution,
despite the CLI help describing acceptance without awaiting task completion.
It accepts HTTP 200/201; explicit async execution returns HTTP 202.

## Required correction

Define a stable caller channel for the CLI, submit explicitly async work, and
accept HTTP 202. Decide whether to expose caller/workspace selection as flags;
workspace selection must not silently use the invoking shell's directory.
Exercise the client against the real run handler, including idempotency scope,
refusal and eventual outcome, instead of only a permissive mock response.

## Resolution

The client now supplies stable `cli.run.submit` by default and always requests
`execution_mode=async`. HTTP 202 is accepted, including idempotent replay.
CLI flags `--channel` and `--workspace` select the caller and registered project;
blank channels fail locally. Optional `--model` forwards the existing API
model selector without changing the agent's global configuration. No workspace path is inferred from the shell.
The response preserves `run_id`, status and replay identity for wait/events.

Tests use the real authenticated run handler with provider work blocked until
acceptance is verified: explicit workspace/root, default/custom channel,
idempotent replay without duplicate dispatch, conflict/401/unknown-workspace
refusal and eventual completion. A compiled native CLI smoke exercises flags,
JSON stdout and replay on the Linux/macOS/Windows PAL workflow.
The Wiki now shows working submission commands from v0.1.54.
Release/install evidence is recorded separately in the v0.1.54 release ledger.

## Real runtime qualification

The candidate CLI submitted to the installed v0.1.53 HTTP daemon. An explicit
fresh qualification session kept unrelated conversations separate. Acceptance
returned in about 22 ms and a retry kept the same run ID. The provider-default
MiMo attempt produced no output and was correctly recorded as failed. A follow-up
through the same owned conversation with explicit `deepseek/deepseek-flash`
completed; requested/effective selector was provider-confirmed, the synthetic
marker appeared in final events, and CLI wait observed completion. Only that
qualification session was cleaned up. This proves the CLI/runtime flow; provider
confirmation is not a physical-model or account-balance certification.
