package zedacp

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestConfiguredProtocolVersionPinsAndFailsClosed(t *testing.T) {
	cases := []struct {
		raw     string
		want    int
		wantErr bool
	}{
		{"", 0, false},
		{"1", ProtocolVersionV1, false},
		{"1.0", ProtocolVersionV1, false},
		{"2", ProtocolVersionV2, false},
		{"2.0", ProtocolVersionV2, false},
		{"0", 0, true},
		{"3", 0, true},
		{"latest", 0, true},
	}
	for _, tc := range cases {
		got, err := ConfiguredProtocolVersion(tc.raw)
		if (err != nil) != tc.wantErr || got != tc.want {
			t.Fatalf("ConfiguredProtocolVersion(%q) = (%d, %v), want (%d, err=%v)", tc.raw, got, err, tc.want, tc.wantErr)
		}
	}
}

func TestValidateInitializeResponseRejectsNonConformingV2Payload(t *testing.T) {
	conforming := &InitializeResponse{
		ProtocolVersion:     ProtocolVersionV2,
		V2PayloadConforming: true,
	}
	if err := ValidateInitializeResponse(conforming, ProtocolVersionV2); err != nil {
		t.Fatalf("a conforming v2 reply must pass: %v", err)
	}
	var decoded InitializeResponse
	if err := json.Unmarshal([]byte(`{"protocolVersion":2,"agentCapabilities":{"terminal":true},"agentInfo":{"name":"echo-v1"}}`), &decoded); err != nil {
		t.Fatalf("decode the non-conforming reply: %v", err)
	}
	if decoded.V2PayloadConforming {
		t.Fatal("a v2 reply using the v1 parameter names must be marked non-conforming")
	}
	err := ValidateInitializeResponse(&decoded, ProtocolVersionV2)
	if err == nil || !strings.Contains(err.Error(), "non-conforming") {
		t.Fatalf("a non-conforming v2 reply must fail explicitly, got %v", err)
	}
	if err := ValidateInitializeResponse(&decoded, ProtocolVersionV1); err != nil {
		t.Fatalf("the version check must be generation-specific: %v", err)
	}
}

func TestInitializeResponseMarksConformingV2Payload(t *testing.T) {
	var resp InitializeResponse
	if err := json.Unmarshal([]byte(`{"protocolVersion":2,"capabilities":{},"info":{"name":"peer"}}`), &resp); err != nil {
		t.Fatalf("decode conforming v2 reply: %v", err)
	}
	if !resp.V2PayloadConforming {
		t.Fatal("a v2 reply with info and capabilities objects must be conforming")
	}
	var missing InitializeResponse
	if err := json.Unmarshal([]byte(`{"protocolVersion":2,"capabilities":{}}`), &missing); err != nil {
		t.Fatalf("decode v2 reply without info: %v", err)
	}
	if missing.V2PayloadConforming {
		t.Fatal("a v2 reply without info must not be conforming")
	}
}
