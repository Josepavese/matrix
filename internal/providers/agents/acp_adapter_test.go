package agents

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/Josepavese/matrix/internal/middleware"
	"github.com/Josepavese/matrix/internal/providers/exec"
	"github.com/Josepavese/matrix/pkg/zedacp"
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

	caps := acpSessionCapabilities(resp)
	if !caps.List {
		t.Fatalf("expected object-style list capability")
	}
	if !caps.Close {
		t.Fatalf("expected object-style close capability")
	}
	if !caps.Delete {
		t.Fatalf("expected object-style delete capability")
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
				"delete":                map[string]interface{}{},
				"additionalDirectories": map[string]interface{}{},
			},
		},
	}
	caps := acpSessionCapabilities(resp)
	if !caps.List || !caps.Load || !caps.Cancel || !caps.Resume || !caps.Fork || !caps.Delete || !caps.AdditionalDirectories {
		t.Fatalf("expected advertised lifecycle support: %#v", caps)
	}
	if caps.Details["list"].Stability != "stable" {
		t.Fatalf("list should be stable: %#v", caps.Details["list"])
	}
	if caps.Details["resume"].Stability != "stable" {
		t.Fatalf("resume should be stable: %#v", caps.Details["resume"])
	}
	if caps.Details["delete"].Stability != "stable" {
		t.Fatalf("delete should be stable: %#v", caps.Details["delete"])
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
	if caps.Details["additional_directories"].Stability != "stable" {
		t.Fatalf("additional directories should follow ACP v1 stable schema: %#v", caps.Details["additional_directories"])
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
		t.Fatalf("mode picker must not consume config options, got %q", modeID)
	}
}

func TestApplyPreferredSessionModeUsesExactConfiguredValue(t *testing.T) {
	fake := &pagedListACPClient{ctx: context.Background()}
	client := &acpConversationClient{client: fake, preferredMode: "agent-full-access"}
	session := &middleware.NewSessionResponse{ConfigOptions: []middleware.ConfigOption{{
		ID: "mode", Category: "mode", Options: []middleware.ConfigOptionValue{{ID: "agent"}, {ID: "agent-full-access"}},
	}}}
	if err := client.applyPreferredSessionMode(context.Background(), session, "remote"); err != nil {
		t.Fatal(err)
	}
	if fake.setConfigReq == nil || fake.setConfigReq.Value != "agent-full-access" {
		t.Fatalf("configured mode was downgraded: %#v", fake.setConfigReq)
	}
}

func TestApplyPreferredSessionModeFailsClosedWhenUnavailable(t *testing.T) {
	client := &acpConversationClient{client: &pagedListACPClient{ctx: context.Background()}, preferredMode: "agent-full-access"}
	session := &middleware.NewSessionResponse{Modes: &middleware.SessionModeState{
		CurrentModeID: "agent", AvailableModes: []middleware.SessionMode{{ID: "agent"}},
	}}
	err := client.applyPreferredSessionMode(context.Background(), session, "remote")
	if err == nil || !strings.Contains(err.Error(), "unavailable") {
		t.Fatalf("expected fail-closed mode error, got %v", err)
	}
}

func TestACPClientCapabilitiesAdvertiseStableBooleanConfig(t *testing.T) {
	caps := acpClientCapabilitiesForDeps(middleware.ConversationFactoryDeps{Process: exec.NewProvider()})
	if !caps.Terminal {
		t.Fatalf("terminal tool capability should follow process backend availability: %#v", caps)
	}
	if caps.Session == nil || caps.Session.ConfigOptions == nil || caps.Session.ConfigOptions.Boolean == nil {
		t.Fatalf("stable session.configOptions.boolean capability should be advertised: %#v", caps)
	}
}

type pagedListACPClient struct {
	ctx                context.Context
	protocolVersion    int
	initializeResponse *acpInitializeResponse
	cursors            []string
	listReqs           []acpListSessionsRequest
	listErr            error
	newReq             *acpNewSessionRequest
	resumeReq          *acpResumeSessionRequest
	resumeErr          error
	promptReq          *acpPromptRequest
	promptUpdates      []acpSessionNotification
	authID             string
	logouts            int
	setModeID          string
	setConfigReq       *acpSetConfigOptionRequest
	setModelReq        *acpSetSessionModelRequest
	setModelErr        error
}

func (c *pagedListACPClient) Context() context.Context            { return c.ctx }
func (c *pagedListACPClient) Close() error                        { return nil }
func (c *pagedListACPClient) SetRequestHandler(acpRequestHandler) {}
func (c *pagedListACPClient) AuthenticatedProtocolVersion() int   { return c.protocolVersion }
func (c *pagedListACPClient) Initialize(context.Context, acpInitializeRequest) (*acpInitializeResponse, error) {
	if c.initializeResponse != nil {
		return c.initializeResponse, nil
	}
	return &acpInitializeResponse{ProtocolVersion: supportedACPProtocolVersion}, nil
}

func (c *pagedListACPClient) Authenticate(_ context.Context, methodID string) error {
	c.authID = methodID
	return nil
}
func (c *pagedListACPClient) Logout(context.Context, acpLogoutRequest) (*acpLogoutResponse, error) {
	c.logouts++
	return &acpLogoutResponse{}, nil
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
	return &acpResumeSessionResponse{}, c.resumeErr
}
func (c *pagedListACPClient) SetSessionModel(_ context.Context, req acpSetSessionModelRequest) (*acpSetSessionModelResponse, error) {
	c.setModelReq = &req
	return &acpSetSessionModelResponse{}, c.setModelErr
}
func (c *pagedListACPClient) ListSessions(context.Context) (*acpListSessionsResponse, error) {
	return c.ListSessionsWithRequest(context.Background(), acpListSessionsRequest{})
}
func (c *pagedListACPClient) ListSessionsWithRequest(_ context.Context, req acpListSessionsRequest) (*acpListSessionsResponse, error) {
	if c.listErr != nil {
		return nil, c.listErr
	}
	c.cursors = append(c.cursors, req.Cursor)
	c.listReqs = append(c.listReqs, req)
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

func TestAttachExistingSessionReportsListFailureButResumesExactID(t *testing.T) {
	fake := &pagedListACPClient{ctx: context.Background(), listErr: errors.New("list temporarily unavailable")}
	client := &acpConversationClient{
		client: fake, cwd: "/workspace", loadedSessions: map[string]bool{},
		sessionCapabilities: middleware.ConversationSessionCapabilities{List: true, Resume: true},
	}
	info, err := client.AttachExistingRemoteSession(context.Background(), "external-id", "/workspace")
	if err != nil || info.RemoteSessionID != "external-id" || info.VerificationMethod != "session/resume" || !strings.Contains(info.ListWarning, "list temporarily unavailable") {
		t.Fatalf("attach receipt=%+v err=%v", info, err)
	}
	if fake.newReq != nil || fake.promptReq != nil || fake.resumeReq == nil || fake.resumeReq.SessionID != "external-id" {
		t.Fatalf("strict attach used wrong method: new=%+v prompt=%+v resume=%+v", fake.newReq, fake.promptReq, fake.resumeReq)
	}
}

func TestAttachExistingSessionPreservesListFailureWhenResumeAlsoFails(t *testing.T) {
	fake := &pagedListACPClient{ctx: context.Background(), listErr: errors.New("list temporarily unavailable"), resumeErr: errors.New("unknown session")}
	client := &acpConversationClient{client: fake, cwd: "/workspace", loadedSessions: map[string]bool{},
		sessionCapabilities: middleware.ConversationSessionCapabilities{List: true, Resume: true}}
	_, err := client.AttachExistingRemoteSession(context.Background(), "external-id", "/workspace")
	if err == nil || !strings.Contains(err.Error(), "list temporarily unavailable") || !strings.Contains(err.Error(), "unknown session") {
		t.Fatalf("attach lost one of the provider errors: %v", err)
	}
	if fake.newReq != nil || fake.promptReq != nil {
		t.Fatal("failed strict attach created a session or sent a prompt")
	}
}
func (c *pagedListACPClient) CancelSession(context.Context, string) error { return nil }
func (c *pagedListACPClient) CloseSession(context.Context, string) error  { return nil }
func (c *pagedListACPClient) DeleteSession(context.Context, string) error { return nil }
func (c *pagedListACPClient) ForkSession(context.Context, acpForkSessionRequest) (*acpForkSessionResponse, error) {
	return &acpForkSessionResponse{}, nil
}
func (c *pagedListACPClient) Prompt(_ context.Context, req acpPromptRequest, observer acpSessionObserver) (*acpPromptResponse, error) {
	c.promptReq = &req
	for _, update := range c.promptUpdates {
		observer.OnUpdate(update)
	}
	return &acpPromptResponse{StopReason: "end_turn"}, nil
}
func (c *pagedListACPClient) SetMode(_ context.Context, _ string, modeID string) error {
	c.setModeID = modeID
	return nil
}
func (c *pagedListACPClient) SetConfigOption(_ context.Context, req acpSetConfigOptionRequest) (*acpSetConfigOptionResponse, error) {
	c.setConfigReq = &req
	if req.ConfigID == "model" {
		if c.setModelErr != nil {
			return nil, c.setModelErr
		}
		return &acpSetConfigOptionResponse{ConfigOptions: []zedacp.ConfigOption{{ID: "model", Current: req.Value.(string)}}}, nil
	}
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
	if len(fake.listReqs) == 0 || fake.listReqs[0].Cwd != "" {
		t.Fatalf("remote session discovery should not force a cwd filter, got %#v", fake.listReqs)
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

func TestExecuteTurnAppliesRequestedModelBeforePromptAndFailsClosed(t *testing.T) {
	for _, reject := range []bool{false, true} {
		fake := &pagedListACPClient{ctx: context.Background(), protocolVersion: 2}
		if reject {
			fake.setModelErr = errors.New("model unavailable")
		}
		client := &acpConversationClient{client: fake, loadedSessions: map[string]bool{}}
		_, err := client.ExecuteTurn(context.Background(), middleware.ConversationTurn{Message: "work", ModelID: "chosen-model"})
		if fake.setModelReq == nil || fake.setModelReq.ModelID != "chosen-model" {
			t.Fatalf("model not sent: %+v", fake.setModelReq)
		}
		if reject {
			if err == nil || fake.promptReq != nil {
				t.Fatalf("rejected model reached prompt: err=%v prompt=%+v", err, fake.promptReq)
			}
		} else if err != nil || fake.promptReq == nil {
			t.Fatalf("accepted model failed: err=%v prompt=%+v", err, fake.promptReq)
		}
	}
}

func TestExecuteTurnUsesACPv1ModelConfigOption(t *testing.T) {
	fake := &pagedListACPClient{ctx: context.Background(), protocolVersion: 1}
	client := &acpConversationClient{client: fake, loadedSessions: map[string]bool{}}
	_, err := client.ExecuteTurn(context.Background(), middleware.ConversationTurn{Message: "work", ModelID: "chosen-model"})
	if err != nil || fake.setConfigReq == nil || fake.setConfigReq.ConfigID != "model" || fake.promptReq == nil {
		t.Fatalf("ACP v1 model selection failed: err=%v config=%+v prompt=%+v", err, fake.setConfigReq, fake.promptReq)
	}
}

func TestExecuteTurnReturnsOnlyExplicitFinalACPMessage(t *testing.T) {
	phase := func(value string) map[string]interface{} {
		return map[string]interface{}{"codex": map[string]interface{}{"phase": value}}
	}
	fake := &pagedListACPClient{
		ctx: context.Background(),
		promptUpdates: []acpSessionNotification{
			{SessionID: "remote-new", Update: acpSessionUpdate{SessionUpdate: "agent_message_chunk", MessageID: "commentary-1", Content: acpContent{Type: "text", Text: "Controllo. "}, Contents: []acpContent{{Type: "text", Text: "Controllo. "}}, Meta: phase("commentary")}},
			{SessionID: "remote-new", Update: acpSessionUpdate{SessionUpdate: "agent_message_chunk", MessageID: "final-1", Content: acpContent{Type: "text", Text: "Ciao "}, Contents: []acpContent{{Type: "text", Text: "Ciao "}}, Meta: phase("final_answer")}},
			{SessionID: "remote-new", Update: acpSessionUpdate{SessionUpdate: "agent_message_chunk", MessageID: "final-1", Content: acpContent{Type: "text", Text: "Jose"}, Contents: []acpContent{{Type: "text", Text: "Jose"}}, Meta: phase("final_answer")}},
			{SessionID: "remote-new", Update: acpSessionUpdate{SessionUpdate: "agent_message_chunk", Content: acpContent{Type: "text", Text: "Warning: disk full"}, Contents: []acpContent{{Type: "text", Text: "Warning: disk full"}}}},
		},
	}
	client := &acpConversationClient{client: fake, loadedSessions: map[string]bool{}}
	result, err := client.ExecuteTurn(context.Background(), middleware.ConversationTurn{AgentID: "codex", Message: "ciao"})
	if err != nil {
		t.Fatalf("execute turn: %v", err)
	}
	if result.Output != "Ciao Jose" {
		t.Fatalf("final output contaminated: %q", result.Output)
	}
	if len(result.ContentBlocks) != 2 || result.ContentBlocks[0].Text != "Ciao " || result.ContentBlocks[1].Text != "Jose" {
		t.Fatalf("final blocks contaminated: %#v", result.ContentBlocks)
	}
}

func TestExecuteTurnRejectsAdditionalDirectoriesWithoutCapability(t *testing.T) {
	fake := &pagedListACPClient{ctx: context.Background()}
	client := &acpConversationClient{
		client:         fake,
		loadedSessions: map[string]bool{},
	}

	_, err := client.ExecuteTurn(context.Background(), middleware.ConversationTurn{
		Message:               "hello",
		AdditionalDirectories: []string{"/workspace/lib"},
	})
	if err == nil || !strings.Contains(err.Error(), "does not advertise additionalDirectories") {
		t.Fatalf("expected unsupported capability, got %v", err)
	}
	if fake.newReq != nil {
		t.Fatal("must reject before session/new")
	}
}

func TestExecuteTurnRejectsRelativeAdditionalDirectoriesWhenAdvertised(t *testing.T) {
	fake := &pagedListACPClient{ctx: context.Background()}
	client := &acpConversationClient{
		client: fake,
		sessionCapabilities: middleware.ConversationSessionCapabilities{
			AdditionalDirectories: true,
		},
		loadedSessions: map[string]bool{},
	}

	_, err := client.ExecuteTurn(context.Background(), middleware.ConversationTurn{
		Message:               "hello",
		AdditionalDirectories: []string{"relative/path"},
	})
	if err == nil {
		t.Fatal("expected relative additionalDirectories path to be rejected")
	}
	if fake.newReq != nil {
		t.Fatalf("invalid additionalDirectories must fail before session/new, got %#v", fake.newReq)
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

func TestExecuteTurnGatesOptionalACPContentBeforeCreatingSession(t *testing.T) {
	fake := &pagedListACPClient{ctx: context.Background()}
	client := &acpConversationClient{client: fake, loadedSessions: map[string]bool{}}

	_, err := client.ExecuteTurn(context.Background(), middleware.ConversationTurn{
		ContentBlocks: []middleware.Content{{Type: "image", Data: "aW1hZ2U=", MimeType: "image/png"}},
	})
	if err == nil || fake.newReq != nil {
		t.Fatalf("unadvertised image must fail before session/new: req=%#v err=%v", fake.newReq, err)
	}
	client.featureCapabilities.promptImage = true
	if _, err := client.ExecuteTurn(context.Background(), middleware.ConversationTurn{
		ContentBlocks: []middleware.Content{{Type: "image", Data: "aW1hZ2U=", MimeType: "image/png"}},
	}); err != nil {
		t.Fatalf("advertised image content failed: %v", err)
	}
}

func TestExecuteTurnGatesACPMCPTransportBeforeCreatingSession(t *testing.T) {
	fake := &pagedListACPClient{ctx: context.Background()}
	client := &acpConversationClient{client: fake, loadedSessions: map[string]bool{}}

	_, err := client.ExecuteTurn(context.Background(), middleware.ConversationTurn{
		Message:    "mcp",
		McpServers: []middleware.McpServerConfig{{Name: "remote", Type: "http", URL: "https://mcp.example"}},
	})
	if err == nil || fake.newReq != nil {
		t.Fatalf("unadvertised HTTP MCP must fail before session/new: req=%#v err=%v", fake.newReq, err)
	}
}

func TestACPAuthenticationUsesStableAgentMethodAndCapabilityGatedLogout(t *testing.T) {
	fake := &pagedListACPClient{ctx: context.Background()}
	client := &acpConversationClient{
		client: fake,
		authMethods: []acpAuthMethod{
			{Type: "agent", ID: "browser", Name: "Browser login"},
			{Type: "env_var", ID: "retired", Name: "Retired draft shape"},
		},
		featureCapabilities: acpFeatureCapabilities{logout: true},
	}
	if methods := client.AuthenticationMethods(); len(methods) != 1 || methods[0].ID != "browser" {
		t.Fatalf("unexpected stable authentication methods: %#v", methods)
	}
	if err := client.Authenticate(context.Background(), "retired"); err == nil {
		t.Fatal("retired authentication shape must fail closed")
	}
	if err := client.Authenticate(context.Background(), "browser"); err != nil || fake.authID != "browser" {
		t.Fatalf("stable authentication failed: auth=%q err=%v", fake.authID, err)
	}
	if err := client.Logout(context.Background()); err != nil || fake.logouts != 1 {
		t.Fatalf("capability-gated logout failed: calls=%d err=%v", fake.logouts, err)
	}
}

func TestACPProtocolCapabilitiesExposeStableSurface(t *testing.T) {
	client := &acpConversationClient{
		endpoint:            middleware.ProtocolEndpoint{Transport: "stdio"},
		featureCapabilities: acpFeatureCapabilities{promptImage: true, fsRead: true},
		sessionCapabilities: middleware.ConversationSessionCapabilities{List: true},
	}
	report := client.ProtocolCapabilities()
	if !report.Operations["session/prompt"].Supported || !report.Operations["session/list"].Supported {
		t.Fatalf("missing stable ACP operations: %#v", report.Operations)
	}
	if !report.Content["image"].Supported || report.Content["audio"].Supported {
		t.Fatalf("content capability gates lost: %#v", report.Content)
	}
}

// TestACPSessionCapabilitiesComeFromTheVersionTwoSurface is the version 2 half
// of the session capability read: version 2 makes the baseline methods implicit
// in capabilities.session, nests the optional markers under it, and removed
// session/load. Reading only the version 1 markers reported every one of them as
// unsupported, so a conforming version 2 agent could never be listed, resumed or
// closed.
func TestACPSessionCapabilitiesComeFromTheVersionTwoSurface(t *testing.T) {
	resp := &acpInitializeResponse{
		ProtocolVersion: zedacp.ProtocolVersionV2,
		Capabilities: map[string]interface{}{
			"session": map[string]interface{}{
				"delete":                map[string]interface{}{},
				"additionalDirectories": map[string]interface{}{},
			},
		},
	}
	caps := acpSessionCapabilities(resp)
	if !caps.List || !caps.Resume || !caps.Close || !caps.Cancel || !caps.InfoUpdate {
		t.Fatalf("capabilities.session advertises the baseline methods: %#v", caps)
	}
	if !caps.Delete || !caps.AdditionalDirectories {
		t.Fatalf("the nested optional markers must be read: %#v", caps)
	}
	if caps.Load {
		t.Fatalf("version 2 removed session/load and must not report it: %#v", caps)
	}
	if caps.Fork {
		t.Fatalf("fork stays unsupported until the unstable marker is advertised: %#v", caps)
	}
}

// TestACPSessionCapabilitiesReportNoSurfaceWhenVersionTwoOmitsIt is the other
// half: an agent that omits capabilities.session says it has no session surface,
// so nothing may be reported as supported.
func TestACPSessionCapabilitiesReportNoSurfaceWhenVersionTwoOmitsIt(t *testing.T) {
	resp := &acpInitializeResponse{
		ProtocolVersion: zedacp.ProtocolVersionV2,
		Capabilities: map[string]interface{}{
			// A version 1 spelling in a version 2 reply is not read: the reply is
			// decoded by the generation that produced it.
			"sessionCapabilities": map[string]interface{}{"list": map[string]interface{}{}},
		},
	}
	caps := acpSessionCapabilities(resp)
	if caps.List || caps.Resume || caps.Close {
		t.Fatalf("no session surface was advertised: %#v", caps)
	}
}

// TestSessionNotFoundIsRecognisedFromTheSpecificationErrorCode pins the second
// structured error shape: both generations assign -32002 to "resource not
// found", which is how a peer reports a session that is gone, so recovery must
// not depend on the wording of the message.
func TestSessionNotFoundIsRecognisedFromTheSpecificationErrorCode(t *testing.T) {
	coded := &zedacp.RPCError{Code: zedacp.ErrCodeResourceNotFound, Message: "no such session"}
	if !isSessionNotFoundError(coded) {
		t.Fatalf("the resource-not-found code must be recognised: %v", coded)
	}
	if isSessionNotFoundError(&zedacp.RPCError{Code: zedacp.ErrCodeInternal, Message: "boom"}) {
		t.Fatal("an unrelated error code must not be read as a lost session")
	}
}

// TestACPProtocolCapabilitiesFollowTheNegotiatedGeneration pins the card against
// the generation the connection agreed on: version 1 names and surfaces for a
// version 1 connection, and the version 2 names with the removed surfaces
// reported unsupported for a version 2 one.
func TestACPProtocolCapabilitiesFollowTheNegotiatedGeneration(t *testing.T) {
	v1 := &acpConversationClient{
		client:              newScriptedAuthACPClient(zedacp.ProtocolVersionV1),
		endpoint:            middleware.ProtocolEndpoint{Transport: "stdio"},
		authMethods:         []acpAuthMethod{{Type: zedacp.AuthMethodTypeAgent, MethodID: "agent-login"}},
		featureCapabilities: acpFeatureCapabilities{fsRead: true, terminal: true, logout: true},
		sessionCapabilities: middleware.ConversationSessionCapabilities{List: true, Load: true},
	}
	v1Report := v1.ProtocolCapabilities()
	for _, operation := range []string{"authenticate", "logout", "session/load", "session/set_mode", "fs/read_text_file", "terminal/create"} {
		if !v1Report.Operations[operation].Supported {
			t.Fatalf("a version 1 connection reports %s as supported: %#v", operation, v1Report.Operations)
		}
	}

	v2 := &acpConversationClient{
		client:              newScriptedAuthACPClient(zedacp.ProtocolVersionV2),
		endpoint:            middleware.ProtocolEndpoint{Transport: "stdio"},
		authMethods:         []acpAuthMethod{{Type: zedacp.AuthMethodTypeAgent, MethodID: "agent-login"}},
		featureCapabilities: acpFeatureCapabilities{fsRead: true, terminal: true, logout: true},
		sessionCapabilities: middleware.ConversationSessionCapabilities{List: true, Resume: true, Close: true},
	}
	v2Report := v2.ProtocolCapabilities()
	for _, operation := range []string{"auth/login", "auth/logout", "session/list", "session/resume", "session/close"} {
		if !v2Report.Operations[operation].Supported {
			t.Fatalf("a version 2 connection reports %s as supported: %#v", operation, v2Report.Operations)
		}
	}
	for _, removed := range []string{"session/load", "session/set_mode", "fs/read_text_file", "terminal/create", "authenticate"} {
		if descriptor, ok := v2Report.Operations[removed]; ok && descriptor.Supported {
			t.Fatalf("version 2 removed %s: %#v", removed, descriptor)
		}
	}
}
