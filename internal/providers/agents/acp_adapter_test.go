package agents

import (
	"testing"

	"github.com/Josepavese/matrix/internal/middleware"
)

func TestSupportsSessionCapabilityAcceptsZedObjectStyle(t *testing.T) {
	resp := &acpInitializeResponse{
		Capabilities: map[string]interface{}{
			"sessionCapabilities": map[string]interface{}{
				"list":   map[string]interface{}{},
				"close":  map[string]interface{}{},
				"delete": map[string]interface{}{},
			},
		},
	}

	if !supportsSessionCapability(resp, "list") {
		t.Fatalf("expected object-style list capability")
	}
	if !supportsSessionCapability(resp, "close") {
		t.Fatalf("expected object-style close capability")
	}
	if !supportsSessionCapability(resp, "delete") {
		t.Fatalf("expected object-style delete capability")
	}
}

func TestSupportsSessionCapabilityAcceptsLegacyBooleanTrueOnly(t *testing.T) {
	resp := &acpInitializeResponse{
		Capabilities: map[string]interface{}{
			"sessionCapabilities": map[string]interface{}{
				"list":   true,
				"close":  false,
				"delete": nil,
			},
		},
	}

	if !supportsSessionCapability(resp, "list") {
		t.Fatalf("expected boolean true list capability")
	}
	if supportsSessionCapability(resp, "close") {
		t.Fatalf("did not expect boolean false close capability")
	}
	if supportsSessionCapability(resp, "delete") {
		t.Fatalf("did not expect nil delete capability")
	}
	if supportsSessionCapability(resp, "fork") {
		t.Fatalf("did not expect absent fork capability")
	}
}

func TestACPSessionCapabilitiesExposeLifecycleStability(t *testing.T) {
	resp := &acpInitializeResponse{
		Capabilities: map[string]interface{}{
			"loadSession": true,
			"sessionCapabilities": map[string]interface{}{
				"list":   map[string]interface{}{},
				"resume": map[string]interface{}{},
				"fork":   map[string]interface{}{},
			},
		},
	}
	caps := acpSessionCapabilities(resp)
	if !caps.List || !caps.Load || !caps.Cancel || !caps.Resume || !caps.Fork {
		t.Fatalf("expected advertised lifecycle support: %#v", caps)
	}
	if caps.Details["list"].Stability != "stable" {
		t.Fatalf("list should be stable: %#v", caps.Details["list"])
	}
	if caps.Details["resume"].Stability != "stable" {
		t.Fatalf("resume should be stable: %#v", caps.Details["resume"])
	}
	if caps.Details["fork"].Stability != "draft" {
		t.Fatalf("fork should be draft: %#v", caps.Details["fork"])
	}
	if caps.Details["fork"].AsyncSupported == nil || !*caps.Details["fork"].AsyncSupported {
		t.Fatalf("fork should advertise Matrix async polling support: %#v", caps.Details["fork"])
	}
	if caps.Details["fork"].LiveInterventionSuitable == nil || *caps.Details["fork"].LiveInterventionSuitable {
		t.Fatalf("fork should not claim timely live intervention suitability: %#v", caps.Details["fork"])
	}
}

func TestPickAutoApproveConfigOptionPrefersStableConfigSurface(t *testing.T) {
	resp := &middleware.NewSessionResponse{
		ConfigOptions: []middleware.ConfigOption{
			{
				ID:       "mode",
				Category: "mode",
				Options: []middleware.ConfigOptionValue{
					{ID: "ask", Name: "Ask"},
					{ID: "build", Name: "Build"},
				},
			},
		},
	}
	configID, value := pickAutoApproveConfigOption(resp)
	if configID != "mode" || value != "build" {
		t.Fatalf("unexpected config selection: %s=%s", configID, value)
	}
	if modeID := pickAutoApproveMode(resp); modeID != "" {
		t.Fatalf("legacy mode picker must not consume config options, got %q", modeID)
	}
}
