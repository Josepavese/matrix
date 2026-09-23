# The A2A surface does not work end to end, and fails without a diagnostic

Date observed: 2026-09-23
Status: fixed — the JSON-RPC binding dispatches (ec451a8); the failing turn was Matrix's
own progress metadata failing A2A task storage, and the cancellation was its
consequence, not its cause

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

## Defect 2 is fixed: the cause, and why the cancellation was a consequence

The turn was not canceled by a timeout, by the HTTP request context, or by Matrix's
"cancellable turn machinery". It was canceled by the protocol SDK *because Matrix failed
the task first*, with its own progress metadata:

1. The ACP observer hands the thought notifier the agent's payload verbatim
   (`internal/providers/agents/router_observer_content.go`, `streamUpdateMetadata`:
   `notif.Update.Content` is a `zedacp.Content`, `notif.Update.Contents` a
   `[]zedacp.Content`, `raw_output` a `json.RawMessage`).
2. `a2aThoughtNotifier.OnThought` put that map into an A2A status message, and A2A
   Metadata is a JSON object (specification §3.2.5; the field is `google.protobuf.Struct`).
   The SDK's task store rejects any value that is not nil, bool, int, float, string,
   `[]any` or `map[string]any` (`a2asrv/taskstore/validator.go`), so processing that
   event failed:

   ```
   WARN task moved to failed state due to a processor error
        a2a.cause="failed to save task state: zedacp.Content is not permitted in Metadata,
                   must be one of nil, bool, int, float, string, []any, map[string]any"
   ```

   The cause *is* logged; the task body carries no message because the SDK deliberately
   does not disclose a processor cause to clients. That is the "no message, no artifacts"
   task in the table above.
3. When the event consumer stops, the execution's `errgroup` cancels the context the agent
   turn runs on (`internal/taskexec/execution_handler.go`, `runProducerConsumer`). **That
   is the `cancel()` that fires.** It reaches `zedacp.Client.doCall`'s `<-ctx.Done()`,
   becomes `context canceled`, is classified as
   `[agent_preflight_failed] phase=session/prompt: ACP prompt failed: context canceled`,
   and then evicts the agent client. The eviction log line was the last symptom in the
   chain, never its cause.
4. The two reverted experiments could not have worked. Detaching from the request context
   was pointless because the SDK already runs the execution on
   `context.WithoutCancel(ctx)` (`internal/taskexec/local_manager.go:182`); completing the
   empty-message path addressed an unrelated branch.

The fix is `internal/providers/a2a/metadata.go`: metadata attached to A2A events is
projected through JSON - the data model the field is defined over - before it reaches the
protocol, and a value with no JSON representation is dropped instead of failing the turn.
`internal/providers/a2a/metadata_test.go` drives the real handler, the real executor and a
router whose turn reports the production metadata shape (a `zedacp.Content`, a
`[]zedacp.Content`, a `json.RawMessage`) and then keeps working. With the projection
removed, all three tests fail with `the running turn was canceled while the agent was
working: context canceled` - the live symptom, reproduced deterministically.

## The advertised surface, method by method

The card publishes `protocolVersion: "1.0"` on both interfaces. In A2A 1.0 the JSON-RPC
method names are PascalCase (`SendMessage`, `SubscribeToTask`, ... - specification §5.3
and §9.4); the slash-separated names (`message/send`, `tasks/resubscribe`, ...) are the
0.3 generation, which `internal/providers/a2a/jsonrpc_names.go` keeps accepting for
callers written against it. The earlier reading in this file - that 1.0 uses the
slash-separated names - was wrong; the translation is compatibility, not conformance.

`internal/providers/a2a/methods_test.go` drives every operation through the real handler,
the real executor and a router, asserting states, artifacts, error codes and the streaming
event sequence; `harness_test.go` holds the shared helpers. `message/send` and
`message/stream` failed against real agents through the metadata defect above; with it
fixed, every advertised method is served:

| Operation (1.0 name / legacy name) | Served | Tested | Advertised |
| --- | --- | --- | --- |
| `SendMessage` / `message/send` | yes | yes | yes, both interfaces |
| `SendStreamingMessage` / `message/stream` | yes | yes (event sequence) | `capabilities.streaming: true`, verified |
| `GetTask` / `tasks/get` | yes | yes | implicit |
| `ListTasks` / `tasks/list` | yes | yes | implicit |
| `CancelTask` / `tasks/cancel` | yes, and it cancels the running agent turn | yes | implicit |
| `SubscribeToTask` / `tasks/resubscribe` | yes for non-terminal tasks | yes | `streaming: true` |
| `CreateTaskPushNotificationConfig` / `.../set` | only when configured | both capability states | only when configured |
| `GetTaskPushNotificationConfig` / `.../get` | only when configured | both capability states | only when configured |
| `ListTaskPushNotificationConfigs` / `.../list` | only when configured | both capability states | only when configured |
| `DeleteTaskPushNotificationConfig` / `.../delete` | only when configured | both capability states | only when configured |
| `GetExtendedAgentCard` / `agent/getAuthenticatedExtendedCard` | only when configured | both capability states | only when configured |

The daemon configures neither push notifications nor an extended card, so the card
advertises neither and both surfaces answer the error the specification fixes for an
unadvertised capability (§3.3.4): -32003 and -32004. `Server.WithPushNotifications` and
`Server.WithExtendedAgentCard` are wired and tested for the day an operator turns one on.

Two errors come from inside the SDK's request handler and do not match the
specification's letter: a message sent to a terminal task answers -32602 where §3.1.1
specifies -32004, and resubscribing to a terminal task answers -32001 where §9.4.6
specifies -32004. Both are asserted by
`TestTerminalStateErrorsAreTheProtocolSDKsOwn`, which records the deviation so an SDK
upgrade that fixes it is noticed. Remapping them would mean replacing the SDK's
`RequestHandler` for both transports.


## Resolution (2026-09-23, c3cfb4d)

The cause was none of the guesses recorded above. There is no Matrix `cancel()` in this
path and the request context was never the source - the SDK already runs the execution on
a detached context, which is exactly why the two fixes that were tried and reverted could
not have worked.

Matrix's A2A notifier put the agent's progress payload into a status message verbatim,
and that payload carries a protocol SDK type while A2A metadata is a JSON object. The
SDK's task-store validator rejected the value, the task was stored failed with no message
and no artifacts, and the SDK's execution errgroup then canceled the context - surfacing
as "ACP prompt failed: context canceled" and the client eviction that this issue recorded
as the symptom. The eviction was the last link in the chain, not the first.

Metadata is now projected per value through JSON, the data model the field is defined
over: a value with no JSON representation is dropped instead of failing the turn.
Reverting the projection reproduces the live symptom exactly, in the message, REST and
streaming regression tests.

The same work audited every operation against the specification: each is either served
and tested or refused with the code the specification requires, `streaming: true` is
backed by a test of the event sequence, push notifications and the extended card answer
-32003 and -32004 when unconfigured, and a dispatch-table test asserts that none of the
twelve method names answers -32601.

Two error codes remain deviations and are recorded rather than hidden: send-to-terminal
answers -32602 and resubscribe-to-terminal -32001 where the specification requires
-32004. Both are decided inside the SDK's request handler before Matrix code runs.

A claim in this issue was also wrong and is corrected: A2A 1.0 uses PascalCase JSON-RPC
method names, and the slash-separated names belong to the previous generation. The
translation added in ec451a8 is backward compatibility, not what a 1.0 client sends.
