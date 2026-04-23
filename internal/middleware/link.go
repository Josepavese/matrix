package middleware

import "context"

// ChannelMessage is the neutral ingress payload emitted by a channel gateway.
type ChannelMessage struct {
	ChannelID       string
	DefaultAgentID  string
	WorkspaceID     string
	WorkspacePath   string
	Input           string
	SidecarCapsules []SidecarCapsule
	Notifier        ThoughtNotifier
}

// ChannelResponse is the neutral egress payload returned to a channel gateway.
type ChannelResponse struct {
	Output string
}

// ConversationRequest is the richer, channel-neutral ingress envelope used by
// workspace-aware callers. It keeps channel identity as ingress metadata while
// letting the runtime resolve work context from workspace hints.
type ConversationRequest struct {
	ChannelID        string
	AgentID          string
	LogicalSessionID string
	WorkspaceID      string
	WorkspacePath    string
	Input            string
	SidecarCapsules  []SidecarCapsule
	Notifier         ThoughtNotifier
	NonInteractive   bool
}

// RunContextAttachmentRequest asks a runtime to inject provider-neutral sidecar
// context into an already selected logical/remote session.
type RunContextAttachmentRequest struct {
	RunID            string
	DeliveryID       string
	ChannelID        string
	AgentID          string
	WorkspaceID      string
	WorkspacePath    string
	LogicalSessionID string
	RemoteSessionID  string
	Reason           string
	SidecarCapsules  []SidecarCapsule
	Notifier         ThoughtNotifier
}

type RunContextAttachmentResult struct {
	Action      string `json:"action"`
	Status      string `json:"status"`
	DeliveryID  string `json:"delivery_id,omitempty"`
	Message     string `json:"message,omitempty"`
	Unsupported bool   `json:"unsupported,omitempty"`
}

type RunContextAttacher interface {
	AttachRunContext(ctx context.Context, req RunContextAttachmentRequest) (RunContextAttachmentResult, error)
}

const (
	SessionPolicyNewEphemeralDeleteAfterRun = "new_ephemeral_delete_after_run"

	SessionCleanupPolicyDeleteRemote                         = "delete_remote"
	SessionCleanupPolicyForgetLocal                          = "forget_local"
	SessionCleanupPolicyDeleteRemoteOrCancelAndForgetLocal   = "delete_remote_or_cancel_and_forget_local"
	SessionCleanupPolicyDeleteRemoteOrForgetLocal            = "delete_remote_or_forget_local"
	SessionCleanupPolicyDeleteRemoteOrCancelForgetLocalAlias = "delete_remote_or_cancel_forget_local"
)

// SessionActionRequest is the typed, channel-neutral request envelope for session lifecycle operations.
type SessionActionRequest struct {
	ChannelID        string
	Action           string
	WorkspaceID      string
	WorkspacePath    string
	Ephemeral        bool
	CleanupPolicy    string
	ForceForgetLocal bool
	MakeActive       *bool
	RestoreParent    bool
	Input            string
	// Target carries the action operand when needed.
	// Examples:
	// - switch/delete/cancel/cleanup: local or remote session selector
	// - new: requested agent id
	// - name: alias to assign to the active logical session
	Target string
}

// HandoffPacket captures the deterministic local transfer context when Matrix
// moves work from one specialist session to another inside the same workspace.
type HandoffPacket struct {
	FromLogicalSessionID string `json:"from_logical_session_id,omitempty"`
	FromRemoteSessionID  string `json:"from_remote_session_id,omitempty"`
	FromAgentID          string `json:"from_agent_id,omitempty"`
	ToAgentID            string `json:"to_agent_id,omitempty"`
	WorkspaceID          string `json:"workspace_id,omitempty"`
	Mode                 string `json:"mode,omitempty"`
	Reason               string `json:"reason,omitempty"`
	Summary              string `json:"summary,omitempty"`
	CreatedAt            string `json:"created_at,omitempty"`
}

// SessionEntry is the typed representation of a local mirrored session.
type SessionEntry struct {
	LogicalSessionID string                 `json:"logical_session_id,omitempty"`
	RemoteSessionID  string                 `json:"remote_session_id,omitempty"`
	AgentID          string                 `json:"agent_id,omitempty"`
	Alias            string                 `json:"alias,omitempty"`
	ProtocolKind     string                 `json:"protocol_kind,omitempty"`
	WorkspaceID      string                 `json:"workspace_id,omitempty"`
	WorkspacePath    string                 `json:"workspace_path,omitempty"`
	WorkspaceBranch  string                 `json:"workspace_branch,omitempty"`
	WorkspaceRole    string                 `json:"workspace_role,omitempty"`
	Mode             string                 `json:"mode,omitempty"`
	Status           string                 `json:"status,omitempty"`
	RemoteStatus     string                 `json:"remote_status,omitempty"`
	Title            string                 `json:"title,omitempty"`
	CreatedAt        string                 `json:"created_at,omitempty"`
	UpdatedAt        string                 `json:"updated_at,omitempty"`
	Active           bool                   `json:"active,omitempty"`
	Ephemeral        bool                   `json:"ephemeral,omitempty"`
	CleanupPolicy    string                 `json:"cleanup_policy,omitempty"`
	Meta             map[string]interface{} `json:"meta,omitempty"`
	PendingHandoff   *HandoffPacket         `json:"pending_handoff,omitempty"`
	LastHandoff      *HandoffPacket         `json:"last_handoff,omitempty"`
	ParentSessionID  string                 `json:"parent_session_id,omitempty"`
	ParentRemoteID   string                 `json:"parent_remote_id,omitempty"`
}

// SessionCleanupResult is the audit record for Matrix session cleanup.
// It is protocol-neutral and intentionally distinguishes remote provider state
// from the local Matrix mirror.
type SessionCleanupResult struct {
	LogicalSessionID        string   `json:"logical_session_id,omitempty"`
	RemoteSessionID         string   `json:"remote_session_id,omitempty"`
	AgentID                 string   `json:"agent_id,omitempty"`
	ProtocolKind            string   `json:"protocol_kind,omitempty"`
	CleanupPolicy           string   `json:"cleanup_policy,omitempty"`
	Clean                   bool     `json:"clean"`
	StrongCleanup           bool     `json:"strong_cleanup"`
	CleanupStrength         string   `json:"cleanup_strength,omitempty"`
	WeakCleanupReason       string   `json:"weak_cleanup_reason,omitempty"`
	RemoteDeleteAttempted   bool     `json:"remote_delete_attempted"`
	RemoteDeleted           bool     `json:"remote_deleted"`
	RemoteDeleteUnsupported bool     `json:"remote_delete_unsupported,omitempty"`
	RemoteCloseAttempted    bool     `json:"remote_close_attempted"`
	RemoteClosed            bool     `json:"remote_closed"`
	RemoteCloseUnsupported  bool     `json:"remote_close_unsupported,omitempty"`
	RemoteCancelAttempted   bool     `json:"remote_cancel_attempted"`
	RemoteCanceled          bool     `json:"remote_canceled"`
	ProcessReapAttempted    bool     `json:"process_reap_attempted"`
	ProcessReaped           bool     `json:"process_reaped"`
	ProcessRetained         bool     `json:"process_retained,omitempty"`
	ProcessRetentionAllowed bool     `json:"process_retention_allowed,omitempty"`
	ProcessRetentionReason  string   `json:"process_retention_reason,omitempty"`
	LocalForgotten          bool     `json:"local_forgotten"`
	Warnings                []string `json:"warnings,omitempty"`
	FailureCode             string   `json:"failure_code,omitempty"`
	Error                   string   `json:"error,omitempty"`
}

type SessionActionError struct {
	Code    string `json:"code,omitempty"`
	Message string `json:"message,omitempty"`
	Target  string `json:"target,omitempty"`
}

// SessionActionResult is the typed, reusable result for session lifecycle operations.
type SessionActionResult struct {
	Action          string                      `json:"action"`
	Message         string                      `json:"message,omitempty"`
	Unsupported     bool                        `json:"unsupported,omitempty"`
	Error           *SessionActionError         `json:"error,omitempty"`
	ActiveSessionID string                      `json:"active_session_id,omitempty"`
	Session         *SessionEntry               `json:"session,omitempty"`
	Sessions        []SessionEntry              `json:"sessions,omitempty"`
	RemoteSessions  []RemoteSessionInfo         `json:"remote_sessions,omitempty"`
	Cleanup         *SessionCleanupResult       `json:"cleanup,omitempty"`
	Capabilities    *ProviderCapabilityReport   `json:"capabilities,omitempty"`
	Fork            *SessionForkResult          `json:"fork,omitempty"`
	Reconcile       *AgentClientReconcileResult `json:"reconcile,omitempty"`
}

// WorkspaceActionRequest is the typed, channel-neutral request envelope for workspace control.
type WorkspaceActionRequest struct {
	ChannelID string
	Action    string
	Target    string
}

// WorkspaceEntry is the typed representation of a configured workspace.
type WorkspaceEntry struct {
	ID              string `json:"id,omitempty"`
	Name            string `json:"name,omitempty"`
	Kind            string `json:"kind,omitempty"`
	RootPath        string `json:"root_path,omitempty"`
	DefaultAgentID  string `json:"default_agent_id,omitempty"`
	ReviewerAgentID string `json:"reviewer_agent_id,omitempty"`
	DefaultMode     string `json:"default_mode,omitempty"`
	PolicyProfile   string `json:"policy_profile,omitempty"`
	Active          bool   `json:"active,omitempty"`
}

// WorkspaceActionResult is the typed, reusable result for workspace operations.
type WorkspaceActionResult struct {
	Action     string           `json:"action"`
	Message    string           `json:"message,omitempty"`
	Workspace  *WorkspaceEntry  `json:"workspace,omitempty"`
	Workspaces []WorkspaceEntry `json:"workspaces,omitempty"`
	Session    *SessionEntry    `json:"session,omitempty"`
}

// WorkspaceReadRequest is the typed, channel-neutral request envelope for
// read-only workspace state and timeline queries.
type WorkspaceReadRequest struct {
	ChannelID   string
	Action      string
	WorkspaceID string
	Limit       int
}

// WorkspaceStateEntry is the typed representation of the current materialized
// state of a workspace.
type WorkspaceStateEntry struct {
	WorkspaceID            string                  `json:"workspace_id,omitempty"`
	ActiveLogicalSessionID string                  `json:"active_logical_session_id,omitempty"`
	ActiveRemoteSessionID  string                  `json:"active_remote_session_id,omitempty"`
	ActiveAgentID          string                  `json:"active_agent_id,omitempty"`
	ActiveMode             string                  `json:"active_mode,omitempty"`
	RemoteStatus           string                  `json:"remote_status,omitempty"`
	LastEventType          string                  `json:"last_event_type,omitempty"`
	LastEventMessage       string                  `json:"last_event_message,omitempty"`
	LastEventAt            string                  `json:"last_event_at,omitempty"`
	LastHandoff            map[string]interface{}  `json:"last_handoff,omitempty"`
	LastDecision           *WorkspaceDecisionTrace `json:"last_decision,omitempty"`
}

// WorkspaceTimelineEvent is the typed representation of one workspace timeline event.
type WorkspaceTimelineEvent struct {
	ID               string                 `json:"id,omitempty"`
	WorkspaceID      string                 `json:"workspace_id,omitempty"`
	Type             string                 `json:"type,omitempty"`
	ChannelID        string                 `json:"channel_id,omitempty"`
	LogicalSessionID string                 `json:"logical_session_id,omitempty"`
	RemoteSessionID  string                 `json:"remote_session_id,omitempty"`
	AgentID          string                 `json:"agent_id,omitempty"`
	Mode             string                 `json:"mode,omitempty"`
	Message          string                 `json:"message,omitempty"`
	Reason           string                 `json:"reason,omitempty"`
	Metadata         map[string]interface{} `json:"metadata,omitempty"`
	CreatedAt        string                 `json:"created_at,omitempty"`
}

// WorkspaceMemoryTurn is the typed representation of one stored workspace turn.
type WorkspaceMemoryTurn struct {
	ID               string `json:"id,omitempty"`
	WorkspaceID      string `json:"workspace_id,omitempty"`
	LogicalSessionID string `json:"logical_session_id,omitempty"`
	RemoteSessionID  string `json:"remote_session_id,omitempty"`
	AgentID          string `json:"agent_id,omitempty"`
	Role             string `json:"role,omitempty"`
	Content          string `json:"content,omitempty"`
	CreatedAt        string `json:"created_at,omitempty"`
}

// WorkspaceSnapshotEntry is the typed representation of one workspace snapshot.
type WorkspaceSnapshotEntry struct {
	ID                     string                  `json:"id,omitempty"`
	WorkspaceID            string                  `json:"workspace_id,omitempty"`
	Title                  string                  `json:"title,omitempty"`
	Note                   string                  `json:"note,omitempty"`
	ActiveLogicalSessionID string                  `json:"active_logical_session_id,omitempty"`
	ActiveRemoteSessionID  string                  `json:"active_remote_session_id,omitempty"`
	ActiveAgentID          string                  `json:"active_agent_id,omitempty"`
	ActiveMode             string                  `json:"active_mode,omitempty"`
	RemoteStatus           string                  `json:"remote_status,omitempty"`
	LastEventType          string                  `json:"last_event_type,omitempty"`
	LastEventAt            string                  `json:"last_event_at,omitempty"`
	LastHandoff            map[string]interface{}  `json:"last_handoff,omitempty"`
	LastDecision           *WorkspaceDecisionTrace `json:"last_decision,omitempty"`
	TurnIDs                []string                `json:"turn_ids,omitempty"`
	EventIDs               []string                `json:"event_ids,omitempty"`
	CreatedAt              string                  `json:"created_at,omitempty"`
}

// WorkspaceDecisionTrace is the typed representation of one orchestration decision.
type WorkspaceDecisionTrace struct {
	Kind              string `json:"kind,omitempty"`
	Source            string `json:"source,omitempty"`
	Explanation       string `json:"explanation,omitempty"`
	RequestedAgentID  string `json:"requested_agent_id,omitempty"`
	SelectedAgentID   string `json:"selected_agent_id,omitempty"`
	SelectedSessionID string `json:"selected_session_id,omitempty"`
	SelectedMode      string `json:"selected_mode,omitempty"`
	FallbackUsed      bool   `json:"fallback_used,omitempty"`
	CreatedAt         string `json:"created_at,omitempty"`
}

// WorkspaceReadResult is the typed, reusable result for workspace state and timeline reads.
type WorkspaceReadResult struct {
	Action    string                   `json:"action"`
	Message   string                   `json:"message,omitempty"`
	Workspace *WorkspaceEntry          `json:"workspace,omitempty"`
	Session   *SessionEntry            `json:"session,omitempty"`
	State     *WorkspaceStateEntry     `json:"state,omitempty"`
	Timeline  []WorkspaceTimelineEvent `json:"timeline,omitempty"`
	Memory    []WorkspaceMemoryTurn    `json:"memory,omitempty"`
	Snapshots []WorkspaceSnapshotEntry `json:"snapshots,omitempty"`
	Decisions []WorkspaceDecisionTrace `json:"decisions,omitempty"`
}

// IntentActionRequest is the typed, channel-neutral request envelope for
// high-level operator intents such as review/resume/continue.
type IntentActionRequest struct {
	ChannelID   string
	Intent      string
	Target      string
	WorkspaceID string
	AgentID     string
	Note        string
}

// IntentActionResult is the typed result of a high-level operator intent.
type IntentActionResult struct {
	Intent    string          `json:"intent"`
	Message   string          `json:"message,omitempty"`
	Workspace *WorkspaceEntry `json:"workspace,omitempty"`
	Session   *SessionEntry   `json:"session,omitempty"`
	Handoff   *HandoffPacket  `json:"handoff,omitempty"`
}

// ChannelHandler processes neutral channel messages.
type ChannelHandler interface {
	HandleMessage(ctx context.Context, msg ChannelMessage) (ChannelResponse, error)
}

// MessagingGateway is the abstraction for a link provider (Telegram, WhatsApp, etc.).
// It bridges asynchronous messaging apps into the Matrix core by forwarding received messages
// into the SessionRouter and writing back the responses.
type MessagingGateway interface {
	// Start connects to the messaging platform (e.g. via long polling or starting a webhook server).
	Start(ctx context.Context) error

	// Stop disconnects the gateway and cleans up resources.
	Stop() error
}

// ConversationRouter is the minimal ingress contract for sending one user turn into Matrix.
type ConversationRouter interface {
	Route(ctx context.Context, channelID string, agentID string, input string, notifier ThoughtNotifier) (string, error)
}

// ConversationRequestRouter is the richer ingress contract for callers that can
// provide workspace hints in addition to the channel and message payload.
type ConversationRequestRouter interface {
	RouteConversation(ctx context.Context, req ConversationRequest) (string, error)
}

// SessionRouter routes messages from an external channel to an agent via the SSOT Vault
// and optionally exposes typed session lifecycle controls.
type SessionRouter interface {
	ConversationRouter
	HandleSessionAction(ctx context.Context, channelID, action, target string) (string, error)
	HandleSessionActionTyped(ctx context.Context, req SessionActionRequest) (SessionActionResult, error)
	HandleWorkspaceAction(ctx context.Context, channelID, action, target string) (string, error)
	HandleWorkspaceActionTyped(ctx context.Context, req WorkspaceActionRequest) (WorkspaceActionResult, error)
	HandleWorkspaceRead(ctx context.Context, channelID, action, workspaceID string, limit int) (string, error)
	HandleWorkspaceReadTyped(ctx context.Context, req WorkspaceReadRequest) (WorkspaceReadResult, error)
	HandleIntent(ctx context.Context, channelID, intent, target string) (string, error)
	HandleIntentTyped(ctx context.Context, req IntentActionRequest) (IntentActionResult, error)
}

// AuthCallbackHandler handles provider-auth callback flows initiated by the channel/runtime layer.
type AuthCallbackHandler interface {
	HandleAuthCallback(channelID, provider, code string) (string, error)
}
