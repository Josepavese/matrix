package agents

import (
	"testing"

	"github.com/jose/matrix-v2/pkg/zedacp"
)

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
