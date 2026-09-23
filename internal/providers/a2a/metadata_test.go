package a2a

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Josepavese/matrix/internal/middleware"
	"github.com/Josepavese/matrix/pkg/zedacp"
)

// protocolTypedThoughtRouter is a router whose turn behaves like a real agent turn:
// it reports progress with the metadata Matrix reads off the agent, and then keeps
// working until the test releases it.
//
// The metadata is the production shape, not a simplification. The ACP observer hands
// the notifier the agent's own payload (internal/providers/agents/router_observer_content.go
// puts notif.Update.Content, a zedacp.Content, and notif.Update.Contents, a
// []zedacp.Content, into the metadata map), and it can also carry the raw JSON the
// agent sent.
type protocolTypedThoughtRouter struct {
	blockingTurnRouter
	notified    chan struct{}
	notifiedOne sync.Once
}

func newProtocolTypedThoughtRouter() *protocolTypedThoughtRouter {
	return &protocolTypedThoughtRouter{
		blockingTurnRouter: *newBlockingTurnRouter(),
		notified:           make(chan struct{}),
	}
}

func (r *protocolTypedThoughtRouter) RouteConversation(ctx context.Context, req middleware.ConversationRequest) (string, error) {
	if req.Notifier != nil {
		req.Notifier.OnThought(middleware.ThoughtUpdate{
			Type:    middleware.ThoughtTypeToolCall,
			Content: "reading the workspace",
			Title:   "read_file",
			Metadata: map[string]interface{}{
				"source_update_type": "tool_call",
				"protocol":           "acp",
				"protocol_method":    "session/update",
				"acp": map[string]interface{}{
					"session_update": "tool_call",
					"content":        zedacp.Content{Type: "text", Text: "reading the workspace"},
					"content_blocks": []zedacp.Content{{Type: "text", Text: "reading the workspace"}},
					"tool_contents":  []zedacp.ToolCallContent{{Type: "content", Content: &zedacp.Content{Type: "text", Text: "ok"}}},
					"raw_output":     json.RawMessage(`{"exit_code":0}`),
				},
			},
		})
		r.notifiedOne.Do(func() { close(r.notified) })
	}
	return r.blockingTurnRouter.RouteConversation(ctx, req)
}

// waitForNotification returns once the router has reported progress.
func (r *protocolTypedThoughtRouter) waitForNotification(t *testing.T) {
	t.Helper()
	select {
	case <-r.notified:
		// The progress event is now queued for the protocol's event processor.
	case <-time.After(5 * time.Second):
		t.Fatalf("the router never reported progress")
	}
	// Give the event processor a moment to accept or reject that event before the
	// turn is allowed to finish: the production failure happens while the agent is
	// still working, not after it returns.
	select {
	case err := <-r.turnDone:
		t.Fatalf("the running turn was canceled while the agent was working: %v. "+
			"The progress event failed task processing and the execution context was canceled with it.", err)
	case <-time.After(250 * time.Millisecond):
	}
	close(r.release)
	if err := <-r.turnDone; err != nil {
		t.Fatalf("the turn context was canceled before the agent finished: %v", err)
	}
}

// TestAMessageTurnSurvivesTheProtocolMetadataMatrixProjects is the regression test for
// the defect that made the A2A surface non-functional against a real agent.
//
// A2A Metadata is a JSON object - the specification says values "can be any valid JSON
// value" (§3.2.5). Matrix put the agent's protocol-typed payload into it, and the
// protocol SDK's task store rejects anything that is not nil, bool, int, float, string,
// []any or map[string]any. The rejection is a processor error: the task moves to
// TASK_STATE_FAILED with no status message and no artifacts, and because the event
// consumer has stopped, the execution's context is canceled - which is the runtime's
// "ACP prompt failed: context canceled" and the agent-client eviction that follows it.
//
// With the projection in metadata.go the turn completes; without it this test fails on
// the canceled turn and on the failed task.
func TestAMessageTurnSurvivesTheProtocolMetadataMatrixProjects(t *testing.T) {
	router := newProtocolTypedThoughtRouter()
	server := newA2ATestServer(t, NewServer(router, "http://127.0.0.1:0", "opencode"))

	type outcome struct {
		envelope rpcEnvelope
		err      error
	}
	responses := make(chan outcome, 1)
	go func() {
		envelope, err := postRPC(server.URL, "SendMessage", sendMessageParams("m-thought", "count the files"))
		responses <- outcome{envelope: envelope, err: err}
	}()

	router.waitForNotification(t)

	got := <-responses
	if got.err != nil {
		t.Fatalf("SendMessage: %v", got.err)
	}
	task := got.envelope.requireTask(t, "SendMessage")
	if task.Status.State != stateCompleted {
		t.Fatalf("task state = %q (%s), want %s: the turn must survive its own progress metadata",
			task.Status.State, task.statusMessageText(), stateCompleted)
	}
	if answer := task.artifactText(); answer != "matrix:count the files" {
		t.Fatalf("artifact = %q, want the routed answer", answer)
	}
}

// TestARESTMessageTurnSurvivesTheProtocolMetadataMatrixProjects drives the same turn
// through the HTTP+JSON binding the card advertises second, which is the binding the
// live failure was first seen on.
func TestARESTMessageTurnSurvivesTheProtocolMetadataMatrixProjects(t *testing.T) {
	router := newProtocolTypedThoughtRouter()
	server := newA2ATestServer(t, NewServer(router, "http://127.0.0.1:0", "opencode"))

	type outcome struct {
		task taskBody
		err  error
	}
	responses := make(chan outcome, 1)
	go func() {
		body := `{"message":{"role":"user","messageId":"m-rest","parts":[{"text":"count the files"}]}}`
		resp, err := http.Post(server.URL+"/a2a/rest/message:send", "application/json", strings.NewReader(body))
		if err != nil {
			responses <- outcome{err: err}
			return
		}
		defer func() { _ = resp.Body.Close() }()
		var wrapper struct {
			Task *taskBody `json:"task"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&wrapper); err != nil {
			responses <- outcome{err: err}
			return
		}
		if wrapper.Task == nil {
			responses <- outcome{err: fmt.Errorf("REST response carried no task")}
			return
		}
		responses <- outcome{task: *wrapper.Task}
	}()

	router.waitForNotification(t)

	got := <-responses
	if got.err != nil {
		t.Fatalf("POST /a2a/rest/message:send: %v", got.err)
	}
	if got.task.Status.State != stateCompleted {
		t.Fatalf("task state = %q (%s), want %s: the turn must survive its own progress metadata",
			got.task.Status.State, got.task.statusMessageText(), stateCompleted)
	}
	if answer := got.task.artifactText(); answer != "matrix:count the files" {
		t.Fatalf("artifact = %q, want the routed answer", answer)
	}
}

// TestAStreamingTurnSurvivesTheProtocolMetadataMatrixProjects covers the streaming
// binding: the same progress event that failed a blocking task failed the stream, and
// the capability assessment depends on it working. The event sequence is asserted
// because the specification fixes it: the stream begins with the Task, carries the
// artifact, and closes on the terminal status (§3.1.2).
func TestAStreamingTurnSurvivesTheProtocolMetadataMatrixProjects(t *testing.T) {
	router := newProtocolTypedThoughtRouter()
	server := newA2ATestServer(t, NewServer(router, "http://127.0.0.1:0", "opencode"))

	frames := openStream(t, server.URL, "SendStreamingMessage", sendMessageParams("m-stream", "count the files"))
	router.waitForNotification(t)

	first := nextFrame(t, frames, "the initial task").result(t)
	if first.Task == nil {
		t.Fatalf("the stream did not begin with a Task object")
	}
	if first.Task.Status.State != stateSubmitted {
		t.Fatalf("initial task state = %q, want %q", first.Task.Status.State, stateSubmitted)
	}

	var states []string
	var answer string
	deadline := time.After(5 * time.Second)
	for {
		frame, ok := <-frames
		if !ok {
			break
		}
		result := frame.result(t)
		switch {
		case result.StatusUpdate != nil:
			states = append(states, result.StatusUpdate.Status.State)
		case result.ArtifactUpdate != nil:
			for _, part := range result.ArtifactUpdate.Artifact.Parts {
				answer += part.Text
			}
		case result.Task != nil:
			states = append(states, result.Task.Status.State)
		}
		if len(states) > 0 && states[len(states)-1] == stateCompleted {
			awaitStreamClosed(t, frames)
			break
		}
		select {
		case <-deadline:
			t.Fatalf("the stream never completed: states=%v", states)
		default:
		}
	}

	if len(states) == 0 || states[0] != stateWorking {
		t.Fatalf("stream states = %v, want it to open with %q", states, stateWorking)
	}
	if states[len(states)-1] != stateCompleted {
		t.Fatalf("stream states = %v, want it to close on %q", states, stateCompleted)
	}
	if answer != "matrix:count the files" {
		t.Fatalf("streamed artifact = %q, want the routed answer", answer)
	}
}

// TestSafeMetadataProjectsProtocolValuesAndDropsTheUnrepresentable covers the two
// halves of the projection: every value that can be JSON is kept in the JSON data
// model, and a value with no JSON representation at all is dropped instead of taking
// the task down with it.
func TestSafeMetadataProjectsProtocolValuesAndDropsTheUnrepresentable(t *testing.T) {
	projected := a2aSafeMetadata(map[string]any{
		"protocol": "acp",
		"attempt":  2,
		"flag":     true,
		"nested": map[string]any{
			"content": zedacp.Content{Type: "text", Text: "hello"},
			"blocks":  []zedacp.Content{{Type: "text", Text: "hello"}},
			"raw":     json.RawMessage(`{"exit_code":0}`),
		},
		"impossible": make(chan int),
	})

	encoded, err := json.Marshal(projected)
	if err != nil {
		t.Fatalf("projected metadata is not JSON: %v", err)
	}
	if _, present := projected["impossible"]; present {
		t.Fatalf("a value with no JSON representation was kept: %s", encoded)
	}
	if projected["protocol"] != "acp" || projected["flag"] != true {
		t.Fatalf("scalars were not preserved: %s", encoded)
	}
	// Numbers reach the protocol as the doubles its data model defines, which is what
	// the field is declared over (google.protobuf.Struct).
	if projected["attempt"] != float64(2) {
		t.Fatalf("number was not projected into the data model: %#v", projected["attempt"])
	}
	nested, ok := projected["nested"].(map[string]any)
	if !ok {
		t.Fatalf("nested metadata lost its shape: %#v", projected["nested"])
	}
	if content, ok := nested["content"].(map[string]any); !ok || content["text"] != "hello" {
		t.Fatalf("protocol struct was not projected into a JSON object: %#v", nested["content"])
	}
	if blocks, ok := nested["blocks"].([]any); !ok || len(blocks) != 1 {
		t.Fatalf("protocol slice was not projected into a JSON array: %#v", nested["blocks"])
	}
	if raw, ok := nested["raw"].(map[string]any); !ok || raw["exit_code"] != float64(0) {
		t.Fatalf("raw JSON was not projected into a JSON object: %#v", nested["raw"])
	}
	if a2aSafeMetadata(map[string]any{"impossible": make(chan int)}) != nil {
		t.Fatalf("metadata that projects to nothing must be dropped entirely")
	}
}
