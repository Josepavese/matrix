package middleware

import "context"

// Message represents a single message in an ACP Run.
type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// RunSpec defines the input payload for an Agent Run.
type RunSpec struct {
	AgentID string    `json:"agent_id"`
	Input   []Message `json:"input"`
}

// RunResult defines the status and output of an Agent Run.
type RunResult struct {
	ID     string    `json:"id"`
	Status string    `json:"status"` // e.g., "created", "in_progress", "completed", "failed"
	Output []Message `json:"output"`
	Error  string    `json:"error,omitempty"`
}

// AgentRunner is the middleware interface that abstracts ACP-compliant agent execution.
// It allows the application to trigger runs and retrieve their statuses without
// depending on a specific backend implementation.
type AgentRunner interface {
	// Run creates and starts a new agent execution synchronously or asynchronously.
	Run(ctx context.Context, spec RunSpec) (*RunResult, error)

	// GetRun retrieves the current status and output of a specific run by ID.
	GetRun(ctx context.Context, id string) (*RunResult, error)
}
