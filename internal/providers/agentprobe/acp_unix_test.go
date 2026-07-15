//go:build linux || darwin

package agentprobe

import (
	"context"
	"strings"
	"testing"

	"github.com/Josepavese/matrix/internal/logic/providerfailure"
	"github.com/Josepavese/matrix/internal/middleware"
)

func TestACPInitializePreservesSanitizedProcessFailure(t *testing.T) {
	endpoint := middleware.ProtocolEndpoint{
		Kind: middleware.ProtocolKindACP, Transport: "stdio", Command: "sh",
		Args: []string{"-c", `IFS= read -r request; echo 'Error: /tmp/config.toml OPENAI_API_KEY=sk-secretvalue123 unknown variant default' >&2; exit 7`},
	}

	err := ACPInitialize(context.Background(), endpoint)
	failure, ok := providerfailure.As(err)
	if !ok {
		t.Fatalf("expected typed provider failure, got %T %[1]v", err)
	}
	if got := failure.Diagnostics["provider_exit_code"]; got != "7" {
		t.Fatalf("provider exit code = %q, diagnostics=%+v", got, failure.Diagnostics)
	}
	stderr := failure.Diagnostics["provider_stderr"]
	if !strings.Contains(stderr, "unknown variant default") {
		t.Fatalf("provider stderr lost cause: %q", stderr)
	}
	if strings.Contains(stderr, "secretvalue123") {
		t.Fatalf("provider stderr leaked secret: %q", stderr)
	}
	if failure.Diagnostics["failure_reason"] != "provider_process_exit" {
		t.Fatalf("failure reason = %q", failure.Diagnostics["failure_reason"])
	}
}
