package middleware

import "context"

// RunCancellationRequest identifies the exact provider session selected by an
// active Matrix run. RemoteSessionID may be empty during session/new.
type RunCancellationRequest struct {
	RunID            string
	AgentID          string
	WorkspacePath    string
	LogicalSessionID string
	RemoteSessionID  string
}

// RunCanceller sends provider-level cancellation for an active run when the
// run already owns a remote session identifier.
type RunCanceller interface {
	CancelRun(ctx context.Context, req RunCancellationRequest) error
}
