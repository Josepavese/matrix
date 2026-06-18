package a2a

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Josepavese/matrix/internal/middleware"
	a2asdk "github.com/a2aproject/a2a-go/v2/a2a"
	"github.com/a2aproject/a2a-go/v2/a2aclient"
)

func newJSONRequest(method, target string, body io.Reader) *http.Request {
	req := httptest.NewRequest(method, target, body)
	if method == http.MethodPost {
		req.Header.Set("Content-Type", "application/json")
	}
	return req
}

type stubSessionRouter struct {
	channelID string
	agentID   string
	input     string
}

func (s *stubSessionRouter) Route(_ context.Context, channelID string, agentID string, input string, _ middleware.ThoughtNotifier) (string, error) {
	s.channelID = channelID
	s.agentID = agentID
	s.input = input
	return "matrix:" + input, nil
}

func TestServer_RegisterRoutesAndHandleMessage(t *testing.T) {
	router := &stubSessionRouter{}
	mux := http.NewServeMux()
	serverAdapter := NewServer(router, "http://127.0.0.1:0", "opencode")
	serverAdapter.RegisterRoutes(mux)

	server := httptest.NewServer(mux)
	defer server.Close()

	cardResp, err := http.Get(server.URL + "/.well-known/agent-card.json")
	if err != nil {
		t.Fatalf("GET agent card failed: %v", err)
	}
	defer func() { _ = cardResp.Body.Close() }()
	if cardResp.StatusCode != http.StatusOK {
		t.Fatalf("unexpected agent card status: %d", cardResp.StatusCode)
	}

	client, err := a2aclient.NewFromEndpoints(context.Background(), []*a2asdk.AgentInterface{
		a2asdk.NewAgentInterface(server.URL+"/a2a", a2asdk.TransportProtocolJSONRPC),
	})
	if err != nil {
		t.Fatalf("NewFromEndpoints failed: %v", err)
	}
	defer func() { _ = client.Destroy() }()

	msg := a2asdk.NewMessage(a2asdk.MessageRoleUser, a2asdk.NewTextPart("hello a2a"))
	msg.Metadata = map[string]any{
		"channel_id": "a2a:test",
		"agent_id":   "gemini",
	}
	resp, err := client.SendMessage(context.Background(), &a2asdk.SendMessageRequest{Message: msg})
	if err != nil {
		t.Fatalf("SendMessage failed: %v", err)
	}

	message, ok := resp.(*a2asdk.Message)
	if !ok {
		t.Fatalf("expected *a2a.Message, got %T", resp)
	}
	if got := strings.TrimSpace(partsText(message.Parts)); got != "matrix:hello a2a" {
		t.Fatalf("unexpected response output: %q", got)
	}
	if router.channelID != "a2a:test" {
		t.Fatalf("expected channel id a2a:test, got %q", router.channelID)
	}
	if router.agentID != "gemini" {
		t.Fatalf("expected agent id gemini, got %q", router.agentID)
	}
	if router.input != "hello a2a" {
		t.Fatalf("expected input hello a2a, got %q", router.input)
	}
}

func TestServer_A2ARequiresAPIKeyWhenConfigured(t *testing.T) {
	router := &stubSessionRouter{}
	mux := http.NewServeMux()
	NewServer(router, "http://127.0.0.1:0", "opencode").WithAPIKey("secret").RegisterRoutes(mux)

	cardReq := newJSONRequest(http.MethodGet, "/.well-known/agent-card.json", nil)
	cardResp := httptest.NewRecorder()
	mux.ServeHTTP(cardResp, cardReq)
	if cardResp.Code != http.StatusOK {
		t.Fatalf("agent card should stay public, got %d", cardResp.Code)
	}

	req := newJSONRequest(http.MethodPost, "/a2a", strings.NewReader(`{}`))
	resp := httptest.NewRecorder()
	mux.ServeHTTP(resp, req)
	if resp.Code != http.StatusUnauthorized {
		t.Fatalf("expected unauthorized without API key, got %d", resp.Code)
	}

	req = newJSONRequest(http.MethodPost, "/a2a", strings.NewReader(`{}`))
	req.Header.Set("Authorization", "Bearer secret")
	resp = httptest.NewRecorder()
	mux.ServeHTTP(resp, req)
	if resp.Code == http.StatusUnauthorized {
		t.Fatalf("bearer token was rejected")
	}
}

func TestServer_A2ARejectsNonJSONContentTypeWithoutAPIKey(t *testing.T) {
	router := &stubSessionRouter{}
	mux := http.NewServeMux()
	NewServer(router, "http://127.0.0.1:0", "opencode").RegisterRoutes(mux)

	req := httptest.NewRequest(http.MethodPost, "/a2a", strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "text/plain")
	resp := httptest.NewRecorder()
	mux.ServeHTTP(resp, req)
	if resp.Code != http.StatusUnsupportedMediaType {
		t.Fatalf("expected unsupported media type, got %d", resp.Code)
	}
	if router.input != "" {
		t.Fatalf("request should not have reached router, input=%q", router.input)
	}
}
