package providerfailure

import (
	"strings"
	"testing"

	"github.com/Josepavese/matrix/internal/middleware"
)

func TestRuntimeFailureRedactsLargeAndStructuredSecrets(t *testing.T) {
	secret := strings.Repeat("private-credential", 1000)
	cause := &codedRPCError{code: -32603, message: `{"apiKey":"` + secret + `"}`, data: map[string]any{"access_token": "private-token", "errorName": "APIError"}}
	diag := Diagnostics(middleware.ProtocolEndpoint{}, cause)
	failure := &Failure{Code: APIError, Err: cause}
	for key, value := range diag {
		if strings.Contains(value, "private-credential") || strings.Contains(value, "private-token") {
			t.Fatalf("diagnostic %s leaked a secret", key)
		}
		if len(value) > maxDiagnosticLength+len("…") {
			t.Fatalf("unbounded diagnostic %s: %d", key, len(value))
		}
	}
	if strings.Contains(failure.Error(), "private-credential") {
		t.Fatal("run error bypassed diagnostic redaction")
	}
}
