package middleware

// ContextCompaction is the current view of one provider-owned compaction.
// It is session state, never an assistant answer or evidence of task completion.
// Providers return detached snapshots; readers may retain them across updates.
type ContextCompaction struct {
	ID      string                 `json:"id"`
	Status  string                 `json:"status"`
	Summary []Content              `json:"summary,omitempty"`
	Error   string                 `json:"error,omitempty"`
	Meta    map[string]interface{} `json:"meta,omitempty"`
}

// SessionMetadataPatch represents explicit session-info changes independently
// of protocol and aggregate turn metadata. Nil fields mean unchanged.
type SessionMetadataPatch struct {
	Title         *string
	UpdatedAt     *string
	ClearMetadata bool
	Metadata      map[string]interface{}
}
