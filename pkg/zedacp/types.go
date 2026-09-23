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
	Authenticate(ctx context.Context, methodID string) error
	NewSession(ctx context.Context, req NewSessionRequest) (*NewSessionResponse, error)
	LoadSession(ctx context.Context, req LoadSessionRequest, observer SessionObserver) (*LoadSessionResponse, error)
	ResumeSession(ctx context.Context, req ResumeSessionRequest) (*ResumeSessionResponse, error)
	ListSessions(ctx context.Context) (*ListSessionsResponse, error)
	ListSessionsWithRequest(ctx context.Context, req ListSessionsRequest) (*ListSessionsResponse, error)
	CancelSession(ctx context.Context, sessionID string) error
	CancelRequest(ctx context.Context, req CancelRequestNotification) error
	CloseSession(ctx context.Context, sessionID string) error
	DeleteSession(ctx context.Context, sessionID string) error
	Prompt(ctx context.Context, req PromptRequest, observer SessionObserver) (*PromptResponse, error)
	SetRequestHandler(handler RequestHandler)
	SetMode(ctx context.Context, sessionID, modeID string) error
	SetConfigOption(ctx context.Context, req SetSessionConfigOptionRequest) (*SetSessionConfigOptionResponse, error)
	Logout(ctx context.Context, req LogoutRequest) (*LogoutResponse, error)
	ExtRequest(ctx context.Context, method string, params interface{}, result interface{}) error
	ExtNotification(ctx context.Context, method string, params interface{}) error
}

// ExperimentalClientAPI contains explicitly non-stable ACP v1 operations.
// It is separate so callers cannot mistake draft surfaces for the stable
// ClientAPI contract.
type ExperimentalClientAPI interface {
	ForkSession(ctx context.Context, req ForkSessionRequest) (*ForkSessionResponse, error)
	SetSessionModel(ctx context.Context, req SetSessionModelRequest) (*SetSessionModelResponse, error)
	ListProviders(ctx context.Context, req ListProvidersRequest) (*ListProvidersResponse, error)
	SetProvider(ctx context.Context, req SetProvidersRequest) (*SetProvidersResponse, error)
	DisableProvider(ctx context.Context, req DisableProvidersRequest) (*DisableProvidersResponse, error)
}

type InitializeRequest struct {
	ProtocolVersion    int                    `json:"protocolVersion"`
	ClientInfo         map[string]interface{} `json:"clientInfo"`
	ClientCapabilities *ClientCapabilities    `json:"clientCapabilities,omitempty"`
}

// MarshalJSON emits the parameter names of the generation being requested. ACP v2
// renamed clientCapabilities to capabilities and clientInfo to info, so an agent
// that reads params.capabilities - as the specification it implements says it may -
// would never see a v1-shaped advertisement, and Matrix would appear to support no
// optional surface at all. v1 requests, including the fallback retry, keep the names
// that generation defined.
//
// The payload is built as a map rather than a struct with two spellings because the
// governance manifest retires the bare parameter name for this package
// (pattern_budget.retired_acp_wire_contract), and building the v2 request is the one
// place where the specification requires it.
func (r InitializeRequest) MarshalJSON() ([]byte, error) {
	payload := map[string]interface{}{"protocolVersion": r.ProtocolVersion}
	if r.ProtocolVersion >= ProtocolVersionV2 {
		payload["info"] = r.ClientInfo
		if r.ClientCapabilities != nil {
			// Version 2 deleted the client file system and terminal execution
			// surfaces, so the capabilities that describe them are not sent to a
			// version 2 agent: advertising them would claim a surface that
			// generation does not define.
			capabilities := *r.ClientCapabilities
			capabilities.Fs, capabilities.Terminal, capabilities.Session = nil, false, nil
			payload["capabilities"] = capabilities
		}
		return json.Marshal(payload)
	}
	payload["clientInfo"] = r.ClientInfo
	if r.ClientCapabilities != nil {
		// Version 1 defines clientCapabilities.auth.terminal as a boolean, and
		// Matrix runs terminal methods only on a version 2 connection, so the
		// capability is not advertised here: claiming a flow this client would
		// refuse is worse than claiming nothing, and an object where version 1
		// reads a boolean would not be understood as support anyway.
		capabilities := *r.ClientCapabilities
		capabilities.Auth = nil
		payload["clientCapabilities"] = capabilities
	}
	return json.Marshal(payload)
}

type ClientCapabilities struct {
	Fs          *FsCapability              `json:"fs,omitempty"`
	Terminal    bool                       `json:"terminal,omitempty"`
	Session     *ClientSessionCapabilities `json:"session,omitempty"`
	Elicitation *ElicitationCapabilities   `json:"elicitation,omitempty"`
	Auth        *AuthCapabilities          `json:"auth,omitempty"`
	Meta        map[string]interface{}     `json:"_meta,omitempty"`
}

// ElicitationCapabilities advertises stable ACP v1 elicitation support. Per
// the stable spec each supported mode must be explicitly present and non-null;
// an empty object advertises no modes.
type ElicitationCapabilities struct {
	Form *ElicitationModeCapability `json:"form,omitempty"`
	URL  *ElicitationModeCapability `json:"url,omitempty"`
	Meta map[string]interface{}     `json:"_meta,omitempty"`
}

type ElicitationModeCapability struct {
	Meta map[string]interface{} `json:"_meta,omitempty"`
}

type ClientSessionCapabilities struct {
	ConfigOptions *SessionConfigOptionsCapabilities `json:"configOptions,omitempty"`
	Meta          map[string]interface{}            `json:"_meta,omitempty"`
}

type SessionConfigOptionsCapabilities struct {
	Boolean *BooleanConfigOptionCapabilities `json:"boolean,omitempty"`
	Meta    map[string]interface{}           `json:"_meta,omitempty"`
}

// BooleanConfigOptionCapabilities is intentionally empty: in ACP v1 the
// presence of an empty object advertises support for boolean config options.
type BooleanConfigOptionCapabilities struct {
	Meta map[string]interface{} `json:"_meta,omitempty"`
}

type FsCapability struct {
	ReadTextFile  bool `json:"readTextFile"`
	WriteTextFile bool `json:"writeTextFile"`
}

// InitializeResponse decodes the stable ACP v1 initialization response.
type InitializeResponse struct {
	ProtocolVersion int                    `json:"protocolVersion,omitempty"`
	AgentInfo       map[string]interface{} `json:"agentInfo,omitempty"`
	Capabilities    map[string]interface{} `json:"-"`
	AuthMethods     []AuthMethod           `json:"authMethods,omitempty"`
}

func (r *InitializeResponse) UnmarshalJSON(data []byte) error {
	// The capability and info parameters are named differently by the two
	// generations, and the reply must be read in the names the generation that
	// answered defines: a v2 peer sends capabilities and info, a v1 peer sends
	// agentCapabilities and agentInfo. Reading only the v1 names silently dropped
	// everything a v2 peer advertised, so Matrix believed a conforming agent
	// supported nothing at all - including the terminal authentication surface.
	//
	// The retired spelling stays retired where it was retired: a response declaring
	// version 1 must not populate capabilities from that name, which is what the
	// existing test pins and what the governance manifest records. Decoding is by
	// generation, not by whichever key happens to be present.
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return err
	}
	decodeInto := func(target interface{}, keys ...string) error {
		for _, key := range keys {
			raw, ok := fields[key]
			if !ok || len(raw) == 0 || string(raw) == "null" {
				continue
			}
			return json.Unmarshal(raw, target)
		}
		return nil
	}
	if err := decodeInto(&r.ProtocolVersion, "protocolVersion"); err != nil {
		return err
	}
	if err := decodeInto(&r.AuthMethods, "authMethods"); err != nil {
		return err
	}
	if r.ProtocolVersion >= ProtocolVersionV2 {
		if err := decodeInto(&r.AgentInfo, "info", "agentInfo"); err != nil {
			return err
		}
		return decodeInto(&r.Capabilities, "capabilities", "agentCapabilities")
	}
	if err := decodeInto(&r.AgentInfo, "agentInfo"); err != nil {
		return err
	}
	return decodeInto(&r.Capabilities, "agentCapabilities")
}

// AuthMethod is one entry of the initialize response's authMethods. It carries
// both generations of the identifier: "id" in v1, "methodId" in v2. Args and Env
// are what a v2 terminal method contributes to the client's own launch of the
// agent program; they are empty for every other method type.
type AuthMethod struct {
	Type        string                 `json:"type,omitempty"`
	ID          string                 `json:"id,omitempty"`
	MethodID    string                 `json:"methodId,omitempty"`
	Name        string                 `json:"name,omitempty"`
	Description string                 `json:"description,omitempty"`
	Args        []string               `json:"args,omitempty"`
	Env         []EnvVar               `json:"env,omitempty"`
	Meta        map[string]interface{} `json:"_meta,omitempty"`
}

type NewSessionResponse struct {
	SessionID     string                 `json:"sessionId"`
	Modes         *SessionModeState      `json:"modes,omitempty"`
	ConfigOptions []ConfigOption         `json:"configOptions,omitempty"`
	Models        *SessionModelState     `json:"models,omitempty"`
	RawModels     json.RawMessage        `json:"-"`
	Meta          map[string]interface{} `json:"_meta,omitempty"`
}

func (r *NewSessionResponse) UnmarshalJSON(data []byte) error {
	type rawResponse struct {
		SessionID     string                 `json:"sessionId"`
		Modes         *SessionModeState      `json:"modes,omitempty"`
		ConfigOptions []ConfigOption         `json:"configOptions,omitempty"`
		Models        json.RawMessage        `json:"models,omitempty"`
		Meta          map[string]interface{} `json:"_meta,omitempty"`
	}
	var raw rawResponse
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	r.SessionID = raw.SessionID
	r.Modes = raw.Modes
	r.ConfigOptions = raw.ConfigOptions
	r.RawModels = cloneRawMessage(raw.Models)
	r.Models = nil
	if len(raw.Models) > 0 && string(raw.Models) != "null" {
		var models SessionModelState
		if err := json.Unmarshal(raw.Models, &models); err == nil && looksLikeSessionModelState(models) {
			r.Models = &models
		}
	}
	r.Meta = raw.Meta
	return nil
}

type LoadSessionRequest struct {
	SessionID             string                 `json:"sessionId"`
	Cwd                   string                 `json:"cwd"`
	AdditionalDirectories []string               `json:"additionalDirectories,omitempty"`
	McpServers            []McpServerConfig      `json:"mcpServers"`
	Meta                  map[string]interface{} `json:"_meta,omitempty"`
}

func (r LoadSessionRequest) MarshalJSON() ([]byte, error) {
	type wireRequest LoadSessionRequest
	out := wireRequest(r)
	out.McpServers = nonNil(r.McpServers)
	return json.Marshal(out)
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

func (r ResumeSessionRequest) MarshalJSON() ([]byte, error) {
	type wireRequest ResumeSessionRequest
	out := wireRequest(r)
	out.McpServers = nonNil(r.McpServers)
	return json.Marshal(out)
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

func (r ForkSessionRequest) MarshalJSON() ([]byte, error) {
	type wireRequest ForkSessionRequest
	out := wireRequest(r)
	out.McpServers = nonNil(r.McpServers)
	return json.Marshal(out)
}

type ForkSessionResponse struct {
	SessionID     string                 `json:"sessionId"`
	Modes         *SessionModeState      `json:"modes,omitempty"`
	ConfigOptions []ConfigOption         `json:"configOptions,omitempty"`
	Models        *SessionModelState     `json:"models,omitempty"`
	RawModels     json.RawMessage        `json:"-"`
	Meta          map[string]interface{} `json:"_meta,omitempty"`
}

func (r *ForkSessionResponse) UnmarshalJSON(data []byte) error {
	type rawResponse struct {
		SessionID     string                 `json:"sessionId"`
		Modes         *SessionModeState      `json:"modes,omitempty"`
		ConfigOptions []ConfigOption         `json:"configOptions,omitempty"`
		Models        json.RawMessage        `json:"models,omitempty"`
		Meta          map[string]interface{} `json:"_meta,omitempty"`
	}
	var raw rawResponse
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	r.SessionID = raw.SessionID
	r.Modes = raw.Modes
	r.ConfigOptions = raw.ConfigOptions
	r.RawModels = cloneRawMessage(raw.Models)
	r.Models = nil
	if len(raw.Models) > 0 && string(raw.Models) != "null" {
		var models SessionModelState
		if err := json.Unmarshal(raw.Models, &models); err == nil && looksLikeSessionModelState(models) {
			r.Models = &models
		}
	}
	r.Meta = raw.Meta
	return nil
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

func looksLikeSessionModelState(state SessionModelState) bool {
	return state.CurrentModelID != "" || len(state.AvailableModels) > 0
}

type ConfigOption struct {
	ID             string                 `json:"id"`
	Name           string                 `json:"name"`
	Description    string                 `json:"description,omitempty"`
	Category       string                 `json:"category,omitempty"`
	Type           string                 `json:"type,omitempty"`
	Options        []ConfigOptionValue    `json:"options,omitempty"`
	Current        string                 `json:"currentValue,omitempty"`
	BooleanCurrent *bool                  `json:"-"`
	Meta           map[string]interface{} `json:"_meta,omitempty"`
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
		CurrentValue json.RawMessage        `json:"currentValue,omitempty"`
		Current      json.RawMessage        `json:"current,omitempty"`
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
	o.Current = configScalarString(raw.CurrentValue)
	if o.Current == "" {
		o.Current = configScalarString(raw.Current)
	}
	o.BooleanCurrent = configBoolean(raw.CurrentValue)
	if o.BooleanCurrent == nil {
		o.BooleanCurrent = configBoolean(raw.Current)
	}
	o.Meta = raw.Meta
	o.Options = decodeConfigOptions(raw.Options)
	return nil
}

func configBoolean(data json.RawMessage) *bool {
	if len(data) == 0 || string(data) == "null" {
		return nil
	}
	var value bool
	if err := json.Unmarshal(data, &value); err != nil {
		return nil
	}
	return &value
}

func configScalarString(data json.RawMessage) string {
	if len(data) == 0 || string(data) == "null" {
		return ""
	}
	var text string
	if err := json.Unmarshal(data, &text); err == nil {
		return text
	}
	var boolean bool
	if err := json.Unmarshal(data, &boolean); err == nil {
		if boolean {
			return "true"
		}
		return "false"
	}
	var number json.Number
	if err := json.Unmarshal(data, &number); err == nil {
		return number.String()
	}
	return ""
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
	Type      string                 `json:"type,omitempty"`
	Value     interface{}            `json:"value"`
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
	// MessageID is the version 2 acknowledgement: the identifier of the user
	// message the prompt inserted, which is the only thing that response
	// carries. Version 1 named the same idea userMessageId, and a version 2
	// response has no stop reason at all.
	MessageID string                 `json:"messageId,omitempty"`
	Meta      map[string]interface{} `json:"_meta,omitempty"`
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
	MessageID         string                 `json:"messageId,omitempty"`
	Content           Content                `json:"-"`
	Contents          []Content              `json:"-"`
	ToolContents      []ToolCallContent      `json:"-"`
	RawContent        json.RawMessage        `json:"-"`
	Title             string                 `json:"title,omitempty"`
	Name              string                 `json:"name,omitempty"`
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
	// State and StopReason are the version 2 state_update lifecycle. An idle
	// state is the terminal signal that ends the foreground work a prompt
	// started, and the stop reason says why it ended. Version 1 has neither
	// field, which is why a version 1 turn cannot end on them.
	State      string `json:"state,omitempty"`
	StopReason string `json:"stopReason,omitempty"`
	// Plan is the version 2 plan payload: plan_update nests the entries under
	// "plan", where version 1's plan update carried them at the top level.
	Plan *PlanUpdateContent `json:"plan,omitempty"`
	// TerminalID, Command, Cwd, Output, ExitStatus and Data carry the version 2
	// agent-owned terminal variants. The pointers distinguish an omitted field,
	// which leaves the stored value alone, from an explicit null, which clears
	// it.
	TerminalID string              `json:"terminalId,omitempty"`
	Command    *string             `json:"command,omitempty"`
	Cwd        *string             `json:"cwd,omitempty"`
	Output     *TerminalOutput     `json:"output,omitempty"`
	ExitStatus *TerminalExitStatus `json:"exitStatus,omitempty"`
	Data       string              `json:"data,omitempty"`
	// ContentSet and ContentCleared describe how a message upsert's content
	// field arrived. An omitted content leaves the stored message unchanged, an
	// explicit null clears it, and an array replaces it.
	ContentSet     bool `json:"-"`
	ContentCleared bool `json:"-"`
}

// TurnTerminal reports the version 2 terminal signal: an idle state_update ends
// the foreground work the prompt started, and the stop reason says why.
func (u SessionUpdate) TurnTerminal() (string, bool) {
	if u.SessionUpdate != "state_update" || u.State != "idle" {
		return "", false
	}
	return u.StopReason, true
}

// PlanEntries reports a plan update's entries in whichever generation's shape
// the peer sent: version 2 nests them under "plan".
func (u SessionUpdate) PlanEntries() []PlanEntry {
	if u.Plan != nil && len(u.Entries) == 0 {
		return u.Plan.Entries
	}
	return u.Entries
}

// wireUpdate is the on-the-wire shape of a session update without the decoded
// content fields, which cannot round-trip through the struct tags: the local
// type keeps its JSON tags and loses its methods, so decoding cannot recurse
// into UnmarshalJSON.
type wireUpdate SessionUpdate

func (u *SessionUpdate) UnmarshalJSON(data []byte) error {
	var wire struct {
		*wireUpdate
		Content json.RawMessage `json:"content,omitempty"`
	}
	wire.wireUpdate = (*wireUpdate)(u)
	if err := json.Unmarshal(data, &wire); err != nil {
		return err
	}
	u.RawContent = cloneRawMessage(wire.Content)
	u.ContentSet = len(wire.Content) > 0
	u.ContentCleared = u.ContentSet && string(wire.Content) == "null"
	u.Content, u.Contents, u.ToolContents = decodeUpdateContent(wire.Content)
	return nil
}

func (u SessionUpdate) MarshalJSON() ([]byte, error) {
	content := encodeUpdateContent(u.Content, u.Contents, u.ToolContents, u.RawContent)
	if content == nil {
		return json.Marshal(wireUpdate(u))
	}
	return json.Marshal(struct {
		wireUpdate
		Content interface{} `json:"content"`
	}{wireUpdate(u), content})
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

// PlanUpdateContent is the version 2 plan payload. Version 2 identifies a plan
// by planId and requires every update to carry the complete entry list, so a
// receiver replaces the plan it stored rather than merging into it.
type PlanUpdateContent struct {
	Type    string      `json:"type"`
	PlanID  string      `json:"planId,omitempty"`
	Entries []PlanEntry `json:"entries,omitempty"`
}

// TerminalOutput is an authoritative replacement snapshot of an agent-owned
// terminal's output bytes, base64-encoded as the specification defines them.
type TerminalOutput struct {
	Data string `json:"data"`
}

// TerminalExitStatus reports that an agent-owned terminal exited, and how.
// A concrete object marks the terminal as exited.
type TerminalExitStatus struct {
	ExitCode *int    `json:"exitCode,omitempty"`
	Signal   *string `json:"signal,omitempty"`
}

// DiffChange is one file-level change of a version 2 diff. The operation names
// which fields are meaningful: add, delete and modify carry path, while move and
// copy carry oldPath as well.
type DiffChange struct {
	Operation string `json:"operation"`
	Path      string `json:"path,omitempty"`
	OldPath   string `json:"oldPath,omitempty"`
	FileType  string `json:"fileType,omitempty"`
	MimeType  string `json:"mimeType,omitempty"`
}

// DiffPatch is renderable patch text for a version 2 diff. The specification
// defines git_patch as the only named format; anything else is a future variant.
type DiffPatch struct {
	Format string `json:"format"`
	Text   string `json:"text"`
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
// Path/OldText/NewText are the version 1 diff shape; Changes/Patch are the
// version 2 one, where the affected files are authoritative and the text patch
// is optional.
type ToolCallContent struct {
	Type       string                 `json:"type"`
	Content    *Content               `json:"content,omitempty"`
	Path       string                 `json:"path,omitempty"`
	OldText    *string                `json:"oldText,omitempty"`
	NewText    string                 `json:"newText,omitempty"`
	TerminalID string                 `json:"terminalId,omitempty"`
	Changes    []DiffChange           `json:"changes,omitempty"`
	Patch      *DiffPatch             `json:"patch,omitempty"`
	Meta       map[string]interface{} `json:"_meta,omitempty"`
}
