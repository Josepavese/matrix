package middleware

import (
	"context"
	"errors"
)

var ErrConversationTurnActive = errors.New("conversation turn already active")

func IsConversationTurnActive(err error) bool {
	return errors.Is(err, ErrConversationTurnActive)
}

// ProtocolKind identifies the agent protocol family used by an endpoint.
type ProtocolKind string

const (
	ProtocolKindACP ProtocolKind = "acp"
	ProtocolKindA2A ProtocolKind = "a2a"
)

// ProtocolEndpoint describes a remotely reachable or locally spawned agent endpoint
// without leaking protocol-specific session semantics into the caller.
type ProtocolEndpoint struct {
	Kind            ProtocolKind
	Transport       string
	Address         string
	Command         string
	Args            []string
	Env             []string
	ProtocolVersion string
	CardURL         string
}

// ConversationTurn is the protocol-neutral representation of a single user turn.
type ConversationTurn struct {
	AgentID           string
	LogicalSessionID  string
	RemoteSessionID   string
	WorkspacePath     string
	Message           string
	SidecarCapsules   []SidecarCapsule
	Tools             []Tool
	ThoughtNotifier   ThoughtNotifier
	LiveContextAttach bool
}

// ConversationResult is the protocol-neutral result of a single agent turn.
type ConversationResult struct {
	Output          string
	RemoteSessionID string
	ToolCalls       []ToolCall
	Metadata        ConversationMetadata
}

// ConversationMetadata carries protocol-neutral session/task metadata emitted by
// providers during or after a turn and suitable for mirroring into the vault.
type ConversationMetadata struct {
	Title     string
	UpdatedAt string
	Status    string
	Meta      map[string]interface{}
}

// ConversationClient executes protocol-specific turns behind a neutral contract.
type ConversationClient interface {
	ExecuteTurn(ctx context.Context, turn ConversationTurn) (ConversationResult, error)
	Close() error
}

// RemoteSessionInfo is the protocol-neutral description of a remote conversation/task.
// RemoteSessionID is the opaque identifier Matrix must persist and reuse.
// DisplayID is the human-facing stable token shown in channels for switch/delete flows.
type RemoteSessionInfo struct {
	RemoteSessionID string       `json:"remote_session_id,omitempty"`
	DisplayID       string       `json:"display_id,omitempty"`
	Title           string       `json:"title,omitempty"`
	Status          string       `json:"status,omitempty"`
	UpdatedAt       string       `json:"updated_at,omitempty"`
	ProtocolKind    ProtocolKind `json:"protocol_kind,omitempty"`
	CanResume       bool         `json:"can_resume,omitempty"`
	CanDelete       bool         `json:"can_delete,omitempty"`
}

// CapabilityDescriptor is the protocol-neutral SSOT for a provider capability.
// Boolean fields on ConversationSessionCapabilities remain convenience shortcuts;
// this descriptor carries the stability/source required for safe orchestration.
type CapabilityDescriptor struct {
	Name                     string `json:"name,omitempty"`
	Supported                bool   `json:"supported"`
	Status                   string `json:"status,omitempty"`
	Stability                string `json:"stability,omitempty"`
	Source                   string `json:"source,omitempty"`
	Detail                   string `json:"detail,omitempty"`
	ActiveParentSafe         *bool  `json:"active_parent_safe,omitempty"`
	RequiresIdleParent       *bool  `json:"requires_idle_parent,omitempty"`
	ArtifactTurn             *bool  `json:"artifact_turn,omitempty"`
	AsyncSupported           *bool  `json:"async_supported,omitempty"`
	Blocking                 *bool  `json:"blocking,omitempty"`
	ArtifactStreaming        *bool  `json:"artifact_streaming,omitempty"`
	LiveInterventionSuitable *bool  `json:"live_intervention_suitable,omitempty"`
}

// ProviderCapabilityReport is a channel-safe provider capability snapshot.
type ProviderCapabilityReport struct {
	AgentID      string                          `json:"agent_id,omitempty"`
	ProtocolKind ProtocolKind                    `json:"protocol_kind,omitempty"`
	Session      map[string]CapabilityDescriptor `json:"session,omitempty"`
}

// ConversationSessionCapabilities declares which session lifecycle features
// a protocol adapter can provide for a live client.
type ConversationSessionCapabilities struct {
	List       bool
	Load       bool
	Cancel     bool
	Close      bool
	Delete     bool
	InfoUpdate bool
	Resume     bool
	Fork       bool
	Details    map[string]CapabilityDescriptor
}

// ConversationSessionControl is an optional interface implemented by protocol clients
// that can enumerate, import or delete/cancel remote sessions/tasks.
type ConversationSessionControl interface {
	SessionCapabilities() ConversationSessionCapabilities
	ListRemoteSessions(ctx context.Context) ([]RemoteSessionInfo, error)
	GetRemoteSession(ctx context.Context, remoteSessionID string) (RemoteSessionInfo, error)
	CancelRemoteSession(ctx context.Context, remoteSessionID string) error
	CloseRemoteSession(ctx context.Context, remoteSessionID string) error
	DeleteRemoteSession(ctx context.Context, remoteSessionID string) error
}

// SessionMaterializeRequest asks a provider client to allocate a real remote
// session without running an LLM turn.
type SessionMaterializeRequest struct {
	LogicalSessionID string
	WorkspacePath    string
	Tools            []Tool
}

// ConversationSessionMaterializer is implemented by protocol clients that can
// create a remote session handle without prompt replay.
type ConversationSessionMaterializer interface {
	MaterializeRemoteSession(ctx context.Context, req SessionMaterializeRequest) (RemoteSessionInfo, ConversationMetadata, error)
}

// ConversationHealth is an optional interface for cached clients that can report liveness.
type ConversationHealth interface {
	Alive() bool
}

// ConversationFactory creates protocol-specific clients for a neutral endpoint.
type ConversationFactory interface {
	NewClient(ctx context.Context, endpoint ProtocolEndpoint, deps ConversationFactoryDeps) (ConversationClient, error)
}

// ConversationFactoryDeps bundles host capabilities exposed to protocol adapters.
type ConversationFactoryDeps struct {
	FS        FS
	Cwd       string
	Process   Process
	TrustMode func() bool
}

// AgentSessionController is an optional router capability for protocol-transparent
// session lifecycle operations used by Matrix channels and ingress APIs.
type AgentSessionController interface {
	ListAgentSessions(ctx context.Context, agentID string) ([]RemoteSessionInfo, ConversationSessionCapabilities, error)
	GetAgentSession(ctx context.Context, agentID string, remoteSessionID string) (RemoteSessionInfo, error)
	CancelAgentSession(ctx context.Context, agentID string, remoteSessionID string) error
	CloseAgentSession(ctx context.Context, agentID string, remoteSessionID string) error
	DeleteAgentSession(ctx context.Context, agentID string, remoteSessionID string) error
}

// AgentWorkspaceSessionController is an optional extension for lifecycle calls
// that must target the same workspace-bound protocol client used for execution.
type AgentWorkspaceSessionController interface {
	CancelAgentSessionForWorkspace(ctx context.Context, agentID string, remoteSessionID string, workspacePath string) error
	CloseAgentSessionForWorkspace(ctx context.Context, agentID string, remoteSessionID string, workspacePath string) error
	DeleteAgentSessionForWorkspace(ctx context.Context, agentID string, remoteSessionID string, workspacePath string) error
}

// AgentSessionMaterializer is a router-level facade for creating remote session
// handles without running prompt content through the provider.
type AgentSessionMaterializer interface {
	MaterializeAgentSession(ctx context.Context, agentID string, req SessionMaterializeRequest) (RemoteSessionInfo, ConversationMetadata, error)
}

// AgentCapabilityReporter reports protocol lifecycle support without forcing
// callers to infer provider behavior from failed lifecycle calls.
type AgentCapabilityReporter interface {
	AgentCapabilities(ctx context.Context, agentID string) (ProviderCapabilityReport, error)
}

// SessionForkRequest asks a provider to create a temporary child session from an
// existing remote session. It is capability-gated because ACP session/fork is a
// Draft RFD and not a production baseline.
type SessionForkRequest struct {
	RemoteSessionID string
	WorkspacePath   string
}

type SessionForkArtifact struct {
	Kind    string `json:"kind,omitempty"`
	Content string `json:"content,omitempty"`
}

// SessionForkJob is Matrix-owned evidence for an asynchronous fork child turn.
type SessionForkJob struct {
	JobID                  string                `json:"job_id,omitempty"`
	Status                 string                `json:"status,omitempty"`
	ParentLogicalSessionID string                `json:"parent_logical_session_id,omitempty"`
	ChildLogicalSessionID  string                `json:"child_logical_session_id,omitempty"`
	ParentRestored         bool                  `json:"parent_restored,omitempty"`
	AcceptedAt             string                `json:"accepted_at,omitempty"`
	StartedAt              string                `json:"started_at,omitempty"`
	CompletedAt            string                `json:"completed_at,omitempty"`
	Artifact               *SessionForkArtifact  `json:"artifact,omitempty"`
	Cleanup                *SessionCleanupResult `json:"cleanup,omitempty"`
	Error                  *SessionActionError   `json:"error,omitempty"`
}

// SessionForkResult records the provider-neutral outcome of a fork attempt.
type SessionForkResult struct {
	ParentLogicalSessionID string                `json:"parent_logical_session_id,omitempty"`
	ParentRemoteSessionID  string                `json:"parent_remote_session_id,omitempty"`
	ChildLogicalSessionID  string                `json:"child_logical_session_id,omitempty"`
	Child                  *RemoteSessionInfo    `json:"child,omitempty"`
	MakeActive             bool                  `json:"make_active"`
	Ephemeral              bool                  `json:"ephemeral,omitempty"`
	CleanupPolicy          string                `json:"cleanup_policy,omitempty"`
	Async                  bool                  `json:"async,omitempty"`
	JobID                  string                `json:"job_id,omitempty"`
	Job                    *SessionForkJob       `json:"job,omitempty"`
	Artifact               *SessionForkArtifact  `json:"artifact,omitempty"`
	Cleanup                *SessionCleanupResult `json:"cleanup,omitempty"`
	ParentRestored         bool                  `json:"parent_restored,omitempty"`
	Unsupported            bool                  `json:"unsupported,omitempty"`
	Reason                 string                `json:"reason,omitempty"`
}

// ConversationSessionForker is optionally implemented by providers that expose
// true protocol-level temporary child sessions.
type ConversationSessionForker interface {
	ForkRemoteSession(ctx context.Context, req SessionForkRequest) (RemoteSessionInfo, error)
}

// AgentSessionForker is the router-level fork facade used by channels and HTTP.
type AgentSessionForker interface {
	ForkAgentSession(ctx context.Context, agentID string, req SessionForkRequest) (RemoteSessionInfo, error)
}

// AgentClientReaper is an optional router capability used by strict ephemeral
// cleanup flows to close the cached protocol client bound to an agent/workspace.
type AgentClientReaper interface {
	ReapAgentClient(ctx context.Context, agentID string, workspacePath string) (bool, error)
}

// AgentSessionClientReaper is the strict cleanup variant used when Matrix knows
// which remote session must be accounted for. Implementations should only return
// reaped=true when the closed/evicted client is known to own that remote session.
type AgentSessionClientReaper interface {
	ReapAgentSessionClient(ctx context.Context, agentID string, remoteSessionID string, workspacePath string) (bool, error)
}

// AgentClientRef identifies one cached provider client binding.
type AgentClientRef struct {
	AgentID       string `json:"agent_id,omitempty"`
	WorkspacePath string `json:"workspace_path,omitempty"`
}

// AgentClientReconcileResult is the audit record for a cache reconciliation pass.
type AgentClientReconcileResult struct {
	Reaped   []AgentClientRef `json:"reaped,omitempty"`
	Retained []AgentClientRef `json:"retained,omitempty"`
}

// AgentClientReconciler closes cached provider clients that have no remaining
// Matrix vault references. It is used after batch/ephemeral work to prevent
// process retention from being silently treated as strong cleanup proof.
type AgentClientReconciler interface {
	ReconcileAgentClients(ctx context.Context, active []AgentClientRef) (AgentClientReconcileResult, error)
}
