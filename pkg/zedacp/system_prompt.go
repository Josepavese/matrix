package zedacp

// SystemPromptMode is how an agent combines a client-provided system prompt
// with its own instructions. The stable specification admits future modes, so
// this is a string type rather than a closed enum: a sender only uses a mode
// the agent advertised, and an unknown mode is preserved rather than coerced.
type SystemPromptMode string

const (
	SystemPromptModeAppend   SystemPromptMode = "append"
	SystemPromptModeOverride SystemPromptMode = "override"
)

// SystemPrompt is the stable ACP v1 client-provided system-level instruction
// set for a session. A client must only send it when the agent advertises
// sessionCapabilities.systemPrompt, and must use a mode the agent advertised.
type SystemPrompt struct {
	Mode    SystemPromptMode       `json:"mode"`
	Content []Content              `json:"content"`
	Meta    map[string]interface{} `json:"_meta,omitempty"`
}

// SessionSystemPromptCapabilities reports client-provided system prompt support.
// Its presence means append mode is supported; override is a nested marker.
type SessionSystemPromptCapabilities struct {
	Mode *SessionSystemPromptModeCapabilities `json:"mode,omitempty"`
	Meta map[string]interface{}               `json:"_meta,omitempty"`
}

type SessionSystemPromptModeCapabilities struct {
	Override *SessionSystemPromptOverrideCapabilities `json:"override,omitempty"`
	Meta     map[string]interface{}                   `json:"_meta,omitempty"`
}

// SessionSystemPromptOverrideCapabilities is intentionally empty: in ACP the
// presence of a non-null object advertises the mode.
type SessionSystemPromptOverrideCapabilities struct {
	Meta map[string]interface{} `json:"_meta,omitempty"`
}
