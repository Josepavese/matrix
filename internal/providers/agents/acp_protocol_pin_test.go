package agents

import (
	"context"
	"strings"
	"testing"

	"github.com/Josepavese/matrix/internal/middleware"
	"github.com/Josepavese/matrix/pkg/zedacp"
)

func TestACPInitializeRequestHonoursTheConfiguredPin(t *testing.T) {
	cases := []struct {
		pin  string
		want int
	}{
		{"", 0},
		{"1", zedacp.ProtocolVersionV1},
		{"2", zedacp.ProtocolVersionV2},
	}
	for _, tc := range cases {
		req, err := acpInitializeRequestFor(middleware.ConversationFactoryDeps{ProtocolVersion: tc.pin})
		if err != nil {
			t.Fatalf("pin %q: %v", tc.pin, err)
		}
		if req.ProtocolVersion != tc.want {
			t.Fatalf("pin %q requested version %d, want %d", tc.pin, req.ProtocolVersion, tc.want)
		}
	}
	if _, err := acpInitializeRequestFor(middleware.ConversationFactoryDeps{ProtocolVersion: "3"}); err == nil {
		t.Fatal("an invalid pin must fail closed")
	}
}

func TestACPInitializeRejectsAV2PeerSendingTheV1Payload(t *testing.T) {
	fake := &pagedListACPClient{
		ctx:             context.Background(),
		protocolVersion: zedacp.ProtocolVersionV2,
		initializeResponse: &acpInitializeResponse{
			ProtocolVersion:     zedacp.ProtocolVersionV2,
			V2PayloadConforming: false,
		},
	}
	_, err := initializeACPConnection(context.Background(), fake, middleware.ConversationFactoryDeps{})
	if err == nil || !strings.Contains(err.Error(), "non-conforming") {
		t.Fatalf("a v2 declaration with the v1 payload must fail explicitly, got %v", err)
	}

	fake.initializeResponse.V2PayloadConforming = true
	if _, err := initializeACPConnection(context.Background(), fake, middleware.ConversationFactoryDeps{}); err != nil {
		t.Fatalf("a conforming v2 reply must connect: %v", err)
	}
}

func TestACPCapabilitiesReportTheNegotiatedGeneration(t *testing.T) {
	v1 := &acpConversationClient{
		client:   newScriptedAuthACPClient(zedacp.ProtocolVersionV1),
		endpoint: middleware.ProtocolEndpoint{Transport: "stdio"},
	}
	v1Report := v1.ProtocolCapabilities()
	if v1Report.ProtocolVersion != "1" {
		t.Fatalf("v1 report version = %q", v1Report.ProtocolVersion)
	}
	if v1Report.Operations["session/prompt"].Stability != "stable" {
		t.Fatalf("v1 operations must stay stable: %#v", v1Report.Operations["session/prompt"])
	}

	v2 := &acpConversationClient{
		client:   newScriptedAuthACPClient(zedacp.ProtocolVersionV2),
		endpoint: middleware.ProtocolEndpoint{Transport: "stdio"},
	}
	v2Report := v2.ProtocolCapabilities()
	if v2Report.ProtocolVersion != "2" {
		t.Fatalf("v2 report version = %q", v2Report.ProtocolVersion)
	}
	prompt := v2Report.Operations["session/prompt"]
	if prompt.Stability != "draft" || !strings.Contains(prompt.Source, "ACP v2 draft") {
		t.Fatalf("v2 operations must be reported as draft: %#v", prompt)
	}
	if v2Report.Content["text"].Stability != "draft" {
		t.Fatalf("v2 content must be reported as draft: %#v", v2Report.Content["text"])
	}
}
