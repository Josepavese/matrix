package middleware

// SessionCleanupRelatedSession records session/process state that was touched by
// a run-level lifecycle but is not the primary cleanup target.
type SessionCleanupRelatedSession struct {
	LogicalSessionID string `json:"logical_session_id,omitempty"`
	RemoteSessionID  string `json:"remote_session_id,omitempty"`
	AgentID          string `json:"agent_id,omitempty"`
	ProtocolKind     string `json:"protocol_kind,omitempty"`
	WorkspaceID      string `json:"workspace_id,omitempty"`
	WorkspacePath    string `json:"workspace_path,omitempty"`
	ParentSessionID  string `json:"parent_session_id,omitempty"`
	ParentRemoteID   string `json:"parent_remote_id,omitempty"`
	Ephemeral        bool   `json:"ephemeral,omitempty"`
	CleanupPolicy    string `json:"cleanup_policy,omitempty"`
	Active           bool   `json:"active,omitempty"`
	Retained         bool   `json:"retained"`
	Reason           string `json:"reason,omitempty"`
}
