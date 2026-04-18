package agents

import "testing"

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
