# Noema/Zed ACP frontend needs Matrix trace projection and OpenCode runtime cleanup

Date: 2026-04-16
Reporter: Noema integration agent
Scope: Matrix runtime as dependency of Noema sidecar frontend

## Context

Noema is integrating Matrix as the execution bridge for the flow:

`Zed ACP frontend -> Noema ACP facade -> Matrix HTTP run API -> coding agent ACP endpoint`

The frontend must be transparent. Zed should see only agent-originated content:

- thinking/delta messages
- final markdown/text answers
- tool/code updates when the underlying agent emits them

It should not see Matrix internals such as `run.started`, `routing.decision`, `session.resumed`, `matrix://...` content refs, or Noema routing plans.

## Findings

### 1. Matrix trace `inline` policy does not currently expose final content enough for ACP frontend projection

Noema requests Matrix runs with:

```json
{
  "trace_policy": {
    "content_mode": "inline",
    "redaction_profile": "frontend",
    "include_protocol_meta": false
  }
}
```

Observed behavior:

- technical events are present as expected
- final agent event may still primarily expose `content_ref`
- `outcome` exposes `summary_ref`
- a frontend consumer must either dereference `matrix://...` or fall back to technical labels, which creates bad Zed UX

Desired behavior:

- when `content_mode=inline`, final agent messages should carry inline text in `event.message`
- when `content_mode=inline`, outcome should expose a frontend-safe inline summary field
- refs may remain present for audit, but frontend consumers should not need them for normal display
- `include_protocol_meta=false` should keep protocol/debug metadata out of frontend-facing projections where possible

Candidate places to inspect:

- `internal/logic/runtrace/lifecycle.go`
- `internal/logic/runtrace/projection.go`
- `internal/logic/runtrace/types.go`
- `internal/providers/runapi/notifier.go`

Suggested acceptance test:

- start a run with `TracePolicy{ContentMode: ContentModeInline, RedactionProfile: "frontend", IncludeProtocolMeta: false}`
- complete it with output `hello from opencode`
- assert trace outcome contains inline final summary
- assert `agent.message.final` contains inline `message`
- assert refs are not required to render the final user-facing answer

### 2. Matrix first-run wizard can intercept Noema/Zed runs even when agents are already configured locally

During a Noema -> Matrix run, Matrix returned:

```text
Please select your language:
[1] English
[2] Italiano

Reply with the number.
```

Root cause found:

- `RouteConversation` checks `wizard.IsConfigured()`
- when `system.configured` is missing/false, every run enters onboarding before routing to the selected agent
- for an integration frontend this looks like the coding agent answered, but it is Matrix onboarding

Desired behavior:

- provide an explicit non-interactive/bootstrap command for installed PAL/runtime setups
- document the required SSOT key/state for headless integrations
- consider returning a structured setup-required error for HTTP `/v1/runs` instead of mixing wizard text into an agent response

This is especially important for Noema/Zed because the frontend expects a coding-agent response, not Matrix setup prompts.

### 3. Installed OpenCode endpoint may point to a broken Matrix-managed binary

Observed effective Matrix agent config before local correction:

```text
opencode -> /home/jose/.matrix/agents/opencode/opencode
```

Direct execution produced:

```text
SyntaxError: Invalid character: '\0'
```

The local user-installed OpenCode binary worked:

```text
/home/jose/.local/bin/opencode acp --help
```

Desired behavior:

- `matrix agent doctor opencode` should detect this kind of broken binary, not only path existence
- agent runtime readiness should probe `opencode acp --help` or a safe ACP initialize path
- installer/registry should avoid leaving Matrix configured to a corrupt binary if a healthy system binary exists

## Noema-side expectation

Noema will filter Matrix technical events before projecting to Zed. Matrix does not need to hide all internal events. It only needs to make the agent-originated content available inline when explicitly requested through frontend trace policy.

The contract Noema needs from Matrix is:

1. HTTP `/v1/runs` accepts an explicit frontend trace policy.
2. `/v1/runs/{id}/trace` returns enough inline, frontend-safe agent content to render without dereferencing `matrix://` refs.
3. setup/onboarding state is explicit and machine-detectable instead of being returned as a normal agent answer.
4. `agent doctor` catches broken ACP binaries, not just missing files.

## Reproduction Outline

1. Ensure Matrix PAL is installed.
2. Start Matrix runtime.
3. POST a run:

```bash
curl -sS -X POST http://127.0.0.1:9091/v1/runs \
  -H 'Content-Type: application/json' \
  -d '{
    "channel_id":"noema-realtest-opencode",
    "agent_id":"opencode",
    "input":"Rispondi esattamente con questa sola stringa: PONG-NOEMA-OPENCODE",
    "execution_mode":"sync",
    "trace_policy":{
      "content_mode":"inline",
      "redaction_profile":"frontend",
      "include_protocol_meta":false
    }
  }'
```

4. Inspect `/v1/runs/{run_id}/trace`.
5. Verify whether frontend content is inline and whether the response is from OpenCode or Matrix onboarding.

## Priority

High for Noema integration.

Without this, Noema can connect Zed to Matrix technically, but cannot guarantee a clean production-grade frontend experience.
