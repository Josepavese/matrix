package providerfailure

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/Josepavese/matrix/internal/middleware"
)

// TestHTTPStatusMapsKnownCodes pins the contract the HTTP surface depends on:
// an unavailable model and an auth mismatch are dependency failures the caller
// can act on, anything else is a bad gateway, and a plain error is a server bug.
func TestHTTPStatusMapsKnownCodes(t *testing.T) {
	cases := []struct {
		name      string
		err       error
		want      int
		wantTyped bool
	}{
		{"model unavailable", &Failure{Code: ModelUnavailable}, http.StatusFailedDependency, true},
		{"auth mismatch", &Failure{Code: AuthMismatch}, http.StatusFailedDependency, true},
		{"preflight failed", &Failure{Code: PreflightFailed}, http.StatusBadGateway, true},
		{"unknown code", &Failure{Code: "something_else"}, http.StatusBadGateway, true},
		{"plain error", errors.New("boom"), http.StatusInternalServerError, false},
		{"nil error", nil, http.StatusInternalServerError, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			status, typed := HTTPStatus(tc.err)
			if status != tc.want || typed != tc.wantTyped {
				t.Fatalf("HTTPStatus = (%d, %v), want (%d, %v)", status, typed, tc.want, tc.wantTyped)
			}
		})
	}
}

// TestAsFindsAWrappedFailure matters because provider failures travel through
// several layers of wrapping before the HTTP surface sees them.
func TestAsFindsAWrappedFailure(t *testing.T) {
	inner := &Failure{Code: ModelUnavailable, Message: "no such model"}
	wrapped := fmt.Errorf("route failed: %w", inner)

	found, ok := As(wrapped)
	if !ok || found != inner {
		t.Fatal("a wrapped provider failure must be recoverable")
	}
	if _, ok := As(errors.New("plain")); ok {
		t.Fatal("a plain error must not be reported as a provider failure")
	}
}

func TestErrorStringCarriesContextAndCause(t *testing.T) {
	failure := &Failure{
		Code:           ModelUnavailable,
		Message:        "model not available",
		AgentID:        "codex",
		Protocol:       "acp",
		Phase:          "initialize",
		RequestedModel: "gpt-5",
		Err:            errors.New("upstream said no"),
	}
	got := failure.Error()
	for _, fragment := range []string{ModelUnavailable, "model not available", "agent=codex", "protocol=acp", "phase=initialize", "requested_model=gpt-5", "upstream said no"} {
		if !strings.Contains(got, fragment) {
			t.Fatalf("error string %q is missing %q", got, fragment)
		}
	}
	// A failure with no optional fields must not render empty decorations.
	bare := (&Failure{Code: "c", Message: "m"}).Error()
	if bare != "[c] m" {
		t.Fatalf("bare error string = %q, want %q", bare, "[c] m")
	}
}

// TestUnwrapOnNilReceiver keeps the nil case from panicking: Errors.Is walks
// through Unwrap even for a typed nil.
func TestUnwrapOnNilReceiver(t *testing.T) {
	var failure *Failure
	if err := failure.Unwrap(); err != nil {
		t.Fatalf("nil receiver must unwrap to nil, got %v", err)
	}
}

func TestDetailsReportsOnlyPresentFields(t *testing.T) {
	details := Details(&Failure{
		AgentID:     "codex",
		Phase:       "initialize",
		Diagnostics: map[string]string{"extra": "value", "blank": ""},
	})
	if details["agent_id"] != "codex" || details["phase"] != "initialize" {
		t.Fatalf("details lost populated fields: %v", details)
	}
	if _, ok := details["protocol"]; ok {
		t.Fatal("details must not report fields that were never set")
	}
	if _, ok := details["blank"]; ok {
		t.Fatal("details must not report empty diagnostics")
	}
	if details["extra"] != "value" {
		t.Fatal("details dropped a diagnostic")
	}
	if Details(nil) != nil {
		t.Fatal("nil failure must produce no details")
	}
}

// TestNewPreflightIsIdempotentAndPreservesTypedFailures stops a re-classified
// error from being double wrapped, which would hide the original code.
func TestNewPreflightIsIdempotentAndPreservesTypedFailures(t *testing.T) {
	if got := NewPreflight("codex", middleware.ProtocolEndpoint{}, "initialize", nil); got != nil {
		t.Fatalf("a nil error must stay nil, got %v", got)
	}
	original := &Failure{Code: ModelUnavailable, Message: "already typed"}
	if got := NewPreflight("codex", middleware.ProtocolEndpoint{}, "initialize", original); got != original {
		t.Fatal("an already typed failure must be returned unchanged")
	}
	plain := errors.New("spawn failed")
	wrapped := NewPreflight("codex", middleware.ProtocolEndpoint{Transport: "stdio", Command: "/usr/bin/codex"}, "launch", plain)
	failure, ok := As(wrapped)
	if !ok {
		t.Fatalf("a plain error must be classified, got %v", wrapped)
	}
	if failure.Code != PreflightFailed || failure.AgentID != "codex" || failure.Phase != "launch" {
		t.Fatalf("unexpected classification: %+v", failure)
	}
	if !errors.Is(wrapped, plain) {
		t.Fatal("classification must preserve the original cause")
	}
	if failure.Diagnostics["command"] != "/usr/bin/codex" || failure.Diagnostics["transport"] != "stdio" {
		t.Fatalf("diagnostics lost the launch evidence: %v", failure.Diagnostics)
	}
}

func TestDiagnosticsIsBoundedAndDescribesTheEndpoint(t *testing.T) {
	diagnostics := Diagnostics(middleware.ProtocolEndpoint{
		Kind:            "acp",
		Transport:       "http",
		Address:         "http://127.0.0.1:8080",
		ProtocolVersion: "1.9.1",
	}, errors.New("connection refused"))
	if diagnostics["transport"] != "http" || diagnostics["address"] != "http://127.0.0.1:8080" || diagnostics["protocol_version"] != "1.9.1" {
		t.Fatalf("diagnostics lost endpoint evidence: %v", diagnostics)
	}
	if diagnostics["provider_error"] != "connection refused" {
		t.Fatalf("diagnostics lost the provider error: %v", diagnostics)
	}
	if _, ok := diagnostics["command"]; ok {
		t.Fatal("a command must not be reported when the endpoint has none")
	}
}
