package runreconcile

import (
	"context"
	"testing"
	"time"

	"github.com/Josepavese/matrix/internal/logic/sessioncleanup"
	"github.com/Josepavese/matrix/internal/middleware"
)

type reconcileTestRouter struct {
	result middleware.SessionActionResult
	err    error
}

func (r reconcileTestRouter) HandleSessionActionTyped(context.Context, middleware.SessionActionRequest) (middleware.SessionActionResult, error) {
	return r.result, r.err
}

func TestApplyMarksRetainedReconcileRefsAsRunFailure(t *testing.T) {
	cleanup := &middleware.SessionCleanupResult{
		LogicalSessionID: "run-session",
		AgentID:          "opencode",
		ProtocolKind:     "acp",
		Clean:            true,
		StrongCleanup:    true,
		CleanupStrength:  sessioncleanup.StrengthStrong,
		LocalForgotten:   true,
		ProcessReaped:    true,
	}
	router := reconcileTestRouter{result: middleware.SessionActionResult{
		Action: "reconcile",
		Reconcile: &middleware.AgentClientReconcileResult{
			Retained: []middleware.AgentClientRef{{
				LogicalSessionID: "logical-retained",
				RemoteSessionID:  "remote-retained",
				AgentID:          "opencode",
				ProtocolKind:     "acp",
				WorkspacePath:    "/tmp/eval-ws",
			}},
		},
	}}

	err := Apply(context.Background(), Request{
		Timeout:       time.Second,
		Router:        router,
		ChannelID:     "eval",
		AgentID:       "opencode",
		WorkspacePath: "/tmp/eval-ws",
		Cleanup:       cleanup,
	})
	if err == nil {
		t.Fatalf("expected retained reconcile client to fail run cleanup")
	}
	if cleanup.Clean || cleanup.StrongCleanup || cleanup.CleanupStrength != sessioncleanup.StrengthFailed {
		t.Fatalf("expected failed cleanup proof, got %+v", cleanup)
	}
	if cleanup.FailureCode != sessioncleanup.FailureRunRelatedSessionRetained {
		t.Fatalf("expected retained related-session failure code, got %q", cleanup.FailureCode)
	}
	if len(cleanup.RelatedSessions) != 1 || !cleanup.RelatedSessions[0].Retained {
		t.Fatalf("expected retained related session proof, got %+v", cleanup.RelatedSessions)
	}
	if cleanup.RelatedSessions[0].Reason != sessioncleanup.WarningRunRelatedSessionRetained {
		t.Fatalf("expected retained reason, got %+v", cleanup.RelatedSessions[0])
	}
	if cleanup.RelatedSessions[0].LogicalSessionID != "logical-retained" ||
		cleanup.RelatedSessions[0].RemoteSessionID != "remote-retained" ||
		cleanup.RelatedSessions[0].ProtocolKind != "acp" {
		t.Fatalf("expected retained ownership details, got %+v", cleanup.RelatedSessions[0])
	}
}

func TestApplyPreservesReapedReconcileProof(t *testing.T) {
	cleanup := &middleware.SessionCleanupResult{
		Clean:           true,
		StrongCleanup:   true,
		CleanupStrength: sessioncleanup.StrengthStrong,
		LocalForgotten:  true,
		ProcessReaped:   true,
	}
	router := reconcileTestRouter{result: middleware.SessionActionResult{
		Action: "reconcile",
		Reconcile: &middleware.AgentClientReconcileResult{
			Reaped: []middleware.AgentClientRef{{AgentID: "opencode", WorkspacePath: "/tmp/eval-ws"}},
		},
	}}

	if err := Apply(context.Background(), Request{
		Timeout:       time.Second,
		Router:        router,
		ChannelID:     "eval",
		AgentID:       "opencode",
		WorkspacePath: "/tmp/eval-ws",
		Cleanup:       cleanup,
	}); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if !cleanup.Clean || !cleanup.StrongCleanup {
		t.Fatalf("expected clean proof to stay strong, got %+v", cleanup)
	}
	if len(cleanup.RelatedSessions) != 1 || cleanup.RelatedSessions[0].Reason != sessioncleanup.ReasonRunUnreferencedAgentClientReaped {
		t.Fatalf("expected reaped related-session proof, got %+v", cleanup.RelatedSessions)
	}
}

func TestApplyIgnoresRetainedClientsOutsideRunWorkspace(t *testing.T) {
	cleanup := &middleware.SessionCleanupResult{
		AgentID:         "opencode",
		Clean:           true,
		StrongCleanup:   true,
		CleanupStrength: sessioncleanup.StrengthStrong,
		ProcessReaped:   true,
	}
	router := reconcileTestRouter{result: middleware.SessionActionResult{
		Action: "reconcile",
		Reconcile: &middleware.AgentClientReconcileResult{
			Retained: []middleware.AgentClientRef{{AgentID: "opencode", WorkspacePath: "/home/jose"}},
		},
	}}

	if err := Apply(context.Background(), Request{
		Timeout:       time.Second,
		Router:        router,
		ChannelID:     "eval",
		AgentID:       "opencode",
		WorkspacePath: "/tmp/eval-ws",
		Cleanup:       cleanup,
	}); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if !cleanup.Clean || !cleanup.StrongCleanup || cleanup.ProcessRetained {
		t.Fatalf("unrelated retained client must not fail run cleanup, got %+v", cleanup)
	}
	if len(cleanup.RelatedSessions) != 0 {
		t.Fatalf("unrelated retained client must not enter run cleanup proof, got %+v", cleanup.RelatedSessions)
	}
}
