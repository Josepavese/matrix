package agents

import (
	"encoding/json"
	"testing"

	"github.com/Josepavese/matrix/internal/logic/elicitation"
	"github.com/Josepavese/matrix/internal/middleware"
)

// TestElicitationAdvertisementMatchesCapabilityReport pins the invariant that
// runtime reporting and the wire advertisement cannot diverge: what the
// capability report claims must be exactly what initialize told the agent.
// A report that overstates support makes callers send requests the adapter
// would refuse; one that understates it hides a working surface.
func TestElicitationAdvertisementMatchesCapabilityReport(t *testing.T) {
	cases := []struct {
		name           string
		frontend       middleware.ElicitationFrontend
		wantAdvertised bool
		wantReported   bool
		wantFormOnWire bool
		wantURLOnWire  bool
	}{
		{name: "no frontend", frontend: nil, wantAdvertised: false, wantReported: false},
		{
			name:           "form only",
			frontend:       &staticFrontend{modes: []string{middleware.ElicitationModeForm}},
			wantAdvertised: true, wantReported: true, wantFormOnWire: true, wantURLOnWire: false,
		},
		{
			name:           "form and url",
			frontend:       &staticFrontend{modes: []string{middleware.ElicitationModeForm, middleware.ElicitationModeURL}},
			wantAdvertised: true, wantReported: true, wantFormOnWire: true, wantURLOnWire: true,
		},
		{
			name:           "frontend with no usable mode",
			frontend:       &staticFrontend{modes: nil},
			wantAdvertised: false, wantReported: false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			deps := middleware.ConversationFactoryDeps{AgentID: "codex", ElicitationFrontend: tc.frontend}
			caps := acpClientCapabilitiesForDeps(deps)
			advertised := caps.Elicitation != nil
			if advertised != tc.wantAdvertised {
				t.Fatalf("advertised=%v, expected %v", advertised, tc.wantAdvertised)
			}
			if advertised {
				if (caps.Elicitation.Form != nil) != tc.wantFormOnWire {
					t.Fatalf("form advertised wrong: %+v", caps.Elicitation)
				}
				if (caps.Elicitation.URL != nil) != tc.wantURLOnWire {
					t.Fatalf("url advertised wrong: %+v", caps.Elicitation)
				}
			}

			handler := NewDefaultRequestHandler(nil)
			if tc.frontend != nil {
				handler.WithElicitationFrontend(tc.frontend).WithAgentIdentity("codex")
			}
			client := &acpConversationClient{handler: handler, endpoint: middleware.ProtocolEndpoint{Transport: "stdio"}}
			report := client.ProtocolCapabilities()
			reported, ok := report.Operations["elicitation/create"]
			if !ok {
				t.Fatal("capability report must always describe elicitation/create")
			}
			if reported.Supported != tc.wantReported {
				t.Fatalf("report says supported=%v, expected %v", reported.Supported, tc.wantReported)
			}
			if reported.Supported != advertised {
				t.Fatalf("report and wire advertisement diverge: report=%v wire=%v", reported.Supported, advertised)
			}
		})
	}
}

// TestCapabilityReportDoesNotPanicWithoutHandler guards the constructor paths
// used by tests and by partially initialised clients.
func TestCapabilityReportDoesNotPanicWithoutHandler(t *testing.T) {
	client := &acpConversationClient{endpoint: middleware.ProtocolEndpoint{Transport: "stdio"}}
	report := client.ProtocolCapabilities()
	if report.Operations["elicitation/create"].Supported {
		t.Fatal("a client without a handler must not claim elicitation support")
	}
}

// TestRouterElicitationFrontendIsWiredIntoDeps proves the production path: the
// frontend set on the router is the one the adapter receives, so enabling the
// surface cannot silently miss the ACP layer.
func TestRouterElicitationFrontendIsWiredIntoDeps(t *testing.T) {
	router := NewRouter(nil)
	if router.elicitation != nil {
		t.Fatal("a fresh router must not enable elicitation")
	}
	router.SetElicitationFrontend(nil)
	if router.elicitation != nil {
		t.Fatal("clearing the frontend must keep the surface disabled")
	}
	service := elicitation.NewService(0)
	router.SetElicitationFrontend(service)
	if router.elicitation == nil {
		t.Fatal("the wired frontend must be visible to the adapter")
	}
	// The adapter's own advertisement must then be enabled.
	if caps := acpClientCapabilitiesForDeps(middleware.ConversationFactoryDeps{
		AgentID: "codex", ElicitationFrontend: router.elicitation,
	}); caps.Elicitation == nil {
		t.Fatal("a wired router frontend must produce an advertised capability")
	}
	encoded, err := json.Marshal(acpClientCapabilitiesForDeps(middleware.ConversationFactoryDeps{
		AgentID: "codex", ElicitationFrontend: router.elicitation,
	}))
	if err != nil {
		t.Fatal(err)
	}
	if len(encoded) == 0 {
		t.Fatal("capabilities must marshal")
	}
}
