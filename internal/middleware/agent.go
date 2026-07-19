// Package middleware defines shared interfaces and domain types for the Matrix runtime.
package middleware

import (
	"context"
)

// AgentTransport abstracts a bidirectional stream for JSON-RPC 2.0 messages (e.g. Stdio, WebSocket).
type AgentTransport interface {
	// Send sends a raw JSON byte slice over the stream.
	Send(ctx context.Context, message []byte) error
	// Receive reads a raw JSON byte slice from the stream. Returns error on closed/EOF.
	Receive(ctx context.Context) ([]byte, error)
	// Close terminates the stream.
	Close() error
}

// AgentRouter is responsible for orchestrating the complete lifecycle (Init -> Session -> Prompt) for a given agent.
type AgentRouter interface {
	// Route executes a full prompt turn, delegating to the proper AgentClient.
	// It returns the response text, the updated AgentSessionID, and any requested ToolCalls.
	Route(ctx context.Context, req RouteRequest) (string, string, []ToolCall, ConversationMetadata, error)
}

// RouteRequest contains the parameters for routing a prompt to an agent.
type RouteRequest struct {
	AgentID                  string
	LogicalSessionID         string
	AgentSessionID           string
	WorkspacePath            string
	Message                  string
	ContentBlocks            []Content
	ExtensionURIs            []string
	ReferencedRemoteSessions []string
	SidecarCapsules          []SidecarCapsule
	Tools                    []Tool
	McpServers               []McpServerConfig
	AdditionalDirectories    []string
	AgentLaunchArgs          []string
	ThoughtNotifier          ThoughtNotifier // optional: receives real-time thought/tool updates during prompt
	StrictSession            bool            // do not recover by creating a replacement remote session
	LiveContextAttach        bool            // true when this route is a best-effort live context injection
}

// ThoughtUpdateType classifies the kind of intermediate update from an agent.
type ThoughtUpdateType int

const (
	// ThoughtTypeThinking indicates the agent is reasoning.
	ThoughtTypeThinking ThoughtUpdateType = iota
	// ThoughtTypeToolCall indicates the agent is invoking a tool.
	ThoughtTypeToolCall
	// ThoughtTypeToolResult indicates a tool execution result.
	ThoughtTypeToolResult
	// ThoughtTypePermission indicates an agent permission request or decision.
	ThoughtTypePermission
)

// ThoughtUpdate represents a real-time intermediate update during agent execution.
type ThoughtUpdate struct {
	Type     ThoughtUpdateType
	Content  string
	Title    string
	Metadata map[string]interface{}
}

// ThoughtNotifier receives real-time thought and tool updates during a prompt turn.
// Implementations can show these as temporary UI elements (e.g. a "thinking" message).
type ThoughtNotifier interface {
	OnThought(update ThoughtUpdate)
	// SetHeader provides agent and session metadata for display.
	SetHeader(agentID, agentSessionID string)
	// FormattedHeader returns a platform-specific styled label for the final response.
	// Returns empty string if no header is available.
	FormattedHeader() string
}

// AgentEndpointResolver maps an agent ID to a protocol-neutral endpoint description.
type AgentEndpointResolver interface {
	GetAgentEndpoint(agentID string) (ProtocolEndpoint, error)
}

// NewSessionRequest is the payload sent to create a new agent session.
type NewSessionRequest struct {
	ClientTitle           string                 `json:"clientTitle,omitempty"`
	Cwd                   string                 `json:"cwd"`
	AdditionalDirectories []string               `json:"additionalDirectories,omitempty"`
	McpServers            []McpServerConfig      `json:"mcpServers"`
	Tools                 []Tool                 `json:"tools,omitempty"`
	Meta                  map[string]interface{} `json:"_meta,omitempty"`
}

// McpServerConfig describes an MCP server connection for session/new per ACP spec.
type McpServerConfig struct {
	Name    string   `json:"name"`
	Type    string   `json:"type,omitempty"`    // "stdio" (default) or "http"
	Command string   `json:"command,omitempty"` // for stdio
	Args    []string `json:"args,omitempty"`    // for stdio
	Env     []EnvVar `json:"env,omitempty"`     // for stdio
	URL     string   `json:"url,omitempty"`     // for http
	Headers []Header `json:"headers,omitempty"` // for http
}

// EnvVar is a name/value pair for environment variable injection.
type EnvVar struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

// Header is a name/value pair for HTTP headers.
type Header struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

// Tool describes a tool that can be made available to an agent session.
type Tool struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	InputSchema map[string]interface{} `json:"inputSchema"`
}

// NewSessionResponse contains the session ID and mode information returned after session creation.
type NewSessionResponse struct {
	SessionID     string            `json:"sessionId"`
	Modes         *SessionModeState `json:"modes,omitempty"`
	ConfigOptions []ConfigOption    `json:"configOptions,omitempty"`
}

// SessionModeState represents the current and available modes for a session.
type SessionModeState struct {
	CurrentModeID  string        `json:"currentModeId"`
	AvailableModes []SessionMode `json:"availableModes"`
}

// SessionMode describes an agent-defined mode (e.g. "code", "ask", "architect").
type SessionMode struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
}

// ConfigOption describes a session-level configuration option.
type ConfigOption struct {
	ID          string              `json:"id"`
	Name        string              `json:"name"`
	Description string              `json:"description,omitempty"`
	Category    string              `json:"category,omitempty"`
	Type        string              `json:"type,omitempty"`
	Options     []ConfigOptionValue `json:"options,omitempty"`
	Current     string              `json:"current_value,omitempty"`
}

// ConfigOptionValue is one of the possible values for a ConfigOption.
type ConfigOptionValue struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
}

// PromptRequest is the payload sent to execute a prompt turn in an agent session.
type PromptRequest struct {
	SessionID string    `json:"sessionId"`
	Prompt    []Content `json:"prompt"`
}

// PromptResponse contains the result of a prompt turn, including stop reason and any tool calls.
type PromptResponse struct {
	StopReason string     `json:"stopReason"` // e.g. "end_turn", "tool_calls"
	ToolCalls  []ToolCall `json:"toolCalls,omitempty"`
}

// ToolCall represents a tool invocation requested by the agent.
type ToolCall struct {
	ID       string           `json:"id"`
	Type     string           `json:"type"` // "function"
	Function ToolCallFunction `json:"function"`
}

// ToolCallFunction describes the function name and arguments within a ToolCall.
type ToolCallFunction struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

// SessionUpdate represents a single update event within a session notification.
type SessionUpdate struct {
	SessionUpdate string  `json:"sessionUpdate"` // e.g. "agent_message_chunk"
	Content       Content `json:"content"`       // e.g. {"type":"text","text":"pong"}
}

// SessionNotification is an asynchronous notification sent from the agent during a session.
type SessionNotification struct {
	SessionID string        `json:"sessionId"`
	Update    SessionUpdate `json:"update"`
}

// Message represents a chat message with a role and content blocks.
type Message struct {
	Role    string    `json:"role"`    // "user", "assistant"
	Content []Content `json:"content"` // Array of content blocks
}

// Content is a single content block within a message.
type Content struct {
	Type        string                 `json:"type"`
	Text        string                 `json:"text,omitempty"`
	Data        string                 `json:"data,omitempty"`
	MimeType    string                 `json:"mimeType,omitempty"`
	URI         string                 `json:"uri,omitempty"`
	Name        string                 `json:"name,omitempty"`
	Title       string                 `json:"title,omitempty"`
	Description string                 `json:"description,omitempty"`
	Size        *int64                 `json:"size,omitempty"`
	Resource    map[string]interface{} `json:"resource,omitempty"`
	Annotations map[string]interface{} `json:"annotations,omitempty"`
	Meta        map[string]interface{} `json:"_meta,omitempty"`
}

// SessionEventType classifies the kind of event emitted during a prompt turn.
type SessionEventType int

// SessionEventType constants classify events emitted during a prompt turn.
const (
	EventChunk         SessionEventType = iota // text chunk from agent
	EventToolCall                              // agent is invoking a tool
	EventProgress                              // agent reports progress
	EventChildComplete                         // a child/background task completed
)

// SessionEvent represents a discrete event during agent execution.
type SessionEvent struct {
	Type     SessionEventType
	Content  string
	Metadata map[string]any
}
