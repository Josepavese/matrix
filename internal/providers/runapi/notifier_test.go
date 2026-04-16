package runapi

import (
	"testing"

	"github.com/jose/matrix-v2/internal/logic/runtrace"
	"github.com/jose/matrix-v2/internal/middleware"
)

func TestRunTraceNotifierRecordsIntermediateEvents(t *testing.T) {
	store := runtrace.NewStore(runtrace.NewMemoryStorage())
	run, _, err := store.Start(runtrace.Run{AgentID: "codex", Protocol: "acp", ChannelID: "http.test"})
	if err != nil {
		t.Fatalf("start run: %v", err)
	}
	notifier := &runTraceNotifier{store: store, runID: run.ID, agentID: "codex", protocol: "acp"}
	notifier.SetHeader("codex", "remote-123")
	notifier.OnThought(middleware.ThoughtUpdate{Type: middleware.ThoughtTypeThinking, Content: "partial"})
	notifier.OnThought(middleware.ThoughtUpdate{Type: middleware.ThoughtTypeToolCall, Content: "shell ls"})
	notifier.OnThought(middleware.ThoughtUpdate{Type: middleware.ThoughtTypeToolResult, Content: "ok"})

	events, err := store.LoadEvents(run.ID, 0)
	if err != nil {
		t.Fatalf("load events: %v", err)
	}
	kinds := map[string]bool{}
	for _, event := range events {
		kinds[event.Kind] = true
	}
	for _, kind := range []string{"agent.message.delta", "tool.call.requested", "tool.result.received"} {
		if !kinds[kind] {
			t.Fatalf("missing event kind %s in %#v", kind, kinds)
		}
	}
	updated, found, err := store.LoadRun(run.ID)
	if err != nil || !found {
		t.Fatalf("load run: found=%v err=%v", found, err)
	}
	if updated.RemoteSessionID != "remote-123" {
		t.Fatalf("expected remote session update, got %s", updated.RemoteSessionID)
	}
}
