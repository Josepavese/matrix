package agents

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/Josepavese/matrix/internal/logic/sidecar"
	"github.com/Josepavese/matrix/internal/middleware"
	"github.com/Josepavese/matrix/internal/providers/sidecarprojection"
)

type acpConversationFactory struct{}

func (f *acpConversationFactory) NewClient(ctx context.Context, endpoint middleware.ProtocolEndpoint, deps middleware.ConversationFactoryDeps) (middleware.ConversationClient, error) {
	transport, err := createTransport(ctx, transportSpec{
		Protocol: endpoint.Transport,
		Address:  endpoint.Address,
		Command:  endpoint.Command,
		Args:     endpoint.Args,
		Env:      endpoint.Env,
	})
	if err != nil {
		return nil, err
	}

	client := NewACPClient(ctx, transport)
	handler := NewDefaultRequestHandler(deps.TrustMode).WithFS(deps.FS, deps.Cwd)
	if deps.Process != nil {
		handler.WithProcess(deps.Process)
	}
	client.SetRequestHandler(handler)

	initReq := acpInitializeRequest{
		ProtocolVersion:    1,
		ClientInfo:         map[string]interface{}{"name": "matrix", "version": "1.0"},
		ClientCapabilities: acpClientCapabilitiesForDeps(deps),
	}
	initResp, err := client.Initialize(ctx, initReq)
	if err != nil {
		_ = transport.Close()
		return nil, classifyProviderFailure("", endpoint, "initialize", fmt.Errorf("ACP initialize failed: %w", err))
	}
	for _, m := range initResp.AuthMethods {
		authenticateACPEnvVarFromEnvironment(ctx, client, m)
	}

	caps := acpSessionCapabilities(initResp)
	return &acpConversationClient{
		client:              client,
		handler:             handler,
		cwd:                 deps.Cwd,
		endpoint:            endpoint,
		sessionCapabilities: caps,
		loadedSessions:      map[string]bool{},
		mcpServers:          toZedACPMCPServers(deps.McpServers),
	}, nil
}

func acpClientCapabilitiesForDeps(deps middleware.ConversationFactoryDeps) *acpClientCapabilities {
	return &acpClientCapabilities{
		Fs: &acpFsCapability{
			ReadTextFile:  deps.FS != nil,
			WriteTextFile: deps.FS != nil,
		},
		Terminal: deps.Process != nil,
		// terminal/create is safe for agent-first automation when a process
		// backend is configured. ACP auth.terminal is different: it promises an
		// interactive login flow, so Matrix keeps it opt-out on the autonomous
		// runtime path until a human-facing frontend can complete it explicitly.
		Auth: &acpAuthCapabilities{},
	}
}

type acpAuthenticator interface {
	Authenticate(ctx context.Context, methodID string, credentials map[string]string) error
}

type acpEnvSpec struct {
	name     string
	optional bool
}

func authenticateACPEnvVarFromEnvironment(ctx context.Context, client acpAuthenticator, method acpAuthMethod) {
	if method.Type != "env_var" || strings.TrimSpace(method.ID) == "" {
		return
	}
	credentials, ok := acpEnvVarCredentials(method)
	if !ok {
		return
	}
	if err := client.Authenticate(ctx, method.ID, nil); err == nil {
		return
	} else if fallbackErr := client.Authenticate(ctx, method.ID, credentials); fallbackErr != nil {
		slog.Warn("ACP authentication failed", "methodId", method.ID, "error", fallbackErr, "initial_error", err)
	}
}

func acpEnvVarCredentials(method acpAuthMethod) (map[string]string, bool) {
	specs := acpEnvVarSpecs(method)
	if len(specs) == 0 {
		return nil, false
	}
	credentials, ok := lookupACPEnvVarCredentials(specs)
	if !ok {
		return nil, false
	}
	addLegacyACPAPIKeyCredential(credentials)
	return credentials, true
}

func acpEnvVarSpecs(method acpAuthMethod) []acpEnvSpec {
	var specs []acpEnvSpec
	if name := strings.TrimSpace(method.EnvVar); name != "" {
		specs = append(specs, acpEnvSpec{name: name})
	}
	seen := map[string]struct{}{}
	for _, variable := range method.Vars {
		specs = appendACPEnvSpec(specs, seen, variable.Name, variable.Optional)
	}
	return specs
}

func appendACPEnvSpec(specs []acpEnvSpec, seen map[string]struct{}, name string, optional bool) []acpEnvSpec {
	name = strings.TrimSpace(name)
	if name == "" {
		return specs
	}
	if _, ok := seen[name]; ok {
		return specs
	}
	seen[name] = struct{}{}
	return append(specs, acpEnvSpec{name: name, optional: optional})
}

func lookupACPEnvVarCredentials(specs []acpEnvSpec) (map[string]string, bool) {
	credentials := map[string]string{}
	for _, spec := range specs {
		value, ok := os.LookupEnv(spec.name)
		if !ok {
			if spec.optional {
				continue
			}
			return nil, false
		}
		credentials[spec.name] = value
	}
	if len(credentials) == 0 {
		return nil, false
	}
	return credentials, true
}

func addLegacyACPAPIKeyCredential(credentials map[string]string) {
	if len(credentials) != 1 {
		return
	}
	for _, value := range credentials {
		credentials["api_key"] = value
	}
}

type acpConversationClient struct {
	client              ACPClient
	handler             *defaultRequestHandler
	cwd                 string
	endpoint            middleware.ProtocolEndpoint
	sessionCapabilities middleware.ConversationSessionCapabilities
	mcpServers          []acpMcpServerConfig
	mu                  sync.Mutex
	loadedSessions      map[string]bool
	activePrompts       map[string]chan struct{}
}

func (c *acpConversationClient) Alive() bool {
	return c.client != nil && c.client.Context().Err() == nil
}

func (c *acpConversationClient) Close() error {
	if c.client == nil {
		return nil
	}
	return c.client.Close()
}

func (c *acpConversationClient) ExecuteTurn(ctx context.Context, turn middleware.ConversationTurn) (middleware.ConversationResult, error) {
	log := slog.With("component", "acp_adapter", "logical_session", turn.LogicalSessionID, "agent", turn.AgentID)
	cwd := c.turnCwd(turn)
	remoteSessionID, err := c.ensureACPRemoteSession(ctx, turn, cwd, log)
	if err != nil {
		return middleware.ConversationResult{}, classifyProviderFailure(turn.AgentID, c.endpoint, "session/new", err)
	}
	c.prepareTurnCallbacks(turn, remoteSessionID)
	endPrompt, err := c.beginPrompt(ctx, remoteSessionID, turn.LiveContextAttach)
	if err != nil {
		return middleware.ConversationResult{RemoteSessionID: remoteSessionID}, err
	}
	defer endPrompt()

	obs := &simpleObserver{updates: make(chan struct{}, 1), notifier: turn.ThoughtNotifier}
	resp, err := c.promptACP(ctx, remoteSessionID, turn, obs)
	if err != nil && remoteSessionID != "" && isSessionNotFoundError(err) {
		log.Warn("ACP session lost, recreating", "agent_session", remoteSessionID)
		return c.retryTurnWithFreshSession(ctx, turn)
	}
	if err != nil {
		return middleware.ConversationResult{
			Output:          obs.GetContent(),
			RemoteSessionID: remoteSessionID,
			Metadata:        obs.Metadata(),
		}, classifyProviderFailure(turn.AgentID, c.endpoint, "session/prompt", fmt.Errorf("ACP prompt failed: %w", err))
	}

	obs.WaitIdle(ctx, 150*time.Millisecond)
	return middleware.ConversationResult{
		Output:          obs.GetContent(),
		RemoteSessionID: remoteSessionID,
		ToolCalls:       fromZedACPToolCalls(resp.ToolCalls),
		Metadata:        obs.Metadata(),
	}, nil
}

func (c *acpConversationClient) ensureACPRemoteSession(ctx context.Context, turn middleware.ConversationTurn, cwd string, log *slog.Logger) (string, error) {
	if turn.RemoteSessionID == "" {
		return c.createAndConfigureACPRemoteSession(ctx, turn, cwd, log)
	}
	if !c.isLoadedSession(turn.RemoteSessionID) && c.sessionCapabilities.Resume {
		resumed, err := c.resumeACPRemoteSession(acpLoadRemoteSessionRequest{
			Ctx:                   ctx,
			RemoteSessionID:       turn.RemoteSessionID,
			Cwd:                   cwd,
			AdditionalDirectories: turn.AdditionalDirectories,
			McpServers:            c.turnMCPServers(turn.McpServers),
			Notifier:              turn.ThoughtNotifier,
			Log:                   log,
		})
		if err != nil {
			return "", err
		}
		if resumed {
			return turn.RemoteSessionID, nil
		}
	}
	if !c.isLoadedSession(turn.RemoteSessionID) && c.sessionCapabilities.Load {
		if err := c.loadACPRemoteSession(acpLoadRemoteSessionRequest{
			Ctx:                   ctx,
			RemoteSessionID:       turn.RemoteSessionID,
			Cwd:                   cwd,
			AdditionalDirectories: turn.AdditionalDirectories,
			McpServers:            c.turnMCPServers(turn.McpServers),
			Notifier:              turn.ThoughtNotifier,
			Log:                   log,
		}); err != nil {
			return "", err
		}
	}
	return turn.RemoteSessionID, nil
}

func (c *acpConversationClient) isLoadedSession(remoteSessionID string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.loadedSessions[remoteSessionID]
}

func (c *acpConversationClient) markLoadedSession(remoteSessionID string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.loadedSessions == nil {
		c.loadedSessions = map[string]bool{}
	}
	c.loadedSessions[remoteSessionID] = true
}

func (c *acpConversationClient) unmarkLoadedSession(remoteSessionID string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.loadedSessions, remoteSessionID)
}

func (c *acpConversationClient) createAndConfigureACPRemoteSession(ctx context.Context, turn middleware.ConversationTurn, cwd string, log *slog.Logger) (string, error) {
	newSessResp, err := c.createACPRemoteSession(ctx, middleware.SessionMaterializeRequest{
		LogicalSessionID:      turn.LogicalSessionID,
		WorkspacePath:         cwd,
		Tools:                 turn.Tools,
		McpServers:            turn.McpServers,
		AdditionalDirectories: turn.AdditionalDirectories,
	})
	if err != nil {
		return "", err
	}
	c.markLoadedSession(newSessResp.SessionID)
	c.setAutoApproveMode(ctx, newSessResp, log)
	return newSessResp.SessionID, nil
}

func (c *acpConversationClient) setAutoApproveMode(ctx context.Context, resp *acpNewSessionResponse, log *slog.Logger) {
	session := fromZedACPSession(resp)
	if configID, value := pickAutoApproveConfigOption(session); configID != "" && value != "" {
		if _, err := c.client.SetConfigOption(ctx, acpSetConfigOptionRequest{
			SessionID: resp.SessionID,
			ConfigID:  configID,
			Value:     value,
		}); err != nil {
			log.Warn("failed to set ACP config option", "config_id", configID, "value", value, "error", err)
		}
		return
	}
	modeID := pickAutoApproveMode(session)
	if modeID == "" {
		return
	}
	if err := c.client.SetMode(ctx, resp.SessionID, modeID); err != nil {
		log.Warn("failed to set ACP mode", "mode", modeID, "error", err)
	}
}

type acpLoadRemoteSessionRequest struct {
	Ctx                   context.Context
	RemoteSessionID       string
	Cwd                   string
	AdditionalDirectories []string
	McpServers            []acpMcpServerConfig
	Notifier              middleware.ThoughtNotifier
	Log                   *slog.Logger
}

func (c *acpConversationClient) resumeACPRemoteSession(req acpLoadRemoteSessionRequest) (bool, error) {
	additionalDirectories, err := c.additionalDirectories(req.AdditionalDirectories)
	if err != nil {
		return false, err
	}
	resp, err := c.client.ResumeSession(req.Ctx, acpResumeSessionRequest{
		SessionID:             req.RemoteSessionID,
		Cwd:                   req.Cwd,
		AdditionalDirectories: additionalDirectories,
		McpServers:            req.McpServers,
	})
	if err != nil {
		req.Log.Warn("ACP session resume failed, will fall back to load/prompt flow", "agent_session", req.RemoteSessionID, "error", err)
		return false, nil
	}
	c.markLoadedSession(req.RemoteSessionID)
	if configID, value := pickAutoApproveConfigOption(fromZedACPResumeSession(resp)); configID != "" && value != "" {
		if _, err := c.client.SetConfigOption(req.Ctx, acpSetConfigOptionRequest{
			SessionID: req.RemoteSessionID,
			ConfigID:  configID,
			Value:     value,
		}); err != nil {
			req.Log.Warn("failed to set ACP resumed config option", "config_id", configID, "value", value, "error", err)
		}
	}
	return true, nil
}

func (c *acpConversationClient) loadACPRemoteSession(req acpLoadRemoteSessionRequest) error {
	obs := &simpleObserver{updates: make(chan struct{}, 1), notifier: req.Notifier}
	additionalDirectories, err := c.additionalDirectories(req.AdditionalDirectories)
	if err != nil {
		return err
	}
	resp, err := c.client.LoadSession(req.Ctx, acpLoadSessionRequest{
		SessionID:             req.RemoteSessionID,
		Cwd:                   req.Cwd,
		AdditionalDirectories: additionalDirectories,
		McpServers:            req.McpServers,
	}, obs)
	if err != nil {
		req.Log.Warn("ACP session load failed, will fall back to prompt/recreate flow", "agent_session", req.RemoteSessionID, "error", err)
		return nil
	}
	c.markLoadedSession(req.RemoteSessionID)
	if configID, value := pickAutoApproveConfigOption(fromZedACPLoadSession(resp)); configID != "" && value != "" {
		if _, err := c.client.SetConfigOption(req.Ctx, acpSetConfigOptionRequest{
			SessionID: req.RemoteSessionID,
			ConfigID:  configID,
			Value:     value,
		}); err != nil {
			req.Log.Warn("failed to set ACP loaded config option", "config_id", configID, "value", value, "error", err)
		}
	}
	obs.WaitIdle(req.Ctx, 150*time.Millisecond)
	return nil
}

func (c *acpConversationClient) prepareTurnCallbacks(turn middleware.ConversationTurn, remoteSessionID string) {
	if turn.ThoughtNotifier != nil {
		turn.ThoughtNotifier.SetHeader(turn.AgentID, remoteSessionID)
	}
	if c.handler != nil {
		c.handler.WithNotifier(turn.ThoughtNotifier)
	}
}

func (c *acpConversationClient) promptACP(ctx context.Context, remoteSessionID string, turn middleware.ConversationTurn, obs *simpleObserver) (*acpPromptResponse, error) {
	promptText := sidecar.ProjectPrompt(turn.Message, turn.SidecarCapsules)
	return c.client.Prompt(ctx, acpPromptRequest{
		SessionID: remoteSessionID,
		Prompt:    acpPromptContent(promptText, turn.ContentBlocks),
		Meta:      sidecarprojection.ACPMeta(turn.SidecarCapsules),
	}, obs)
}

func (c *acpConversationClient) retryTurnWithFreshSession(ctx context.Context, turn middleware.ConversationTurn) (middleware.ConversationResult, error) {
	return c.ExecuteTurn(ctx, middleware.ConversationTurn{
		AgentID:               turn.AgentID,
		LogicalSessionID:      turn.LogicalSessionID,
		WorkspacePath:         turn.WorkspacePath,
		Message:               turn.Message,
		ContentBlocks:         turn.ContentBlocks,
		SidecarCapsules:       turn.SidecarCapsules,
		Tools:                 turn.Tools,
		McpServers:            turn.McpServers,
		AdditionalDirectories: turn.AdditionalDirectories,
		ThoughtNotifier:       turn.ThoughtNotifier,
		LiveContextAttach:     turn.LiveContextAttach,
	})
}

func acpPromptContent(promptText string, blocks []middleware.Content) []acpContent {
	out := make([]acpContent, 0, len(blocks)+1)
	if strings.TrimSpace(promptText) != "" {
		out = append(out, acpContent{Type: "text", Text: promptText})
	}
	for _, block := range blocks {
		if converted, ok := toZedACPContent(block); ok {
			out = append(out, converted)
		}
	}
	if len(out) == 0 {
		return []acpContent{{Type: "text", Text: ""}}
	}
	return out
}

func toZedACPContent(content middleware.Content) (acpContent, bool) {
	if strings.TrimSpace(content.Type) == "" {
		return acpContent{}, false
	}
	return acpContent{
		Type:        content.Type,
		Text:        content.Text,
		Data:        content.Data,
		MimeType:    content.MimeType,
		URI:         content.URI,
		Name:        content.Name,
		Title:       content.Title,
		Description: content.Description,
		Size:        content.Size,
		Resource:    content.Resource,
		Annotations: content.Annotations,
		Meta:        content.Meta,
	}, true
}

func (c *acpConversationClient) turnCwd(turn middleware.ConversationTurn) string {
	if strings.TrimSpace(turn.WorkspacePath) != "" {
		return strings.TrimSpace(turn.WorkspacePath)
	}
	return c.cwd
}

func (c *acpConversationClient) turnMCPServers(turnServers []middleware.McpServerConfig) []acpMcpServerConfig {
	if len(turnServers) > 0 {
		return toZedACPMCPServers(turnServers)
	}
	return cloneACPMCPServers(c.mcpServers)
}

func (c *acpConversationClient) additionalDirectories(values []string) ([]string, error) {
	if !c.sessionCapabilities.AdditionalDirectories || len(values) == 0 {
		return nil, nil
	}
	out := make([]string, 0, len(values))
	seen := map[string]struct{}{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if !filepath.IsAbs(value) {
			return nil, fmt.Errorf("ACP additionalDirectories path must be absolute: %s", value)
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	if len(out) == 0 {
		return nil, nil
	}
	return out, nil
}

func (c *acpConversationClient) SessionCapabilities() middleware.ConversationSessionCapabilities {
	return c.sessionCapabilities
}

func (c *acpConversationClient) ListRemoteSessions(ctx context.Context) ([]middleware.RemoteSessionInfo, error) {
	if !c.sessionCapabilities.List {
		return nil, fmt.Errorf("ACP agent does not advertise session/list")
	}
	var out []middleware.RemoteSessionInfo
	cursor := ""
	for page := 0; page < 100; page++ {
		resp, err := c.client.ListSessionsWithRequest(ctx, acpListSessionsRequest{Cursor: cursor})
		if err != nil {
			return nil, err
		}
		for _, session := range resp.Sessions {
			out = append(out, c.remoteSessionInfo(session))
		}
		if strings.TrimSpace(resp.NextCursor) == "" {
			return out, nil
		}
		cursor = resp.NextCursor
	}
	return nil, fmt.Errorf("ACP session/list pagination exceeded safety limit")
}

func (c *acpConversationClient) remoteSessionInfo(session acpSessionInfo) middleware.RemoteSessionInfo {
	return middleware.RemoteSessionInfo{
		RemoteSessionID: session.SessionID,
		DisplayID:       session.SessionID,
		Title:           session.Title,
		UpdatedAt:       session.UpdatedAt,
		ProtocolKind:    middleware.ProtocolKindACP,
		CanResume:       c.sessionCapabilities.Load || c.sessionCapabilities.Resume,
		CanDelete:       c.sessionCapabilities.Delete,
	}
}

func (c *acpConversationClient) GetRemoteSession(ctx context.Context, remoteSessionID string) (middleware.RemoteSessionInfo, error) {
	if c.sessionCapabilities.List {
		sessions, err := c.ListRemoteSessions(ctx)
		if err != nil {
			return middleware.RemoteSessionInfo{}, err
		}
		for _, session := range sessions {
			if session.RemoteSessionID == remoteSessionID || session.DisplayID == remoteSessionID {
				return session, nil
			}
		}
	}
	if c.sessionCapabilities.Resume {
		if _, err := c.client.ResumeSession(ctx, acpResumeSessionRequest{
			SessionID:  remoteSessionID,
			Cwd:        c.cwd,
			McpServers: cloneACPMCPServers(c.mcpServers),
		}); err == nil {
			c.markLoadedSession(remoteSessionID)
			return middleware.RemoteSessionInfo{
				RemoteSessionID: remoteSessionID,
				DisplayID:       remoteSessionID,
				ProtocolKind:    middleware.ProtocolKindACP,
				CanResume:       true,
				CanDelete:       c.sessionCapabilities.Delete,
			}, nil
		}
	}
	if c.sessionCapabilities.Load {
		if _, err := c.client.LoadSession(ctx, acpLoadSessionRequest{
			SessionID:  remoteSessionID,
			Cwd:        c.cwd,
			McpServers: cloneACPMCPServers(c.mcpServers),
		}, nil); err == nil {
			c.markLoadedSession(remoteSessionID)
			return middleware.RemoteSessionInfo{
				RemoteSessionID: remoteSessionID,
				DisplayID:       remoteSessionID,
				ProtocolKind:    middleware.ProtocolKindACP,
				CanResume:       c.sessionCapabilities.Load || c.sessionCapabilities.Resume,
				CanDelete:       c.sessionCapabilities.Delete,
			}, nil
		}
	}
	return middleware.RemoteSessionInfo{}, fmt.Errorf("ACP session %s not found", remoteSessionID)
}

func (c *acpConversationClient) DeleteRemoteSession(ctx context.Context, remoteSessionID string) error {
	if !c.sessionCapabilities.Delete {
		return fmt.Errorf("ACP agent does not advertise session/delete")
	}
	if err := c.client.DeleteSession(ctx, remoteSessionID); err != nil {
		return err
	}
	c.unmarkLoadedSession(remoteSessionID)
	return nil
}

func (c *acpConversationClient) CancelRemoteSession(ctx context.Context, remoteSessionID string) error {
	if strings.TrimSpace(remoteSessionID) == "" {
		return fmt.Errorf("ACP session id is required")
	}
	return c.client.CancelSession(ctx, remoteSessionID)
}

func (c *acpConversationClient) CloseRemoteSession(ctx context.Context, remoteSessionID string) error {
	if !c.sessionCapabilities.Close {
		return fmt.Errorf("ACP agent does not advertise session/close")
	}
	if strings.TrimSpace(remoteSessionID) == "" {
		return fmt.Errorf("ACP session id is required")
	}
	if err := c.client.CloseSession(ctx, remoteSessionID); err != nil {
		return err
	}
	c.unmarkLoadedSession(remoteSessionID)
	return nil
}

type transportSpec struct {
	Protocol string
	Address  string
	Command  string
	Args     []string
	Env      []string
}

func createTransport(ctx context.Context, spec transportSpec) (middleware.AgentTransport, error) {
	switch strings.ToLower(spec.Protocol) {
	case "ws":
		addr := spec.Address
		if !strings.HasPrefix(addr, "ws://") && !strings.HasPrefix(addr, "wss://") {
			addr = "ws://" + addr
		}
		return NewWSTransport(ctx, addr)
	case "stdio", "acp":
		return NewStdioTransport(ctx, spec.Command, spec.Env, spec.Args...)
	case "unix":
		return NewUnixTransport(ctx, spec.Address)
	default:
		return nil, fmt.Errorf("unsupported ACP transport: %s", spec.Protocol)
	}
}

func toZedACPTools(tools []middleware.Tool) []acpTool {
	if len(tools) == 0 {
		return nil
	}
	out := make([]acpTool, 0, len(tools))
	for _, tool := range tools {
		out = append(out, acpTool{
			Name:        tool.Name,
			Description: tool.Description,
			InputSchema: tool.InputSchema,
		})
	}
	return out
}

func toZedACPMCPServers(servers []middleware.McpServerConfig) []acpMcpServerConfig {
	if len(servers) == 0 {
		return []acpMcpServerConfig{}
	}
	out := make([]acpMcpServerConfig, 0, len(servers))
	for _, server := range servers {
		out = append(out, acpMcpServerConfig{
			Name:    server.Name,
			Type:    server.Type,
			Command: server.Command,
			Args:    append([]string(nil), server.Args...),
			Env:     toZedACPEnvVars(server.Env),
			URL:     server.URL,
			Headers: toZedACPHeaders(server.Headers),
		})
	}
	return out
}

func cloneACPMCPServers(servers []acpMcpServerConfig) []acpMcpServerConfig {
	if len(servers) == 0 {
		return []acpMcpServerConfig{}
	}
	out := make([]acpMcpServerConfig, 0, len(servers))
	for _, server := range servers {
		out = append(out, acpMcpServerConfig{
			Name:    server.Name,
			Type:    server.Type,
			Command: server.Command,
			Args:    append([]string(nil), server.Args...),
			Env:     append([]zedacpEnvVar(nil), server.Env...),
			URL:     server.URL,
			Headers: append([]zedacpHeader(nil), server.Headers...),
		})
	}
	return out
}

func toZedACPEnvVars(values []middleware.EnvVar) []zedacpEnvVar {
	if len(values) == 0 {
		return nil
	}
	out := make([]zedacpEnvVar, 0, len(values))
	for _, value := range values {
		out = append(out, zedacpEnvVar{Name: value.Name, Value: value.Value})
	}
	return out
}

func toZedACPHeaders(values []middleware.Header) []zedacpHeader {
	if len(values) == 0 {
		return nil
	}
	out := make([]zedacpHeader, 0, len(values))
	for _, value := range values {
		out = append(out, zedacpHeader{Name: value.Name, Value: value.Value})
	}
	return out
}

func fromZedACPToolCalls(calls []acpToolCall) []middleware.ToolCall {
	if len(calls) == 0 {
		return nil
	}
	out := make([]middleware.ToolCall, 0, len(calls))
	for _, call := range calls {
		out = append(out, middleware.ToolCall{
			ID:   call.ID,
			Type: call.Type,
			Function: middleware.ToolCallFunction{
				Name:      call.Function.Name,
				Arguments: call.Function.Arguments,
			},
		})
	}
	return out
}
