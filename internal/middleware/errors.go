package middleware

import "fmt"

// Error represents a normalized Matrix error
type Error struct {
	Code    string
	Message string
	Op      string
	Err     error
}

func (e *Error) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("[%s] %s: %s (op: %s)", e.Code, e.Message, e.Err.Error(), e.Op)
	}
	return fmt.Sprintf("[%s] %s (op: %s)", e.Code, e.Message, e.Op)
}

// Sentinel errors for use with errors.Is
var (
	ErrSessionNotFound  = &Error{Code: "SESSION_NOT_FOUND", Message: "session not found"}
	ErrAgentNotRunning  = &Error{Code: "AGENT_NOT_RUNNING", Message: "agent is not running"}
	ErrAgentUnavailable = &Error{Code: "AGENT_UNAVAILABLE", Message: "agent endpoint unreachable"}
	ErrAuthFailed       = &Error{Code: "AUTH_FAILED", Message: "authentication failed"}
	ErrAgentNotFound    = &Error{Code: "AGENT_NOT_FOUND", Message: "agent not found in registry"}
	ErrSetupRequired    = &Error{Code: "SETUP_REQUIRED", Message: "matrix setup is required before non-interactive routing"}
)
