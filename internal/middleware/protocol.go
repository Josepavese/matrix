package middleware

import "context"

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
	AgentID          string
	LogicalSessionID string
	RemoteSessionID  string
	Message          string
	Tools            []Tool
	ThoughtNotifier  ThoughtNotifier
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
	RemoteSessionID string
	DisplayID       string
	Title           string
	Status          string
	UpdatedAt       string
	ProtocolKind    ProtocolKind
	CanResume       bool
	CanDelete       bool
}

// ConversationSessionCapabilities declares which session lifecycle features
// a protocol adapter can provide for a live client.
type ConversationSessionCapabilities struct {
	List   bool
	Load   bool
	Cancel bool
	Delete bool
}

// ConversationSessionControl is an optional interface implemented by protocol clients
// that can enumerate, import or delete/cancel remote sessions/tasks.
type ConversationSessionControl interface {
	SessionCapabilities() ConversationSessionCapabilities
	ListRemoteSessions(ctx context.Context) ([]RemoteSessionInfo, error)
	GetRemoteSession(ctx context.Context, remoteSessionID string) (RemoteSessionInfo, error)
	CancelRemoteSession(ctx context.Context, remoteSessionID string) error
	DeleteRemoteSession(ctx context.Context, remoteSessionID string) error
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
	DeleteAgentSession(ctx context.Context, agentID string, remoteSessionID string) error
}
