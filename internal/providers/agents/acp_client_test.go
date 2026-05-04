package agents

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Josepavese/matrix/pkg/zedacp"
)

// mockTransport simulates an AgentTransport sending raw JSON back and forth.
type mockTransport struct {
	mu       sync.Mutex
	received [][]byte

	reader chan []byte
	writer chan []byte

	closeCalled bool
}

type testJSONRPCRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      int64           `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type testJSONRPCResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      *int64          `json:"id,omitempty"`
	Method  *string         `json:"method,omitempty"`
	Params  json.RawMessage `json:"params,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
}

func newMockTransport() *mockTransport {
	return &mockTransport{
		reader: make(chan []byte, 100),
		writer: make(chan []byte, 100),
	}
}

func (m *mockTransport) Send(_ context.Context, message []byte) error {
	m.mu.Lock()
	m.received = append(m.received, message)
	m.mu.Unlock()

	// In the test, we immediately queue a fake response based on the request method
	var req testJSONRPCRequest
	if err := json.Unmarshal(message, &req); err == nil {
		resp := testJSONRPCResponse{
			JSONRPC: "2.0",
			ID:      &req.ID,
		}

		switch req.Method {
		case "initialize":
			resp.Result = []byte(`{"agentCapabilities": {"edit": {}, "loadSession": true}}`)
		case "session/new":
			resp.Result = []byte(`{"sessionId": "test-session-123"}`)
		case "session/load":
			notif := testJSONRPCResponse{
				JSONRPC: "2.0",
				Method:  ptr("session/update"),
				Params:  []byte(`{"sessionId": "test-session-123", "update": {"sessionUpdate": "user_message_chunk", "content": {"type": "text", "text": "Earlier user message"}}}`),
			}
			nBytes, err := json.Marshal(notif)
			if err != nil {
				return err
			}
			m.reader <- nBytes
			resp.Result = []byte(`null`)
		case "session/list":
			resp.Result = []byte(`{"sessions": [{"sessionId": "test-session-123", "title": "Recovered Session"}]}`)
		case "session/cancel":
			return nil
		case "session/close":
			resp.Result = []byte(`{}`)
		case "session/delete":
			resp.Result = []byte(`null`)
		case "session/fork":
			resp.Result = []byte(`{"sessionId": "fork-session-456"}`)
		case "session/prompt":
			resp.Result = []byte(`{"stopReason": "end_turn"}`)
			// Also queue a notification right before the response
			notif := testJSONRPCResponse{
				JSONRPC: "2.0",
				Method:  ptr("session/update"),
				Params:  []byte(`{"sessionId": "test-session-123", "update": {"sessionUpdate": "agent_message_chunk", "content": {"type": "text", "text": "Hello from mock agent!"}}}`),
			}
			nBytes, err := json.Marshal(notif)
			if err != nil {
				return err
			}
			m.reader <- nBytes
			infoNotif := testJSONRPCResponse{
				JSONRPC: "2.0",
				Method:  ptr("session_info_update"),
				Params:  []byte(`{"sessionId":"test-session-123","update":{"sessionUpdate":"info","title":"Recovered Session","updatedAt":"2026-04-14T20:00:00Z","_meta":{"source":"agent"}}}`),
			}
			infoBytes, err := json.Marshal(infoNotif)
			if err != nil {
				return err
			}
			m.reader <- infoBytes
		}

		rBytes, err := json.Marshal(resp)
		if err != nil {
			return err
		}
		// Small artificial delay to ensure listener is ready
		go func() {
			time.Sleep(10 * time.Millisecond)
			m.reader <- rBytes
		}()
	}

	return nil
}

func (m *mockTransport) Receive(ctx context.Context) ([]byte, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case msg := <-m.reader:
		return msg, nil
	}
}

func (m *mockTransport) Close() error {
	m.mu.Lock()
	m.closeCalled = true
	m.mu.Unlock()
	return nil
}

func ptr(s string) *string {
	return &s
}

type testObserver struct {
	updates []string
	title   string
	updated string
	sync.Mutex
}

func (o *testObserver) OnUpdate(notif zedacp.SessionNotification) {
	o.Lock()
	defer o.Unlock()
	o.updates = append(o.updates, notif.Update.Content.Text)
	if notif.Update.Title != "" {
		o.title = notif.Update.Title
	}
	if notif.Update.UpdatedAt != "" {
		o.updated = notif.Update.UpdatedAt
	}
}

func TestACPClient_FullLifecycle(t *testing.T) {
	transport := newMockTransport()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	client := NewACPClient(ctx, transport)

	// 1. Initialize
	t.Run("Initialize", func(t *testing.T) {
		req := zedacp.InitializeRequest{
			ProtocolVersion: 1,
			ClientInfo:      map[string]interface{}{"name": "matrix", "version": "1.0"},
		}
		res, err := client.Initialize(ctx, req)
		if err != nil {
			t.Fatalf("Initialize failed: %v", err)
		}
		if res.Capabilities == nil {
			t.Errorf("Expected capabilities in response")
		}
	})

	t.Run("LoadSession", func(t *testing.T) {
		obs := &testObserver{}
		_, err := client.LoadSession(ctx, zedacp.LoadSessionRequest{
			SessionID:  "test-session-123",
			Cwd:        "/tmp",
			McpServers: nil,
		}, obs)
		if err != nil {
			t.Fatalf("LoadSession failed: %v", err)
		}
		time.Sleep(50 * time.Millisecond)
		obs.Lock()
		defer obs.Unlock()
		if len(obs.updates) == 0 {
			t.Fatalf("expected replayed updates from session/load")
		}
	})

	t.Run("ListSessions", func(t *testing.T) {
		res, err := client.ListSessions(ctx)
		if err != nil {
			t.Fatalf("ListSessions failed: %v", err)
		}
		if len(res.Sessions) != 1 || res.Sessions[0].SessionID != "test-session-123" {
			t.Fatalf("unexpected list response: %+v", res)
		}
	})

	t.Run("CancelSession", func(t *testing.T) {
		if err := client.CancelSession(ctx, "test-session-123"); err != nil {
			t.Fatalf("CancelSession failed: %v", err)
		}
		transport.mu.Lock()
		defer transport.mu.Unlock()
		last := transport.received[len(transport.received)-1]
		var raw map[string]interface{}
		if err := json.Unmarshal(last, &raw); err != nil {
			t.Fatalf("unmarshal sent notification: %v", err)
		}
		if raw["method"] != "session/cancel" {
			t.Fatalf("expected session/cancel notification, got %+v", raw)
		}
		if _, ok := raw["id"]; ok {
			t.Fatalf("expected notification without id, got %+v", raw)
		}
	})

	t.Run("CloseSession", func(t *testing.T) {
		if err := client.CloseSession(ctx, "test-session-123"); err != nil {
			t.Fatalf("CloseSession failed: %v", err)
		}
		transport.mu.Lock()
		defer transport.mu.Unlock()
		last := transport.received[len(transport.received)-1]
		var raw map[string]interface{}
		if err := json.Unmarshal(last, &raw); err != nil {
			t.Fatalf("unmarshal sent request: %v", err)
		}
		if raw["method"] != "session/close" {
			t.Fatalf("expected session/close request, got %+v", raw)
		}
		if _, ok := raw["id"]; !ok {
			t.Fatalf("expected request id, got %+v", raw)
		}
	})

	t.Run("DeleteSession", func(t *testing.T) {
		if err := client.DeleteSession(ctx, "test-session-123"); err != nil {
			t.Fatalf("DeleteSession failed: %v", err)
		}
	})

	t.Run("ForkSession", func(t *testing.T) {
		res, err := client.ForkSession(ctx, zedacp.ForkSessionRequest{SessionID: "test-session-123", Cwd: "/tmp"})
		if err != nil {
			t.Fatalf("ForkSession failed: %v", err)
		}
		if res.SessionID != "fork-session-456" {
			t.Fatalf("unexpected fork response: %+v", res)
		}
	})

	// 2. NewSession
	t.Run("NewSession", func(t *testing.T) {
		req := zedacp.NewSessionRequest{ClientTitle: "Test"}
		res, err := client.NewSession(ctx, req)
		if err != nil {
			t.Fatalf("NewSession failed: %v", err)
		}
		if res.SessionID != "test-session-123" {
			t.Errorf("Expected test-session-123, got %s", res.SessionID)
		}
	})

	// 3. Prompt
	t.Run("Prompt", func(t *testing.T) {
		req := zedacp.PromptRequest{
			SessionID: "test-session-123",
			Prompt:    []zedacp.Content{{Type: "text", Text: "Hello"}},
		}
		obs := &testObserver{}
		res, err := client.Prompt(ctx, req, obs)
		if err != nil {
			t.Fatalf("Prompt failed: %v", err)
		}
		if res.StopReason != "end_turn" {
			t.Errorf("Expected stopReason end_turn, got %s", res.StopReason)
		}

		// Wait briefly to allow async sessionUpdate to process
		time.Sleep(50 * time.Millisecond)

		obs.Lock()
		defer obs.Unlock()
		if len(obs.updates) == 0 {
			t.Errorf("Expected observer updates, got none")
		} else {
			joined := strings.Join(obs.updates, "")
			if joined != "Hello from mock agent!" {
				t.Errorf("Unexpected output content: %s", joined)
			}
		}
		if obs.title != "Recovered Session" {
			t.Fatalf("expected title from session_info_update, got %q", obs.title)
		}
		if obs.updated != "2026-04-14T20:00:00Z" {
			t.Fatalf("expected updatedAt from session_info_update, got %q", obs.updated)
		}
	})
}
