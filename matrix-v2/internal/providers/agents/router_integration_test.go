package agents

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gorilla/websocket"
	"github.com/jose/matrix-v2/internal/middleware"
)

type mockResolver struct {
	protocol string
	address  string
}

func (m *mockResolver) GetAgentEndpoint(agentID string) (string, string, []string, []string, error) {
	return m.protocol, m.address, nil, nil, nil
}

func TestRouter_Integration_Stdio(t *testing.T) {
	// Get absolute path to the mock-agent binary built in the previous step
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Failed to get working directory: %v", err)
	}
	// Relative path to matrix-v2 root where mock-agent was built
	mockPath := filepath.Join(wd, "../../../mock-agent")

	// Ensure absolute path
	absPath, err := filepath.Abs(mockPath)
	if err != nil {
		t.Fatalf("Failed to get abs path for mock-agent: %v", err)
	}

	if _, err := os.Stat(absPath); err != nil {
		t.Skip("mock-agent binary not found at", absPath, ". Run 'go build -o mock-agent ./cmd/mock-agent/main.go' in root first.")
	}

	resolver := &mockResolver{
		protocol: "stdio",
		address:  absPath,
	}
	router := NewRouter(resolver)

	ctx := context.Background()
	output, agentSessionID, _, err := router.Route(ctx, middleware.RouteRequest{
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
		defer conn.Close()

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
	output, agentSessionID, _, err := router.Route(ctx, middleware.RouteRequest{
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
