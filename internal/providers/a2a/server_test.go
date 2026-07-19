package a2a

import (
	"context"
	"encoding/json"
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

type richSessionRouter struct {
	stubSessionRouter
	content []middleware.Content
}

func (r *richSessionRouter) RouteConversation(_ context.Context, req middleware.ConversationRequest) (string, error) {
	r.channelID = req.ChannelID
	r.agentID = req.AgentID
	r.input = req.Input
	r.content = append([]middleware.Content(nil), req.ContentBlocks...)
	if req.Notifier != nil {
		req.Notifier.OnThought(middleware.ThoughtUpdate{Content: "working", Metadata: map[string]interface{}{"phase": "route"}})
	}
	return "matrix:" + req.Input, nil
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
	var card a2asdk.AgentCard
	if err := json.NewDecoder(cardResp.Body).Decode(&card); err != nil {
		t.Fatalf("decode agent card: %v", err)
	}
	if card.Name != "Matrix" || card.Version != "2" {
		t.Fatalf("unexpected agent card identity: %#v", card)
	}
	if !card.Capabilities.Streaming {
		t.Fatalf("agent card must advertise the implemented SSE streaming surface")
	}
	if len(card.DefaultInputModes) < 3 || card.DefaultInputModes[0] != "text/plain" {
		t.Fatalf("unexpected input modes: %#v", card.DefaultInputModes)
	}
	if len(card.SupportedInterfaces) != 2 {
		t.Fatalf("expected JSON-RPC and HTTP+JSON interfaces, got %#v", card.SupportedInterfaces)
	}
	if got := card.SupportedInterfaces[0]; got.ProtocolBinding != a2asdk.TransportProtocolJSONRPC || string(got.ProtocolVersion) != "1.0" {
		t.Fatalf("unexpected supported interface: %#v", got)
	}
	if got := card.SupportedInterfaces[1]; got.ProtocolBinding != a2asdk.TransportProtocolHTTPJSON || string(got.ProtocolVersion) != "1.0" {
		t.Fatalf("unexpected HTTP+JSON interface: %#v", got)
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

	task, ok := resp.(*a2asdk.Task)
	if !ok {
		t.Fatalf("expected task lifecycle result, got %T", resp)
	}
	if len(task.Artifacts) != 1 {
		t.Fatalf("expected one final artifact, got %#v", task.Artifacts)
	}
	if got := strings.TrimSpace(partsText(task.Artifacts[0].Parts)); got != "matrix:hello a2a" {
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

func TestServer_AgentCardDeclaresAPIKeySecurity(t *testing.T) {
	router := &stubSessionRouter{}
	mux := http.NewServeMux()
	NewServer(router, "http://127.0.0.1:0", "opencode").WithAPIKey("secret").RegisterRoutes(mux)

	req := newJSONRequest(http.MethodGet, "/.well-known/agent-card.json", nil)
	resp := httptest.NewRecorder()
	mux.ServeHTTP(resp, req)
	if resp.Code != http.StatusOK {
		t.Fatalf("agent card status: %d", resp.Code)
	}
	var card a2asdk.AgentCard
	if err := json.Unmarshal(resp.Body.Bytes(), &card); err != nil {
		t.Fatalf("decode card: %v", err)
	}
	if _, ok := card.SecuritySchemes[a2aMatrixAPIKeyScheme].(a2asdk.APIKeySecurityScheme); !ok {
		t.Fatalf("missing X-Matrix-Key security scheme: %#v", card.SecuritySchemes)
	}
	if _, ok := card.SecuritySchemes[a2aMatrixBearerScheme].(a2asdk.HTTPAuthSecurityScheme); !ok {
		t.Fatalf("missing bearer security scheme: %#v", card.SecuritySchemes)
	}
	if len(card.SecurityRequirements) != 2 {
		t.Fatalf("expected API key or bearer security requirements, got %#v", card.SecurityRequirements)
	}
}

func TestServer_A2AAcceptsProtocolJSONContentType(t *testing.T) {
	router := &stubSessionRouter{}
	mux := http.NewServeMux()
	NewServer(router, "http://127.0.0.1:0", "opencode").RegisterRoutes(mux)

	req := httptest.NewRequest(http.MethodPost, "/a2a", strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"ListTasks","params":{"pageSize":1}}`))
	req.Header.Set("Content-Type", "application/a2a+json")
	resp := httptest.NewRecorder()
	mux.ServeHTTP(resp, req)
	if resp.Code == http.StatusUnsupportedMediaType {
		t.Fatalf("application/a2a+json was rejected")
	}
}

func TestServer_A2AStreamsTaskProgressAndArtifacts(t *testing.T) {
	router := &richSessionRouter{}
	mux := http.NewServeMux()
	NewServer(router, "http://127.0.0.1:0", "opencode").RegisterRoutes(mux)
	server := httptest.NewServer(mux)
	defer server.Close()

	client, err := a2aclient.NewFromEndpoints(context.Background(), []*a2asdk.AgentInterface{
		a2asdk.NewAgentInterface(server.URL+"/a2a", a2asdk.TransportProtocolJSONRPC),
	})
	if err != nil {
		t.Fatalf("NewFromEndpoints failed: %v", err)
	}
	defer func() { _ = client.Destroy() }()

	var submitted, working, artifacts, completed int
	for event, streamErr := range client.SendStreamingMessage(context.Background(), &a2asdk.SendMessageRequest{
		Message: a2asdk.NewMessage(a2asdk.MessageRoleUser, a2asdk.NewTextPart("stream me")),
	}) {
		if streamErr != nil {
			t.Fatalf("stream failed: %v", streamErr)
		}
		switch value := event.(type) {
		case *a2asdk.Task:
			if value.Status.State == a2asdk.TaskStateSubmitted {
				submitted++
			}
		case *a2asdk.TaskStatusUpdateEvent:
			switch value.Status.State {
			case a2asdk.TaskStateWorking:
				working++
			case a2asdk.TaskStateCompleted:
				completed++
			}
		case *a2asdk.TaskArtifactUpdateEvent:
			artifacts++
		}
	}
	if submitted != 1 || working < 2 || artifacts != 1 || completed != 1 {
		t.Fatalf("incomplete task stream: submitted=%d working=%d artifacts=%d completed=%d", submitted, working, artifacts, completed)
	}
}

func TestServer_HTTPJSONAcceptsRichA2AContent(t *testing.T) {
	router := &richSessionRouter{}
	mux := http.NewServeMux()
	NewServer(router, "http://127.0.0.1:0", "opencode").RegisterRoutes(mux)
	server := httptest.NewServer(mux)
	defer server.Close()

	client, err := a2aclient.NewFromEndpoints(context.Background(), []*a2asdk.AgentInterface{
		a2asdk.NewAgentInterface(server.URL+"/a2a/rest", a2asdk.TransportProtocolHTTPJSON),
	})
	if err != nil {
		t.Fatalf("NewFromEndpoints failed: %v", err)
	}
	defer func() { _ = client.Destroy() }()

	file := a2asdk.NewRawPart([]byte("png"))
	file.Filename = "image.png"
	file.MediaType = "image/png"
	data := a2asdk.NewDataPart(map[string]any{"priority": "high"})
	data.MediaType = "application/json"
	_, err = client.SendMessage(context.Background(), &a2asdk.SendMessageRequest{
		Message: a2asdk.NewMessage(a2asdk.MessageRoleUser, a2asdk.NewTextPart("inspect"), file, data),
	})
	if err != nil {
		t.Fatalf("HTTP+JSON SendMessage failed: %v", err)
	}
	if router.input != "inspect" || len(router.content) != 3 {
		t.Fatalf("rich request was not projected: input=%q content=%#v", router.input, router.content)
	}
	if router.content[1].Type != "image" || router.content[1].Name != "image.png" || router.content[1].Data == "" {
		t.Fatalf("file part was not preserved: %#v", router.content[1])
	}
	if router.content[2].Type != "data" || !strings.Contains(router.content[2].Data, "priority") {
		t.Fatalf("data part was not preserved: %#v", router.content[2])
	}
}

func TestServer_A2ARejectsNonTextParts(t *testing.T) {
	router := &stubSessionRouter{}
	mux := http.NewServeMux()
	serverAdapter := NewServer(router, "http://127.0.0.1:0", "opencode")
	serverAdapter.RegisterRoutes(mux)

	server := httptest.NewServer(mux)
	defer server.Close()

	client, err := a2aclient.NewFromEndpoints(context.Background(), []*a2asdk.AgentInterface{
		a2asdk.NewAgentInterface(server.URL+"/a2a", a2asdk.TransportProtocolJSONRPC),
	})
	if err != nil {
		t.Fatalf("NewFromEndpoints failed: %v", err)
	}
	defer func() { _ = client.Destroy() }()

	msg := a2asdk.NewMessage(a2asdk.MessageRoleUser, a2asdk.NewDataPart(map[string]any{"not": "text"}))
	result, err := client.SendMessage(context.Background(), &a2asdk.SendMessageRequest{Message: msg})
	if err != nil {
		t.Fatalf("protocol should return a failed task rather than break transport: %v", err)
	}
	task, ok := result.(*a2asdk.Task)
	if !ok || task.Status.State != a2asdk.TaskStateFailed {
		t.Fatalf("expected failed task for router without rich ingress, got %#v", result)
	}
	if router.input != "" {
		t.Fatalf("non-text request should not have reached router, input=%q", router.input)
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
