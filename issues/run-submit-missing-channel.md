# CLI run submit omits the required run channel

Status: open — discovered while verifying README/Wiki examples on 2026-10-09.
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

## Documentation containment

The README uses authenticated HTTP with explicit agent/channel/workspace.
The Wiki labels this CLI limitation and uses HTTP for async delegation. This
report is open; no runtime correction or new release is claimed by the docs change.
