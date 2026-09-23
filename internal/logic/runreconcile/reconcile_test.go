package runreconcile

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/Josepavese/matrix/internal/logic/sessioncleanup"
	"github.com/Josepavese/matrix/internal/middleware"
)

type reconcileTestRouter struct {
	result middleware.SessionActionResult
	err    error
	// attempts drives a scripted reconcile sequence. When non-empty each element
	// is consumed by one call, so a test can model "denied, then success" and a
	// persistent denial without inventing a second provider.
	attempts []reconcileAttemptResult
	calls    int
	requests []middleware.SessionActionRequest
}

type reconcileAttemptResult struct {
	result middleware.SessionActionResult
	err    error
}

func (r *reconcileTestRouter) HandleSessionActionTyped(_ context.Context, req middleware.SessionActionRequest) (middleware.SessionActionResult, error) {
	r.calls++
	r.requests = append(r.requests, req)
	if len(r.attempts) > 0 {
		attempt := r.attempts[0]
		if len(r.attempts) > 1 {
			r.attempts = r.attempts[1:]
		}
		return attempt.result, attempt.err
	}
	return r.result, r.err
}

// terminationDeniedError is the Windows shape reported by the issue: the client
// close failed because TerminateProcess was refused, while the cache entry was
// already evicted so the same process is also reported absent.
func terminationDeniedError() error {
	return errors.New("close retained client codex\x00C:\\Users\\rober\\Documents\\Half Pocket: exit status 128\nTerminateProcess: Accesso negato")
}

func TestApplyMarksRetainedRunScopedChildAsRunFailure(t *testing.T) {
	cleanup := &middleware.SessionCleanupResult{
		LogicalSessionID: "run-session",
		RemoteSessionID:  "run-remote",
		AgentID:          "opencode",
		ProtocolKind:     "acp",
		Clean:            true,
		StrongCleanup:    true,
		CleanupStrength:  sessioncleanup.StrengthStrong,
		LocalForgotten:   true,
		ProcessReaped:    true,
	}
	router := &reconcileTestRouter{result: middleware.SessionActionResult{
		Action: "reconcile",
		Reconcile: &middleware.AgentClientReconcileResult{
			Retained: []middleware.AgentClientRef{{
				LogicalSessionID: "run-session",
				RemoteSessionID:  "run-remote",
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
		t.Fatalf("expected retained run-scoped child to fail run cleanup")
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
	if cleanup.RelatedSessions[0].LogicalSessionID != "run-session" ||
		cleanup.RelatedSessions[0].RemoteSessionID != "run-remote" ||
		cleanup.RelatedSessions[0].ProtocolKind != "acp" {
		t.Fatalf("expected retained ownership details, got %+v", cleanup.RelatedSessions[0])
	}
	if cleanup.ProcessRetentionScope != sessioncleanup.ScopeRunScopedAgentChild {
		t.Fatalf("evidence must name the run-scoped child, got %q", cleanup.ProcessRetentionScope)
	}
}

func TestApplyTreatsRetainedCachedSharedClientAsDegradedCleanup(t *testing.T) {
	cleanup := &middleware.SessionCleanupResult{
		LogicalSessionID: "run-session",
		RemoteSessionID:  "run-remote",
		AgentID:          "codex",
		ProtocolKind:     "acp",
		Clean:            true,
		StrongCleanup:    true,
		CleanupStrength:  sessioncleanup.StrengthStrong,
		LocalForgotten:   true,
		RemoteDeleted:    true,
	}
	router := &reconcileTestRouter{result: middleware.SessionActionResult{
		Action: "reconcile",
		Reconcile: &middleware.AgentClientReconcileResult{
			Retained: []middleware.AgentClientRef{{
				LogicalSessionID: "other-logical",
				RemoteSessionID:  "other-remote",
				AgentID:          "codex",
				ProtocolKind:     "acp",
				WorkspacePath:    "/tmp/eval-ws",
			}},
		},
	}}

	err := Apply(context.Background(), Request{
		Timeout:       time.Second,
		Router:        router,
		ChannelID:     "eval",
		AgentID:       "codex",
		WorkspacePath: "/tmp/eval-ws",
		Cleanup:       cleanup,
	})
	if err != nil {
		t.Fatalf("retained cached shared client must not fail a completed run, got %v", err)
	}
	if cleanup.CleanupStrength != sessioncleanup.StrengthRetained {
		t.Fatalf("expected degraded retained cleanup strength, got %q", cleanup.CleanupStrength)
	}
	if cleanup.StrongCleanup {
		t.Fatalf("degraded cleanup must not claim strong cleanup, got %+v", cleanup)
	}
	if cleanup.WeakCleanupReason != sessioncleanup.WeakCleanupProcessRetained {
		t.Fatalf("expected weak process-retained reason, got %q", cleanup.WeakCleanupReason)
	}
	if cleanup.ProcessRetentionScope != sessioncleanup.ScopeCachedSharedClient {
		t.Fatalf("evidence must name the shared client, got %q", cleanup.ProcessRetentionScope)
	}
	if !cleanup.ProcessRetentionAllowed {
		t.Fatalf("shared-client retention must be marked allowed, got %+v", cleanup)
	}
	if len(cleanup.RelatedSessions) != 1 || cleanup.RelatedSessions[0].RemoteSessionID != "other-remote" {
		t.Fatalf("expected retained shared client ownership proof, got %+v", cleanup.RelatedSessions)
	}
}

func TestApplyNamesWrapperWhenRetainedRefHasNoSessionIdentity(t *testing.T) {
	cleanup := &middleware.SessionCleanupResult{
		LogicalSessionID: "run-session",
		RemoteSessionID:  "run-remote",
		AgentID:          "codex",
		Clean:            true,
		CleanupStrength:  sessioncleanup.StrengthStrong,
		RemoteDeleted:    true,
	}
	router := &reconcileTestRouter{result: middleware.SessionActionResult{
		Action: "reconcile",
		Reconcile: &middleware.AgentClientReconcileResult{
			Retained: []middleware.AgentClientRef{{AgentID: "codex", WorkspacePath: "/tmp/eval-ws"}},
		},
	}}

	err := Apply(context.Background(), Request{
		Timeout:       time.Second,
		Router:        router,
		ChannelID:     "eval",
		AgentID:       "codex",
		WorkspacePath: "/tmp/eval-ws",
		Cleanup:       cleanup,
	})
	if err == nil {
		t.Fatalf("a retained wrapper with no session identity must fail the run")
	}
	if cleanup.ProcessRetentionScope != sessioncleanup.ScopeAgentWrapper {
		t.Fatalf("evidence must name the wrapper, got %q", cleanup.ProcessRetentionScope)
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
	router := &reconcileTestRouter{result: middleware.SessionActionResult{
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
	router := &reconcileTestRouter{result: middleware.SessionActionResult{
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

// runScopedCleanup is the cleanup proof shape produced before reconciliation for
// an ephemeral run whose remote session and local mirror were already handled.
func runScopedCleanup() *middleware.SessionCleanupResult {
	return &middleware.SessionCleanupResult{
		LogicalSessionID: "run-session",
		RemoteSessionID:  "run-remote",
		AgentID:          "codex",
		ProtocolKind:     "acp",
		Clean:            true,
		StrongCleanup:    true,
		CleanupStrength:  sessioncleanup.StrengthStrong,
		RemoteDeleted:    true,
		LocalForgotten:   true,
	}
}

func runScopedRequest(router Router, cleanup *middleware.SessionCleanupResult) Request {
	return Request{
		Timeout:       time.Second,
		Router:        router,
		ChannelID:     "eval",
		AgentID:       "codex",
		WorkspacePath: "/tmp/eval-ws",
		Cleanup:       cleanup,
	}
}

func runScopedRef() middleware.AgentClientRef {
	return middleware.AgentClientRef{
		LogicalSessionID: "run-session",
		RemoteSessionID:  "run-remote",
		AgentID:          "codex",
		ProtocolKind:     "acp",
		WorkspacePath:    "/tmp/eval-ws",
	}
}

func sharedClientRef() middleware.AgentClientRef {
	return middleware.AgentClientRef{
		LogicalSessionID: "other-logical",
		RemoteSessionID:  "other-remote",
		AgentID:          "codex",
		ProtocolKind:     "acp",
		WorkspacePath:    "/tmp/eval-ws",
	}
}

func deniedReconcileResult(ref middleware.AgentClientRef) middleware.SessionActionResult {
	return middleware.SessionActionResult{
		Action: "reconcile",
		Reconcile: &middleware.AgentClientReconcileResult{
			Retained: []middleware.AgentClientRef{ref},
		},
	}
}

// (a) A transient access denial must be retried, and a retry that succeeds must
// leave the completed run successful instead of converting it into run.failed.
func TestApplyRetriesTransientTerminationDenialAndPreservesRunOutcome(t *testing.T) {
	cleanup := runScopedCleanup()
	router := &reconcileTestRouter{attempts: []reconcileAttemptResult{
		{result: deniedReconcileResult(sharedClientRef()), err: terminationDeniedError()},
		{result: middleware.SessionActionResult{Action: "reconcile"}},
	}}

	if err := Apply(context.Background(), runScopedRequest(router, cleanup)); err != nil {
		t.Fatalf("a transient denial recovered by retry must not fail the run: %v", err)
	}
	if router.calls != 2 {
		t.Fatalf("expected the denial to be retried exactly once, got %d calls", router.calls)
	}
	if !cleanup.Clean || cleanup.CleanupStrength != sessioncleanup.StrengthStrong {
		t.Fatalf("recovered cleanup must keep its strong proof, got %+v", cleanup)
	}
	if cleanup.ProcessRetained || cleanup.FailureCode != "" || cleanup.Error != "" {
		t.Fatalf("a recovered denial must leave no retention failure evidence, got %+v", cleanup)
	}
}

// (b) A persistent denial on the run-scoped child is a real leak: the run fails
// and the evidence names the child rather than a bare process_retained=true.
func TestApplyFailsRunWhenRunScopedChildTerminationIsDenied(t *testing.T) {
	cleanup := runScopedCleanup()
	router := &reconcileTestRouter{attempts: []reconcileAttemptResult{
		{result: deniedReconcileResult(runScopedRef()), err: terminationDeniedError()},
	}}

	err := Apply(context.Background(), runScopedRequest(router, cleanup))
	if err == nil {
		t.Fatalf("persistent denial on the run-scoped child must fail the run")
	}
	// A run-scoped denial is not retried: the reconcile action evicts before
	// closing, so a second pass could report success while the child is alive.
	if router.calls != 1 {
		t.Fatalf("run-scoped denial must not be retried, got %d calls", router.calls)
	}
	if !strings.Contains(cleanup.Error, "Accesso negato") {
		t.Fatalf("expected the OS denial in the evidence, got %q", cleanup.Error)
	}
	if cleanup.Clean || cleanup.StrongCleanup || cleanup.CleanupStrength != sessioncleanup.StrengthFailed {
		t.Fatalf("expected failed cleanup proof, got %+v", cleanup)
	}
	if cleanup.ProcessRetentionScope != sessioncleanup.ScopeRunScopedAgentChild {
		t.Fatalf("evidence must name the run-scoped child, got %q", cleanup.ProcessRetentionScope)
	}
	if cleanup.FailureCode != sessioncleanup.FailureRunRelatedSessionRetained {
		t.Fatalf("expected retained-child failure code, got %q", cleanup.FailureCode)
	}
	if len(cleanup.RelatedSessions) != 1 ||
		cleanup.RelatedSessions[0].LogicalSessionID != "run-session" ||
		cleanup.RelatedSessions[0].RemoteSessionID != "run-remote" {
		t.Fatalf("expected the retained child identity in evidence, got %+v", cleanup.RelatedSessions)
	}
}

// (c) A persistent denial on a cached shared client after a completed run is a
// degraded cleanup: the run stays successful and the evidence names the shared
// client.
func TestApplyKeepsCompletedRunWhenSharedClientTerminationIsDenied(t *testing.T) {
	cleanup := runScopedCleanup()
	router := &reconcileTestRouter{attempts: []reconcileAttemptResult{
		{result: deniedReconcileResult(sharedClientRef()), err: terminationDeniedError()},
	}}

	if err := Apply(context.Background(), runScopedRequest(router, cleanup)); err != nil {
		t.Fatalf("a retained cached shared client must not fail a completed run: %v", err)
	}
	if router.calls != sessioncleanup.TerminationMaxAttempts {
		t.Fatalf("expected %d bounded attempts, got %d", sessioncleanup.TerminationMaxAttempts, router.calls)
	}
	if cleanup.CleanupStrength != sessioncleanup.StrengthRetained {
		t.Fatalf("expected degraded retained cleanup, got %q", cleanup.CleanupStrength)
	}
	if cleanup.StrongCleanup {
		t.Fatalf("degraded cleanup must not claim strong cleanup, got %+v", cleanup)
	}
	if cleanup.WeakCleanupReason != sessioncleanup.WeakCleanupProcessRetained {
		t.Fatalf("expected weak process-retained reason, got %q", cleanup.WeakCleanupReason)
	}
	if cleanup.ProcessRetentionScope != sessioncleanup.ScopeCachedSharedClient {
		t.Fatalf("evidence must name the cached shared client, got %q", cleanup.ProcessRetentionScope)
	}
	if !cleanup.ProcessRetentionAllowed {
		t.Fatalf("shared-client retention must be marked allowed, got %+v", cleanup)
	}
	if !cleanup.Clean {
		t.Fatalf("degraded shared-client retention stays operationally clean, got %+v", cleanup)
	}
	if cleanup.FailureCode != "" {
		t.Fatalf("degraded cleanup must not carry a failure code, got %q", cleanup.FailureCode)
	}
	if len(cleanup.RelatedSessions) != 1 ||
		cleanup.RelatedSessions[0].LogicalSessionID != "other-logical" ||
		cleanup.RelatedSessions[0].RemoteSessionID != "other-remote" {
		t.Fatalf("expected the retained shared client identity in evidence, got %+v", cleanup.RelatedSessions)
	}
	if !sessioncleanup.HasWarning(cleanup.Warnings, sessioncleanup.WarningCachedSharedClientRetained) {
		t.Fatalf("expected the shared-client retention warning, got %+v", cleanup.Warnings)
	}
}

// A non-transient reconcile error must not be retried: only an OS termination
// denial is retryable.
func TestApplyDoesNotRetryUnrelatedReconcileError(t *testing.T) {
	cleanup := runScopedCleanup()
	router := &reconcileTestRouter{attempts: []reconcileAttemptResult{
		{err: errors.New("reconcile transport closed")},
	}}

	if err := Apply(context.Background(), runScopedRequest(router, cleanup)); err == nil {
		t.Fatalf("unrelated reconcile error must still fail the run")
	}
	if router.calls != 1 {
		t.Fatalf("unrelated reconcile error must not be retried, got %d calls", router.calls)
	}
}
