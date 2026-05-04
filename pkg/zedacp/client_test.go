package zedacp

import (
	"bytes"
	"context"
	"encoding/json"
	"sync"
	"testing"
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

func TestSessionLifecycleRequestsMarshalLatestUnstableFields(t *testing.T) {
	req := ForkSessionRequest{
		SessionID:             "sess_parent",
		Cwd:                   "/workspace/main",
		AdditionalDirectories: []string{"/workspace/lib", "/workspace/docs"},
		McpServers:            []McpServerConfig{},
		Meta:                  map[string]interface{}{"trace": "matrix"},
	}

	got, err := json.Marshal(req)
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
		Cwd:                   "/workspace/main",
		AdditionalDirectories: []string{"/workspace/lib"},
		Cursor:                "cursor-1",
		Meta:                  map[string]interface{}{"source": "test"},
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
		[]byte(`"additionalDirectories":["/workspace/lib"]`),
		[]byte(`"cursor":"cursor-1"`),
		[]byte(`"_meta":{"source":"test"}`),
	} {
		if !bytes.Contains(sent.Params, want) {
			t.Fatalf("expected %s in params: %s", want, sent.Params)
		}
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
		SessionID:  "sess-1",
		Cwd:        "/workspace/main",
		McpServers: []McpServerConfig{},
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
}

func TestSessionUpdateUnmarshalStructuredVariants(t *testing.T) {
	var notif SessionNotification
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
	t.mu.Unlock()
	result, ok := t.responses[req.Method]
	if !ok {
		t.t.Fatalf("unexpected method: %s", req.Method)
	}
	resp := jsonRPCResponse{JSONRPC: "2.0", ID: &req.ID, Result: result}
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
		ID:      &req.ID,
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
