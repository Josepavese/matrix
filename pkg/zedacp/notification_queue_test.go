package zedacp

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestPromptRetainsBurstChunksAndKeepsRPCResponsive(t *testing.T) {
	client, transport, observer, done := heldPrompt(t)
	const chunks = 2048
	for i := 0; i < chunks; i++ {
		client.handleNotification(sessionUpdateResponse(t, "s", " x\n"))
	}
	client.handleNotification(sessionUpdateResponse(t, "s", "final"))
	transport.push(t, `{"jsonrpc":"2.0","id":1,"result":{"stopReason":"end_turn"}}`)
	rpcDone := make(chan error, 1)
	go func() { rpcDone <- client.ExtRequest(context.Background(), "_probe", nil, nil) }()
	waitForSent(t, transport, func(s string) bool { return strings.Contains(s, `"_probe"`) }, "independent RPC")
	transport.push(t, `{"jsonrpc":"2.0","id":2,"result":{}}`)
	select {
	case err := <-rpcDone:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("slow observer blocked RPC responses")
	}
	observer.release <- struct{}{}
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("burst never drained")
	}
	if got, want := observer.joined(), "hold"+strings.Repeat(" x\n", chunks)+"final"; got != want {
		t.Fatalf("burst lost text or whitespace: got %d bytes, want %d", len(got), len(want))
	}
}

func TestNotificationQueueByteBudgetIncludesInflightCallback(t *testing.T) {
	queue := &sessionNotifications{ready: make(chan struct{}, 1)}
	if !queue.enqueue(observedNotification{bytes: notificationBacklogBytes}) {
		t.Fatal("first bounded update refused")
	}
	inflight, ok := queue.take()
	if !ok {
		t.Fatal("missing update")
	}
	if queue.enqueue(observedNotification{bytes: 1}) {
		t.Fatal("inflight bytes escaped the resource budget")
	}
	queue.delivered(inflight)
	if !queue.enqueue(observedNotification{bytes: 1}) {
		t.Fatal("completed callback did not release budget")
	}
}

func TestChunkCoalescingPreservesRawExtensionsAndCorrelation(t *testing.T) {
	observer := &recordingObserver{}
	makeChunk := func(raw string) observedNotification {
		var update SessionUpdate
		if err := json.Unmarshal([]byte(raw), &update); err != nil {
			t.Fatal(err)
		}
		return observedNotification{update: SessionNotification{SessionID: "s", Update: update}, observers: map[uint64]SessionObserver{1: observer}, bytes: len(raw)}
	}
	first := makeChunk(`{"sessionUpdate":"agent_message_chunk","messageId":"m","content":{"type":"text","text":"one ","future":{"id":9007199254740993}},"_meta":{"phase":"final"}}`)
	second := makeChunk(`{"sessionUpdate":"agent_message_chunk","messageId":"m","content":{"type":"text","text":" two\n","future":{"id":9007199254740993}},"_meta":{"phase":"final"}}`)
	if !mergeNotificationText(&first, second) {
		t.Fatal("matching chunks did not merge")
	}
	if first.update.Update.Content.Text != "one  two\n" || first.update.Update.Contents[0].Text != "one  two\n" {
		t.Fatal("text projection changed")
	}
	if !strings.Contains(string(first.update.Update.RawContent), `9007199254740993`) {
		t.Fatal("opaque raw extension lost precision")
	}
	for _, change := range []string{"message", "phase", "observer", "reasoning", "extension"} {
		next := second
		next.update.Update = second.update.Update
		switch change {
		case "message":
			next.update.Update.MessageID = "other"
		case "phase":
			next.update.Update.Meta = map[string]interface{}{"phase": "progress"}
		case "observer":
			next.observers = map[uint64]SessionObserver{2: observer}
		case "reasoning":
			next.update.Update.SessionUpdate = "agent_thought_chunk"
		case "extension":
			next.update.Update.RawContent = json.RawMessage(`{"type":"text","text":" two\n","future":{"id":1}}`)
		}
		if mergeNotificationText(&first, next) {
			t.Fatalf("crossed %s barrier", change)
		}
	}
}
