package zedacp

import (
	"encoding/json"
	"testing"
)

// TestSessionSurfaceResolvesSystemPromptCapability pins the stable ACP v1
// system-prompt capability added in schema-v1.30.0: the presence of the
// sessionCapabilities.systemPrompt group advertises append mode, and a nested
// mode.override object advertises replacement mode. Version 2 nests the same
// group under capabilities.session. An agent that advertises neither must not be
// sent a system prompt, which is why this is resolved instead of guessed.
func TestSessionSurfaceResolvesSystemPromptCapability(t *testing.T) {
	appendOnly := map[string]interface{}{"systemPrompt": map[string]interface{}{}}
	withOverride := map[string]interface{}{
		"systemPrompt": map[string]interface{}{
			"mode": map[string]interface{}{"override": map[string]interface{}{}},
		},
	}
	cases := []struct {
		name string
		resp *InitializeResponse
		want bool
		over bool
	}{
		{"version 1 without the capability", &InitializeResponse{ProtocolVersion: ProtocolVersionV1, Capabilities: map[string]interface{}{}}, false, false},
		{"version 1 append only", &InitializeResponse{ProtocolVersion: ProtocolVersionV1, Capabilities: map[string]interface{}{"sessionCapabilities": appendOnly}}, true, false},
		{"version 1 append and override", &InitializeResponse{ProtocolVersion: ProtocolVersionV1, Capabilities: map[string]interface{}{"sessionCapabilities": withOverride}}, true, true},
		{"version 2 append only", &InitializeResponse{ProtocolVersion: ProtocolVersionV2, Capabilities: map[string]interface{}{"session": appendOnly}}, true, false},
		{"version 2 append and override", &InitializeResponse{ProtocolVersion: ProtocolVersionV2, Capabilities: map[string]interface{}{"session": withOverride}}, true, true},
		{
			name: "version 1 does not read the version 2 spelling",
			resp: &InitializeResponse{ProtocolVersion: ProtocolVersionV1, Capabilities: map[string]interface{}{"session": withOverride}},
		},
		{
			name: "a non-object marker is not support",
			resp: &InitializeResponse{ProtocolVersion: ProtocolVersionV1, Capabilities: map[string]interface{}{"sessionCapabilities": map[string]interface{}{"systemPrompt": true}}},
		},
		{
			name: "a malformed mode is append only",
			resp: &InitializeResponse{ProtocolVersion: ProtocolVersionV1, Capabilities: map[string]interface{}{"sessionCapabilities": map[string]interface{}{"systemPrompt": map[string]interface{}{"mode": "override"}}}},
			want: true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := tc.resp.SessionSurface()
			if got.SystemPrompt != tc.want || got.SystemPromptOverride != tc.over {
				t.Fatalf("SessionSurface() system prompt = (%t, %t), want (%t, %t)", got.SystemPrompt, got.SystemPromptOverride, tc.want, tc.over)
			}
		})
	}
}

// TestSystemPromptRequestsMarshalTheStableField pins that every stable request
// that accepts a client system prompt carries it under the wire name
// "systemPrompt", and that an absent prompt stays absent instead of becoming
// null or an empty object.
func TestSystemPromptRequestsMarshalTheStableField(t *testing.T) {
	prompt := &SystemPrompt{
		Mode:    SystemPromptModeOverride,
		Content: []Content{{Type: "text", Text: "answer in Italian"}},
	}
	requests := map[string]interface{}{
		"new":    NewSessionRequest{Cwd: "/workspace", SystemPrompt: prompt},
		"load":   LoadSessionRequest{SessionID: "s", Cwd: "/workspace", SystemPrompt: prompt},
		"resume": ResumeSessionRequest{SessionID: "s", Cwd: "/workspace", SystemPrompt: prompt},
	}
	for name, request := range requests {
		t.Run(name, func(t *testing.T) {
			encoded, err := json.Marshal(request)
			if err != nil {
				t.Fatalf("marshal: %v", err)
			}
			var fields map[string]json.RawMessage
			if err := json.Unmarshal(encoded, &fields); err != nil {
				t.Fatalf("decode %s: %v", encoded, err)
			}
			var decoded SystemPrompt
			if err := json.Unmarshal(fields["systemPrompt"], &decoded); err != nil {
				t.Fatalf("systemPrompt in %s: %v", encoded, err)
			}
			if decoded.Mode != SystemPromptModeOverride || len(decoded.Content) != 1 || decoded.Content[0].Text != "answer in Italian" {
				t.Fatalf("systemPrompt round-trip = %+v in %s", decoded, encoded)
			}
		})
	}

	encoded, err := json.Marshal(NewSessionRequest{Cwd: "/workspace"})
	if err != nil {
		t.Fatalf("marshal without prompt: %v", err)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(encoded, &fields); err != nil {
		t.Fatalf("decode without prompt: %v", err)
	}
	if _, ok := fields["systemPrompt"]; ok {
		t.Fatalf("an absent system prompt must stay absent, got %s", encoded)
	}
}
