# The A2A surface does not work end to end, and fails without a diagnostic

Date observed: 2026-09-23
Status: half fixed — the JSON-RPC binding dispatches (ec451a8); the task still fails, cause not established

## How it was found

A real turn was driven through the running daemon against the real `codex` agent
installed from the ACP registry in a scratch PAL home. The runtime API worked; the
A2A surface, which the agent card advertises, did not:

| Surface | Request | Result |
| --- | --- | --- |
| `POST /v1/runs` | real prompt | **works**: `status: completed`, `output: MATRIX_E2E_OK`; a second turn on the same channel recalled the first |
| `GET /v1/agent-auth?agent=codex` | — | **works**: returns the agent's real methods (`api-key`, `chat-gpt`, both `type: agent`) |
| `POST /a2a` (JSON-RPC) | `message/send` | `-32601 method not found` |
| `POST /a2a` (JSON-RPC) | `message/stream`, `tasks/send`, `tasks/get`, `message/sendSubscribe` | `-32601 method not found` for every one |
| `POST /a2a/rest/message:send` | real prompt | HTTP 200 with a task that ends `TASK_STATE_FAILED`, **no message, no artifacts** |

The daemon log for that REST task says exactly:

```
WARN task moved to failed state due to a processor error None
```

An error of `None`. The task fails and the runtime records nothing that says why.

## Defect 1 is fixed (ec451a8)

The JSON-RPC binding answered `-32601` because the protocol SDK dispatches PascalCase
identifiers (`MethodMessageSend = "SendMessage"`) while the A2A specification's
JSON-RPC binding - the one the card advertises as `protocolVersion: "1.0"` with
`protocolBinding: JSONRPC` - uses slash-separated names (`message/send`, `tasks/get`,
...). The route was wired correctly; the names simply never matched. The names are now
translated on the way in, over the decoded request rather than the raw text, and the
tests drive a spec-named `message/send` through the real handler and executor: with the
translation removed the test fails with the production error, `code=-32601 message=method
not found`, and it passes live against the real agent (the request now reaches the
executor instead of being rejected).

This also explains the second symptom in the table: the REST binding worked because the
SDK's REST handler takes its method from the HTTP path, which already matched.

## Defect 2 is still open, and the cause is NOT established

Two attempted fixes were written and both were reverted, because their tests passed
with and without them - which means neither addressed anything:

- **Detaching the turn from the request context** (`context.WithoutCancel` plus a
  budget). The reasoning was that the SDK hands the executor a context tied to the
  HTTP request, so a client that goes away cancels the turn. A test that cancels the
  request mid-turn while a stub router is working passes either way, so either the SDK
  does not propagate that cancellation or the live cancellation comes from somewhere
  else. Reverted rather than kept on a story.
- **Completing the task on the empty-message path**, on the theory that a sequence
  ending without a terminal status left the task failed with nothing to show. Removing
  the terminal event again leaves the test green, so the SDK already completes that
  path. Reverted.

What the live run does establish:

The task fails while the agent is working, and the runtime says why:

```
INFO resolved agent endpoint
INFO conversation client initialized
INFO session update received
WARN task moved to failed state due to a processor error
INFO evicted agent client after cancellable turn failure
     [agent_preflight_failed] ... phase=session/prompt: ACP prompt failed: context canceled
```

The turn is canceled while it runs. The same prompt through
`POST /v1/runs` completes with the same agent, and that path is asynchronous: it
returns a run id immediately and the turn continues on its own context, whereas the
A2A `message/send` turn runs on the caller's request context. There is no
`WriteTimeout` on the runtime's HTTP server (`cmd/matrix/run.go:209-210` sets only
`ReadHeaderTimeout` and `IdleTimeout`), so this is not a server timeout.

The log line that needs following is the one before the failure:
`evicted agent client after cancellable turn failure`. Matrix's routing layer has a
cancellable-turn mechanism of its own, and the cancellation is likelier to come from
there than from the SDK's request context - which is exactly what the reverted
experiment showed. The next step is to instrument that path (or reproduce the
cancellation in a test that uses the real router rather than a stub) before changing
anything: two plausible-sounding fixes have already been tried and disproved here.

## The original two defects

1. **The JSON-RPC binding dispatches nothing.** The agent card lists it first
   (`supportedInterfaces[0]`, `protocolBinding: JSONRPC`, `protocolVersion: 1.0`,
   served at `/a2a`), and the route is registered in
   `internal/providers/a2a/server_config.go:43` as
   `mux.Handle("/a2a", s.authMiddleware(a2asrv.NewJSONRPCHandler(handler), true))`.
   Every method name the A2A specification defines comes back `-32601`, so an A2A
   client that picks the advertised JSON-RPC interface cannot talk to Matrix at all.
   The REST binding registered alongside it (`/a2a/rest/message:send`) does dispatch.

2. **The REST path fails anonymously.** It accepts the message, creates a task with a
   `contextId` and the user's message in `history`, then moves to
   `TASK_STATE_FAILED` with no `status.message`. The same prompt through `/v1/runs`
   with the same agent completes. A caller cannot tell whether the agent was missing,
   the routing failed, or the agent errored — and neither can an operator, because the
   runtime logs the processor error as `None`.

## Why no test caught it

The A2A tests in this repository are unit tests: the agent card's shape, the declared
security scheme, the inbound authentication decision. Nothing drives a message through
the A2A ingress into a session and back. The two defects are both in the wiring
between the SDK handler and the runtime, which unit tests of either side do not
exercise. This is the same class of gap the Windows and ACP v2 real-peer runs closed
elsewhere: an advertised surface asserted rather than driven.

## What to do

1. Make the failure say something. A processor error that logs as `None` is the
   highest-value fix here: whatever the underlying fault, an operator currently has no
   path from the symptom to a cause.
2. Establish which binding is meant to be primary. If JSON-RPC is advertised, it must
   dispatch; if only REST is supported, the card must not advertise a JSON-RPC
   interface that answers `-32601` to every method.
3. Add an end-to-end test that drives one message through the A2A ingress into a real
   (mock, stdio) agent and asserts the answer comes back — the same shape as
   `tests/integration/acp_v2_terminal_auth_real_test.go`, so the surface is proven
   rather than described.

## How it was reproduced

```
nido-free, scratch PAL home:
  MATRIX_HOME=/tmp/e2e/home matrix install codex
  MATRIX_HOME=/tmp/e2e/home matrix config set jsonrpc_addr 127.0.0.1:9190
  MATRIX_HOME=/tmp/e2e/home matrix config set matrix_http_addr 127.0.0.1:9191
  MATRIX_HOME=/tmp/e2e/home matrix config set default_agent codex
  MATRIX_HOME=/tmp/e2e/home matrix vault set system.configured true
  MATRIX_HOME=/tmp/e2e/home matrix run
  curl -X POST http://127.0.0.1:9191/v1/runs -d '{"channel_id":"e2e","agent_id":"codex",...}'
  curl -X POST http://127.0.0.1:9191/a2a/rest/message:send -d '{"message":{...}}'
```

The daemon was a second instance on its own ports and its own PAL home; the
operator's runtime was not touched.
