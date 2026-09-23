# The A2A surface does not work end to end, and fails without a diagnostic

Date observed: 2026-09-23
Status: fixed — the JSON-RPC binding dispatches (ec451a8); the failing turn was Matrix's
own progress metadata failing A2A task storage, and the cancellation was its
consequence, not its cause. Follow-up (2026-09-23): the two recorded error-code
deviations are corrected on both bindings by `task_state_guard.go` (which also turns the
`-32603` a message to a running task received into the `-32004` it should be), the
`ListTasks` artifact shape is recorded as an accepted difference, and the missing
real-peer proof exists in `tests/integration/a2a_jsonrpc_router_e2e_test.go`.

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

Two errors came from inside the SDK's request handler and did not match the
specification's letter: a message sent to a terminal task answered -32602 where §3.1.1
specifies -32004, and resubscribing to a terminal task answered -32001 where §9.4.6
specifies -32004. `TestTerminalStateErrorsAreTheProtocolSDKsOwn` pinned both. The
follow-up pass recorded at the end of this file corrected them through the SDK's own
call-interceptor hook, for both advertised bindings, without replacing the SDK's
`RequestHandler`; that pinning test is now
`TestProtocolSDKTerminalTaskCodesWithoutTheCorrection`, which drives the SDK without
Matrix's correction so an upstream fix is still noticed.


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

Two error codes remained deviations at that point and were recorded rather than
hidden: send-to-terminal answered -32602 and resubscribe-to-terminal -32001 where the
specification requires -32004. Both were decided inside the SDK's request handler
before Matrix code runs. The follow-up pass below corrected both through the SDK's
call-interceptor hook.

A claim in this issue was also wrong and is corrected: A2A 1.0 uses PascalCase JSON-RPC
method names, and the slash-separated names belong to the previous generation. The
translation added in ec451a8 is backward compatibility, not what a 1.0 client sends.

## Follow-up (2026-09-23): the two recorded deviations, and the proof that was missing

Three things remained open when this issue was closed: the two error codes recording a
deviation instead of conforming, the `ListTasks` artifact shape, and the absence of any
run through a real agent after the metadata fix. The first is fixed, the second is now
recorded as an accepted difference, and the third is closed by a new integration test.

### The terminal-state codes are corrected, in Matrix, on both bindings

The audit was right about where the codes are chosen - both decisions happen inside the
SDK before Matrix's executor runs - and wrong about the remedy. The SDK's
`a2asrv.NewHandler` returns an `*a2asrv.InterceptedHandler` around its
`defaultRequestHandler`, both bindings call that one value, and an
`a2asrv.CallInterceptor` may refuse a request in `Before`. So one interceptor corrects
both transports; nothing of the SDK is forked or replaced.

`internal/providers/a2a/task_state_guard.go` looks the addressed task up in the same
in-memory store the SDK would have created (`a2asrv.WithTaskStore` is given the store the
guard reads, with the SDK's own `NewTaskStoreAuthenticator`), and only when the stored
task's state is terminal does it refuse the request with
`a2a.ErrUnsupportedOperation`. The SDK paths it steps in front of are:

- `a2asrv/agentexec.go:228` (a2a-go v2.5.0): `task in a terminal state %q: %w` with
  `a2a.ErrInvalidParams` - the -32602 a message to a finished task used to receive.
- `internal/taskexec/local_manager.go:134`: `no active execution` for a finished task,
  which `a2asrv/handler.go:373` wraps in `a2a.ErrTaskNotFound` - the -32001 a
  resubscription used to receive.

What a caller now gets, against the specification:

| Operation | Specification | Before | Now |
| --- | --- | --- | --- |
| `SendMessage` / `message/send` to a terminal task | UnsupportedOperationError, -32004 (§3.1.1, §5.4) | -32602 | -32004 |
| `SendStreamingMessage` / `message/stream` to a terminal task | UnsupportedOperationError, -32004 (§3.1.2, §5.4) | -32602 | -32004 |
| `SubscribeToTask` / `tasks/resubscribe` to a terminal task | UnsupportedOperationError, -32004 (§3.1.6, §9.4.6, §5.4) | -32001 | -32004 |
| HTTP+JSON `message:send` to a terminal task | FAILED_PRECONDITION, 400 (§5.4) | INVALID_ARGUMENT, 400 | FAILED_PRECONDITION, 400 |
| HTTP+JSON `tasks/{id}:subscribe` to a terminal task | FAILED_PRECONDITION, 400 (§5.4) | NOT_FOUND, 404 | FAILED_PRECONDITION, 400 |

An unknown task id is untouched and still answers TaskNotFoundError, which is what the
guard's store lookup is for: it reads "the task exists and is terminal", not "the
operation is scary".

Tests: `internal/providers/a2a/task_state_guard_test.go`.

- `TestTerminalTaskOperationsAnswerUnsupportedOperation` drives all five rows above plus
  the unknown-id control. Removing `&taskStateGuard{...}` from
  `newRequestHandler` (the only production change that carries the fix) fails four of
  its five subtests with the production symptoms: `-32602 (executor setup failed: failed
  to load exec ctx: task in a terminal state "TASK_STATE_COMPLETED": invalid params)`,
  the SSE `-32001` frame, REST `INVALID_ARGUMENT`, and REST `NOT_FOUND`. The unknown-id
  subtest passes either way, which is the point of it.
- `TestProtocolSDKTerminalTaskCodesWithoutTheCorrection` keeps the old pinning test's
  job: it wires the SDK's handler with the same executor and capability checks but
  without the guard, and asserts -32602 and -32001, so an SDK upgrade that fixes either
  code fails this test and the deviation record gets revisited.

### The `ListTasks` artifact shape is an accepted difference

§3.1.4 (A2A v1.0.1) says: "When `includeArtifacts` is false (the default), the artifacts
field MUST be omitted entirely from each Task object in the response. ... When
`includeArtifacts` is true, the artifacts field should be included with its actual
content (which may be an empty array if the task has no artifacts)." Matrix satisfies the
MUST and omits the field in both cases, so a task with no artifacts does not carry an
empty array when `includeArtifacts` is true.

This cannot be corrected from Matrix at a price worth paying, and the reason is the
SDK's wire type rather than the store: both bindings marshal the response with
`encoding/json` (`a2asrv/jsonrpc.go:361`, `a2asrv/rest.go:206`), and
`a2a.Task.Artifacts` is tagged `json:"artifacts,omitempty"` (`a2a/core.go:347` in
v2.5.0). Go's `omitempty` drops an empty slice whether or not it is nil, so no value
Matrix can place in the response - the store's result, an interceptor's payload - can
make the key appear. Emitting it would mean rewriting response bodies in a transport
wrapper or forking the SDK's core type, and the sentence is the specification's
lowercase "should", not a MUST.

Recorded, not hidden: `TestListTasksOmitsTheArtifactsFieldForATaskWithoutArtifacts`
pins the shape in both directions (a silent turn omits the key; a turn with an artifact
carries it, which proves the pointer decode distinguishes the two). When it fails, the
SDK has changed and this paragraph is outdated.

### Found while fixing: a message to a running task was an "internal error"

The audit for the two codes above turned up a third case the same guard corrects. A
second message addressed to a task whose first turn is still in flight is refused by the
execution manager with its internal `ErrExecutionInProgress`
(`internal/taskexec/local_manager.go:197`), a value that is not one of the A2A error
types and lives in a package Matrix cannot import. Both bindings therefore fell back to
an internal error for a condition the client caused and can see coming: `-32603
task execution is already in progress` on JSON-RPC, HTTP 500 on the HTTP+JSON binding.
Matrix serves one turn per task, which is a limitation rather than a fault, so the
refusal is now `UnsupportedOperationError` on both bindings (§3.3.2: "a specific aspect
of it is not supported by this server agent implementation"). Two operations are
deliberately exempt, and the test asserts both: resubscribing to the running task still
streams it, and a task awaiting input would still accept the follow-up message §3.4.3
documents. `TestMessageToARunningTaskAnswersUnsupportedOperationNotAnInternalError`
fails with `-32603 task execution is already in progress` when the guard's
running-state branch is removed.

### The end-to-end proof

`tests/integration/a2a_jsonrpc_router_e2e_test.go` drives a specification-named JSON-RPC
request through the production `matrixa2a.Server` into the production `session.Manager`
and `agents.Router` and into the repository's own `./cmd/mock-agent` compiled from its
package path and spoken to over real stdio. It asserts a `TASK_STATE_COMPLETED` task
whose artifact carries the peer's answer, for both the 1.0 name and the 0.3 legacy name,
and reads the task back through `GetTask`. The task completing at all is part of the
assertion: the metadata defect that failed every task ran through this same notifier.

It is not a `matrix run` daemon: the HTTP listener is `httptest` and the CLI's
vault/config bootstrap is replaced by an in-memory store. Everything from the wire
request to the agent process is production code, and the peer is a real process rather
than an in-process stub.

The test reproduces both live defects when either fix is removed, which is what makes it
the proof this issue was missing. Reverting `message.Metadata = a2aSafeMetadata(metadata)`
to `message.Metadata = metadata` in `internal/providers/a2a/server.go` makes both subtests
fail with the original symptom - `state = "TASK_STATE_FAILED" (no status message)` - with
the real peer running the same turn; removing `withSpecJSONRPCMethodNames` from the route
makes the `message/send` subtest fail with `-32601 method not found` while the 1.0 name
still passes.

One test-harness defect was found while writing it and fixed where it lives: the A2A
streams are read by a goroutine that owns the response body, and a test that read one
frame and returned left `Body.Close` racing that read on the shared HTTP transport. The
wedged connection stalled whichever request reused it next until the transport's idle
timeout, which showed up as an intermittent ~90-second test rather than a failure.
`harness_test.go` now documents that a test which stops at the first frame must drain
the stream, and `drainStream` is the helper for it.
