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
	LoadSession(ctx context.Context, req LoadSessionRequest, observer SessionObserver) (*LoadSessionResponse, error)
	ResumeSession(ctx context.Context, req ResumeSessionRequest) (*ResumeSessionResponse, error)
	ListSessions(ctx context.Context) (*ListSessionsResponse, error)
	ListSessionsWithRequest(ctx context.Context, req ListSessionsRequest) (*ListSessionsResponse, error)
	CancelSession(ctx context.Context, sessionID string) error
	CancelRequest(ctx context.Context, req CancelRequestNotification) error
	CloseSession(ctx context.Context, sessionID string) error
	DeleteSession(ctx context.Context, sessionID string) error
	ForkSession(ctx context.Context, req ForkSessionRequest) (*ForkSessionResponse, error)
	Prompt(ctx context.Context, req PromptRequest, observer SessionObserver) (*PromptResponse, error)
	SetRequestHandler(handler RequestHandler)
	SetMode(ctx context.Context, sessionID, modeID string) error
	SetConfigOption(ctx context.Context, req SetSessionConfigOptionRequest) (*SetSessionConfigOptionResponse, error)
	SetSessionModel(ctx context.Context, req SetSessionModelRequest) (*SetSessionModelResponse, error)
	ListProviders(ctx context.Context, req ListProvidersRequest) (*ListProvidersResponse, error)
	SetProvider(ctx context.Context, req SetProvidersRequest) (*SetProvidersResponse, error)
	DisableProvider(ctx context.Context, req DisableProvidersRequest) (*DisableProvidersResponse, error)
	Logout(ctx context.Context, req LogoutRequest) (*LogoutResponse, error)
	ExtRequest(ctx context.Context, method string, params interface{}, result interface{}) error
	ExtNotification(ctx context.Context, method string, params interface{}) error
}

type InitializeRequest struct {
	ProtocolVersion    int                    `json:"protocolVersion"`
	ClientInfo         map[string]interface{} `json:"clientInfo"`
	ClientCapabilities *ClientCapabilities    `json:"clientCapabilities,omitempty"`
}

type ClientCapabilities struct {
	Fs                *FsCapability          `json:"fs,omitempty"`
	Terminal          bool                   `json:"terminal,omitempty"`
	Auth              *AuthCapabilities      `json:"auth,omitempty"`
	Elicitation       map[string]interface{} `json:"elicitation,omitempty"`
	Nes               map[string]interface{} `json:"nes,omitempty"`
	PositionEncodings []string               `json:"positionEncodings,omitempty"`
	Meta              map[string]interface{} `json:"_meta,omitempty"`
}

type AuthCapabilities struct {
	Terminal bool                   `json:"terminal,omitempty"`
	Meta     map[string]interface{} `json:"_meta,omitempty"`
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
	Type        string                 `json:"type,omitempty"`
	ID          string                 `json:"id,omitempty"`
	Name        string                 `json:"name,omitempty"`
	Description string                 `json:"description,omitempty"`
	Link        string                 `json:"link,omitempty"`
	Vars        []AuthEnvVar           `json:"vars,omitempty"`
	Args        []string               `json:"args,omitempty"`
	Env         map[string]string      `json:"env,omitempty"`
	EnvVar      string                 `json:"envVar,omitempty"`
	Meta        map[string]interface{} `json:"_meta,omitempty"`
}

type AuthEnvVar struct {
	Name     string                 `json:"name"`
	Label    string                 `json:"label,omitempty"`
	Optional bool                   `json:"optional,omitempty"`
	Secret   bool                   `json:"secret,omitempty"`
	Meta     map[string]interface{} `json:"_meta,omitempty"`
}

type NewSessionRequest struct {
	ClientTitle           string                 `json:"clientTitle,omitempty"`
	Cwd                   string                 `json:"cwd"`
	AdditionalDirectories []string               `json:"additionalDirectories,omitempty"`
	McpServers            []McpServerConfig      `json:"mcpServers"`
	Tools                 []Tool                 `json:"tools,omitempty"`
	Meta                  map[string]interface{} `json:"_meta,omitempty"`
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
	SessionID     string                 `json:"sessionId"`
	Modes         *SessionModeState      `json:"modes,omitempty"`
	ConfigOptions []ConfigOption         `json:"configOptions,omitempty"`
	Models        *SessionModelState     `json:"models,omitempty"`
	Meta          map[string]interface{} `json:"_meta,omitempty"`
}

type LoadSessionRequest struct {
	SessionID             string                 `json:"sessionId"`
	Cwd                   string                 `json:"cwd"`
	AdditionalDirectories []string               `json:"additionalDirectories,omitempty"`
	McpServers            []McpServerConfig      `json:"mcpServers"`
	Meta                  map[string]interface{} `json:"_meta,omitempty"`
}

type LoadSessionResponse struct {
	Modes         *SessionModeState      `json:"modes,omitempty"`
	ConfigOptions []ConfigOption         `json:"configOptions,omitempty"`
	Meta          map[string]interface{} `json:"_meta,omitempty"`
}

type ResumeSessionRequest struct {
	SessionID             string                 `json:"sessionId"`
	Cwd                   string                 `json:"cwd"`
	AdditionalDirectories []string               `json:"additionalDirectories,omitempty"`
	McpServers            []McpServerConfig      `json:"mcpServers"`
	Meta                  map[string]interface{} `json:"_meta,omitempty"`
}

type ResumeSessionResponse struct {
	Modes         *SessionModeState      `json:"modes,omitempty"`
	ConfigOptions []ConfigOption         `json:"configOptions,omitempty"`
	Meta          map[string]interface{} `json:"_meta,omitempty"`
}

type ForkSessionRequest struct {
	SessionID             string                 `json:"sessionId"`
	Cwd                   string                 `json:"cwd"`
	AdditionalDirectories []string               `json:"additionalDirectories,omitempty"`
	McpServers            []McpServerConfig      `json:"mcpServers"`
	Meta                  map[string]interface{} `json:"_meta,omitempty"`
}

type ForkSessionResponse struct {
	SessionID     string                 `json:"sessionId"`
	Modes         *SessionModeState      `json:"modes,omitempty"`
	ConfigOptions []ConfigOption         `json:"configOptions,omitempty"`
	Models        *SessionModelState     `json:"models,omitempty"`
	Meta          map[string]interface{} `json:"_meta,omitempty"`
}

type ListSessionsRequest struct {
	Cwd    string                 `json:"cwd,omitempty"`
	Cursor string                 `json:"cursor,omitempty"`
	Meta   map[string]interface{} `json:"_meta,omitempty"`
}

type ListSessionsResponse struct {
	Sessions   []SessionInfo          `json:"sessions"`
	NextCursor string                 `json:"nextCursor,omitempty"`
	Meta       map[string]interface{} `json:"_meta,omitempty"`
}

type SessionInfo struct {
	SessionID             string                 `json:"sessionId"`
	Cwd                   string                 `json:"cwd,omitempty"`
	AdditionalDirectories []string               `json:"additionalDirectories,omitempty"`
	Title                 string                 `json:"title,omitempty"`
	UpdatedAt             string                 `json:"updatedAt,omitempty"`
	Meta                  map[string]interface{} `json:"_meta,omitempty"`
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

type SessionModelState struct {
	CurrentModelID  string                 `json:"currentModelId"`
	AvailableModels []ModelInfo            `json:"availableModels"`
	Meta            map[string]interface{} `json:"_meta,omitempty"`
}

type ModelInfo struct {
	ModelID     string                 `json:"modelId"`
	Name        string                 `json:"name"`
	Description string                 `json:"description,omitempty"`
	Meta        map[string]interface{} `json:"_meta,omitempty"`
}

type ConfigOption struct {
	ID          string                 `json:"id"`
	Name        string                 `json:"name"`
	Description string                 `json:"description,omitempty"`
	Category    string                 `json:"category,omitempty"`
	Type        string                 `json:"type,omitempty"`
	Options     []ConfigOptionValue    `json:"options,omitempty"`
	Current     string                 `json:"currentValue,omitempty"`
	Meta        map[string]interface{} `json:"_meta,omitempty"`
}

type ConfigOptionValue struct {
	ID          string                 `json:"value"`
	Name        string                 `json:"name"`
	Description string                 `json:"description,omitempty"`
	Meta        map[string]interface{} `json:"_meta,omitempty"`
}

func (o *ConfigOption) UnmarshalJSON(data []byte) error {
	type rawOption struct {
		ID           string                 `json:"id"`
		Name         string                 `json:"name"`
		Description  string                 `json:"description,omitempty"`
		Category     string                 `json:"category,omitempty"`
		Type         string                 `json:"type,omitempty"`
		Options      json.RawMessage        `json:"options,omitempty"`
		CurrentValue string                 `json:"currentValue,omitempty"`
		Current      string                 `json:"current,omitempty"`
		Meta         map[string]interface{} `json:"_meta,omitempty"`
	}
	var raw rawOption
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	o.ID = raw.ID
	o.Name = raw.Name
	o.Description = raw.Description
	o.Category = raw.Category
	o.Type = raw.Type
	o.Current = raw.CurrentValue
	if o.Current == "" {
		o.Current = raw.Current
	}
	o.Meta = raw.Meta
	o.Options = decodeConfigOptions(raw.Options)
	return nil
}

func (v *ConfigOptionValue) UnmarshalJSON(data []byte) error {
	type rawValue struct {
		ID          string                 `json:"id,omitempty"`
		Value       string                 `json:"value,omitempty"`
		Name        string                 `json:"name"`
		Description string                 `json:"description,omitempty"`
		Meta        map[string]interface{} `json:"_meta,omitempty"`
	}
	var raw rawValue
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	v.ID = raw.Value
	if v.ID == "" {
		v.ID = raw.ID
	}
	v.Name = raw.Name
	v.Description = raw.Description
	v.Meta = raw.Meta
	return nil
}

func decodeConfigOptions(data json.RawMessage) []ConfigOptionValue {
	if len(data) == 0 || string(data) == "null" {
		return nil
	}
	var values []ConfigOptionValue
	if err := json.Unmarshal(data, &values); err == nil && looksLikeConfigValues(values) {
		return values
	}
	var groups []struct {
		Options []ConfigOptionValue `json:"options"`
	}
	if err := json.Unmarshal(data, &groups); err != nil {
		return nil
	}
	for _, group := range groups {
		values = append(values, group.Options...)
	}
	return values
}

func looksLikeConfigValues(values []ConfigOptionValue) bool {
	for _, value := range values {
		if value.ID != "" || value.Name != "" {
			return true
		}
	}
	return len(values) == 0
}

type SetSessionConfigOptionRequest struct {
	SessionID string                 `json:"sessionId"`
	ConfigID  string                 `json:"configId"`
	Value     string                 `json:"value"`
	Meta      map[string]interface{} `json:"_meta,omitempty"`
}

type SetSessionConfigOptionResponse struct {
	ConfigOptions []ConfigOption         `json:"configOptions,omitempty"`
	Meta          map[string]interface{} `json:"_meta,omitempty"`
}

type SetSessionModelRequest struct {
	SessionID string                 `json:"sessionId"`
	ModelID   string                 `json:"modelId"`
	Meta      map[string]interface{} `json:"_meta,omitempty"`
}

type SetSessionModelResponse struct {
	Meta map[string]interface{} `json:"_meta,omitempty"`
}

type ListProvidersRequest struct {
	Meta map[string]interface{} `json:"_meta,omitempty"`
}

type ListProvidersResponse struct {
	Providers []ProviderInfo         `json:"providers"`
	Meta      map[string]interface{} `json:"_meta,omitempty"`
}

type ProviderInfo struct {
	ID        string                 `json:"id"`
	Supported []string               `json:"supported"`
	Required  bool                   `json:"required"`
	Current   *ProviderCurrentConfig `json:"current,omitempty"`
	Meta      map[string]interface{} `json:"_meta,omitempty"`
}

type ProviderCurrentConfig struct {
	APIType string `json:"apiType"`
	BaseURL string `json:"baseUrl"`
}

type SetProvidersRequest struct {
	ID      string                 `json:"id"`
	APIType string                 `json:"apiType"`
	BaseURL string                 `json:"baseUrl"`
	Headers map[string]string      `json:"headers,omitempty"`
	Meta    map[string]interface{} `json:"_meta,omitempty"`
}

type SetProvidersResponse struct {
	Meta map[string]interface{} `json:"_meta,omitempty"`
}

type DisableProvidersRequest struct {
	ID   string                 `json:"id"`
	Meta map[string]interface{} `json:"_meta,omitempty"`
}

type DisableProvidersResponse struct {
	Meta map[string]interface{} `json:"_meta,omitempty"`
}

type LogoutRequest struct {
	Meta map[string]interface{} `json:"_meta,omitempty"`
}

type LogoutResponse struct {
	Meta map[string]interface{} `json:"_meta,omitempty"`
}

type CancelRequestNotification struct {
	RequestID interface{}            `json:"requestId"`
	Meta      map[string]interface{} `json:"_meta,omitempty"`
}

type PromptRequest struct {
	SessionID string                 `json:"sessionId"`
	Prompt    []Content              `json:"prompt"`
	MessageID string                 `json:"messageId,omitempty"`
	Meta      map[string]interface{} `json:"_meta,omitempty"`
}

type PromptResponse struct {
	StopReason    string                 `json:"stopReason"`
	ToolCalls     []ToolCall             `json:"toolCalls,omitempty"`
	Usage         map[string]interface{} `json:"usage,omitempty"`
	UserMessageID string                 `json:"userMessageId,omitempty"`
	Meta          map[string]interface{} `json:"_meta,omitempty"`
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
	SessionUpdate     string                 `json:"sessionUpdate"`
	Content           Content                `json:"-"`
	Contents          []Content              `json:"-"`
	ToolContents      []ToolCallContent      `json:"-"`
	RawContent        json.RawMessage        `json:"-"`
	Title             string                 `json:"title,omitempty"`
	UpdatedAt         string                 `json:"updatedAt,omitempty"`
	ToolCallID        string                 `json:"toolCallId,omitempty"`
	Kind              string                 `json:"kind,omitempty"`
	Status            string                 `json:"status,omitempty"`
	RawInput          map[string]interface{} `json:"rawInput,omitempty"`
	RawOutput         interface{}            `json:"rawOutput,omitempty"`
	Locations         []interface{}          `json:"locations,omitempty"`
	Entries           []PlanEntry            `json:"entries,omitempty"`
	AvailableCommands []AvailableCommand     `json:"availableCommands,omitempty"`
	CurrentModeID     string                 `json:"currentModeId,omitempty"`
	ConfigOptions     []ConfigOption         `json:"configOptions,omitempty"`
	Usage             map[string]interface{} `json:"usage,omitempty"`
	Meta              map[string]interface{} `json:"_meta,omitempty"`
}

func (u *SessionUpdate) UnmarshalJSON(data []byte) error {
	type rawUpdate struct {
		SessionUpdate     string                 `json:"sessionUpdate"`
		Content           json.RawMessage        `json:"content,omitempty"`
		Title             string                 `json:"title,omitempty"`
		UpdatedAt         string                 `json:"updatedAt,omitempty"`
		ToolCallID        string                 `json:"toolCallId,omitempty"`
		Kind              string                 `json:"kind,omitempty"`
		Status            string                 `json:"status,omitempty"`
		RawInput          map[string]interface{} `json:"rawInput,omitempty"`
		RawOutput         interface{}            `json:"rawOutput,omitempty"`
		Locations         []interface{}          `json:"locations,omitempty"`
		Entries           []PlanEntry            `json:"entries,omitempty"`
		AvailableCommands []AvailableCommand     `json:"availableCommands,omitempty"`
		CurrentModeID     string                 `json:"currentModeId,omitempty"`
		ConfigOptions     []ConfigOption         `json:"configOptions,omitempty"`
		Usage             map[string]interface{} `json:"usage,omitempty"`
		Meta              map[string]interface{} `json:"_meta,omitempty"`
	}
	var raw rawUpdate
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	u.SessionUpdate = raw.SessionUpdate
	u.Title = raw.Title
	u.UpdatedAt = raw.UpdatedAt
	u.ToolCallID = raw.ToolCallID
	u.Kind = raw.Kind
	u.Status = raw.Status
	u.RawInput = raw.RawInput
	u.RawOutput = raw.RawOutput
	u.Locations = raw.Locations
	u.Entries = raw.Entries
	u.AvailableCommands = raw.AvailableCommands
	u.CurrentModeID = raw.CurrentModeID
	u.ConfigOptions = raw.ConfigOptions
	u.Usage = raw.Usage
	u.Meta = raw.Meta
	u.RawContent = cloneRawMessage(raw.Content)
	u.Content, u.Contents, u.ToolContents = decodeUpdateContent(raw.Content)
	return nil
}

func (u SessionUpdate) MarshalJSON() ([]byte, error) {
	type rawUpdate struct {
		SessionUpdate     string                 `json:"sessionUpdate"`
		Content           interface{}            `json:"content,omitempty"`
		Title             string                 `json:"title,omitempty"`
		UpdatedAt         string                 `json:"updatedAt,omitempty"`
		ToolCallID        string                 `json:"toolCallId,omitempty"`
		Kind              string                 `json:"kind,omitempty"`
		Status            string                 `json:"status,omitempty"`
		RawInput          map[string]interface{} `json:"rawInput,omitempty"`
		RawOutput         interface{}            `json:"rawOutput,omitempty"`
		Locations         []interface{}          `json:"locations,omitempty"`
		Entries           []PlanEntry            `json:"entries,omitempty"`
		AvailableCommands []AvailableCommand     `json:"availableCommands,omitempty"`
		CurrentModeID     string                 `json:"currentModeId,omitempty"`
		ConfigOptions     []ConfigOption         `json:"configOptions,omitempty"`
		Usage             map[string]interface{} `json:"usage,omitempty"`
		Meta              map[string]interface{} `json:"_meta,omitempty"`
	}
	content := encodeUpdateContent(u.Content, u.Contents, u.ToolContents, u.RawContent)
	return json.Marshal(rawUpdate{
		SessionUpdate:     u.SessionUpdate,
		Content:           content,
		Title:             u.Title,
		UpdatedAt:         u.UpdatedAt,
		ToolCallID:        u.ToolCallID,
		Kind:              u.Kind,
		Status:            u.Status,
		RawInput:          u.RawInput,
		RawOutput:         u.RawOutput,
		Locations:         u.Locations,
		Entries:           u.Entries,
		AvailableCommands: u.AvailableCommands,
		CurrentModeID:     u.CurrentModeID,
		ConfigOptions:     u.ConfigOptions,
		Usage:             u.Usage,
		Meta:              u.Meta,
	})
}

func cloneRawMessage(data json.RawMessage) json.RawMessage {
	if len(data) == 0 {
		return nil
	}
	return append(json.RawMessage(nil), data...)
}

func decodeUpdateContent(data json.RawMessage) (Content, []Content, []ToolCallContent) {
	if len(data) == 0 || string(data) == "null" {
		return Content{}, nil, nil
	}
	var one Content
	if err := json.Unmarshal(data, &one); err == nil && one.Type != "" {
		if looksLikeContentBlock(one) {
			return one, []Content{one}, nil
		}
	}
	var many []Content
	if err := json.Unmarshal(data, &many); err == nil && looksLikeContentBlocks(many) {
		return many[0], many, nil
	}
	if toolContents := decodeToolCallContents(data); len(toolContents) > 0 {
		contents := contentsFromToolCallContents(toolContents)
		if len(contents) > 0 {
			return contents[0], contents, toolContents
		}
		return Content{}, nil, toolContents
	}
	return Content{}, nil, nil
}

func looksLikeContentBlock(content Content) bool {
	switch content.Type {
	case "text", "image", "audio", "resource", "resource_link":
		return true
	default:
		return false
	}
}

func looksLikeContentBlocks(contents []Content) bool {
	if len(contents) == 0 {
		return false
	}
	for _, content := range contents {
		if !looksLikeContentBlock(content) {
			return false
		}
	}
	return true
}

func decodeToolCallContents(data json.RawMessage) []ToolCallContent {
	var one ToolCallContent
	if err := json.Unmarshal(data, &one); err == nil && one.Type != "" {
		return []ToolCallContent{one}
	}
	var many []ToolCallContent
	if err := json.Unmarshal(data, &many); err != nil || len(many) == 0 {
		return nil
	}
	for _, content := range many {
		if content.Type == "" {
			return nil
		}
	}
	return many
}

func contentsFromToolCallContents(toolContents []ToolCallContent) []Content {
	if len(toolContents) == 0 {
		return nil
	}
	contents := make([]Content, 0, len(toolContents))
	for _, item := range toolContents {
		if item.Type == "content" && item.Content != nil && item.Content.Type != "" {
			contents = append(contents, *item.Content)
		}
	}
	return contents
}

func encodeUpdateContent(one Content, many []Content, toolContents []ToolCallContent, raw json.RawMessage) interface{} {
	if len(toolContents) > 0 {
		return oneOrManyToolContents(toolContents)
	}
	if len(many) > 1 {
		return many
	}
	if hasContentValue(one) {
		return one
	}
	if len(many) == 1 {
		return many[0]
	}
	return decodeRawContent(raw)
}

func oneOrManyToolContents(contents []ToolCallContent) interface{} {
	if len(contents) == 1 {
		return contents[0]
	}
	return contents
}

func hasContentValue(content Content) bool {
	return content.Type != "" || content.Text != "" || content.Resource != nil || content.URI != ""
}

func decodeRawContent(raw json.RawMessage) interface{} {
	if len(raw) == 0 || string(raw) == "null" {
		return nil
	}
	var decoded interface{}
	if err := json.Unmarshal(raw, &decoded); err == nil {
		return decoded
	}
	return nil
}

type PlanEntry struct {
	Content  string                 `json:"content"`
	Priority string                 `json:"priority"`
	Status   string                 `json:"status"`
	Meta     map[string]interface{} `json:"_meta,omitempty"`
}

type AvailableCommand struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	Input       map[string]interface{} `json:"input,omitempty"`
	Meta        map[string]interface{} `json:"_meta,omitempty"`
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

// ToolCallContent preserves ACP tool-call payload variants losslessly enough for
// Matrix projections while keeping the public package independent from codegen.
type ToolCallContent struct {
	Type       string                 `json:"type"`
	Content    *Content               `json:"content,omitempty"`
	Path       string                 `json:"path,omitempty"`
	OldText    *string                `json:"oldText,omitempty"`
	NewText    string                 `json:"newText,omitempty"`
	TerminalID string                 `json:"terminalId,omitempty"`
	Meta       map[string]interface{} `json:"_meta,omitempty"`
}
