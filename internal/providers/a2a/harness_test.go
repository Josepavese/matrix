package a2a

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Josepavese/matrix/internal/middleware"
)

// The JSON-RPC codes the A2A specification fixes for the A2A error types. Every
// binding must map them the same way (§5.4, "Error Code Mappings"). Asserting the
// code rather than "the call failed" is what makes these tests evidence that the
// ingress speaks A2A and not merely HTTP.
const (
	codeTaskNotFound      = -32001
	codeTaskNotCancelable = -32002
	codePushNotSupported  = -32003
	codeUnsupportedOp     = -32004
	// codeInvalidParams is the JSON-RPC standard code for invalid method parameters.
	codeInvalidParams = -32602
)

// taskStates are the wire values of a2a.TaskState, spelled out so a response body is
// asserted against the specification's strings rather than against the SDK constant.
const (
	stateSubmitted = "TASK_STATE_SUBMITTED"
	stateWorking   = "TASK_STATE_WORKING"
	stateCompleted = "TASK_STATE_COMPLETED"
	stateCanceled  = "TASK_STATE_CANCELED"
	stateFailed    = "TASK_STATE_FAILED"
)

type rpcEnvelope struct {
	Result json.RawMessage `json:"result"`
	Error  *rpcErrorBody   `json:"error"`
}

type rpcErrorBody struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// taskBody is the part of the A2A Task object these tests assert on (§4.1.1).
type taskBody struct {
	ID        string `json:"id"`
	ContextID string `json:"contextId"`
	Status    struct {
		State   string `json:"state"`
		Message *struct {
			Parts []struct {
				Text string `json:"text"`
			} `json:"parts"`
		} `json:"message"`
	} `json:"status"`
	Artifacts []struct {
		Parts []struct {
			Text string `json:"text"`
		} `json:"parts"`
	} `json:"artifacts"`
	History []struct {
		MessageID string `json:"messageId"`
	} `json:"history"`
}

func (t taskBody) artifactText() string {
	var text strings.Builder
	for _, artifact := range t.Artifacts {
		for _, part := range artifact.Parts {
			text.WriteString(part.Text)
		}
	}
	return text.String()
}

func (t taskBody) statusMessageText() string {
	if t.Status.Message == nil {
		return ""
	}
	var text strings.Builder
	for _, part := range t.Status.Message.Parts {
		text.WriteString(part.Text)
	}
	return text.String()
}

// newA2ATestServer wires the real mux, the real protocol handler and the real
// executor over the provided router, and returns the running HTTP server.
func newA2ATestServer(t *testing.T, server *Server) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	server.RegisterRoutes(mux)
	httpServer := httptest.NewServer(mux)
	t.Cleanup(httpServer.Close)
	return httpServer
}

func sendMessageParams(messageID, text string) string {
	return fmt.Sprintf(`{"message":{"role":"user","messageId":%q,"parts":[{"text":%q}]}}`, messageID, text)
}

func postRPC(url, method, params string) (rpcEnvelope, error) {
	body := fmt.Sprintf(`{"jsonrpc":"2.0","id":"matrix-test","method":%q,"params":%s}`, method, params)
	resp, err := http.Post(url+"/a2a", "application/json", strings.NewReader(body))
	if err != nil {
		return rpcEnvelope{}, fmt.Errorf("%s: %w", method, err)
	}
	defer func() { _ = resp.Body.Close() }()
	var envelope rpcEnvelope
	if err := json.NewDecoder(resp.Body).Decode(&envelope); err != nil {
		return rpcEnvelope{}, fmt.Errorf("%s: decode response: %w", method, err)
	}
	return envelope, nil
}

func callRPC(t *testing.T, url, method, params string) rpcEnvelope {
	t.Helper()
	envelope, err := postRPC(url, method, params)
	if err != nil {
		t.Fatalf("%v", err)
	}
	return envelope
}

// requireError asserts the exact A2A error code the operation is specified to return.
func (e rpcEnvelope) requireError(t *testing.T, method string, code int) {
	t.Helper()
	if e.Error == nil {
		t.Fatalf("%s: expected error code %d, got result %s", method, code, e.Result)
	}
	if e.Error.Code != code {
		t.Fatalf("%s: error code = %d (%s), want %d", method, e.Error.Code, e.Error.Message, code)
	}
}

// requireTask accepts both shapes the bindings use: the JSON-RPC SendMessage result
// wraps the task ({"task": …}, §9.4.1) while GetTask and CancelTask return it directly.
func (e rpcEnvelope) requireTask(t *testing.T, method string) taskBody {
	t.Helper()
	if e.Error != nil {
		t.Fatalf("%s: unexpected error %d: %s", method, e.Error.Code, e.Error.Message)
	}
	var direct taskBody
	if err := json.Unmarshal(e.Result, &direct); err == nil && direct.ID != "" {
		return direct
	}
	var wrapped struct {
		Task *taskBody `json:"task"`
	}
	if err := json.Unmarshal(e.Result, &wrapped); err != nil || wrapped.Task == nil {
		t.Fatalf("%s: result is neither a task nor a task wrapper: %s", method, e.Result)
	}
	return *wrapped.Task
}

// streamResult is one StreamResponse: exactly one of the fields is set (§3.2.3).
type streamResult struct {
	Task           *taskBody `json:"task"`
	StatusUpdate   *streamStatusUpdate
	ArtifactUpdate *streamArtifactUpdate
}

type streamStatusUpdate struct {
	TaskID string `json:"taskId"`
	Status struct {
		State string `json:"state"`
	} `json:"status"`
}

type streamArtifactUpdate struct {
	TaskID   string `json:"taskId"`
	Artifact struct {
		Parts []struct {
			Text string `json:"text"`
		} `json:"parts"`
	} `json:"artifact"`
}

type streamFrame struct {
	Result json.RawMessage `json:"result"`
	Error  *rpcErrorBody   `json:"error"`
}

func (f streamFrame) result(t *testing.T) streamResult {
	t.Helper()
	if f.Error != nil {
		t.Fatalf("stream error %d: %s", f.Error.Code, f.Error.Message)
	}
	var result streamResult
	if err := json.Unmarshal(f.Result, &result); err != nil {
		t.Fatalf("decode stream frame %s: %v", f.Result, err)
	}
	return result
}

// openStream performs a streaming JSON-RPC call and returns the frames as they arrive
// on the wire. The response body is closed by the cleanup registered for the test.
//
// A test that needs only the first frame must still drain the stream to its end (see
// drainStream) before it returns. Closing a body whose reader goroutine is still
// consuming it races on the shared HTTP transport, and a connection left in that state
// stalls the next request that reuses it until the transport's idle timeout, which shows
// up as an intermittent ~90s test rather than a failure.
func openStream(t *testing.T, url, method, params string) <-chan streamFrame {
	t.Helper()
	body := fmt.Sprintf(`{"jsonrpc":"2.0","id":"matrix-stream","method":%q,"params":%s}`, method, params)
	resp, err := http.Post(url+"/a2a", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatalf("%s: %v", method, err)
	}
	t.Cleanup(func() { _ = resp.Body.Close() })
	if contentType := resp.Header.Get("Content-Type"); !strings.HasPrefix(contentType, "text/event-stream") {
		t.Fatalf("%s: content-type = %q, want text/event-stream", method, contentType)
	}

	frames := make(chan streamFrame, 16)
	go func() {
		defer close(frames)
		scanner := bufio.NewScanner(resp.Body)
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if !strings.HasPrefix(line, "data:") {
				continue
			}
			var frame streamFrame
			if err := json.Unmarshal([]byte(strings.TrimSpace(strings.TrimPrefix(line, "data:"))), &frame); err != nil {
				continue
			}
			frames <- frame
		}
	}()
	return frames
}

func nextFrame(t *testing.T, frames <-chan streamFrame, what string) streamFrame {
	t.Helper()
	select {
	case frame, ok := <-frames:
		if !ok {
			t.Fatalf("the stream closed before %s", what)
		}
		return frame
	case <-time.After(5 * time.Second):
		t.Fatalf("timed out waiting for %s", what)
	}
	return streamFrame{}
}

func awaitStreamClosed(t *testing.T, frames <-chan streamFrame) {
	t.Helper()
	select {
	case _, ok := <-frames:
		if ok {
			t.Fatalf("the stream delivered another event after the task reached a terminal state")
		}
	case <-time.After(5 * time.Second):
		t.Fatalf("the stream did not close after the task reached a terminal state")
	}
}

// drainStream reads a stream to its end, discarding every frame. It is how a test that
// stops at the first frame leaves the connection clean: the reader goroutine reaches
// EOF before the test's cleanup closes the body, so no read is in flight against the
// close. See the note on openStream for why that matters.
func drainStream(t *testing.T, frames <-chan streamFrame) {
	t.Helper()
	deadline := time.After(5 * time.Second)
	for {
		select {
		case _, ok := <-frames:
			if !ok {
				return
			}
		case <-deadline:
			t.Fatalf("the stream did not close")
		}
	}
}

// blockingTurnRouter is a router whose turn stays in flight until the test releases it
// or the turn context dies. It is how these tests observe cancellation: a turn that is
// canceled reports the context error instead of its answer.
type blockingTurnRouter struct {
	stubSessionRouter
	started  chan struct{}
	release  chan struct{}
	turnDone chan error
	once     sync.Once
}

func newBlockingTurnRouter() *blockingTurnRouter {
	return &blockingTurnRouter{
		started:  make(chan struct{}),
		release:  make(chan struct{}),
		turnDone: make(chan error, 1),
	}
}

func (r *blockingTurnRouter) RouteConversation(ctx context.Context, req middleware.ConversationRequest) (string, error) {
	r.channelID = req.ChannelID
	r.agentID = req.AgentID
	r.input = req.Input
	r.once.Do(func() { close(r.started) })
	select {
	case <-r.release:
		r.turnDone <- ctx.Err()
		return "matrix:" + req.Input, nil
	case <-ctx.Done():
		r.turnDone <- ctx.Err()
		return "", ctx.Err()
	}
}
