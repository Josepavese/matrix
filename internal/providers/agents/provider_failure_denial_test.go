package agents

import (
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/Josepavese/matrix/internal/logic/providerfailure"
	"github.com/Josepavese/matrix/internal/middleware"
)

func classifyPromptFailure(t *testing.T, message string) *providerfailure.Failure {
	t.Helper()
	err := classifyProviderFailure("opencode", middleware.ProtocolEndpoint{Kind: middleware.ProtocolKindACP}, "session/prompt", errors.New(message))
	failure, ok := providerfailure.As(err)
	if !ok {
		t.Fatalf("expected a typed provider failure, got %T %[1]v", err)
	}
	return failure
}

// TestClassifyProviderFailureRecognizesToolPermissionAbort pins the positive
// evidence: a provider that stops a turn because a tool permission was denied is
// reported as a tool-permission denial, with the provider's own bounded
// diagnostics.
func TestClassifyProviderFailureRecognizesToolPermissionAbort(t *testing.T) {
	cases := []string{
		"ACP prompt failed: tool_permission_denied: call-1",
		"ACP prompt failed: permission_denied",
		"ACP prompt failed: tool permission denied",
		"ACP prompt failed: stream ended: permission denied",
		"ACP prompt failed: the user rejected the permission request",
		"ACP prompt failed: rejected permission for the command",
		"ACP prompt failed: the tool call was rejected",
		"ACP prompt failed: fs/read_text_file denied because agent.trust_mode is false",
	}
	for _, message := range cases {
		t.Run(message, func(t *testing.T) {
			failure := classifyPromptFailure(t, message)
			if failure.Code != providerfailure.ProviderToolPermissionDenied {
				t.Fatalf("code = %q, want %q", failure.Code, providerfailure.ProviderToolPermissionDenied)
			}
			if failure.Phase != "session/prompt" {
				t.Fatalf("phase = %q", failure.Phase)
			}
			if failure.Diagnostics["provider_error"] == "" {
				t.Fatalf("the provider evidence must be preserved, got %#v", failure.Diagnostics)
			}
			if status, typed := providerfailure.HTTPStatus(failure); !typed || status != http.StatusBadGateway {
				t.Fatalf("HTTPStatus = (%d, %v), want 502 typed", status, typed)
			}
			if kind := failure.EventKind(); kind != "provider.runtime.failed" {
				t.Fatalf("EventKind = %q, want a runtime failure", kind)
			}
		})
	}
}

// TestClassifyProviderFailureKeepsAuthFailuresAsAuth pins the conservative
// boundary: an authentication refusal must not become a tool-permission denial,
// even when the provider also used permission wording.
func TestClassifyProviderFailureKeepsAuthFailuresAsAuth(t *testing.T) {
	cases := []string{
		"ACP prompt failed: provider authentication failed",
		"ACP prompt failed: 401 unauthorized",
		"ACP prompt failed: invalid api key",
		"ACP prompt failed: permission denied: invalid api key",
		"ACP prompt failed: forbidden: the credential is not authorized",
	}
	for _, message := range cases {
		t.Run(message, func(t *testing.T) {
			failure := classifyPromptFailure(t, message)
			if failure.Code == providerfailure.ProviderToolPermissionDenied {
				t.Fatalf("an authentication refusal was misclassified as a tool-permission denial: %q", message)
			}
			if failure.Code != providerfailure.AuthMismatch {
				t.Fatalf("code = %q, want %q", failure.Code, providerfailure.AuthMismatch)
			}
		})
	}
}

// TestToolPermissionDiagnosticsAreBoundedAndRedacted keeps provider prose from
// turning a run failure into a secret or prompt transcript.
func TestToolPermissionDiagnosticsAreBoundedAndRedacted(t *testing.T) {
	message := "ACP prompt failed: tool permission denied for Bearer super-secret-token " + strings.Repeat("x", 4096)
	failure := classifyPromptFailure(t, message)
	providerError := failure.Diagnostics["provider_error"]
	if strings.Contains(providerError, "super-secret-token") {
		t.Fatalf("diagnostics leaked a bearer secret: %q", providerError)
	}
	if len(providerError) > 1024 {
		t.Fatalf("diagnostics are unbounded: %d bytes", len(providerError))
	}
}
