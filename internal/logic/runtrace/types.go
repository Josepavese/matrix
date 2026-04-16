// Package runtrace stores and projects protocol-neutral Matrix communication runs.
package runtrace

import "time"

const (
	SchemaAgentCommunicationRunTraceV0 = "matrix.agent_communication_run_trace.v0"

	ExecutionModeSync   = "sync"
	ExecutionModeAsync  = "async"
	ExecutionModeStream = "stream"

	StatusRunning   = "running"
	StatusCompleted = "completed"
	StatusFailed    = "failed"
	StatusCancelled = "cancelled"

	ContentModeRefs     = "refs"
	ContentModeRedacted = "redacted"
	ContentModeInline   = "inline"
)

// Run is the canonical Matrix operational record for one communication run.
type Run struct {
	ID               string                 `json:"id"`
	AgentID          string                 `json:"agent_id"`
	Protocol         string                 `json:"protocol,omitempty"`
	WorkspaceID      string                 `json:"workspace_id,omitempty"`
	WorkspacePath    string                 `json:"workspace_path,omitempty"`
	LogicalSessionID string                 `json:"logical_session_id,omitempty"`
	RemoteSessionID  string                 `json:"remote_session_id,omitempty"`
	ChannelID        string                 `json:"channel_id"`
	ExecutionMode    string                 `json:"execution_mode"`
	Status           string                 `json:"status"`
	InputKind        string                 `json:"input_kind"`
	InputRef         string                 `json:"input_ref,omitempty"`
	InputDigest      string                 `json:"input_digest,omitempty"`
	OutputRef        string                 `json:"output_ref,omitempty"`
	OutputDigest     string                 `json:"output_digest,omitempty"`
	Output           string                 `json:"output,omitempty"`
	StopReason       string                 `json:"stop_reason,omitempty"`
	Error            string                 `json:"error,omitempty"`
	Context          []ContextRef           `json:"context,omitempty"`
	ClientMeta       map[string]interface{} `json:"client_meta,omitempty"`
	TracePolicy      TracePolicy            `json:"trace_policy"`
	StartedAt        time.Time              `json:"started_at"`
	CompletedAt      time.Time              `json:"completed_at,omitempty"`
	UpdatedAt        time.Time              `json:"updated_at"`
}

// ContextRef is caller-supplied neutral context. Matrix does not interpret it.
type ContextRef struct {
	Kind       string                 `json:"kind"`
	ContentRef string                 `json:"content_ref"`
	Digest     string                 `json:"digest,omitempty"`
	Metadata   map[string]interface{} `json:"metadata,omitempty"`
}

// TracePolicy controls how content appears in exported traces.
type TracePolicy struct {
	ContentMode         string `json:"content_mode"`
	RedactionProfile    string `json:"redaction_profile,omitempty"`
	IncludeProtocolMeta bool   `json:"include_protocol_meta"`
}

// Event is one ordered operational event in a run.
type Event struct {
	ID             string                 `json:"id"`
	RunID          string                 `json:"run_id"`
	Kind           string                 `json:"kind"`
	Actor          string                 `json:"actor"`
	Status         string                 `json:"status,omitempty"`
	Timestamp      time.Time              `json:"timestamp"`
	Protocol       string                 `json:"protocol,omitempty"`
	ProtocolMethod string                 `json:"protocol_method,omitempty"`
	ProtocolMeta   map[string]interface{} `json:"protocol_meta,omitempty"`
	ToolName       string                 `json:"tool_name,omitempty"`
	ContentRef     string                 `json:"content_ref,omitempty"`
	ContentDigest  string                 `json:"content_digest,omitempty"`
	ArtifactRefs   []string               `json:"artifact_refs,omitempty"`
	DecisionID     string                 `json:"decision_id,omitempty"`
	Message        string                 `json:"message,omitempty"`
	Metadata       map[string]interface{} `json:"metadata,omitempty"`
}

// Trace is the versioned exported projection consumed by external systems.
type Trace struct {
	Schema      string       `json:"schema"`
	Run         TraceRun     `json:"run"`
	Surface     Surface      `json:"surface"`
	Routing     Routing      `json:"routing,omitempty"`
	Events      []Event      `json:"events"`
	Outcome     Outcome      `json:"outcome"`
	TracePolicy TracePolicy  `json:"trace_policy"`
	Context     []ContextRef `json:"context,omitempty"`
}

type TraceRun struct {
	ID               string    `json:"id"`
	AgentID          string    `json:"agent_id"`
	Protocol         string    `json:"protocol,omitempty"`
	WorkspaceID      string    `json:"workspace_id,omitempty"`
	LogicalSessionID string    `json:"logical_session_id,omitempty"`
	RemoteSessionID  string    `json:"remote_session_id,omitempty"`
	StartedAt        time.Time `json:"started_at"`
	CompletedAt      time.Time `json:"completed_at,omitempty"`
	Status           string    `json:"status"`
}

type Surface struct {
	Channel       string `json:"channel"`
	InputKind     string `json:"input_kind"`
	ContentRef    string `json:"content_ref,omitempty"`
	ContentDigest string `json:"content_digest,omitempty"`
	Redaction     string `json:"redaction"`
}

type Routing struct {
	DecisionID         string `json:"decision_id,omitempty"`
	SelectedAgentID    string `json:"selected_agent_id,omitempty"`
	SelectedSessionID  string `json:"selected_session_id,omitempty"`
	FallbackUsed       bool   `json:"fallback_used"`
	Explanation        string `json:"explanation,omitempty"`
	SelectedProtocol   string `json:"selected_protocol,omitempty"`
	SelectedWorkspace  string `json:"selected_workspace_id,omitempty"`
	SelectedRemoteSess string `json:"selected_remote_session_id,omitempty"`
}

type Outcome struct {
	Status     string `json:"status"`
	StopReason string `json:"stop_reason,omitempty"`
	SummaryRef string `json:"summary_ref,omitempty"`
	Error      string `json:"error,omitempty"`
}

type Sink struct {
	ID         string                 `json:"id"`
	URL        string                 `json:"url"`
	EventKinds []string               `json:"event_kinds,omitempty"`
	Metadata   map[string]interface{} `json:"metadata,omitempty"`
	CreatedAt  time.Time              `json:"created_at"`
}
