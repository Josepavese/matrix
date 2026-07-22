package zedacp

import (
	"bytes"
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"
)

type recordingObserver struct {
	mu      sync.Mutex
	updates []string
}

func (o *recordingObserver) OnUpdate(notification SessionNotification) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.updates = append(o.updates, notification.Update.Content.Text)
}

func (o *recordingObserver) joined() string {
	o.mu.Lock()
	defer o.mu.Unlock()
	out := ""
	for _, update := range o.updates {
		out += update
	}
	return out
}

func TestClientFansOutConcurrentSessionObservers(t *testing.T) {
	client := &Client{observers: make(map[string]map[uint64]SessionObserver)}
	main := &recordingObserver{}
	attach := &recordingObserver{}

	removeMain := client.registerObserver("session-1", main)
	removeAttach := client.registerObserver("session-1", attach)
	client.handleNotification(sessionUpdateResponse(t, "session-1", "part-1"))

	if main.joined() != "part-1" || attach.joined() != "part-1" {
		t.Fatalf("expected both observers to receive first chunk, main=%q attach=%q", main.joined(), attach.joined())
	}

	removeAttach()
	client.handleNotification(sessionUpdateResponse(t, "session-1", "part-2"))

	if main.joined() != "part-1part-2" {
		t.Fatalf("expected main observer to keep receiving chunks, got %q", main.joined())
	}
	if attach.joined() != "part-1" {
		t.Fatalf("expected removed observer to stop receiving chunks, got %q", attach.joined())
	}

	removeMain()
	client.handleNotification(sessionUpdateResponse(t, "session-1", "part-3"))
	if main.joined() != "part-1part-2" {
		t.Fatalf("expected no updates after all observers removed, got %q", main.joined())
	}
}

func TestClientPromptKeepsObserverDuringPostResponseDrain(t *testing.T) {
	transport := newMethodRecordingTransport(t, map[string]json.RawMessage{
		"session/prompt": json.RawMessage(`{"stopReason":"end_turn"}`),
	})
	client := NewClient(context.Background(), transport)
	defer client.Close()

	observer := &postResponseDrainObserver{
		recordingObserver: &recordingObserver{},
		client:            client,
		sessionID:         "session-1",
		t:                 t,
	}
	if _, err := client.Prompt(context.Background(), PromptRequest{
		SessionID: "session-1",
		Prompt:    []Content{{Type: "text", Text: "hello"}},
	}, observer); err != nil {
		t.Fatalf("prompt: %v", err)
	}
	if !observer.waited {
		t.Fatalf("expected prompt to wait for observer idle before unregistering")
	}
	if got := observer.joined(); got != "late-final" {
		t.Fatalf("expected post-response chunk to be observed before unregister, got %q", got)
	}

	client.handleNotification(sessionUpdateResponse(t, "session-1", "after-unregister"))
	if got := observer.joined(); got != "late-final" {
		t.Fatalf("expected observer to be unregistered after prompt returns, got %q", got)
	}
}

type postResponseDrainObserver struct {
	*recordingObserver
	client    *Client
	sessionID string
	t         *testing.T
	waited    bool
}

func (o *postResponseDrainObserver) WaitIdle(_ context.Context, _ time.Duration) {
	o.waited = true
	o.client.handleNotification(sessionUpdateResponse(o.t, o.sessionID, "late-final"))
}

func TestSessionLifecycleRequestsMarshalStableDirectoriesAndDraftFork(t *testing.T) {
	forkReq := ForkSessionRequest{
		SessionID:             "sess_parent",
		Cwd:                   "/workspace/main",
		AdditionalDirectories: []string{"/workspace/lib", "/workspace/docs"},
		McpServers:            []McpServerConfig{},
		Meta:                  map[string]interface{}{"trace": "matrix"},
	}

	got, err := json.Marshal(forkReq)
	if err != nil {
		t.Fatalf("marshal fork request: %v", err)
	}
	for _, want := range [][]byte{
		[]byte(`"additionalDirectories":["/workspace/lib","/workspace/docs"]`),
		[]byte(`"_meta":{"trace":"matrix"}`),
	} {
		if !bytes.Contains(got, want) {
			t.Fatalf("expected %s in marshaled fork request: %s", want, got)
		}
	}

	resumeReq := ResumeSessionRequest{
		SessionID:             "sess_1",
		Cwd:                   "/workspace/main",
		AdditionalDirectories: []string{"/workspace/lib"},
		McpServers:            []McpServerConfig{},
	}
	got, err = json.Marshal(resumeReq)
	if err != nil {
		t.Fatalf("marshal resume request: %v", err)
	}
	if !bytes.Contains(got, []byte(`"additionalDirectories":["/workspace/lib"]`)) {
		t.Fatalf("expected additionalDirectories in resume request: %s", got)
	}
}

func TestMCPServerConfigMarshalsRequiredEmptyArrays(t *testing.T) {
	reqBytes, err := json.Marshal(NewSessionRequest{Cwd: "/workspace"})
	if err != nil {
		t.Fatalf("marshal new session request: %v", err)
	}
	if !bytes.Contains(reqBytes, []byte(`"mcpServers":[]`)) {
		t.Fatalf("new session request must marshal empty mcpServers array: %s", reqBytes)
	}

	stdio, err := json.Marshal(McpServerConfig{Name: "repo", Command: "repo-mcp"})
	if err != nil {
		t.Fatalf("marshal stdio mcp server: %v", err)
	}
	for _, want := range [][]byte{
		[]byte(`"args":[]`),
		[]byte(`"env":[]`),
	} {
		if !bytes.Contains(stdio, want) {
			t.Fatalf("expected %s in stdio mcp server: %s", want, stdio)
		}
	}
	if bytes.Contains(stdio, []byte(`"headers"`)) {
		t.Fatalf("stdio mcp server must not send headers: %s", stdio)
	}

	httpServer, err := json.Marshal(McpServerConfig{Name: "remote", Type: "http", URL: "https://mcp.example"})
	if err != nil {
		t.Fatalf("marshal http mcp server: %v", err)
	}
	for _, want := range [][]byte{
		[]byte(`"type":"http"`),
		[]byte(`"headers":[]`),
	} {
		if !bytes.Contains(httpServer, want) {
			t.Fatalf("expected %s in http mcp server: %s", want, httpServer)
		}
	}
}

func TestPromptRequestMarshalMessageID(t *testing.T) {
	req := PromptRequest{
		SessionID: "sess_1",
		MessageID: "550e8400-e29b-41d4-a716-446655440000",
		Prompt:    []Content{{Type: "text", Text: "hello"}},
	}

	got, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("marshal prompt request: %v", err)
	}
	if !bytes.Contains(got, []byte(`"messageId":"550e8400-e29b-41d4-a716-446655440000"`)) {
		t.Fatalf("expected messageId in marshaled prompt request: %s", got)
	}
}

func TestClientListSessionsWithRequestSendsLatestFilters(t *testing.T) {
	transport := newListSessionsTransport(t)
	client := NewClient(context.Background(), transport)
	defer client.Close()

	resp, err := client.ListSessionsWithRequest(context.Background(), ListSessionsRequest{
		Cwd:    "/workspace/main",
		Cursor: "cursor-1",
		Meta:   map[string]interface{}{"source": "test"},
	})
	if err != nil {
		t.Fatalf("list sessions with request: %v", err)
	}
	if resp.NextCursor != "cursor-2" {
		t.Fatalf("expected next cursor, got %#v", resp)
	}
	if len(resp.Sessions) != 1 || resp.Sessions[0].Cwd != "/workspace/main" {
		t.Fatalf("unexpected sessions response: %#v", resp)
	}
	if got := resp.Sessions[0].AdditionalDirectories; len(got) != 1 || got[0] != "/workspace/lib" {
		t.Fatalf("unexpected additional directories: %#v", got)
	}

	var sent jsonRPCRequest
	if err := json.Unmarshal(transport.lastSent(), &sent); err != nil {
		t.Fatalf("unmarshal sent request: %v", err)
	}
	if sent.Method != "session/list" {
		t.Fatalf("expected session/list, got %q", sent.Method)
	}
	for _, want := range [][]byte{
		[]byte(`"cwd":"/workspace/main"`),
		[]byte(`"cursor":"cursor-1"`),
		[]byte(`"_meta":{"source":"test"}`),
	} {
		if !bytes.Contains(sent.Params, want) {
			t.Fatalf("expected %s in params: %s", want, sent.Params)
		}
	}
	if bytes.Contains(sent.Params, []byte(`"additionalDirectories"`)) {
		t.Fatalf("session/list must not send additionalDirectories per current schema: %s", sent.Params)
	}
}

func TestPromptResponseUnmarshalLatestFields(t *testing.T) {
	var resp PromptResponse
	if err := json.Unmarshal([]byte(`{
		"stopReason": "end_turn",
		"userMessageId": "550e8400-e29b-41d4-a716-446655440000",
		"usage": {"inputTokens": 12},
		"_meta": {"trace": "matrix"}
	}`), &resp); err != nil {
		t.Fatalf("unmarshal prompt response: %v", err)
	}
	if resp.UserMessageID != "550e8400-e29b-41d4-a716-446655440000" {
		t.Fatalf("unexpected userMessageId: %#v", resp)
	}
	inputTokens, ok := resp.Usage["inputTokens"].(float64)
	if !ok || inputTokens != 12 {
		t.Fatalf("unexpected usage: %#v", resp.Usage)
	}
	if resp.Meta["trace"] != "matrix" {
		t.Fatalf("unexpected meta: %#v", resp.Meta)
	}
}

func TestClientResumeAndSetConfigOptionUseStableMethods(t *testing.T) {
	transport := newMethodRecordingTransport(t, map[string]json.RawMessage{
		"session/resume":            json.RawMessage(`{"configOptions":[{"id":"mode","name":"Mode","category":"mode","type":"select","currentValue":"ask","options":[{"value":"build","name":"Build"}]}]}`),
		"session/set_config_option": json.RawMessage(`{"configOptions":[{"id":"mode","name":"Mode","type":"select","currentValue":"build","options":[{"value":"build","name":"Build"}]}]}`),
	})
	client := NewClient(context.Background(), transport)
	defer client.Close()

	resp, err := client.ResumeSession(context.Background(), ResumeSessionRequest{
		SessionID:             "sess-1",
		Cwd:                   "/workspace/main",
		AdditionalDirectories: []string{"/workspace/lib"},
		McpServers:            []McpServerConfig{},
	})
	if err != nil {
		t.Fatalf("resume session: %v", err)
	}
	if len(resp.ConfigOptions) != 1 || resp.ConfigOptions[0].Current != "ask" || resp.ConfigOptions[0].Options[0].ID != "build" {
		t.Fatalf("unexpected resume config options: %#v", resp.ConfigOptions)
	}
	configResp, err := client.SetConfigOption(context.Background(), SetSessionConfigOptionRequest{
		SessionID: "sess-1",
		ConfigID:  "mode",
		Value:     "build",
	})
	if err != nil {
		t.Fatalf("set config option: %v", err)
	}
	if len(configResp.ConfigOptions) != 1 || configResp.ConfigOptions[0].Current != "build" {
		t.Fatalf("unexpected set config option response: %#v", configResp)
	}

	methods := transport.seenMethods()
	if len(methods) != 2 || methods[0] != "session/resume" || methods[1] != "session/set_config_option" {
		t.Fatalf("unexpected methods: %#v", methods)
	}
	var sent jsonRPCRequest
	if err := json.Unmarshal(transport.sentForMethod("session/resume"), &sent); err != nil {
		t.Fatalf("unmarshal last sent request: %v", err)
	}
	if !bytes.Contains(sent.Params, []byte(`"additionalDirectories":["/workspace/lib"]`)) {
		t.Fatalf("expected additionalDirectories in resume params: %s", sent.Params)
	}
}

func TestConfigOptionAcceptsExtensionScalarCurrentValue(t *testing.T) {
	var opt ConfigOption
	if err := json.Unmarshal([]byte(`{"id":"thinking","name":"Thinking","type":"boolean","currentValue":true}`), &opt); err != nil {
		t.Fatalf("unmarshal boolean config option: %v", err)
	}
	if opt.Current != "true" {
		t.Fatalf("expected boolean currentValue to become string, got %#v", opt.Current)
	}
	if opt.BooleanCurrent == nil || !*opt.BooleanCurrent {
		t.Fatalf("expected typed boolean currentValue, got %#v", opt.BooleanCurrent)
	}
}

func TestBooleanConfigOptionWireShape(t *testing.T) {
	transport := newMethodRecordingTransport(t, map[string]json.RawMessage{
		"session/set_config_option": json.RawMessage(`{"configOptions":[{"id":"reasoning","name":"Reasoning","type":"boolean","currentValue":true}]}`),
	})
	client := NewClient(context.Background(), transport)
	defer client.Close()

	response, err := client.SetConfigOption(context.Background(), SetSessionConfigOptionRequest{
		SessionID: "session-1",
		ConfigID:  "reasoning",
		Type:      "boolean",
		Value:     true,
	})
	if err != nil {
		t.Fatalf("set boolean config option: %v", err)
	}
	if len(response.ConfigOptions) != 1 || response.ConfigOptions[0].BooleanCurrent == nil || !*response.ConfigOptions[0].BooleanCurrent {
		t.Fatalf("boolean current value was not preserved: %#v", response.ConfigOptions)
	}
	var sent jsonRPCRequest
	if err := json.Unmarshal(transport.sentForMethod("session/set_config_option"), &sent); err != nil {
		t.Fatalf("decode request: %v", err)
	}
	var params map[string]interface{}
	if err := json.Unmarshal(sent.Params, &params); err != nil {
		t.Fatalf("decode params: %v", err)
	}
	if params["type"] != "boolean" || params["value"] != true {
		t.Fatalf("unexpected boolean config wire shape: %#v", params)
	}
}

func TestInitializeUnmarshalStableAuthMethodsAndModels(t *testing.T) {
	var resp InitializeResponse
	if err := json.Unmarshal([]byte(`{
		"protocolVersion": 1,
		"authMethods": [{
			"type": "agent",
			"id": "openai",
			"name": "OpenAI",
			"description": "Authenticate in the agent",
			"_meta": {"source": "test"}
		}]
	}`), &resp); err != nil {
		t.Fatalf("unmarshal initialize response: %v", err)
	}
	if len(resp.AuthMethods) != 1 || resp.AuthMethods[0].Type != "agent" || resp.AuthMethods[0].Name != "OpenAI" {
		t.Fatalf("unexpected auth methods: %#v", resp.AuthMethods)
	}

	var newResp NewSessionResponse
	if err := json.Unmarshal([]byte(`{
		"sessionId": "sess-1",
		"models": {
			"currentModelId": "gpt-5.4",
			"availableModels": [{"modelId": "gpt-5.4", "name": "GPT-5.4"}]
		}
	}`), &newResp); err != nil {
		t.Fatalf("unmarshal new session response: %v", err)
	}
	if newResp.Models == nil || newResp.Models.CurrentModelID != "gpt-5.4" || len(newResp.Models.AvailableModels) != 1 {
		t.Fatalf("expected structured session model state: %#v", newResp.Models)
	}
}

func TestInitializeResponseRejectsRetiredCapabilitiesField(t *testing.T) {
	var response InitializeResponse
	if err := json.Unmarshal([]byte(`{"protocolVersion":1,"capabilities":{"loadSession":true}}`), &response); err != nil {
		t.Fatalf("unmarshal initialize response: %v", err)
	}
	if response.Capabilities != nil {
		t.Fatalf("retired capabilities field must not be accepted: %#v", response.Capabilities)
	}
}

func TestSessionResponsePreservesUnknownDraftModels(t *testing.T) {
	var resp NewSessionResponse
	if err := json.Unmarshal([]byte(`{
		"sessionId": "sess-1",
		"models": {"current": "unknown-draft-shape", "available": ["x"]}
	}`), &resp); err != nil {
		t.Fatalf("unmarshal permissive models response: %v", err)
	}
	if resp.Models != nil {
		t.Fatalf("unknown draft models shape should not decode as typed state: %#v", resp.Models)
	}
	if !bytes.Contains(resp.RawModels, []byte(`"unknown-draft-shape"`)) {
		t.Fatalf("raw models should preserve unknown draft shape: %s", resp.RawModels)
	}
}

func TestAuthenticateUsesStableMethodIDOnly(t *testing.T) {
	transport := newMethodRecordingTransport(t, map[string]json.RawMessage{
		"authenticate": json.RawMessage(`{}`),
	})
	client := NewClient(context.Background(), transport)
	defer client.Close()

	if err := client.Authenticate(context.Background(), "agent"); err != nil {
		t.Fatalf("authenticate: %v", err)
	}
	var sent jsonRPCRequest
	if err := json.Unmarshal(transport.sentForMethod("authenticate"), &sent); err != nil {
		t.Fatalf("unmarshal authenticate request: %v", err)
	}
	if bytes.Contains(sent.Params, []byte(`"credentials"`)) || !bytes.Contains(sent.Params, []byte(`"methodId":"agent"`)) {
		t.Fatalf("stable authenticate request must contain methodId only: %s", sent.Params)
	}
}

func TestClientExperimentalProviderModelAndStableControlSurfaces(t *testing.T) {
	transport := newMethodRecordingTransport(t, map[string]json.RawMessage{
		"providers/list":    json.RawMessage(`{"providers":[{"id":"main","supported":["openai","anthropic"],"required":false,"current":{"apiType":"openai","baseUrl":"https://api.openai.com/v1"}}]}`),
		"providers/set":     json.RawMessage(`{"_meta":{"ok":true}}`),
		"providers/disable": json.RawMessage(`{}`),
		"session/set_model": json.RawMessage(`{}`),
		"logout":            json.RawMessage(`{}`),
	})
	client := NewClient(context.Background(), transport)
	defer client.Close()

	providers, err := client.ListProviders(context.Background(), ListProvidersRequest{})
	if err != nil {
		t.Fatalf("list providers: %v", err)
	}
	if len(providers.Providers) != 1 || providers.Providers[0].Current.APIType != "openai" {
		t.Fatalf("unexpected provider response: %#v", providers)
	}
	if _, err := client.SetProvider(context.Background(), SetProvidersRequest{
		ID:      "main",
		APIType: "anthropic",
		BaseURL: "https://api.anthropic.com",
		Headers: map[string]string{
			"authorization": "Bearer token",
		},
	}); err != nil {
		t.Fatalf("set provider: %v", err)
	}
	if _, err := client.DisableProvider(context.Background(), DisableProvidersRequest{ID: "main"}); err != nil {
		t.Fatalf("disable provider: %v", err)
	}
	if _, err := client.SetSessionModel(context.Background(), SetSessionModelRequest{SessionID: "sess-1", ModelID: "claude"}); err != nil {
		t.Fatalf("set model: %v", err)
	}
	if _, err := client.Logout(context.Background(), LogoutRequest{}); err != nil {
		t.Fatalf("logout: %v", err)
	}
	methods := transport.seenMethods()
	want := []string{"providers/list", "providers/set", "providers/disable", "session/set_model", "logout"}
	if len(methods) != len(want) {
		t.Fatalf("unexpected methods: %#v", methods)
	}
	for i := range want {
		if methods[i] != want[i] {
			t.Fatalf("unexpected method order: got %#v want %#v", methods, want)
		}
	}

	notif := newNotificationRecordingTransport(t)
	notifClient := NewClient(context.Background(), notif)
	defer notifClient.Close()
	if err := notifClient.CancelRequest(context.Background(), CancelRequestNotification{RequestID: "req-1"}); err != nil {
		t.Fatalf("cancel request: %v", err)
	}
	var sent map[string]interface{}
	if err := json.Unmarshal(notif.lastSent(), &sent); err != nil {
		t.Fatalf("unmarshal cancel request notification: %v", err)
	}
	if sent["method"] != "$/cancel_request" {
		t.Fatalf("unexpected cancel request method: %#v", sent)
	}
	if _, hasID := sent["id"]; hasID {
		t.Fatalf("cancel request must be a notification: %#v", sent)
	}
}

func TestSessionUpdateUnmarshalStructuredVariants(t *testing.T) {
	var notif SessionNotification
	if err := json.Unmarshal([]byte(`{
		"sessionId": "sess-1",
		"update": {
			"sessionUpdate": "agent_message_chunk",
			"messageId": "message-final",
			"content": {"type":"text","text":"Ciao "},
			"_meta": {"codex":{"phase":"final_answer"}}
		}
	}`), &notif); err != nil {
		t.Fatalf("unmarshal message update: %v", err)
	}
	if notif.Update.MessageID != "message-final" || notif.Update.Content.Text != "Ciao " {
		t.Fatalf("message identity or exact content lost: %#v", notif.Update)
	}
	encoded, err := json.Marshal(notif.Update)
	if err != nil {
		t.Fatalf("marshal message update: %v", err)
	}
	if !bytes.Contains(encoded, []byte(`"messageId":"message-final"`)) {
		t.Fatalf("messageId lost during round trip: %s", encoded)
	}

	if err := json.Unmarshal([]byte(`{
		"sessionId": "sess-1",
		"update": {
			"sessionUpdate": "tool_call",
			"toolCallId": "tool-1",
			"title": "Read files",
			"kind": "read",
			"status": "pending",
			"content": [{"type":"text","text":"reading"}],
			"rawInput": {"path": "/workspace/main.go"},
			"locations": [{"path":"/workspace/main.go"}]
		}
	}`), &notif); err != nil {
		t.Fatalf("unmarshal tool update: %v", err)
	}
	if notif.Update.Content.Text != "reading" || len(notif.Update.Contents) != 1 {
		t.Fatalf("expected content array to be preserved: %#v", notif.Update)
	}
	if notif.Update.ToolCallID != "tool-1" || notif.Update.Kind != "read" || notif.Update.Status != "pending" {
		t.Fatalf("unexpected tool fields: %#v", notif.Update)
	}

	if err := json.Unmarshal([]byte(`{
		"sessionId": "sess-1",
		"update": {
			"sessionUpdate": "plan",
			"entries": [{"content":"Test","priority":"high","status":"pending"}]
		}
	}`), &notif); err != nil {
		t.Fatalf("unmarshal plan update: %v", err)
	}
	if len(notif.Update.Entries) != 1 || notif.Update.Entries[0].Content != "Test" {
		t.Fatalf("unexpected plan entries: %#v", notif.Update.Entries)
	}
}

func TestSessionUpdateUnmarshalToolCallContentVariants(t *testing.T) {
	var notif SessionNotification
	if err := json.Unmarshal([]byte(`{
		"sessionId": "sess-1",
		"update": {
			"sessionUpdate": "tool_call_update",
			"toolCallId": "tool-1",
			"content": [
				{"type":"content","content":{"type":"text","text":"done"}},
				{"type":"diff","path":"/workspace/main.go","oldText":"old","newText":"new"},
				{"type":"terminal","terminalId":"terminal-1"}
			]
		}
	}`), &notif); err != nil {
		t.Fatalf("unmarshal tool call content variants: %v", err)
	}
	if len(notif.Update.ToolContents) != 3 {
		t.Fatalf("expected tool content variants, got %#v", notif.Update.ToolContents)
	}
	if notif.Update.Content.Text != "done" || len(notif.Update.Contents) != 1 {
		t.Fatalf("expected nested content text to be projected, got %#v", notif.Update)
	}
	if notif.Update.ToolContents[1].Path != "/workspace/main.go" || notif.Update.ToolContents[1].NewText != "new" {
		t.Fatalf("expected diff content preserved: %#v", notif.Update.ToolContents[1])
	}
	if notif.Update.ToolContents[2].TerminalID != "terminal-1" {
		t.Fatalf("expected terminal content preserved: %#v", notif.Update.ToolContents[2])
	}
	roundTrip, err := json.Marshal(notif.Update)
	if err != nil {
		t.Fatalf("marshal update: %v", err)
	}
	if !bytes.Contains(roundTrip, []byte(`"type":"terminal"`)) || !bytes.Contains(roundTrip, []byte(`"type":"diff"`)) {
		t.Fatalf("expected tool contents in round trip: %s", roundTrip)
	}
}

func TestClientExtensionRequestAndNotification(t *testing.T) {
	transport := newMethodRecordingTransport(t, map[string]json.RawMessage{
		"matrix/example": json.RawMessage(`{"accepted":true}`),
	})
	client := NewClient(context.Background(), transport)
	defer client.Close()

	var result struct {
		Accepted bool `json:"accepted"`
	}
	if err := client.ExtRequest(context.Background(), "matrix/example", map[string]interface{}{"x": 1}, &result); err != nil {
		t.Fatalf("extension request: %v", err)
	}
	if !result.Accepted {
		t.Fatalf("unexpected extension response: %#v", result)
	}

	notif := newNotificationRecordingTransport(t)
	notifClient := NewClient(context.Background(), notif)
	defer notifClient.Close()
	if err := notifClient.ExtNotification(context.Background(), "matrix/event", map[string]interface{}{"x": 2}); err != nil {
		t.Fatalf("extension notification: %v", err)
	}
	raw := notif.lastSent()
	var sent map[string]interface{}
	if err := json.Unmarshal(raw, &sent); err != nil {
		t.Fatalf("unmarshal notification: %v", err)
	}
	if sent["method"] != "matrix/event" {
		t.Fatalf("unexpected notification method: %#v", sent)
	}
	if _, hasID := sent["id"]; hasID {
		t.Fatalf("extension notification must not carry id: %#v", sent)
	}
}

func TestClientIncomingRequestUsesRPCErrorCode(t *testing.T) {
	transport := &responseCaptureTransport{}
	client := &Client{
		transport:      transport,
		requestHandler: methodNotFoundHandler{},
		ctx:            context.Background(),
	}
	client.handleIncomingRequest(jsonRPCRequest{JSONRPC: "2.0", ID: newJSONRPCID(7), Method: "matrix/unknown"})

	var resp jsonRPCResponse
	if err := json.Unmarshal(transport.lastSent(), &resp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if resp.Error == nil || resp.Error.Code != ErrCodeMethodNotFound {
		t.Fatalf("expected method-not-found response, got %#v", resp.Error)
	}
}

func TestClientIncomingRequestPreservesStringID(t *testing.T) {
	transport := &responseCaptureTransport{}
	client := &Client{
		transport:      transport,
		requestHandler: echoHandler{},
		ctx:            context.Background(),
	}
	client.handleIncomingRequest(jsonRPCRequest{JSONRPC: "2.0", ID: json.RawMessage(`"req-1"`), Method: "matrix/echo"})

	var resp map[string]interface{}
	if err := json.Unmarshal(transport.lastSent(), &resp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if resp["id"] != "req-1" {
		t.Fatalf("expected string id to round-trip, got %#v", resp["id"])
	}
}

type methodNotFoundHandler struct{}

func (methodNotFoundHandler) HandleRequest(context.Context, string, json.RawMessage) (interface{}, error) {
	return nil, NewMethodNotFoundError("matrix/unknown")
}

type echoHandler struct{}

func (echoHandler) HandleRequest(context.Context, string, json.RawMessage) (interface{}, error) {
	return map[string]interface{}{"ok": true}, nil
}

type listSessionsTransport struct {
	t    *testing.T
	recv chan []byte

	mu   sync.Mutex
	sent []byte
}

type methodRecordingTransport struct {
	t         *testing.T
	responses map[string]json.RawMessage
	recv      chan []byte

	mu   sync.Mutex
	seen []string
	sent [][]byte
}

func newMethodRecordingTransport(t *testing.T, responses map[string]json.RawMessage) *methodRecordingTransport {
	t.Helper()
	return &methodRecordingTransport{t: t, responses: responses, recv: make(chan []byte, 8)}
}

func (t *methodRecordingTransport) Send(_ context.Context, message []byte) error {
	var req jsonRPCRequest
	if err := json.Unmarshal(message, &req); err != nil {
		t.t.Fatalf("unmarshal request: %v", err)
	}
	t.mu.Lock()
	t.seen = append(t.seen, req.Method)
	t.sent = append(t.sent, append([]byte(nil), message...))
	t.mu.Unlock()
	result, ok := t.responses[req.Method]
	if !ok {
		t.t.Fatalf("unexpected method: %s", req.Method)
	}
	resp := jsonRPCResponse{JSONRPC: "2.0", ID: cloneRawMessage(req.ID), Result: result}
	msg, err := json.Marshal(resp)
	if err != nil {
		t.t.Fatalf("marshal response: %v", err)
	}
	t.recv <- msg
	return nil
}

func (t *methodRecordingTransport) Receive(ctx context.Context) ([]byte, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case msg := <-t.recv:
		return msg, nil
	}
}

func (t *methodRecordingTransport) Close() error { return nil }

func (t *methodRecordingTransport) seenMethods() []string {
	t.mu.Lock()
	defer t.mu.Unlock()
	return append([]string(nil), t.seen...)
}

func (t *methodRecordingTransport) sentForMethod(method string) []byte {
	t.mu.Lock()
	defer t.mu.Unlock()
	for _, msg := range t.sent {
		var req jsonRPCRequest
		if err := json.Unmarshal(msg, &req); err == nil && req.Method == method {
			return append([]byte(nil), msg...)
		}
	}
	return nil
}

func newListSessionsTransport(t *testing.T) *listSessionsTransport {
	t.Helper()
	return &listSessionsTransport{t: t, recv: make(chan []byte, 1)}
}

func (t *listSessionsTransport) Send(_ context.Context, message []byte) error {
	t.mu.Lock()
	t.sent = append([]byte(nil), message...)
	t.mu.Unlock()

	var req jsonRPCRequest
	if err := json.Unmarshal(message, &req); err != nil {
		t.t.Fatalf("unmarshal outbound request: %v", err)
	}
	if req.Method != "session/list" {
		t.t.Fatalf("unexpected method: %s", req.Method)
	}
	resp := jsonRPCResponse{
		JSONRPC: "2.0",
		ID:      cloneRawMessage(req.ID),
		Result: []byte(`{
			"sessions": [{
				"sessionId": "sess-1",
				"cwd": "/workspace/main",
				"additionalDirectories": ["/workspace/lib"],
				"title": "Session"
			}],
			"nextCursor": "cursor-2"
		}`),
	}
	msg, err := json.Marshal(resp)
	if err != nil {
		t.t.Fatalf("marshal response: %v", err)
	}
	t.recv <- msg
	return nil
}

func (t *listSessionsTransport) Receive(ctx context.Context) ([]byte, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case msg := <-t.recv:
		return msg, nil
	}
}

func (t *listSessionsTransport) Close() error {
	return nil
}

func (t *listSessionsTransport) lastSent() []byte {
	t.mu.Lock()
	defer t.mu.Unlock()
	return append([]byte(nil), t.sent...)
}

type notificationRecordingTransport struct {
	t *testing.T

	mu   sync.Mutex
	sent []byte
}

func newNotificationRecordingTransport(t *testing.T) *notificationRecordingTransport {
	t.Helper()
	return &notificationRecordingTransport{t: t}
}

func (t *notificationRecordingTransport) Send(_ context.Context, message []byte) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.sent = append([]byte(nil), message...)
	return nil
}

func (t *notificationRecordingTransport) Receive(ctx context.Context) ([]byte, error) {
	<-ctx.Done()
	return nil, ctx.Err()
}

func (t *notificationRecordingTransport) Close() error { return nil }

func (t *notificationRecordingTransport) lastSent() []byte {
	t.mu.Lock()
	defer t.mu.Unlock()
	return append([]byte(nil), t.sent...)
}

type responseCaptureTransport struct {
	mu   sync.Mutex
	sent []byte
}

func (t *responseCaptureTransport) Send(_ context.Context, message []byte) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.sent = append([]byte(nil), message...)
	return nil
}

func (t *responseCaptureTransport) Receive(ctx context.Context) ([]byte, error) {
	<-ctx.Done()
	return nil, ctx.Err()
}

func (t *responseCaptureTransport) Close() error { return nil }

func (t *responseCaptureTransport) lastSent() []byte {
	t.mu.Lock()
	defer t.mu.Unlock()
	return append([]byte(nil), t.sent...)
}

func sessionUpdateResponse(t *testing.T, sessionID, text string) *jsonRPCResponse {
	t.Helper()
	method := "session/update"
	params, err := json.Marshal(SessionNotification{
		SessionID: sessionID,
		Update: SessionUpdate{
			SessionUpdate: "agent_message_chunk",
			Content:       Content{Type: "text", Text: text},
		},
	})
	if err != nil {
		t.Fatalf("marshal notification: %v", err)
	}
	return &jsonRPCResponse{Method: &method, Params: params}
}
