package agents

import (
	"context"
	"testing"

	"github.com/Josepavese/matrix/internal/middleware"
)

func TestSupportsSessionCapabilityAcceptsZedObjectStyle(t *testing.T) {
	resp := &acpInitializeResponse{
		Capabilities: map[string]interface{}{
			"sessionCapabilities": map[string]interface{}{
				"list":   map[string]interface{}{},
				"close":  map[string]interface{}{},
				"delete": map[string]interface{}{},
			},
		},
	}

	if !supportsSessionCapability(resp, "list") {
		t.Fatalf("expected object-style list capability")
	}
	if !supportsSessionCapability(resp, "close") {
		t.Fatalf("expected object-style close capability")
	}
	if !supportsSessionCapability(resp, "delete") {
		t.Fatalf("expected object-style delete capability")
	}
}

func TestSupportsSessionCapabilityAcceptsLegacyBooleanTrueOnly(t *testing.T) {
	resp := &acpInitializeResponse{
		Capabilities: map[string]interface{}{
			"sessionCapabilities": map[string]interface{}{
				"list":   true,
				"close":  false,
				"delete": nil,
			},
		},
	}

	if !supportsSessionCapability(resp, "list") {
		t.Fatalf("expected boolean true list capability")
	}
	if supportsSessionCapability(resp, "close") {
		t.Fatalf("did not expect boolean false close capability")
	}
	if supportsSessionCapability(resp, "delete") {
		t.Fatalf("did not expect nil delete capability")
	}
	if supportsSessionCapability(resp, "fork") {
		t.Fatalf("did not expect absent fork capability")
	}
}

func TestACPSessionCapabilitiesExposeLifecycleStability(t *testing.T) {
	resp := &acpInitializeResponse{
		Capabilities: map[string]interface{}{
			"loadSession": true,
			"sessionCapabilities": map[string]interface{}{
				"list":                  map[string]interface{}{},
				"resume":                map[string]interface{}{},
				"fork":                  map[string]interface{}{},
				"additionalDirectories": map[string]interface{}{},
			},
		},
	}
	caps := acpSessionCapabilities(resp)
	if !caps.List || !caps.Load || !caps.Cancel || !caps.Resume || !caps.Fork || !caps.AdditionalDirectories {
		t.Fatalf("expected advertised lifecycle support: %#v", caps)
	}
	if caps.Details["list"].Stability != "stable" {
		t.Fatalf("list should be stable: %#v", caps.Details["list"])
	}
	if caps.Details["resume"].Stability != "stable" {
		t.Fatalf("resume should be stable: %#v", caps.Details["resume"])
	}
	if caps.Details["fork"].Stability != "draft" {
		t.Fatalf("fork should be draft: %#v", caps.Details["fork"])
	}
	if caps.Details["fork"].AsyncSupported == nil || !*caps.Details["fork"].AsyncSupported {
		t.Fatalf("fork should advertise Matrix async polling support: %#v", caps.Details["fork"])
	}
	if caps.Details["fork"].LiveInterventionSuitable == nil || *caps.Details["fork"].LiveInterventionSuitable {
		t.Fatalf("fork should not claim timely live intervention suitability: %#v", caps.Details["fork"])
	}
	if caps.Details["additional_directories"].Stability != "draft" {
		t.Fatalf("additional directories should be draft: %#v", caps.Details["additional_directories"])
	}
}

func TestPickAutoApproveConfigOptionPrefersStableConfigSurface(t *testing.T) {
	resp := &middleware.NewSessionResponse{
		ConfigOptions: []middleware.ConfigOption{
			{
				ID:       "mode",
				Category: "mode",
				Options: []middleware.ConfigOptionValue{
					{ID: "ask", Name: "Ask"},
					{ID: "build", Name: "Build"},
				},
			},
		},
	}
	configID, value := pickAutoApproveConfigOption(resp)
	if configID != "mode" || value != "build" {
		t.Fatalf("unexpected config selection: %s=%s", configID, value)
	}
	if modeID := pickAutoApproveMode(resp); modeID != "" {
		t.Fatalf("legacy mode picker must not consume config options, got %q", modeID)
	}
}

type pagedListACPClient struct {
	ctx       context.Context
	cursors   []string
	newReq    *acpNewSessionRequest
	resumeReq *acpResumeSessionRequest
	promptReq *acpPromptRequest
}

func (c *pagedListACPClient) Context() context.Context            { return c.ctx }
func (c *pagedListACPClient) Close() error                        { return nil }
func (c *pagedListACPClient) SetRequestHandler(acpRequestHandler) {}
func (c *pagedListACPClient) Initialize(context.Context, acpInitializeRequest) (*acpInitializeResponse, error) {
	return &acpInitializeResponse{}, nil
}
func (c *pagedListACPClient) Authenticate(context.Context, string, map[string]string) error {
	return nil
}
func (c *pagedListACPClient) NewSession(_ context.Context, req acpNewSessionRequest) (*acpNewSessionResponse, error) {
	c.newReq = &req
	return &acpNewSessionResponse{SessionID: "remote-new"}, nil
}
func (c *pagedListACPClient) LoadSession(context.Context, acpLoadSessionRequest, acpSessionObserver) (*acpLoadSessionResponse, error) {
	return &acpLoadSessionResponse{}, nil
}
func (c *pagedListACPClient) ResumeSession(_ context.Context, req acpResumeSessionRequest) (*acpResumeSessionResponse, error) {
	c.resumeReq = &req
	return &acpResumeSessionResponse{}, nil
}
func (c *pagedListACPClient) ListSessions(context.Context) (*acpListSessionsResponse, error) {
	return c.ListSessionsWithRequest(context.Background(), acpListSessionsRequest{})
}
func (c *pagedListACPClient) ListSessionsWithRequest(_ context.Context, req acpListSessionsRequest) (*acpListSessionsResponse, error) {
	c.cursors = append(c.cursors, req.Cursor)
	if req.Cursor == "" {
		return &acpListSessionsResponse{
			Sessions:   []acpSessionInfo{{SessionID: "one", Title: "One"}},
			NextCursor: "cursor-2",
		}, nil
	}
	return &acpListSessionsResponse{
		Sessions: []acpSessionInfo{{SessionID: "two", Title: "Two"}},
	}, nil
}
func (c *pagedListACPClient) CancelSession(context.Context, string) error { return nil }
func (c *pagedListACPClient) CloseSession(context.Context, string) error  { return nil }
func (c *pagedListACPClient) DeleteSession(context.Context, string) error { return nil }
func (c *pagedListACPClient) ForkSession(context.Context, acpForkSessionRequest) (*acpForkSessionResponse, error) {
	return &acpForkSessionResponse{}, nil
}
func (c *pagedListACPClient) Prompt(_ context.Context, req acpPromptRequest, _ acpSessionObserver) (*acpPromptResponse, error) {
	c.promptReq = &req
	return &acpPromptResponse{StopReason: "end_turn"}, nil
}
func (c *pagedListACPClient) SetMode(context.Context, string, string) error { return nil }
func (c *pagedListACPClient) SetConfigOption(context.Context, acpSetConfigOptionRequest) (*acpSetConfigOptionResponse, error) {
	return &acpSetConfigOptionResponse{}, nil
}
func (c *pagedListACPClient) ExtRequest(context.Context, string, interface{}, interface{}) error {
	return nil
}
func (c *pagedListACPClient) ExtNotification(context.Context, string, interface{}) error {
	return nil
}

func TestListRemoteSessionsIteratesACPPagination(t *testing.T) {
	fake := &pagedListACPClient{ctx: context.Background()}
	client := &acpConversationClient{
		client: fake,
		cwd:    "/workspace",
		sessionCapabilities: middleware.ConversationSessionCapabilities{
			List:   true,
			Resume: true,
		},
		loadedSessions: map[string]bool{},
	}

	sessions, err := client.ListRemoteSessions(context.Background())
	if err != nil {
		t.Fatalf("list remote sessions: %v", err)
	}
	if len(sessions) != 2 || sessions[0].RemoteSessionID != "one" || sessions[1].RemoteSessionID != "two" {
		t.Fatalf("unexpected sessions: %#v", sessions)
	}
	if len(fake.cursors) != 2 || fake.cursors[0] != "" || fake.cursors[1] != "cursor-2" {
		t.Fatalf("expected cursor pagination, got %#v", fake.cursors)
	}
}

func TestExecuteTurnPropagatesPromptBlocksAndMCPServers(t *testing.T) {
	fake := &pagedListACPClient{ctx: context.Background()}
	client := &acpConversationClient{
		client: fake,
		sessionCapabilities: middleware.ConversationSessionCapabilities{
			AdditionalDirectories: true,
		},
		loadedSessions: map[string]bool{},
	}

	_, err := client.ExecuteTurn(context.Background(), middleware.ConversationTurn{
		Message:               "summarize",
		AdditionalDirectories: []string{"/workspace/lib", "/workspace/lib", "  "},
		ContentBlocks: []middleware.Content{
			{Type: "resource_link", URI: "file:///workspace/main.go", Name: "main.go"},
		},
		McpServers: []middleware.McpServerConfig{
			{Name: "repo", Command: "repo-mcp", Args: []string{"--stdio"}},
		},
	})
	if err != nil {
		t.Fatalf("execute turn: %v", err)
	}
	if fake.promptReq == nil {
		t.Fatal("expected prompt request")
	}
	if fake.newReq == nil || len(fake.newReq.McpServers) != 1 || fake.newReq.McpServers[0].Name != "repo" {
		t.Fatalf("expected MCP server propagation, got %#v", fake.newReq)
	}
	if got := fake.newReq.AdditionalDirectories; len(got) != 1 || got[0] != "/workspace/lib" {
		t.Fatalf("expected capability-gated additional directories, got %#v", got)
	}
	if len(fake.promptReq.Prompt) != 2 {
		t.Fatalf("expected text plus resource_link prompt blocks, got %#v", fake.promptReq.Prompt)
	}
	if fake.promptReq.Prompt[1].Type != "resource_link" || fake.promptReq.Prompt[1].URI != "file:///workspace/main.go" {
		t.Fatalf("expected resource link prompt block, got %#v", fake.promptReq.Prompt[1])
	}
}

func TestExecuteTurnDoesNotSendAdditionalDirectoriesWithoutCapability(t *testing.T) {
	fake := &pagedListACPClient{ctx: context.Background()}
	client := &acpConversationClient{
		client:         fake,
		loadedSessions: map[string]bool{},
	}

	_, err := client.ExecuteTurn(context.Background(), middleware.ConversationTurn{
		Message:               "hello",
		AdditionalDirectories: []string{"/workspace/lib"},
	})
	if err != nil {
		t.Fatalf("execute turn: %v", err)
	}
	if fake.newReq == nil {
		t.Fatal("expected session/new request")
	}
	if len(fake.newReq.AdditionalDirectories) != 0 {
		t.Fatalf("additionalDirectories must be gated on provider capability, got %#v", fake.newReq.AdditionalDirectories)
	}
}

func TestExecuteTurnSendsEmptyMCPServersArray(t *testing.T) {
	fake := &pagedListACPClient{ctx: context.Background()}
	client := &acpConversationClient{
		client:         fake,
		loadedSessions: map[string]bool{},
	}

	_, err := client.ExecuteTurn(context.Background(), middleware.ConversationTurn{Message: "hello"})
	if err != nil {
		t.Fatalf("execute turn: %v", err)
	}
	if fake.newReq == nil {
		t.Fatal("expected session/new request")
	}
	if fake.newReq.McpServers == nil {
		t.Fatalf("mcpServers must serialize as [] instead of null for strict ACP providers")
	}
	if len(fake.newReq.McpServers) != 0 {
		t.Fatalf("expected no MCP servers, got %#v", fake.newReq.McpServers)
	}
}
