package middleware

const (
	SidecarVisibilityLLMVisible = "llm_visible"
	SidecarVisibilityTraceOnly  = "trace_only"
	SidecarFormatNoemaXML       = "noema_xml"
	SidecarFormatText           = "text"
	SidecarA2AExtensionURI      = "https://matrix.dev/extensions/sidecar/v0"
)

// SidecarCapsule is optional machine-trackable context supplied alongside a
// user task. Matrix preserves it without interpreting provider semantics.
type SidecarCapsule struct {
	Provider   string                 `json:"provider,omitempty"`
	ID         string                 `json:"id,omitempty"`
	Schema     string                 `json:"schema,omitempty"`
	Version    string                 `json:"version,omitempty"`
	Visibility string                 `json:"visibility,omitempty"`
	Format     string                 `json:"format,omitempty"`
	Content    string                 `json:"content,omitempty"`
	Metadata   map[string]interface{} `json:"metadata,omitempty"`
}
