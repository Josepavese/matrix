package agents

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Josepavese/matrix/internal/middleware"
	"github.com/Josepavese/matrix/pkg/zedacp"
)

// ----------------------------------------------------------------------------
// The version 2 prompt lifecycle in the adapter
//
// The protocol package proves the shapes decode. These tests prove the adapter
// does something with them: a turn ends on the peer's terminal state instead of
// the prompt response, the wait is bounded and visible when the peer never
// reports one, every streaming variant reaches Matrix's neutral representation,
// and a version 1 turn still ends the way it always did.
// ----------------------------------------------------------------------------

// streamingACPClient is a protocol client whose turn continues after the prompt
// response, the way a version 2 peer's does. It mirrors the real client's
// registration: a version 1 prompt hands it the observer, a version 2 turn
// watches the session.
type streamingACPClient struct {
	*pagedListACPClient

	mu        sync.Mutex
	watchers  map[string]acpSessionObserver
	prompted  chan struct{}
	streaming func(emit func(acpSessionUpdate))
}

func newStreamingACPClient(version int) *streamingACPClient {
	return &streamingACPClient{
		pagedListACPClient: &pagedListACPClient{ctx: context.Background(), protocolVersion: version},
		watchers:           map[string]acpSessionObserver{},
		prompted:           make(chan struct{}),
	}
}

func (c *streamingACPClient) WatchSession(sessionID string, observer acpSessionObserver) func() {
	c.mu.Lock()
	c.watchers[sessionID] = observer
	c.mu.Unlock()
	return func() {
		c.mu.Lock()
		delete(c.watchers, sessionID)
		c.mu.Unlock()
	}
}

func (c *streamingACPClient) Prompt(_ context.Context, req acpPromptRequest, observer acpSessionObserver) (*acpPromptResponse, error) {
	c.promptReq = &req
	if observer != nil {
		c.WatchSession(req.SessionID, observer)
	}
	close(c.prompted)
	if c.streaming != nil {
		// The turn continues on its own, after the response: that is what an
		// acknowledgement of insertion means.
		go c.streaming(func(update acpSessionUpdate) {
			c.mu.Lock()
			watcher := c.watchers[req.SessionID]
			c.mu.Unlock()
			if watcher != nil {
				watcher.OnUpdate(acpSessionNotification{SessionID: req.SessionID, Update: update})
			}
		})
	}
	return &acpPromptResponse{MessageID: "msg-user-1"}, nil
}

func (c *streamingACPClient) turnClient() *acpConversationClient {
	return &acpConversationClient{
		client:         c,
		deps:           middleware.ConversationFactoryDeps{AgentID: "codex-agent"},
		cwd:            "/workspace",
		endpoint:       middleware.ProtocolEndpoint{Command: "/opt/agent"},
		loadedSessions: map[string]bool{"remote-1": true},
	}
}

func turnForLifecycleTest() middleware.ConversationTurn {
	return middleware.ConversationTurn{
		AgentID:          "codex-agent",
		LogicalSessionID: "logical-1",
		RemoteSessionID:  "remote-1",
		Message:          "hello",
	}
}

func chunkUpdate(messageID, text string) acpSessionUpdate {
	return acpSessionUpdate{
		SessionUpdate: "agent_message_chunk",
		MessageID:     messageID,
		Content:       acpContent{Type: "text", Text: text},
		Contents:      []acpContent{{Type: "text", Text: text}},
	}
}

// TestExecuteTurnWaitsForTheV2TerminalState is the completion proof: the peer
// acknowledges the prompt, streams part of the answer, goes quiet for longer
// than the version 1 quiet window, and only then streams the rest and reports
// idle. The turn must return the whole answer, which the quiet wait cannot do.
func TestExecuteTurnWaitsForTheV2TerminalState(t *testing.T) {
	fake := newStreamingACPClient(zedacp.ProtocolVersionV2)
	fake.streaming = func(emit func(acpSessionUpdate)) {
		emit(chunkUpdate("msg-1", "part one"))
		time.Sleep(400 * time.Millisecond)
		emit(chunkUpdate("msg-1", " and part two"))
		emit(acpSessionUpdate{SessionUpdate: "state_update", State: "idle", StopReason: "end_turn"})
	}

	result, err := fake.turnClient().ExecuteTurn(context.Background(), turnForLifecycleTest())
	if err != nil {
		t.Fatalf("the turn must succeed once the peer reports idle: %v", err)
	}
	if result.Output != "part one and part two" {
		t.Fatalf("the turn must return the complete answer, got %q", result.Output)
	}
	if got := result.Metadata.Meta["acp_turn_completion"]; got != "idle" {
		t.Fatalf("the result must say the peer ended the turn, got %#v", result.Metadata.Meta)
	}
	if got := result.Metadata.Meta["acp_stop_reason"]; got != "end_turn" {
		t.Fatalf("the specification's stop reason must survive into the result, got %#v", got)
	}
}

// TestExecuteTurnEndsAtTheV2TurnBudgetAndSaysSo is the bound: a peer that
// acknowledges insertion and then reports nothing must not hold the turn open,
// and the early return must be visible in the result rather than silent.
func TestExecuteTurnEndsAtTheV2TurnBudgetAndSaysSo(t *testing.T) {
	t.Setenv(acpV2TurnEnv, "60ms")
	fake := newStreamingACPClient(zedacp.ProtocolVersionV2)
	fake.streaming = func(emit func(acpSessionUpdate)) {
		emit(chunkUpdate("msg-1", "partial"))
	}

	started := time.Now()
	result, err := fake.turnClient().ExecuteTurn(context.Background(), turnForLifecycleTest())
	elapsed := time.Since(started)
	if err != nil {
		t.Fatalf("a silent peer must bound the turn, not fail it: %v", err)
	}
	if elapsed < 60*time.Millisecond {
		t.Fatalf("the turn must wait for the budget before giving up, returned after %s", elapsed)
	}
	if elapsed > 5*time.Second {
		t.Fatalf("the turn must end within its budget, took %s", elapsed)
	}
	if result.Output != "partial" {
		t.Fatalf("what the peer did send must still be returned, got %q", result.Output)
	}
	if got := result.Metadata.Meta["acp_turn_completion"]; got != "timeout" {
		t.Fatalf("the result must record that no terminal state arrived, got %#v", result.Metadata.Meta)
	}
	if got := result.Metadata.Meta["acp_turn_budget_ms"]; got != int64(60) {
		t.Fatalf("the result must record the budget it used, got %#v", got)
	}
}

// TestExecuteTurnKeepsTheV1QuietWait pins the generation this change must not
// touch: a version 1 turn still ends on the prompt response plus the quiet wait,
// and carries no version 2 completion metadata.
func TestExecuteTurnKeepsTheV1QuietWait(t *testing.T) {
	fake := newStreamingACPClient(zedacp.ProtocolVersionV1)
	fake.streaming = func(emit func(acpSessionUpdate)) {
		emit(chunkUpdate("msg-1", "part one"))
		time.Sleep(400 * time.Millisecond)
		emit(chunkUpdate("msg-1", " and part two"))
		emit(acpSessionUpdate{SessionUpdate: "state_update", State: "idle", StopReason: "end_turn"})
	}

	result, err := fake.turnClient().ExecuteTurn(context.Background(), turnForLifecycleTest())
	if err != nil {
		t.Fatalf("the version 1 turn must succeed: %v", err)
	}
	if result.Output != "part one" {
		t.Fatalf("a version 1 turn ends on the quiet wait, so only what arrived inside it is returned, got %q", result.Output)
	}
	for _, key := range []string{"acp_turn_completion", "acp_stop_reason", "acp_turn_budget_ms"} {
		if _, ok := result.Metadata.Meta[key]; ok {
			t.Fatalf("version 1 must not carry the version 2 key %q: %#v", key, result.Metadata.Meta)
		}
	}
}

// TestExecuteTurnAppliesTheV2MessageUpsert proves the whole-message shape is
// projected rather than appended: the upsert replaces what the chunks built, and
// the turn ends with the replacement text once.
func TestExecuteTurnAppliesTheV2MessageUpsert(t *testing.T) {
	fake := newStreamingACPClient(zedacp.ProtocolVersionV2)
	fake.streaming = func(emit func(acpSessionUpdate)) {
		emit(chunkUpdate("msg-1", "streamed"))
		emit(acpSessionUpdate{
			SessionUpdate: "agent_message",
			MessageID:     "msg-1",
			ContentSet:    true,
			Contents:      []acpContent{{Type: "text", Text: "replacement"}},
		})
		emit(chunkUpdate("msg-2", " tail"))
		emit(acpSessionUpdate{SessionUpdate: "state_update", State: "idle", StopReason: "end_turn"})
	}

	result, err := fake.turnClient().ExecuteTurn(context.Background(), turnForLifecycleTest())
	if err != nil {
		t.Fatalf("the turn must succeed: %v", err)
	}
	if result.Output != "replacement tail" {
		t.Fatalf("an upsert replaces its message's content, got %q", result.Output)
	}
}

// TestObserverProjectsTheV2StreamingShapes drives every variant the adapter used
// to drop through the observer and asserts what a caller can see afterwards.
func TestObserverProjectsTheV2StreamingShapes(t *testing.T) {
	notifier := &observerTestNotifier{}
	obs := &simpleObserver{updates: make(chan struct{}, 1), notifier: notifier}
	session := "remote-1"
	emit := func(update acpSessionUpdate) {
		obs.OnUpdate(acpSessionNotification{SessionID: session, Update: update})
	}

	emit(acpSessionUpdate{
		SessionUpdate: "tool_call_content_chunk", ToolCallID: "call-1",
		ToolContents: []acpToolCallContent{{
			Type:    "content",
			Content: &acpContent{Type: "text", Text: "checked syntax"},
		}},
	})
	emit(acpSessionUpdate{
		SessionUpdate: "tool_call_update", ToolCallID: "call-1", Status: "completed",
		ToolContents: []acpToolCallContent{{
			Type: "diff",
			Changes: []acpDiffChange{
				{Operation: "modify", Path: "/tmp/a.go", FileType: "text"},
				{Operation: "move", OldPath: "/tmp/b.go", Path: "/tmp/c.go"},
			},
			Patch: &zedacp.DiffPatch{Format: "git_patch", Text: "--- a\n+++ b\n"},
		}},
	})
	emit(acpSessionUpdate{
		SessionUpdate: "plan_update",
		Plan:          &zedacp.PlanUpdateContent{Type: "items", PlanID: "plan-7", Entries: []zedacp.PlanEntry{{Content: "read", Priority: "high", Status: "pending"}}},
	})
	emit(acpSessionUpdate{
		SessionUpdate: "terminal_update", TerminalID: "term-1",
		Command:    ptrTo("go test ./..."),
		Cwd:        ptrTo("/tmp"),
		Output:     &zedacp.TerminalOutput{Data: "b2sK"},
		ExitStatus: &zedacp.TerminalExitStatus{ExitCode: ptrTo(0)},
	})
	emit(acpSessionUpdate{SessionUpdate: "terminal_output_chunk", TerminalID: "term-1", Data: "bW9yZQ=="})

	if len(notifier.updates) != 5 {
		t.Fatalf("every variant must reach the live surface, got %d updates: %#v", len(notifier.updates), notifier.updates)
	}

	toolChunk := notifier.updates[0].Metadata
	if toolChunk["tool_call_id"] != "call-1" {
		t.Fatalf("a tool call content chunk must project its tool call, got %#v", toolChunk)
	}
	if !strings.Contains(notifier.updates[0].Content, "checked syntax") {
		t.Fatalf("a tool call content chunk's text must reach the projection, got %q", notifier.updates[0].Content)
	}

	diff := notifier.updates[1].Metadata
	changes, ok := diff["diff_changes"].([]acpDiffChange)
	if !ok || len(changes) != 2 || changes[0].Path != "/tmp/a.go" || changes[1].OldPath != "/tmp/b.go" {
		t.Fatalf("a version 2 diff must project its structured changes, got %#v", diff["diff_changes"])
	}
	if diff["path"] != "/tmp/a.go" {
		t.Fatalf("the first affected path must be projected, got %#v", diff["path"])
	}
	if diff["diff_patch"] != "--- a\n+++ b\n" || diff["diff_patch_format"] != "git_patch" {
		t.Fatalf("the renderable patch must be projected, got %#v", diff)
	}

	plan := notifier.updates[2].Metadata
	entries, ok := plan["plan_entries"].([]zedacp.PlanEntry)
	if !ok || len(entries) != 1 || entries[0].Content != "read" {
		t.Fatalf("a nested v2 plan must project its entries, got %#v", plan["plan_entries"])
	}
	if plan["plan_id"] != "plan-7" {
		t.Fatalf("a v2 plan must project its identity, got %#v", plan["plan_id"])
	}

	terminal := notifier.updates[3].Metadata
	if terminal["terminal_id"] != "term-1" || terminal["command"] != "go test ./..." {
		t.Fatalf("a terminal update must project its identity and command, got %#v", terminal)
	}
	if terminal["terminal_output_bytes"] != len("b2sK") {
		t.Fatalf("a terminal snapshot must project its size, got %#v", terminal["terminal_output_bytes"])
	}
	if exit, ok := terminal["exit_status"].(*zedacp.TerminalExitStatus); !ok || exit.ExitCode == nil || *exit.ExitCode != 0 {
		t.Fatalf("a terminal update must project its exit status, got %#v", terminal["exit_status"])
	}
	if chunkMeta := notifier.updates[4].Metadata; chunkMeta["terminal_output_chunk_bytes"] != len("bW9yZQ==") {
		t.Fatalf("a terminal output chunk must project its size, got %#v", chunkMeta)
	}
}

// TestPermissionRequestUnderstandsTheV2Fields reads the request shape the
// specification defines for version 2 — a prompt title, an optional description
// and a structured subject — and asserts the decision the client returns is
// still the option the peer offered.
func TestPermissionRequestUnderstandsTheV2Fields(t *testing.T) {
	notifier := &observerTestNotifier{}
	handler := newConfigurableRequestHandler(func() bool { return true })
	handler.WithNotifier(notifier)

	params := json.RawMessage(`{
		"sessionId": "session-1",
		"title": "Run the test suite?",
		"description": "This runs go test over the workspace.",
		"subject": {"type": "command", "command": "go test ./...", "cwd": "/workspace"},
		"options": [
			{"optionId": "allow-once", "name": "Allow once", "kind": "allow_once"},
			{"optionId": "reject-once", "name": "Reject", "kind": "reject_once"}
		]}`)
	result, err := handler.HandleRequest(context.Background(), "session/request_permission", params)
	if err != nil {
		t.Fatalf("permission request: %v", err)
	}
	outcome, _ := result.(map[string]interface{})["outcome"].(map[string]interface{})
	if outcome["outcome"] != "selected" || outcome["optionId"] != "allow-once" {
		t.Fatalf("the option the peer offered must be selected, got %#v", outcome)
	}
	if len(notifier.updates) != 1 {
		t.Fatalf("the decision must be reported once, got %d updates", len(notifier.updates))
	}
	meta := notifier.updates[0].Metadata
	if meta["permission_title"] != "Run the test suite?" {
		t.Fatalf("the v2 prompt title must reach the operator, got %#v", meta)
	}
	if meta["permission_description"] != "This runs go test over the workspace." {
		t.Fatalf("the v2 description must reach the operator, got %#v", meta)
	}
	if meta["permission_subject_type"] != "command" {
		t.Fatalf("the v2 subject type must reach the operator, got %#v", meta)
	}
	if _, ok := meta["permission_subject"].(map[string]interface{}); !ok {
		t.Fatalf("the v2 subject must be preserved, got %#v", meta["permission_subject"])
	}
}

// TestPermissionRequestKeepsTheV1ToolCallTitle pins the other generation: a
// version 1 request carries no title of its own, and the title on the tool call
// is what a human sees.
func TestPermissionRequestKeepsTheV1ToolCallTitle(t *testing.T) {
	notifier := &observerTestNotifier{}
	handler := newConfigurableRequestHandler(func() bool { return false })
	handler.WithNotifier(notifier)

	params := json.RawMessage(`{
		"sessionId": "session-1",
		"toolCall": {"toolCallId": "call-1", "title": "Write the file", "kind": "edit"},
		"options": [{"optionId": "reject-once", "kind": "reject_once"}]}`)
	if _, err := handler.HandleRequest(context.Background(), "session/request_permission", params); err != nil {
		t.Fatalf("permission request: %v", err)
	}
	if meta := notifier.updates[0].Metadata; meta["permission_title"] != "Write the file" {
		t.Fatalf("a version 1 tool call title must still reach the operator, got %#v", meta)
	}
}

func ptrTo[T any](value T) *T { return &value }
