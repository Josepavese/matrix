package agentprobe

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/Josepavese/matrix/internal/logic/providerfailure"
	"github.com/Josepavese/matrix/internal/middleware"
)

// TestACPInitializeReportsAnUnreachableProvider keeps a probe of a missing agent
// from being reported as healthy: the failure must be classified, or the doctor
// would tell an operator that a broken agent is ready.
func TestACPInitializeReportsAnUnreachableProvider(t *testing.T) {
	endpoint := middleware.ProtocolEndpoint{
		Kind:      middleware.ProtocolKindACP,
		Transport: "stdio",
		Command:   "/nonexistent/matrix-probe-target",
	}
	err := ACPInitialize(context.Background(), endpoint)
	if err == nil {
		t.Fatal("probing a missing command must fail")
	}
	failure, ok := providerfailure.As(err)
	if !ok {
		t.Fatalf("the failure must be classified as a provider failure, got %T", err)
	}
	if failure.Code == "" || failure.Phase != "initialize" {
		t.Fatalf("the classification must name the phase, got %+v", failure)
	}
	if !strings.Contains(failure.Diagnostics["command"], "matrix-probe-target") {
		t.Fatalf("the diagnostics must name the command, got %v", failure.Diagnostics)
	}
	if !errors.Is(err, err) {
		t.Fatal("unreachable")
	}
}

// TestACPInitializeRefusesACancelledContext keeps a probe from hanging when the
// caller has already given up.
func TestACPInitializeRefusesACancelledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := ACPInitialize(ctx, middleware.ProtocolEndpoint{
		Kind:      middleware.ProtocolKindACP,
		Transport: "stdio",
		Command:   "/bin/sh",
	})
	if err == nil {
		t.Fatal("a cancelled probe must not report success")
	}
}
