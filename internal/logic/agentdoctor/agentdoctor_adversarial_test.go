package agentdoctor

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/Josepavese/matrix/internal/middleware"
)

// TestEndpointAddressPicksTheAddressableTarget pins the rule the doctor uses to
// report where an agent lives: a stdio ACP agent is addressed by its command,
// everything else by its address.
func TestEndpointAddressPicksTheAddressableTarget(t *testing.T) {
	stdio := middleware.ProtocolEndpoint{Kind: middleware.ProtocolKindACP, Transport: "stdio", Command: "/usr/bin/codex", Address: "ignored"}
	if got := EndpointAddress(stdio); got != "/usr/bin/codex" {
		t.Fatalf("a stdio ACP agent is addressed by its command, got %q", got)
	}
	httpEndpoint := middleware.ProtocolEndpoint{Kind: middleware.ProtocolKindACP, Transport: "http", Command: "/usr/bin/codex", Address: "http://127.0.0.1:8080"}
	if got := EndpointAddress(httpEndpoint); got != "http://127.0.0.1:8080" {
		t.Fatalf("an http agent is addressed by its address, got %q", got)
	}
	a2a := middleware.ProtocolEndpoint{Kind: middleware.ProtocolKindA2A, Transport: "stdio", Command: "/usr/bin/agent", Address: "http://127.0.0.1:9000"}
	if got := EndpointAddress(a2a); got != "http://127.0.0.1:9000" {
		t.Fatalf("a non-ACP endpoint is addressed by its address, got %q", got)
	}
}

// TestProbeHandshakeReportsBothOutcomes keeps a failed handshake from being
// reported as ready: the doctor's advice is load-bearing for operators.
func TestProbeHandshakeReportsBothOutcomes(t *testing.T) {
	ready := ProbeHandshake(middleware.ProtocolEndpoint{}, func(context.Context, middleware.ProtocolEndpoint) error { return nil })
	if ready["provider_handshake_ok"] != true || ready["provider_status"] != "ready_on_demand" {
		t.Fatalf("a successful handshake must report ready, got %v", ready)
	}

	failed := ProbeHandshake(middleware.ProtocolEndpoint{}, func(context.Context, middleware.ProtocolEndpoint) error {
		return errors.New("initialize rejected")
	})
	if failed["provider_handshake_ok"] != false {
		t.Fatalf("a failed handshake must not report ok, got %v", failed)
	}
	if failed["provider_status"] == "ready_on_demand" {
		t.Fatalf("a failed handshake must not report ready: %v", failed)
	}
	if failed["provider_handshake_error"] == nil {
		t.Fatalf("a failed handshake must carry the reason, got %v", failed)
	}
}

// TestInspectACPIgnoresNonStdioEndpoints keeps the ACP inspector from probing a
// transport it does not own.
func TestInspectACPIgnoresNonStdioEndpoints(t *testing.T) {
	cases := map[string]middleware.ProtocolEndpoint{
		"http ACP":   {Kind: middleware.ProtocolKindACP, Transport: "http", Command: "/usr/bin/codex"},
		"stdio A2A":  {Kind: middleware.ProtocolKindA2A, Transport: "stdio", Command: "/usr/bin/agent"},
		"no command": {Kind: middleware.ProtocolKindACP, Transport: "stdio"},
		"empty":      {},
	}
	for name, endpoint := range cases {
		t.Run(name, func(t *testing.T) {
			result, warnings := InspectACP(endpoint, func(context.Context, middleware.ProtocolEndpoint) error {
				t.Fatal("an endpoint the inspector does not own must not be probed")
				return nil
			})
			if len(result) != 0 || len(warnings) != 0 {
				t.Fatalf("expected no report, got result=%v warnings=%v", result, warnings)
			}
		})
	}
}

// TestInspectACPWarnsWhenTheProviderCannotBeReached checks that an unreachable
// agent produces an explicit warning rather than a silent empty report.
func TestInspectACPWarnsWhenTheProviderCannotBeReached(t *testing.T) {
	endpoint := middleware.ProtocolEndpoint{
		Kind:      middleware.ProtocolKindACP,
		Transport: "stdio",
		Command:   "/nonexistent/matrix-doctor-probe",
	}
	result, warnings := InspectACP(endpoint, func(context.Context, middleware.ProtocolEndpoint) error {
		return errors.New("spawn failed")
	})
	if _, ok := result["command_probe_ok"]; !ok {
		t.Fatalf("the report must record the command probe, got %v", result)
	}
	if len(warnings) == 0 {
		t.Fatalf("an unreachable provider must warn, got result=%v", result)
	}
	joined := ""
	for _, warning := range warnings {
		joined += warning + "\n"
	}
	if result["provider_handshake_ok"] == true {
		t.Fatal("a failed handshake must not be reported as ok")
	}
	if !strings.Contains(joined, "handshake") {
		t.Fatalf("the warnings must name the failed handshake, got %q", joined)
	}
}
