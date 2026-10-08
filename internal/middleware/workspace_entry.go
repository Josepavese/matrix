package middleware

// WorkspaceEntry is a logical workspace view with optional live capacity.
type WorkspaceEntry struct {
	Capacity        *CapacitySnapshot `json:"capacity,omitempty"`
	ID              string            `json:"id,omitempty"`
	Name            string            `json:"name,omitempty"`
	Kind            string            `json:"kind,omitempty"`
	RootPath        string            `json:"root_path,omitempty"`
	DefaultAgentID  string            `json:"default_agent_id,omitempty"`
	ReviewerAgentID string            `json:"reviewer_agent_id,omitempty"`
	DefaultMode     string            `json:"default_mode,omitempty"`
	PolicyProfile   string            `json:"policy_profile,omitempty"`
	Active          bool              `json:"active,omitempty"`
}
