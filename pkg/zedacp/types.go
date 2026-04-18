package zedacp

import (
	"context"
	"encoding/json"
)

// Transport abstracts a bidirectional JSON-RPC 2.0 stream.
type Transport interface {
	Send(ctx context.Context, message []byte) error
	Receive(ctx context.Context) ([]byte, error)
	Close() error
}

// SessionObserver receives asynchronous updates during a prompt execution.
type SessionObserver interface {
	OnUpdate(notification SessionNotification)
}

// RequestHandler handles incoming JSON-RPC requests from the agent.
type RequestHandler interface {
	HandleRequest(ctx context.Context, method string, params json.RawMessage) (interface{}, error)
}

// ClientAPI defines the typed ACP methods exposed by the client.
type ClientAPI interface {
	Initialize(ctx context.Context, req InitializeRequest) (*InitializeResponse, error)
	Authenticate(ctx context.Context, methodID string, credentials map[string]string) error
	NewSession(ctx context.Context, req NewSessionRequest) (*NewSessionResponse, error)
	LoadSession(ctx context.Context, req LoadSessionRequest, observer SessionObserver) error
	ListSessions(ctx context.Context) (*ListSessionsResponse, error)
	CancelSession(ctx context.Context, sessionID string) error
	CloseSession(ctx context.Context, sessionID string) error
	DeleteSession(ctx context.Context, sessionID string) error
	Prompt(ctx context.Context, req PromptRequest, observer SessionObserver) (*PromptResponse, error)
	SetRequestHandler(handler RequestHandler)
	SetMode(ctx context.Context, sessionID, modeID string) error
}

type InitializeRequest struct {
	ProtocolVersion    int                    `json:"protocolVersion"`
	ClientInfo         map[string]interface{} `json:"clientInfo"`
	ClientCapabilities *ClientCapabilities    `json:"clientCapabilities,omitempty"`
}

type ClientCapabilities struct {
	Fs       *FsCapability `json:"fs,omitempty"`
	Terminal bool          `json:"terminal,omitempty"`
}

type FsCapability struct {
	ReadTextFile  bool `json:"readTextFile"`
	WriteTextFile bool `json:"writeTextFile"`
}

// InitializeResponse accepts both the current `agentCapabilities` field name and
// the older compatibility field name `capabilities`.
type InitializeResponse struct {
	ProtocolVersion int                    `json:"protocolVersion,omitempty"`
	AgentInfo       map[string]interface{} `json:"agentInfo,omitempty"`
	Capabilities    map[string]interface{} `json:"-"`
	AuthMethods     []AuthMethod           `json:"authMethods,omitempty"`
}

func (r *InitializeResponse) UnmarshalJSON(data []byte) error {
	type alias struct {
		ProtocolVersion   int                    `json:"protocolVersion,omitempty"`
		AgentInfo         map[string]interface{} `json:"agentInfo,omitempty"`
		AgentCapabilities map[string]interface{} `json:"agentCapabilities,omitempty"`
		Capabilities      map[string]interface{} `json:"capabilities,omitempty"`
		AuthMethods       []AuthMethod           `json:"authMethods,omitempty"`
	}
	var raw alias
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	r.ProtocolVersion = raw.ProtocolVersion
	r.AgentInfo = raw.AgentInfo
	r.AuthMethods = raw.AuthMethods
	if raw.AgentCapabilities != nil {
		r.Capabilities = raw.AgentCapabilities
	} else {
		r.Capabilities = raw.Capabilities
	}
	return nil
}

type AuthMethod struct {
	Type        string `json:"type"`
	ID          string `json:"id,omitempty"`
	Description string `json:"description,omitempty"`
	EnvVar      string `json:"envVar,omitempty"`
}

type NewSessionRequest struct {
	ClientTitle string            `json:"clientTitle,omitempty"`
	Cwd         string            `json:"cwd"`
	McpServers  []McpServerConfig `json:"mcpServers"`
	Tools       []Tool            `json:"tools,omitempty"`
}

type McpServerConfig struct {
	Name    string   `json:"name"`
	Type    string   `json:"type,omitempty"`
	Command string   `json:"command,omitempty"`
	Args    []string `json:"args,omitempty"`
	Env     []EnvVar `json:"env,omitempty"`
	URL     string   `json:"url,omitempty"`
	Headers []Header `json:"headers,omitempty"`
}

type EnvVar struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

type Header struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

type Tool struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	InputSchema map[string]interface{} `json:"inputSchema"`
}

type NewSessionResponse struct {
	SessionID     string            `json:"sessionId"`
	Modes         *SessionModeState `json:"modes,omitempty"`
	ConfigOptions []ConfigOption    `json:"configOptions,omitempty"`
}

type LoadSessionRequest struct {
	SessionID  string            `json:"sessionId"`
	Cwd        string            `json:"cwd"`
	McpServers []McpServerConfig `json:"mcpServers"`
}

type ListSessionsResponse struct {
	Sessions []SessionInfo `json:"sessions"`
}

type SessionInfo struct {
	SessionID string                 `json:"sessionId"`
	Title     string                 `json:"title,omitempty"`
	UpdatedAt string                 `json:"updatedAt,omitempty"`
	Meta      map[string]interface{} `json:"_meta,omitempty"`
}

type SessionModeState struct {
	CurrentModeID  string        `json:"currentModeId"`
	AvailableModes []SessionMode `json:"availableModes"`
}

type SessionMode struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
}

type ConfigOption struct {
	ID       string              `json:"id"`
	Name     string              `json:"name"`
	Category string              `json:"category"`
	Options  []ConfigOptionValue `json:"options"`
	Current  string              `json:"current,omitempty"`
}

type ConfigOptionValue struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type PromptRequest struct {
	SessionID string    `json:"sessionId"`
	Prompt    []Content `json:"prompt"`
}

type PromptResponse struct {
	StopReason string     `json:"stopReason"`
	ToolCalls  []ToolCall `json:"toolCalls,omitempty"`
}

type ToolCall struct {
	ID       string           `json:"id"`
	Type     string           `json:"type"`
	Function ToolCallFunction `json:"function"`
}

type ToolCallFunction struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type SessionUpdate struct {
	SessionUpdate string                 `json:"sessionUpdate"`
	Content       Content                `json:"content"`
	Title         string                 `json:"title,omitempty"`
	UpdatedAt     string                 `json:"updatedAt,omitempty"`
	Meta          map[string]interface{} `json:"_meta,omitempty"`
}

type SessionNotification struct {
	SessionID string        `json:"sessionId"`
	Update    SessionUpdate `json:"update"`
}

type Message struct {
	Role    string    `json:"role"`
	Content []Content `json:"content"`
}

type Content struct {
	Type string `json:"type"`
	Text string `json:"text"`
}
