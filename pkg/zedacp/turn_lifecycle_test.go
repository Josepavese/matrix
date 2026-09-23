package zedacp

import (
	"encoding/json"
	"sync"
	"testing"
	"time"
)

// ----------------------------------------------------------------------------
// The version 2 prompt lifecycle, as the specification defines it
//
// `SessionUpdate` is the one place a session notification is decoded, so these
// tests pin the shapes the client has to understand before any adapter can
// project them:
//
//   - state_update is the terminal signal: the prompt response only acknowledges
//     insertion, and an idle state with a stop reason ends the turn;
//   - plan_update nests its entries under "plan" where version 1 carried them at
//     the top level;
//   - terminal_update and terminal_output_chunk describe an agent-owned
//     terminal, whose bytes never become turn content;
//   - a message upsert's content field has patch semantics, so "absent",
//     "null" and "array" are three different things;
//   - a version 2 diff carries structured changes and an optional patch.
// ----------------------------------------------------------------------------

func decodeUpdate(t *testing.T, raw string) SessionUpdate {
	t.Helper()
	var update SessionUpdate
	if err := json.Unmarshal([]byte(raw), &update); err != nil {
		t.Fatalf("decode session update %s: %v", raw, err)
	}
	return update
}

func TestSessionUpdateCarriesTheV2TerminalState(t *testing.T) {
	idle := decodeUpdate(t, `{"sessionUpdate":"state_update","state":"idle","stopReason":"end_turn"}`)
	if reason, terminal := idle.TurnTerminal(); !terminal || reason != "end_turn" {
		t.Fatalf("an idle state_update is the terminal signal, got terminal=%v reason=%q", terminal, reason)
	}

	// An idle state with no stop reason is still terminal: the specification
	// says the reason is optional and only SHOULD be present when the transition
	// ends foreground work.
	if reason, terminal := decodeUpdate(t, `{"sessionUpdate":"state_update","state":"idle"}`).TurnTerminal(); !terminal || reason != "" {
		t.Fatalf("idle without a stop reason is terminal with an empty reason, got terminal=%v reason=%q", terminal, reason)
	}

	// Everything else must not end a turn: a client that treated running or
	// requires_action as completion would cut the answer off.
	for _, raw := range []string{
		`{"sessionUpdate":"state_update","state":"running"}`,
		`{"sessionUpdate":"state_update","state":"requires_action"}`,
		`{"sessionUpdate":"state_update","state":"_custom"}`,
		`{"sessionUpdate":"state_update"}`,
		`{"sessionUpdate":"agent_message_chunk","content":{"type":"text","text":"hi"}}`,
		`{"sessionUpdate":"agent_message_chunk","state":"idle","stopReason":"end_turn"}`,
	} {
		if _, terminal := decodeUpdate(t, raw).TurnTerminal(); terminal {
			t.Fatalf("%s must not be a terminal signal", raw)
		}
	}
}

func TestSessionUpdateDecodesTheV2StreamingShapes(t *testing.T) {
	plan := decodeUpdate(t, `{"sessionUpdate":"plan_update","plan":{"type":"items","planId":"plan-7","entries":[
		{"content":"read","priority":"high","status":"in_progress"},
		{"content":"write","priority":"low","status":"pending"}]}}`)
	if plan.Plan == nil || plan.Plan.PlanID != "plan-7" {
		t.Fatalf("plan_update must carry the plan it identifies, got %#v", plan.Plan)
	}
	if entries := plan.PlanEntries(); len(entries) != 2 || entries[0].Content != "read" || entries[1].Status != "pending" {
		t.Fatalf("plan_update entries must be readable in the v2 shape, got %#v", entries)
	}
	// The version 1 shape still resolves through the same accessor.
	if entries := decodeUpdate(t, `{"sessionUpdate":"plan","entries":[{"content":"one","priority":"high","status":"pending"}]}`).PlanEntries(); len(entries) != 1 {
		t.Fatalf("a version 1 plan update must keep its top-level entries, got %#v", entries)
	}

	terminal := decodeUpdate(t, `{"sessionUpdate":"terminal_update","terminalId":"term-1","command":"go test","cwd":"/tmp",
		"output":{"data":"b2sK"},"exitStatus":{"exitCode":0}}`)
	if terminal.TerminalID != "term-1" || terminal.Command == nil || *terminal.Command != "go test" {
		t.Fatalf("terminal_update must carry its identity and command, got %#v", terminal)
	}
	if terminal.Cwd == nil || *terminal.Cwd != "/tmp" || terminal.Output == nil || terminal.Output.Data != "b2sK" {
		t.Fatalf("terminal_update must carry its cwd and output snapshot, got %#v", terminal)
	}
	if terminal.ExitStatus == nil || terminal.ExitStatus.ExitCode == nil || *terminal.ExitStatus.ExitCode != 0 {
		t.Fatalf("terminal_update must carry its exit status, got %#v", terminal.ExitStatus)
	}
	if chunk := decodeUpdate(t, `{"sessionUpdate":"terminal_output_chunk","terminalId":"term-1","data":"bW9yZQ=="}`); chunk.Data != "bW9yZQ==" {
		t.Fatalf("terminal_output_chunk must carry its bytes, got %q", chunk.Data)
	}

	diff := decodeUpdate(t, `{"sessionUpdate":"tool_call_update","toolCallId":"call-1","status":"completed","content":[
		{"type":"diff","changes":[{"operation":"modify","path":"/tmp/a.go","fileType":"text"},
		 {"operation":"move","oldPath":"/tmp/b.go","path":"/tmp/c.go"}],
		 "patch":{"format":"git_patch","text":"--- a\n+++ b\n"}}]}`)
	if len(diff.ToolContents) != 1 {
		t.Fatalf("a v2 diff tool content must decode, got %#v", diff.ToolContents)
	}
	content := diff.ToolContents[0]
	if len(content.Changes) != 2 || content.Changes[0].Path != "/tmp/a.go" || content.Changes[1].OldPath != "/tmp/b.go" {
		t.Fatalf("a v2 diff must keep its structured changes, got %#v", content.Changes)
	}
	if content.Patch == nil || content.Patch.Format != "git_patch" || content.Patch.Text == "" {
		t.Fatalf("a v2 diff must keep its patch, got %#v", content.Patch)
	}

	// A tool-call content chunk carries one ToolCallContent item, which is what
	// makes it projectable without a second decoder.
	chunk := decodeUpdate(t, `{"sessionUpdate":"tool_call_content_chunk","toolCallId":"call-1",
		"content":{"type":"content","content":{"type":"text","text":"checked syntax"}}}`)
	if len(chunk.ToolContents) != 1 || chunk.ToolContents[0].Content == nil || chunk.ToolContents[0].Content.Text != "checked syntax" {
		t.Fatalf("tool_call_content_chunk must decode its content item, got %#v", chunk.ToolContents)
	}
}

func TestSessionUpdateMessageUpsertContentHasPatchSemantics(t *testing.T) {
	replacement := decodeUpdate(t, `{"sessionUpdate":"agent_message","messageId":"m1","content":[{"type":"text","text":"full"}]}`)
	if !replacement.ContentSet || replacement.ContentCleared {
		t.Fatalf("a concrete content array must be a replacement, got set=%v cleared=%v", replacement.ContentSet, replacement.ContentCleared)
	}
	if len(replacement.Contents) != 1 || replacement.Contents[0].Text != "full" {
		t.Fatalf("the replacement content must decode, got %#v", replacement.Contents)
	}

	cleared := decodeUpdate(t, `{"sessionUpdate":"agent_message","messageId":"m1","content":null}`)
	if !cleared.ContentSet || !cleared.ContentCleared {
		t.Fatalf("an explicit null must be distinguishable from an omitted field, got set=%v cleared=%v", cleared.ContentSet, cleared.ContentCleared)
	}

	untouched := decodeUpdate(t, `{"sessionUpdate":"agent_message","messageId":"m1"}`)
	if untouched.ContentSet || untouched.ContentCleared {
		t.Fatalf("an omitted content must leave the message alone, got set=%v cleared=%v", untouched.ContentSet, untouched.ContentCleared)
	}

	chunk := decodeUpdate(t, `{"sessionUpdate":"agent_message_chunk","messageId":"m1","content":{"type":"text","text":" part"}}`)
	if chunk.MessageID != "m1" || chunk.Content.Text != " part" {
		t.Fatalf("a chunk must carry its message id and content, got %#v", chunk)
	}
}

func TestSessionUpdateRoundTripsTheV2LifecycleFields(t *testing.T) {
	for _, raw := range []string{
		`{"sessionUpdate":"state_update","state":"idle","stopReason":"max_tokens"}`,
		`{"sessionUpdate":"plan_update","plan":{"type":"items","planId":"plan-7","entries":[{"content":"read","priority":"high","status":"pending"}]}}`,
		`{"sessionUpdate":"terminal_update","terminalId":"term-1","command":"go test","cwd":"/tmp","output":{"data":"b2sK"},"exitStatus":{"exitCode":0}}`,
	} {
		update := decodeUpdate(t, raw)
		encoded, err := json.Marshal(update)
		if err != nil {
			t.Fatalf("marshal %s: %v", raw, err)
		}
		if again := decodeUpdate(t, string(encoded)); !jsonEqual(t, update, again) {
			t.Fatalf("round trip changed the update: %s -> %s", raw, encoded)
		}
	}
}

func jsonEqual(t *testing.T, left, right SessionUpdate) bool {
	t.Helper()
	leftBytes, err := json.Marshal(left)
	if err != nil {
		t.Fatalf("marshal left: %v", err)
	}
	rightBytes, err := json.Marshal(right)
	if err != nil {
		t.Fatalf("marshal right: %v", err)
	}
	return string(leftBytes) == string(rightBytes)
}

// ----------------------------------------------------------------------------
// Watching a session outside a call
//
// The adapter cannot end a version 2 turn inside the prompt call, because the
// turn ends after it. WatchSession is the primitive that keeps the observer
// registered for as long as the caller owns the turn, and these tests pin both
// halves: updates arrive while watching, and stop arriving after the caller is
// done.
// ----------------------------------------------------------------------------

func waitForJoinedText(t *testing.T, want string, observer *lifecycleObserver) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if observer.joined() == want {
			return
		}
		time.Sleep(time.Millisecond)
	}
}

func TestWatchSessionDeliversUpdatesUntilTheCallerStops(t *testing.T) {
	client := &Client{observers: make(map[string]map[uint64]SessionObserver)}
	observer := &lifecycleObserver{}

	stop := client.WatchSession("session-1", observer)
	client.handleNotification(sessionUpdateResponse(t, "session-1", "streamed"))
	waitForJoinedText(t, "streamed", observer)
	if got := observer.joined(); got != "streamed" {
		t.Fatalf("a watched session must deliver updates, got %q", got)
	}

	// The terminal state is an update like any other, which is the whole point:
	// it arrives after the prompt call has returned.
	client.handleNotification(sessionUpdateResponseOf(t, "session-1", map[string]interface{}{
		"sessionUpdate": "state_update", "state": "idle", "stopReason": "end_turn",
	}))
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if _, terminal := observer.terminalState(); terminal {
			break
		}
		time.Sleep(time.Millisecond)
	}
	if _, terminal := observer.terminalState(); !terminal {
		t.Fatal("the terminal state update must reach a watched observer")
	}

	stop()
	client.handleNotification(sessionUpdateResponse(t, "session-1", "after-stop"))
	time.Sleep(50 * time.Millisecond)
	if got := observer.joined(); got != "streamed" {
		t.Fatalf("a stopped watch must not deliver more updates, got %q", got)
	}
}

// lifecycleObserver keeps the joined content of every chunk it receives and the
// terminal state it was told about.
type lifecycleObserver struct {
	mu         sync.Mutex
	updates    []string
	stopReason string
	terminal   bool
}

func (o *lifecycleObserver) OnUpdate(notification SessionNotification) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.updates = append(o.updates, notification.Update.Content.Text)
	if reason, terminal := notification.Update.TurnTerminal(); terminal {
		o.stopReason, o.terminal = reason, true
	}
}

func (o *lifecycleObserver) joined() string {
	o.mu.Lock()
	defer o.mu.Unlock()
	out := ""
	for _, update := range o.updates {
		out += update
	}
	return out
}

func (o *lifecycleObserver) terminalState() (string, bool) {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.stopReason, o.terminal
}

func sessionUpdateResponseOf(t *testing.T, sessionID string, update map[string]interface{}) *jsonRPCResponse {
	t.Helper()
	method := "session/update"
	params, err := json.Marshal(map[string]interface{}{"sessionId": sessionID, "update": update})
	if err != nil {
		t.Fatalf("marshal notification: %v", err)
	}
	return &jsonRPCResponse{Method: &method, Params: params}
}
