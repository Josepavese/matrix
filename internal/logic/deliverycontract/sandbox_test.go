package deliverycontract

import (
	"context"
	"errors"
	"testing"

	"github.com/Josepavese/matrix/internal/middleware"
)

func TestMissingSandboxRunnerNeverFallsBackToHostValidator(t *testing.T) {
	called := false
	restore := stubValidator(func(context.Context, string, []string) (int, error) { called = true; return 0, nil })
	defer restore()
	contract := Contract{Validator: &Validator{Command: []string{"must-not-run"}, Sandbox: validatorSandboxPolicy()}}
	verdict := Evaluate(context.Background(), t.TempDir(), contract)
	if called || verdict.Status != StatusUnverifiable {
		t.Fatal("requested isolation downgraded", called, verdict.Status)
	}
}

func TestSandboxRunnerFailureCannotAcceptDelivery(t *testing.T) {
	contract := Contract{Validator: &Validator{Command: []string{"check"}, Sandbox: validatorSandboxPolicy()}}
	called := false
	verdict := EvaluateWithValidator(context.Background(), t.TempDir(), contract, func(context.Context, string, Validator) (int, error) {
		called = true
		return 0, errors.New("isolation unavailable")
	})
	if !called || verdict.Status != StatusUnverifiable {
		t.Fatal("sandbox error accepted delivery")
	}
}

func validatorSandboxPolicy() *middleware.ContainerSandbox {
	return &middleware.ContainerSandbox{Engine: "missing", Image: "cached", WorkspaceAccess: "read-only", Network: "none", StateDir: "explicit", User: "1000:1000", MemoryBytes: 64 << 20, CPUs: 1, Pids: 32}
}
