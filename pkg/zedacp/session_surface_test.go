package zedacp

import (
	"encoding/json"
	"testing"
)

// TestSessionSurfaceResolvesBothGenerations pins the capability read the adapter
// depends on. Version 2 nests the session markers under capabilities.session and
// makes the baseline methods implicit in that object; version 1 marks each method
// separately and has no session-surface marker at all. Reading only the version 1
// spelling reported a conforming version 2 agent as supporting nothing, so list,
// resume and close were never attempted against it.
func TestSessionSurfaceResolvesBothGenerations(t *testing.T) {
	object := map[string]interface{}{}
	cases := []struct {
		name string
		resp *InitializeResponse
		want SessionSurface
	}{
		{
			name: "version 2 baseline",
			resp: &InitializeResponse{
				ProtocolVersion: ProtocolVersionV2,
				Capabilities:    map[string]interface{}{"session": map[string]interface{}{}},
			},
			want: SessionSurface{Advertised: true, List: true, Resume: true, Close: true},
		},
		{
			name: "version 2 optional markers",
			resp: &InitializeResponse{
				ProtocolVersion: ProtocolVersionV2,
				Capabilities: map[string]interface{}{"session": map[string]interface{}{
					"delete": object, "fork": object, "additionalDirectories": object,
				}},
			},
			want: SessionSurface{Advertised: true, List: true, Resume: true, Close: true, Delete: true, Fork: true, AdditionalDirectories: true},
		},
		{
			name: "version 2 without a session surface",
			resp: &InitializeResponse{ProtocolVersion: ProtocolVersionV2, Capabilities: map[string]interface{}{}},
		},
		{
			name: "version 2 does not read the version 1 spelling",
			resp: &InitializeResponse{
				ProtocolVersion: ProtocolVersionV2,
				Capabilities:    map[string]interface{}{"sessionCapabilities": map[string]interface{}{"list": object}},
			},
		},
		{
			name: "version 1 markers",
			resp: &InitializeResponse{
				ProtocolVersion: ProtocolVersionV1,
				Capabilities: map[string]interface{}{
					"loadSession": true,
					"sessionCapabilities": map[string]interface{}{
						"list": object, "resume": object, "close": object, "delete": object, "fork": object, "additionalDirectories": object,
					},
				},
			},
			want: SessionSurface{Advertised: true, Load: true, List: true, Resume: true, Close: true, Delete: true, Fork: true, AdditionalDirectories: true},
		},
		{
			name: "version 1 always has sessions but no optional markers",
			resp: &InitializeResponse{ProtocolVersion: ProtocolVersionV1, Capabilities: map[string]interface{}{}},
			want: SessionSurface{Advertised: true},
		},
		{
			name: "no response at all",
			resp: nil,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.resp.SessionSurface(); got != tc.want {
				t.Fatalf("SessionSurface() = %+v, want %+v", got, tc.want)
			}
		})
	}
}

// TestInitializeRequestOmitsTheClientSurfacesVersionTwoRemoved: version 2 deleted
// the client file system and terminal execution surfaces, so a version 2 request
// must not advertise them. A version 1 request keeps them, because that
// generation defines them and its agents call them.
func TestInitializeRequestOmitsTheClientSurfacesVersionTwoRemoved(t *testing.T) {
	capabilities := &ClientCapabilities{
		Fs:          &FsCapability{ReadTextFile: true, WriteTextFile: true},
		Terminal:    true,
		Session:     &ClientSessionCapabilities{ConfigOptions: &SessionConfigOptionsCapabilities{Boolean: &BooleanConfigOptionCapabilities{}}},
		Elicitation: &ElicitationCapabilities{Form: &ElicitationModeCapability{}},
		Auth:        &AuthCapabilities{Terminal: &TerminalAuthCapabilities{}},
	}

	req := InitializeRequest{ProtocolVersion: ProtocolVersionV2, ClientInfo: map[string]interface{}{"name": "matrix"}, ClientCapabilities: capabilities}
	v2, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("marshal v2 request: %v", err)
	}
	var decoded struct {
		Capabilities       map[string]json.RawMessage `json:"capabilities"`
		ClientCapabilities map[string]json.RawMessage `json:"clientCapabilities"`
	}
	if err := json.Unmarshal(v2, &decoded); err != nil {
		t.Fatalf("decode v2 request %s: %v", v2, err)
	}
	for _, removed := range []string{"fs", "terminal", "session"} {
		if _, ok := decoded.Capabilities[removed]; ok {
			t.Fatalf("a version 2 request must not advertise the removed surface %q: %s", removed, v2)
		}
	}
	for _, kept := range []string{"elicitation", "auth"} {
		if decoded.Capabilities[kept] == nil {
			t.Fatalf("a version 2 request must keep the shared capability %q: %s", kept, v2)
		}
	}

	req.ProtocolVersion = ProtocolVersionV1
	v1, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("marshal v1 request: %v", err)
	}
	if err := json.Unmarshal(v1, &decoded); err != nil {
		t.Fatalf("decode v1 request %s: %v", v1, err)
	}
	for _, kept := range []string{"fs", "terminal", "session"} {
		if decoded.ClientCapabilities[kept] == nil {
			t.Fatalf("a version 1 request must keep %q: %s", kept, v1)
		}
	}
}

// TestInitializeRequestDoesNotClaimTerminalAuthToAVersionOneAgent: version 1
// defines clientCapabilities.auth.terminal as a boolean, and Matrix runs a
// terminal method only on a version 2 connection, so a version 1 request must
// not advertise the capability at all — the agent would be entitled to offer a
// terminal method this client refuses to run. Version 2, where the capability is
// an object and the flow is implemented, keeps it.
func TestInitializeRequestDoesNotClaimTerminalAuthToAVersionOneAgent(t *testing.T) {
	capabilities := &ClientCapabilities{Auth: &AuthCapabilities{Terminal: &TerminalAuthCapabilities{}}}

	v1, err := json.Marshal(InitializeRequest{
		ProtocolVersion:    ProtocolVersionV1,
		ClientInfo:         map[string]interface{}{"name": "matrix"},
		ClientCapabilities: capabilities,
	})
	if err != nil {
		t.Fatalf("marshal v1 request: %v", err)
	}
	var v1Decoded struct {
		ClientCapabilities map[string]json.RawMessage `json:"clientCapabilities"`
	}
	if err := json.Unmarshal(v1, &v1Decoded); err != nil {
		t.Fatalf("decode v1 request %s: %v", v1, err)
	}
	if raw, ok := v1Decoded.ClientCapabilities["auth"]; ok {
		t.Fatalf("a version 1 request must not claim terminal authentication, got %s: %s", raw, v1)
	}

	v2, err := json.Marshal(InitializeRequest{
		ProtocolVersion:    ProtocolVersionV2,
		ClientInfo:         map[string]interface{}{"name": "matrix"},
		ClientCapabilities: capabilities,
	})
	if err != nil {
		t.Fatalf("marshal v2 request: %v", err)
	}
	var v2Decoded struct {
		Capabilities map[string]json.RawMessage `json:"capabilities"`
	}
	if err := json.Unmarshal(v2, &v2Decoded); err != nil {
		t.Fatalf("decode v2 request %s: %v", v2, err)
	}
	if v2Decoded.Capabilities["auth"] == nil {
		t.Fatalf("a version 2 request must advertise the capability it implements: %s", v2)
	}
}
