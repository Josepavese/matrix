package agents

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	goexec "os/exec"
	"strings"
	"testing"

	"github.com/Josepavese/matrix/internal/middleware"
	"github.com/gorilla/websocket"
)

type mockResolver struct {
	protocol string
	address  string
}

func (m *mockResolver) GetAgentEndpoint(_ string) (middleware.ProtocolEndpoint, error) {
	return middleware.ProtocolEndpoint{
		Kind:      middleware.ProtocolKindACP,
		Transport: m.protocol,
		Address:   m.address,
		Command:   m.address,
	}, nil
}

func buildRouterMockAgent(t *testing.T) string {
	t.Helper()
	tmpDir := t.TempDir()
	mockBin := tmpDir + "/mock-agent"

	buildCmd := goexec.Command("go", "build", "-o", mockBin, "github.com/Josepavese/matrix/cmd/mock-agent")
	if output, err := buildCmd.CombinedOutput(); err != nil {
		t.Fatalf("failed to build mock-agent: %v\n%s", err, output)
	}
	return mockBin
}

func TestRouter_Integration_Stdio(t *testing.T) {
	resolver := &mockResolver{
		protocol: "stdio",
		address:  buildRouterMockAgent(t),
	}
	router := NewRouter(resolver)

	ctx := context.Background()
	output, agentSessionID, _, _, err := router.Route(ctx, middleware.RouteRequest{
		AgentID:          "test-agent",
		LogicalSessionID: "session-1",
		Message:          "ping",
	})
	if err != nil {
		t.Fatalf("Route failed: %v", err)
	}
	if agentSessionID == "" {
		t.Errorf("Expected non-empty agentSessionID")
	}

	expected := "I am a mock agent responding via stdio."
	if output != expected {
		t.Errorf("Expected %q, got %q", expected, output)
	}
}

func TestRouter_Integration_WS(t *testing.T) {
	const (
		mockSessionID = "ws-session-id"
		mockContent   = "I am a mock agent responding via WebSocket."
	)

	// Start a mock WebSocket server
	mux := http.NewServeMux()
	upgrader := websocket.Upgrader{}
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer func() { _ = conn.Close() }()

		for {
			_, message, err := conn.ReadMessage()
			if err != nil {
				return
			}

			var req jsonRPCRequest
			if err := json.Unmarshal(message, &req); err != nil {
				continue
			}

			resp := jsonRPCResponse{
				JSONRPC: "2.0",
				ID:      &req.ID,
			}

			switch req.Method {
			case "initialize":
				resp.Result = json.RawMessage(`{"capabilities": {"edit": {}}}`)
			case "session/new":
				resp.Result = json.RawMessage(fmt.Sprintf(`{"sessionId": "%s"}`, mockSessionID))
			case "session/prompt":
				var params struct {
					SessionID string `json:"sessionId"`
				}
				if err := json.Unmarshal(req.Params, &params); err != nil {
					return
				}

				// Notification
				notif := jsonRPCResponse{
					JSONRPC: "2.0",
					Method:  ptr("session/update"),
					Params:  json.RawMessage(fmt.Sprintf(`{"sessionId": "%s", "update": {"sessionUpdate": "agent_message_chunk", "content": {"type": "text", "text": "%s"}}}`, params.SessionID, mockContent)),
				}
				nBytes, err := json.Marshal(notif)
				if err != nil {
					return
				}
				if err := conn.WriteMessage(websocket.TextMessage, nBytes); err != nil {
					return
				}

				resp.Result = json.RawMessage(`{"stopReason": "end_turn"}`)
			}

			rBytes, err := json.Marshal(resp)
			if err != nil {
				return
			}
			if err := conn.WriteMessage(websocket.TextMessage, rBytes); err != nil {
				return
			}
		}
	})

	server := httptest.NewServer(mux)
	defer server.Close()

	// Convert http:// to ws://
	wsAddr := "ws" + strings.TrimPrefix(server.URL, "http")

	resolver := &mockResolver{
		protocol: "ws",
		address:  wsAddr,
	}
	router := NewRouter(resolver)

	ctx := context.Background()
	output, agentSessionID, _, _, err := router.Route(ctx, middleware.RouteRequest{
		AgentID:          "test-agent-ws",
		LogicalSessionID: "session-ws",
		Message:          "ping",
	})
	if err != nil {
		t.Fatalf("Route failed: %v", err)
	}
	if agentSessionID != mockSessionID {
		t.Errorf("Expected agentSessionID %q, got %q", mockSessionID, agentSessionID)
	}

	if output != mockContent {
		t.Errorf("Expected %q, got %q", mockContent, output)
	}
}
