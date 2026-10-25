package providerfailure

import (
	"net/http"
	"testing"
)

// TestProviderToolPermissionDeniedStaysAGatewayFailure pins the public code and
// the HTTP contract: a provider abort after a denied tool permission is a
// runtime failure, reported as 502 Bad Gateway like the other provider runtime
// failures, not as a dependency failure the caller can fix by changing config.
func TestProviderToolPermissionDeniedStaysAGatewayFailure(t *testing.T) {
	if ProviderToolPermissionDenied != "provider_tool_permission_denied" {
		t.Fatalf("public code changed: %q", ProviderToolPermissionDenied)
	}
	failure := &Failure{Code: ProviderToolPermissionDenied, Phase: "session/prompt"}
	status, typed := HTTPStatus(failure)
	if !typed || status != http.StatusBadGateway {
		t.Fatalf("HTTPStatus = (%d, %v), want (502, true)", status, typed)
	}
	if kind := failure.EventKind(); kind != "provider.runtime.failed" {
		t.Fatalf("EventKind = %q, want a runtime failure", kind)
	}
	details := Details(failure)
	if details["phase"] != "session/prompt" {
		t.Fatalf("details lost the phase: %#v", details)
	}
}
