package agents

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jose/matrix-v2/internal/middleware"
)

// mockTransport simulates an AgentTransport sending raw JSON back and forth.
type mockTransport struct {
	mu       sync.Mutex
	received [][]byte

	reader chan []byte
	writer chan []byte

	closeCalled bool
}

func newMockTransport() *mockTransport {
	return &mockTransport{
		reader: make(chan []byte, 100),
		writer: make(chan []byte, 100),
	}
}

func (m *mockTransport) Send(ctx context.Context, message []byte) error {
	m.mu.Lock()
	m.received = append(m.received, message)
	m.mu.Unlock()

	// In the test, we immediately queue a fake response based on the request method
	var req jsonRPCRequest
	if err := json.Unmarshal(message, &req); err == nil {
		resp := jsonRPCResponse{
			JSONRPC: "2.0",
			ID:      &req.ID,
		}

		switch req.Method {
		case "initialize":
			resp.Result = []byte(`{"capabilities": {"edit": {}}}`)
		case "session/new":
			resp.Result = []byte(`{"sessionId": "test-session-123"}`)
		case "session/prompt":
			resp.Result = []byte(`{"stopReason": "end_turn"}`)
			// Also queue a notification right before the response
			notif := jsonRPCResponse{
				JSONRPC: "2.0",
				Method:  ptr("session/update"),
				Params:  []byte(`{"sessionId": "test-session-123", "update": {"sessionUpdate": "agent_message_chunk", "content": {"type": "text", "text": "Hello from mock agent!"}}}`),
			}
			nBytes, err := json.Marshal(notif)
			if err != nil {
				return err
			}
			m.reader <- nBytes
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
	sync.Mutex
}

func (o *testObserver) OnUpdate(notif middleware.SessionNotification) {
	o.Lock()
	defer o.Unlock()
	o.updates = append(o.updates, notif.Update.Content.Text)
}

func TestACPClient_FullLifecycle(t *testing.T) {
	transport := newMockTransport()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	client := NewACPClient(ctx, transport)

	// 1. Initialize
	t.Run("Initialize", func(t *testing.T) {
		req := middleware.InitializeRequest{
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

	// 2. NewSession
	t.Run("NewSession", func(t *testing.T) {
		req := middleware.NewSessionRequest{ClientTitle: "Test"}
		res, err := client.NewSession(ctx, req)
		if err != nil {
			t.Fatalf("NewSession failed: %v", err)
		}
		if res.SessionId != "test-session-123" {
			t.Errorf("Expected test-session-123, got %s", res.SessionId)
		}
	})

	// 3. Prompt
	t.Run("Prompt", func(t *testing.T) {
		req := middleware.PromptRequest{
			SessionId: "test-session-123",
			Prompt:    []middleware.Content{{Type: "text", Text: "Hello"}},
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
	})
}
