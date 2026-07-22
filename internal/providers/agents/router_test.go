package agents

import (
	"testing"

	"github.com/Josepavese/matrix/internal/middleware"
	"github.com/Josepavese/matrix/pkg/zedacp"
)

type observerTestNotifier struct {
	updates []middleware.ThoughtUpdate
}

func (n *observerTestNotifier) OnThought(update middleware.ThoughtUpdate) {
	n.updates = append(n.updates, update)
}

func (n *observerTestNotifier) SetHeader(_, _ string) {}

func (n *observerTestNotifier) FormattedHeader() string { return "" }

func TestToolUpdateMetadataPreservesNativeACPPayload(t *testing.T) {
	meta := toolUpdateMetadata(acpSessionNotification{
		SessionID: "remote-123",
		Update: zedacp.SessionUpdate{
			SessionUpdate: "tool_call",
			Content:       acpContent{Type: "text", Text: "write"},
			Title:         "write_file",
			UpdatedAt:     "2026-04-16T12:00:00Z",
			Meta:          map[string]interface{}{"path": "/tmp/noema_matrix_contract.sh"},
		},
	})

	if meta["protocol"] != "acp" || meta["protocol_method"] != "session/update" {
		t.Fatalf("expected ACP protocol metadata, got %#v", meta)
	}
	raw, ok := meta["acp"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected raw ACP metadata map, got %#v", meta["acp"])
	}
	if raw["session_id"] != "remote-123" || raw["session_update"] != "tool_call" {
		t.Fatalf("unexpected ACP raw identity: %#v", raw)
	}
	content, ok := raw["content"].(map[string]interface{})
	if !ok || content["text"] != "write" {
		t.Fatalf("expected ACP raw content, got %#v", raw["content"])
	}
}

func TestObserverForwardsMetadataOnlyToolUpdates(t *testing.T) {
	notifier := &observerTestNotifier{}
	obs := &simpleObserver{notifier: notifier}

	obs.OnUpdate(acpSessionNotification{
		SessionID: "remote-123",
		Update: zedacp.SessionUpdate{
			SessionUpdate: "tool_call",
			Title:         "write_file",
			ToolCallID:    "tool-123",
			Kind:          "edit",
			Status:        "pending",
			RawInput: map[string]interface{}{
				"path": "/tmp/noema_matrix_contract.go",
			},
		},
	})
	obs.OnUpdate(acpSessionNotification{
		SessionID: "remote-123",
		Update: zedacp.SessionUpdate{
			SessionUpdate: "tool_call_update",
			ToolCallID:    "tool-123",
			Kind:          "edit",
			Status:        "completed",
			RawInput: map[string]interface{}{
				"path": "/tmp/noema_matrix_contract.go",
			},
		},
	})

	if len(notifier.updates) != 2 {
		t.Fatalf("expected metadata-only tool updates to be forwarded, got %#v", notifier.updates)
	}
	if notifier.updates[0].Type != middleware.ThoughtTypeToolCall || notifier.updates[1].Type != middleware.ThoughtTypeToolResult {
		t.Fatalf("unexpected forwarded update types: %#v", notifier.updates)
	}
	if notifier.updates[0].Content != "" || notifier.updates[0].Title != "write_file" {
		t.Fatalf("expected empty content and preserved title, got %#v", notifier.updates[0])
	}
	if notifier.updates[0].Metadata["tool_call_id"] != "tool-123" || notifier.updates[0].Metadata["tool_kind"] != "edit" {
		t.Fatalf("expected ACP tool metadata, got %#v", notifier.updates[0].Metadata)
	}
}

func TestObserverPreservesRichAgentMessageBlocks(t *testing.T) {
	obs := &simpleObserver{}
	obs.OnUpdate(acpSessionNotification{
		SessionID: "remote-123",
		Update: zedacp.SessionUpdate{
			SessionUpdate: "agent_message_chunk",
			Content:       acpContent{Type: "image", Data: "aW1hZ2U=", MimeType: "image/png"},
			Contents:      []acpContent{{Type: "image", Data: "aW1hZ2U=", MimeType: "image/png"}},
		},
	})
	blocks := obs.ContentBlocks()
	if len(blocks) != 1 || blocks[0].Type != "image" || blocks[0].MimeType != "image/png" {
		t.Fatalf("rich ACP output was not preserved: %#v", blocks)
	}
}

func TestObserverSelectsExplicitFinalMessageWithoutCommentaryOrDiagnostic(t *testing.T) {
	notifier := &observerTestNotifier{}
	obs := &simpleObserver{notifier: notifier}
	updates := []zedacp.SessionUpdate{
		{
			SessionUpdate: "agent_message_chunk",
			MessageID:     "commentary-1",
			Content:       acpContent{Type: "text", Text: "Leggo il contesto. "},
			Contents:      []acpContent{{Type: "text", Text: "Leggo il contesto. "}},
			Meta:          map[string]interface{}{"codex": map[string]interface{}{"phase": "commentary"}},
		},
		{
			SessionUpdate: "agent_message_chunk",
			MessageID:     "final-1",
			Content:       acpContent{Type: "text", Text: "Ciao "},
			Contents:      []acpContent{{Type: "text", Text: "Ciao "}},
			Meta:          map[string]interface{}{"codex": map[string]interface{}{"phase": "final_answer"}},
		},
		{
			SessionUpdate: "agent_message_chunk",
			MessageID:     "final-1",
			Content:       acpContent{Type: "text", Text: "Jose"},
			Contents:      []acpContent{{Type: "text", Text: "Jose"}},
			Meta:          map[string]interface{}{"codex": map[string]interface{}{"phase": "final_answer"}},
		},
		{
			SessionUpdate: "agent_message_chunk",
			Content:       acpContent{Type: "text", Text: "Warning: disk full"},
			Contents:      []acpContent{{Type: "text", Text: "Warning: disk full"}},
		},
	}
	for _, update := range updates {
		obs.OnUpdate(acpSessionNotification{SessionID: "remote-123", Update: update})
	}

	if got := obs.GetContent(); got != "Ciao Jose" {
		t.Fatalf("expected explicit final only, got %q", got)
	}
	blocks := obs.ContentBlocks()
	if len(blocks) != 2 || blocks[0].Text != "Ciao " || blocks[1].Text != "Jose" {
		t.Fatalf("expected final content blocks only, got %#v", blocks)
	}
	if got := obs.RawContent(); got != "Leggo il contesto. Ciao JoseWarning: disk full" {
		t.Fatalf("raw ACP evidence lost, got %q", got)
	}
	if len(notifier.updates) != len(updates) {
		t.Fatalf("expected all updates to be forwarded, got %#v", notifier.updates)
	}
	if notifier.updates[0].Metadata["message_id"] != "commentary-1" || notifier.updates[0].Metadata["message_phase"] != "commentary" {
		t.Fatalf("commentary identity lost: %#v", notifier.updates[0].Metadata)
	}
	if notifier.updates[1].Metadata["message_classification"] != "final" {
		t.Fatalf("final phase not classified: %#v", notifier.updates[1].Metadata)
	}
	if notifier.updates[3].Metadata["message_classification"] != "unclassified" {
		t.Fatalf("unphased diagnostic must remain explicit: %#v", notifier.updates[3].Metadata)
	}
}

func TestObserverFallsBackToUnclassifiedACPMessage(t *testing.T) {
	obs := &simpleObserver{}
	for _, text := range []string{"con", "testo"} {
		obs.OnUpdate(acpSessionNotification{
			Update: zedacp.SessionUpdate{
				SessionUpdate: "agent_message_chunk",
				Content:       acpContent{Type: "text", Text: text},
			},
		})
	}
	if got := obs.GetContent(); got != "contesto" {
		t.Fatalf("unclassified ACP fallback changed chunk boundaries: %q", got)
	}
}

func TestObserverPreservesToolContentAndPlanMetadata(t *testing.T) {
	notifier := &observerTestNotifier{}
	obs := &simpleObserver{notifier: notifier}
	oldText := "old"

	obs.OnUpdate(acpSessionNotification{
		SessionID: "remote-123",
		Update: zedacp.SessionUpdate{
			SessionUpdate: "tool_call_update",
			ToolCallID:    "tool-123",
			Kind:          "edit",
			Status:        "completed",
			ToolContents: []zedacp.ToolCallContent{
				{Type: "content", Content: &zedacp.Content{Type: "text", Text: "done"}},
				{Type: "diff", Path: "/tmp/matrix.go", OldText: &oldText, NewText: "new"},
				{Type: "terminal", TerminalID: "terminal-1"},
			},
		},
	})
	obs.OnUpdate(acpSessionNotification{
		SessionID: "remote-123",
		Update: zedacp.SessionUpdate{
			SessionUpdate: "plan",
			Entries:       []zedacp.PlanEntry{{Content: "verify", Priority: "high", Status: "pending"}},
		},
	})

	if len(notifier.updates) != 2 {
		t.Fatalf("expected two forwarded updates, got %#v", notifier.updates)
	}
	toolMeta := notifier.updates[0].Metadata
	if toolMeta["path"] != "/tmp/matrix.go" || toolMeta["terminal_id"] != "terminal-1" {
		t.Fatalf("expected tool content projection, got %#v", toolMeta)
	}
	if notifier.updates[0].Content != "done" {
		t.Fatalf("expected nested content text, got %q", notifier.updates[0].Content)
	}
	planMeta := notifier.updates[1].Metadata
	if planMeta["source_update_type"] != "plan" || planMeta["plan_entries"] == nil {
		t.Fatalf("expected plan metadata, got %#v", planMeta)
	}
}
