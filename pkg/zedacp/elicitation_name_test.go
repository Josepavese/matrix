package zedacp

import (
	"encoding/json"
	"testing"
)

// TestSessionUpdateToolCallNameRoundTrip covers the stable ACP v1 tool-call
// `name` field (stabilized upstream 2026-09-17). It must survive decode and
// re-encode without polluting other update types.
func TestSessionUpdateToolCallNameRoundTrip(t *testing.T) {
	raw := `{
		"sessionUpdate": "tool_call",
		"toolCallId": "call_001",
		"name": "read_file",
		"title": "Reading configuration file",
		"kind": "read",
		"status": "pending"
	}`
	var update SessionUpdate
	if err := json.Unmarshal([]byte(raw), &update); err != nil {
		t.Fatalf("unmarshal tool_call update: %v", err)
	}
	if update.Name != "read_file" {
		t.Fatalf("expected decoded name %q, got %q", "read_file", update.Name)
	}
	if update.Title != "Reading configuration file" {
		t.Fatalf("expected title preserved, got %q", update.Title)
	}
	encoded, err := json.Marshal(update)
	if err != nil {
		t.Fatalf("marshal tool_call update: %v", err)
	}
	var re map[string]interface{}
	if err := json.Unmarshal(encoded, &re); err != nil {
		t.Fatalf("unmarshal re-encoded update: %v", err)
	}
	if re["name"] != "read_file" {
		t.Fatalf("expected re-encoded name, got %v", re["name"])
	}
	// Updates without a name must not emit an empty field.
	var plain SessionUpdate
	if err := json.Unmarshal([]byte(`{"sessionUpdate":"user_message_chunk","content":{"type":"text","text":"hi"}}`), &plain); err != nil {
		t.Fatalf("unmarshal plain update: %v", err)
	}
	plainEncoded, err := json.Marshal(plain)
	if err != nil {
		t.Fatalf("marshal plain update: %v", err)
	}
	if string(plainEncoded) == "" || containsEmptyNameField(string(plainEncoded)) {
		t.Fatalf("plain update must not encode an empty name field: %s", plainEncoded)
	}
}

func containsEmptyNameField(s string) bool {
	return len(s) > 0 && (s == `"name":""` || stringContains(s, `"name":"",`))
}

func stringContains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
