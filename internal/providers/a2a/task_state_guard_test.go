package a2a

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	a2asdk "github.com/a2aproject/a2a-go/v2/a2a"
	"github.com/a2aproject/a2a-go/v2/a2asrv"
)

// TestTerminalTaskOperationsAnswerUnsupportedOperation drives an operation the
// specification forbids on a finished task through both advertised bindings and
// asserts the error the specification fixes for it:
//
//   - a message to a task in a terminal state is UnsupportedOperationError
//     §3.1.1 and §3.1.2 ("Messages sent to Tasks that are in a terminal state
//     ... cannot accept further messages");
//   - resubscribing to a terminal task is UnsupportedOperationError §3.1.6;
//   - §5.4 maps that error to -32004 on the JSON-RPC binding and to
//     FAILED_PRECONDITION / 400 Bad Request on the HTTP+JSON binding, and §5.1
//     requires both advertised bindings to refuse the operation the same way.
//
// With internal/providers/a2a/task_state_guard.go's interceptor removed, this test is
// the one that fails: message/send answers -32602 from a2asrv/agentexec.go:228 and
// tasks/resubscribe answers -32001 from a2asrv/handler.go:373, exactly as
// TestProtocolSDKTerminalTaskCodesWithoutTheCorrection records.
func TestTerminalTaskOperationsAnswerUnsupportedOperation(t *testing.T) {
	server := newA2ATestServer(t, NewServer(&stubSessionRouter{}, "http://127.0.0.1:0", "opencode"))

	done := callRPC(t, server.URL, "SendMessage", sendMessageParams("m-terminal", "done")).requireTask(t, "SendMessage")
	if done.Status.State != stateCompleted {
		t.Fatalf("the turn did not finish: %q", done.Status.State)
	}
	followUp := fmt.Sprintf(`{"message":{"role":"user","messageId":"m-follow-up","taskId":%q,"parts":[{"text":"more"}]}}`, done.ID)

	t.Run("message to a terminal task", func(t *testing.T) {
		callRPC(t, server.URL, "SendMessage", followUp).requireError(t, "SendMessage", codeUnsupportedOp)

		streamed := openStream(t, server.URL, "SendStreamingMessage", followUp)
		if frame := nextFrame(t, streamed, "the streaming answer to a terminal task"); frame.Error == nil || frame.Error.Code != codeUnsupportedOp {
			t.Fatalf("SendStreamingMessage to a terminal task answered %+v, want error %d", frame, codeUnsupportedOp)
		}
		drainStream(t, streamed)
	})

	t.Run("subscription to a terminal task", func(t *testing.T) {
		subscription := openStream(t, server.URL, "SubscribeToTask", fmt.Sprintf(`{"id":%q}`, done.ID))
		if frame := nextFrame(t, subscription, "the subscription answer"); frame.Error == nil || frame.Error.Code != codeUnsupportedOp {
			t.Fatalf("SubscribeToTask on a terminal task answered %+v, want error %d", frame, codeUnsupportedOp)
		}
		drainStream(t, subscription)
	})

	// The same two decisions on the HTTP+JSON binding. message:send answers a plain
	// error document, while tasks/{id}:subscribe has already opened its event stream
	// and reports the refusal as an SSE frame.
	t.Run("REST message to a terminal task", func(t *testing.T) {
		posted := restRequest(t, http.MethodPost, server.URL+"/a2a/rest/message:send", followUp)
		if posted.status != http.StatusBadRequest {
			t.Fatalf("REST message:send to a terminal task status = %d, body %s", posted.status, posted.body)
		}
		if status := restErrorStatus(t, posted.body); status != "FAILED_PRECONDITION" {
			t.Fatalf("REST message:send to a terminal task reported %q, want FAILED_PRECONDITION: %s", status, posted.body)
		}
	})

	t.Run("REST subscription to a terminal task", func(t *testing.T) {
		subscribed := restRequest(t, http.MethodGet, server.URL+"/a2a/rest/tasks/"+done.ID+":subscribe", "")
		if subscribed.status != http.StatusOK {
			t.Fatalf("REST tasks/{id}:subscribe status = %d, body %s", subscribed.status, subscribed.body)
		}
		if status := restErrorStatus(t, firstSSEData(t, string(subscribed.body))); status != "FAILED_PRECONDITION" {
			t.Fatalf("REST tasks/{id}:subscribe to a terminal task reported %q, want FAILED_PRECONDITION: %s", status, subscribed.body)
		}
	})

	// The guard refuses a finished task, not an unknown one: an id the store does
	// not hold keeps the specification's own TaskNotFoundError (§3.3.1), and the
	// same is true of a subscription nothing can be attached to.
	t.Run("unknown task keeps its own error", func(t *testing.T) {
		callRPC(t, server.URL, "SendMessage",
			`{"message":{"role":"user","messageId":"m-unknown","taskId":"no-such-task","parts":[{"text":"more"}]}}`).
			requireError(t, "SendMessage", codeTaskNotFound)
		unknown := openStream(t, server.URL, "SubscribeToTask", `{"id":"no-such-task"}`)
		if frame := nextFrame(t, unknown, "the subscription answer for an unknown task"); frame.Error == nil || frame.Error.Code != codeTaskNotFound {
			t.Fatalf("SubscribeToTask on an unknown task answered %+v, want error %d", frame, codeTaskNotFound)
		}
		drainStream(t, unknown)
	})
}

// TestMessageToARunningTaskAnswersUnsupportedOperationNotAnInternalError covers the
// third case the guard corrects: a second message addressed to a task whose first turn
// is still in flight. The SDK's execution manager refuses it with its internal
// ErrExecutionInProgress (internal/taskexec/local_manager.go:197), which is not one of
// the A2A error types and lives in a package Matrix cannot import, so both bindings
// fall back to an internal error - -32603 on JSON-RPC, 500 on HTTP+JSON - for a
// condition the client caused and can see coming. Matrix serves one turn at a time, so
// the refusal is an UnsupportedOperationError (§3.3.2), and the REST binding says the
// same in its status.
//
// The two exemptions are asserted rather than assumed: resubscribing to the running
// task still streams it (that is what the operation is for), and the task keeps
// running, so the guard refuses the second message without disturbing the first.
//
// Removing the guard's running-state branch makes this test fail with
// `-32603 task execution is already in progress` and a REST 500.
func TestMessageToARunningTaskAnswersUnsupportedOperationNotAnInternalError(t *testing.T) {
	router := newBlockingTurnRouter()
	server := newA2ATestServer(t, NewServer(router, "http://127.0.0.1:0", "opencode"))

	responses := make(chan rpcEnvelope, 1)
	go func() {
		envelope, _ := postRPC(server.URL, "SendMessage",
			`{"message":{"role":"user","messageId":"m-running","parts":[{"text":"long job"}]},"configuration":{"returnImmediately":true}}`)
		responses <- envelope
	}()
	select {
	case <-router.started:
	case <-time.After(5 * time.Second):
		t.Fatalf("the turn never reached the router")
	}
	running := (<-responses).requireTask(t, "SendMessage")
	if running.Status.State != stateSubmitted && running.Status.State != stateWorking {
		t.Fatalf("the first message answered %q, want a task whose turn is still in flight", running.Status.State)
	}
	followUp := fmt.Sprintf(`{"message":{"role":"user","messageId":"m-running-follow-up","taskId":%q,"parts":[{"text":"more"}]}}`, running.ID)

	callRPC(t, server.URL, "SendMessage", followUp).requireError(t, "SendMessage", codeUnsupportedOp)

	posted := restRequest(t, http.MethodPost, server.URL+"/a2a/rest/message:send", followUp)
	if posted.status != http.StatusBadRequest {
		t.Fatalf("REST message:send to a running task status = %d, body %s", posted.status, posted.body)
	}
	if status := restErrorStatus(t, posted.body); status != "FAILED_PRECONDITION" {
		t.Fatalf("REST message:send to a running task reported %q, want FAILED_PRECONDITION: %s", status, posted.body)
	}

	subscription := openStream(t, server.URL, "SubscribeToTask", fmt.Sprintf(`{"id":%q}`, running.ID))
	first := nextFrame(t, subscription, "the task the subscription starts with").result(t)
	if first.Task == nil || first.Task.ID != running.ID {
		t.Fatalf("SubscribeToTask on a running task answered %+v, want the task", first)
	}

	close(router.release)
	select {
	case <-router.turnDone:
	case <-time.After(5 * time.Second):
		t.Fatalf("the released turn never finished")
	}
	drainStream(t, subscription)
}

// TestProtocolSDKTerminalTaskCodesWithoutTheCorrection pins the protocol SDK's own
// choice of error codes for the two terminal-state operations, wrapped in nothing
// but the authentication interceptor Matrix also uses. It is the record of where
// the decision is made and the tripwire for an SDK upgrade:
//
//   - a2asrv/agentexec.go:228 (a2a-go v2.5.0) refuses a message to a terminal task
//     with a2a.ErrInvalidParams, which the JSON-RPC binding reports as -32602;
//   - internal/taskexec/local_manager.go:134 has no execution left to resubscribe
//     to, and a2asrv/handler.go:373 wraps that in a2a.ErrTaskNotFound, reported as
//     -32001.
//
// This test asserts what the SDK does, not what the specification requires. If it
// ever fails, the SDK has changed one of those codes: the guard in
// task_state_guard.go may then be removable, and the deviation recorded in
// issues/closed/2026-09-23-a2a-surface-not-functional.md and
// docs/protocol_coverage.md is outdated and must be updated.
func TestProtocolSDKTerminalTaskCodesWithoutTheCorrection(t *testing.T) {
	server := newRawProtocolSDKServer(t)

	done := callRPC(t, server.URL, "SendMessage", sendMessageParams("m-raw-terminal", "done")).requireTask(t, "SendMessage")
	if done.Status.State != stateCompleted {
		t.Fatalf("the turn did not finish: %q", done.Status.State)
	}

	followUp := fmt.Sprintf(`{"message":{"role":"user","messageId":"m-raw-follow-up","taskId":%q,"parts":[{"text":"more"}]}}`, done.ID)
	callRPC(t, server.URL, "SendMessage", followUp).requireError(t, "SendMessage", codeInvalidParams)

	subscription := openStream(t, server.URL, "SubscribeToTask", fmt.Sprintf(`{"id":%q}`, done.ID))
	if frame := nextFrame(t, subscription, "the SDK's subscription answer"); frame.Error == nil || frame.Error.Code != codeTaskNotFound {
		t.Fatalf("the SDK answered SubscribeToTask on a terminal task with %+v, want its recorded error %d", frame, codeTaskNotFound)
	}
	drainStream(t, subscription)
}

// newRawProtocolSDKServer wires the protocol SDK's handler with the same executor
// and the same capability checks the production routes use, but without the
// task-state guard, so a test can observe the SDK's unaided decision.
func newRawProtocolSDKServer(t *testing.T) *httptest.Server {
	t.Helper()
	capabilities := a2asdk.AgentCapabilities{Streaming: true}
	handler := a2asrv.NewHandler(
		&executor{router: &stubSessionRouter{}, defaultAgent: "opencode"},
		a2asrv.WithCapabilityChecks(&capabilities),
		a2asrv.WithCallInterceptors(&matrixAuthenticatedUserInterceptor{userName: "matrix-local"}),
	)
	mux := http.NewServeMux()
	mux.Handle("/a2a", withSpecJSONRPCMethodNames(a2asrv.NewJSONRPCHandler(handler)))
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)
	return server
}

// restErrorStatus reads the gRPC status string out of an HTTP+JSON error document.
func restErrorStatus(t *testing.T, body []byte) string {
	t.Helper()
	var decoded struct {
		Error struct {
			Status string `json:"status"`
		} `json:"error"`
	}
	if err := json.Unmarshal(body, &decoded); err != nil {
		t.Fatalf("decode error document %s: %v", body, err)
	}
	return decoded.Error.Status
}

// firstSSEData returns the payload of the first data frame in an event stream.
func firstSSEData(t *testing.T, body string) []byte {
	t.Helper()
	scanner := bufio.NewScanner(strings.NewReader(body))
	for scanner.Scan() {
		if line := strings.TrimSpace(scanner.Text()); strings.HasPrefix(line, "data:") {
			return []byte(strings.TrimSpace(strings.TrimPrefix(line, "data:")))
		}
	}
	t.Fatalf("no data frame in the event stream: %q", body)
	return nil
}
