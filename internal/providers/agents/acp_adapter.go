package agents

import (
	"context"
	"fmt"
	"log/slog"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/Josepavese/matrix/internal/logic/agentlaunch"
	"github.com/Josepavese/matrix/internal/logic/runtrace"
	"github.com/Josepavese/matrix/internal/middleware"
)

type acpConversationFactory struct{}

func (f *acpConversationFactory) NewClient(ctx context.Context, endpoint middleware.ProtocolEndpoint, deps middleware.ConversationFactoryDeps) (middleware.ConversationClient, error) {
	transport, err := createTransport(ctx, transportSpec{
		Protocol:     endpoint.Transport,
		Address:      endpoint.Address,
		Command:      endpoint.Command,
		Args:         endpoint.Args,
		Env:          endpoint.Env,
		EnvIsolation: endpoint.EnvIsolation,
		Cwd:          deps.Cwd,
	})
	if err != nil {
		return nil, err
	}
	return f.initializeConversation(ctx, endpoint, deps, transport)
}

// newACPClientWithHandler builds the client and the request handler that serves
// everything the agent asks back: filesystem, processes, and elicitation.
func newACPClientWithHandler(ctx context.Context, deps middleware.ConversationFactoryDeps, transport middleware.AgentTransport) (ACPClient, *defaultRequestHandler) {
	client := NewACPClient(ctx, transport)
	handler := NewDefaultRequestHandler(deps.TrustMode).WithFS(deps.FS, deps.Cwd)
	if deps.Process != nil {
		handler.WithProcess(deps.Process)
	}
	if deps.ElicitationFrontend != nil {
		handler.WithElicitationFrontend(deps.ElicitationFrontend).WithAgentIdentity(deps.AgentID)
	}
	client.SetRequestHandler(handler)
	return client, handler
}

// initializeConversation performs the ACP handshake and assembles the
// conversation client, closing the transport on every failure path.
func (f *acpConversationFactory) initializeConversation(ctx context.Context, endpoint middleware.ProtocolEndpoint, deps middleware.ConversationFactoryDeps, transport middleware.AgentTransport) (middleware.ConversationClient, error) {
	client, handler := newACPClientWithHandler(ctx, deps, transport)

	initResp, err := initializeACPConnection(ctx, client, deps)
	if err != nil {
		_ = transport.Close()
		return nil, classifyProviderFailure("", endpoint, "initialize", err)
	}
	conversation := &acpConversationClient{
		client:              client,
		handler:             handler,
		deps:                deps,
		cwd:                 deps.Cwd,
		endpoint:            endpoint,
		sessionCapabilities: acpSessionCapabilities(initResp),
		featureCapabilities: acpFeatureCapabilitiesFromDeps(initResp, deps),
		authMethods:         append([]acpAuthMethod(nil), initResp.AuthMethods...),
		loadedSessions:      map[string]bool{},
		mcpServers:          toZedACPMCPServers(deps.McpServers),
		preferredMode:       agentlaunch.PreferredSessionMode(endpoint),
	}
	// A terminal login has to reconnect with the same endpoint and dependencies
	// this connection was built from, so the factory owns that rebuild.
	conversation.reconnect = conversation.reconnectACPConnection
	if err := conversation.validateMCPServers(conversation.mcpServers); err != nil {
		_ = transport.Close()
		return nil, classifyProviderFailure("", endpoint, "initialize", err)
	}
	return conversation, nil
}

// acpFeatureCapabilitiesFromDeps merges the provider's advertised capabilities
// with what Matrix actually wired.
func acpFeatureCapabilitiesFromDeps(initResp *acpInitializeResponse, deps middleware.ConversationFactoryDeps) acpFeatureCapabilities {
	features := parseACPFeatureCapabilities(initResp)
	features.fsRead = deps.FS != nil
	features.fsWrite = deps.FS != nil
	features.terminal = deps.Process != nil
	return features
}

func acpClientCapabilitiesForDeps(deps middleware.ConversationFactoryDeps) *acpClientCapabilities {
	return &acpClientCapabilities{
		Fs: &acpFsCapability{
			ReadTextFile:  deps.FS != nil,
			WriteTextFile: deps.FS != nil,
		},
		Terminal: deps.Process != nil,
		Session: &acpClientSessionCapabilities{
			ConfigOptions: &acpSessionConfigOptionsCapabilities{
				Boolean: &acpBooleanConfigOptionCapabilities{},
			},
		},
		Elicitation: elicitationAdvertisement(deps.ElicitationFrontend),
		Auth:        terminalAuthAdvertisement(deps),
	}
}

type acpConversationClient struct {
	client              ACPClient
	handler             *defaultRequestHandler
	deps                middleware.ConversationFactoryDeps
	cwd                 string
	endpoint            middleware.ProtocolEndpoint
	sessionCapabilities middleware.ConversationSessionCapabilities
	featureCapabilities acpFeatureCapabilities
	authMethods         []acpAuthMethod
	mcpServers          []acpMcpServerConfig
	preferredMode       string
	// reconnect rebuilds the connection after a terminal authentication method
	// ran the configured agent program; nil means this client cannot reconnect.
	reconnect          func(context.Context) error
	mu                 sync.Mutex
	loadedSessions     map[string]bool
	activePrompts      map[string]chan struct{}
	activeClientLeases int
	closeRequested     bool
	closed             bool
	closeErr           error
}

func (c *acpConversationClient) Alive() bool {
	client := c.currentACPClient()
	return client != nil && client.Context().Err() == nil
}

// executeTurnOnce runs one turn. ExecuteTurn wraps it with the authentication
// retry, so this body owns the turn itself and nothing about logging in again.
func (c *acpConversationClient) executeTurnOnce(ctx context.Context, turn middleware.ConversationTurn) (middleware.ConversationResult, error) {
	log := slog.With("component", "acp_adapter", "logical_session", turn.LogicalSessionID, "agent", turn.AgentID)
	remoteSessionID, err := c.prepareTurnSession(ctx, turn, log)
	if err != nil {
		return middleware.ConversationResult{RemoteSessionID: remoteSessionID}, err
	}
	c.prepareTurnCallbacks(turn, remoteSessionID)
	endPrompt, err := c.beginPrompt(ctx, remoteSessionID, turn.LiveContextAttach)
	if err != nil {
		return middleware.ConversationResult{RemoteSessionID: remoteSessionID}, err
	}
	defer endPrompt()
	if c.handler != nil {
		c.handler.BindTurnContext(ctx, remoteSessionID)
		defer c.handler.ClearTurnContext(remoteSessionID)
	}

	turnObs := c.observeTurn(remoteSessionID, turn)
	defer turnObs.stop()
	obs := turnObs.observer
	resp, err := c.promptACP(ctx, remoteSessionID, turn, turnObs.prompt)
	if err != nil && remoteSessionID != "" && isSessionNotFoundError(err) && !turn.StrictSession {
		log.Warn("ACP session lost, recreating", "agent_session", remoteSessionID)
		return c.retryTurnWithFreshSession(ctx, turn)
	}
	if err != nil {
		return middleware.ConversationResult{
			Output:          obs.GetContent(),
			ContentBlocks:   obs.ContentBlocks(),
			RemoteSessionID: remoteSessionID,
			Metadata:        obs.Metadata(),
		}, classifyProviderFailure(turn.AgentID, c.endpoint, "session/prompt", fmt.Errorf("ACP prompt failed: %w", err))
	}

	if turnObs.terminal {
		// The prompt response acknowledges insertion; the turn ends on the idle
		// state update, which may still be coming. Wait for it, and record the
		// timeout in the result when it never arrives rather than returning a
		// partial answer that looks complete.
		if budget := acpV2TurnBudget(); !obs.AwaitTerminal(ctx, budget) {
			log.Warn("ACP v2 turn ended without a terminal state update",
				"event", "acp_v2_turn_budget_exhausted", "agent_session", remoteSessionID, "budget", budget)
		}
	} else {
		obs.WaitIdle(ctx, 150*time.Millisecond)
	}
	stopReason := resolveTurnStopReason(resp.StopReason, turnObs.stopReason)
	reportTurnStopReason(turn, stopReason)
	return middleware.ConversationResult{
		Output:          obs.GetContent(),
		ContentBlocks:   obs.ContentBlocks(),
		RemoteSessionID: remoteSessionID,
		ToolCalls:       fromZedACPToolCalls(resp.ToolCalls),
		Metadata:        metadataWithStopReason(obs.Metadata(), stopReason),
		StopReason:      stopReason,
	}, nil
}

// reportTurnStopReason tells the run's notifier what the peer said ended the
// turn, so the terminal transition records the peer's own word. A notifier that
// does not offer the capability records nothing, and neither does a turn where
// the peer said nothing: the run then reports the reason as unreported, which is
// true, instead of a reason Matrix made up.
func reportTurnStopReason(turn middleware.ConversationTurn, stopReason string) {
	reporter, ok := turn.ThoughtNotifier.(runtrace.TurnStopReasonReporter)
	if !ok || strings.TrimSpace(stopReason) == "" {
		return
	}
	reporter.OnTurnStopReason(stopReason)
}

// resolveTurnStopReason reports what the peer said ended the turn. A prompt
// response carries the stop reason for every generation that ends a turn on the
// response; a generation whose turn ends on a terminal state update reports it
// there instead, so the observed terminal reason is the fallback. An empty
// result means the peer reported nothing and Matrix records exactly that rather
// than a reason of its own.
func resolveTurnStopReason(responseReason, observedReason string) string {
	return firstNonEmpty(responseReason, observedReason)
}

func (c *acpConversationClient) prepareTurnSession(ctx context.Context, turn middleware.ConversationTurn, log *slog.Logger) (string, error) {
	if err := c.validatePromptContent(turn.ContentBlocks); err != nil {
		return "", err
	}
	remoteSessionID, origin, err := c.ensureACPRemoteSession(ctx, turn, c.turnCwd(turn), log)
	if err != nil {
		return "", classifyProviderFailure(turn.AgentID, c.endpoint, "session/new", err)
	}
	selection, err := c.selectTurnModelFor(ctx, turnModelSelection{
		SessionID:       remoteSessionID,
		ModelID:         turn.ModelID,
		FallbackModelID: turn.FallbackModelID,
		Origin:          origin,
	})
	if err != nil {
		return remoteSessionID, classifyProviderFailure(turn.AgentID, c.endpoint, "session/set_model", err)
	}
	if turn.ModelID != "" {
		if notifier, ok := turn.ThoughtNotifier.(middleware.ModelSelectionNotifier); ok {
			notifier.OnModelSelection(selection)
		}
	}
	return remoteSessionID, nil
}

// ensureACPRemoteSession returns the remote session this turn runs against and
// how it reached the turn. The origin is read from what the adapter did — a
// session it created, resumed, loaded, or already had — and never guessed from
// the session ID or from provider metadata.
func (c *acpConversationClient) ensureACPRemoteSession(ctx context.Context, turn middleware.ConversationTurn, cwd string, log *slog.Logger) (string, sessionOrigin, error) {
	if turn.RemoteSessionID == "" {
		if turn.StrictSession {
			return "", sessionOriginUnknown, fmt.Errorf("strict remote session requires an existing remote session ID")
		}
		sessionID, err := c.createAndConfigureACPRemoteSession(ctx, turn, cwd, log)
		return sessionID, sessionOriginCreated, err
	}
	if origin, attached, err := c.attachVerifiedSession(ctx, turn, cwd); err != nil || attached {
		return turn.RemoteSessionID, origin, err
	}
	if c.isLoadedSession(turn.RemoteSessionID) {
		return turn.RemoteSessionID, sessionOriginReused, nil
	}
	origin, err := c.restoreACPRemoteSession(ctx, turn, cwd, log)
	return turn.RemoteSessionID, origin, err
}

// attachVerifiedSession performs the verified attach a strict turn requires. The
// reported bool distinguishes "already attached, nothing was asked" from "the
// provider was asked and answered", which is what makes `reused` different from
// `resumed`/`loaded`.
func (c *acpConversationClient) attachVerifiedSession(ctx context.Context, turn middleware.ConversationTurn, cwd string) (sessionOrigin, bool, error) {
	if !turn.StrictSession || c.isLoadedSession(turn.RemoteSessionID) {
		return sessionOriginUnknown, false, nil
	}
	info, err := c.AttachExistingRemoteSession(ctx, turn.RemoteSessionID, cwd)
	if err != nil {
		return sessionOriginUnknown, true, err
	}
	return originFromVerificationMethod(info.VerificationMethod), true, nil
}

// originFromVerificationMethod maps the attach handshake the provider answered
// onto the origin it establishes. An unrecognised or empty method stays unknown:
// the origin is only ever what the provider confirmed.
func originFromVerificationMethod(method string) sessionOrigin {
	switch strings.TrimSpace(method) {
	case "session/resume":
		return sessionOriginResumed
	case "session/load":
		return sessionOriginLoaded
	default:
		return sessionOriginUnknown
	}
}

// originIf reports the origin a confirmed transition establishes.
func originIf(confirmed bool, origin sessionOrigin) sessionOrigin {
	if confirmed {
		return origin
	}
	return sessionOriginUnknown
}

// restoreACPRemoteSession restores a remote session this process does not hold.
// It reports how the session was restored, which is what a caller needs to state
// whether the provider confirmed its state on this turn.
func (c *acpConversationClient) restoreACPRemoteSession(ctx context.Context, turn middleware.ConversationTurn, cwd string, log *slog.Logger) (sessionOrigin, error) {
	if !c.sessionCapabilities.Resume && !c.sessionCapabilities.Load {
		return sessionOriginUnknown, nil
	}
	mcpServers, err := c.turnMCPServers(turn.McpServers)
	if err != nil {
		return sessionOriginUnknown, err
	}
	req := acpLoadRemoteSessionRequest{
		Ctx: ctx, RemoteSessionID: turn.RemoteSessionID, Cwd: cwd,
		AdditionalDirectories: turn.AdditionalDirectories, McpServers: mcpServers,
		Notifier: turn.ThoughtNotifier, Log: log,
	}
	if c.sessionCapabilities.Resume {
		resumed, err := c.resumeACPRemoteSession(req)
		if err != nil || resumed {
			return originIf(resumed, sessionOriginResumed), err
		}
	}
	if c.sessionCapabilities.Load {
		if err := c.loadACPRemoteSession(req); err != nil {
			return sessionOriginUnknown, err
		}
		return sessionOriginLoaded, nil
	}
	return sessionOriginUnknown, nil
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
	if err := c.applySessionMode(ctx, fromZedACPSession(newSessResp), newSessResp.SessionID, log); err != nil {
		c.unmarkLoadedSession(newSessResp.SessionID)
		return "", err
	}
	return newSessResp.SessionID, nil
}

func (c *acpConversationClient) applySessionMode(ctx context.Context, session *middleware.NewSessionResponse, sessionID string, log *slog.Logger) error {
	if c.preferredMode != "" {
		return c.applyPreferredSessionMode(ctx, session, sessionID)
	}
	if configID, value := pickAutoApproveConfigOption(session); configID != "" && value != "" {
		if _, err := c.currentACPClient().SetConfigOption(ctx, acpSetConfigOptionRequest{
			SessionID: sessionID,
			ConfigID:  configID,
			Value:     value,
		}); err != nil {
			log.Warn("failed to set ACP config option", "config_id", configID, "value", value, "error", err)
		}
		return nil
	}
	modeID := pickAutoApproveMode(session)
	if modeID == "" {
		return nil
	}
	if err := c.currentACPClient().SetMode(ctx, sessionID, modeID); err != nil {
		log.Warn("failed to set ACP mode", "mode", modeID, "error", err)
	}
	return nil
}

func (c *acpConversationClient) applyPreferredSessionMode(ctx context.Context, session *middleware.NewSessionResponse, sessionID string) error {
	if session == nil {
		return fmt.Errorf("configured provider mode %q cannot be verified: empty session state", c.preferredMode)
	}
	if currentSessionMode(session) == c.preferredMode {
		return nil
	}
	if configID, value := findSessionConfigMode(session, c.preferredMode); configID != "" {
		_, err := c.currentACPClient().SetConfigOption(ctx, acpSetConfigOptionRequest{SessionID: sessionID, ConfigID: configID, Value: value})
		return wrapConfiguredModeError(c.preferredMode, err)
	}
	if !hasSessionMode(session, c.preferredMode) {
		return fmt.Errorf("configured provider mode %q is unavailable in ACP session state", c.preferredMode)
	}
	err := c.currentACPClient().SetMode(ctx, sessionID, c.preferredMode)
	return wrapConfiguredModeError(c.preferredMode, err)
}

func findSessionConfigMode(session *middleware.NewSessionResponse, preferred string) (string, string) {
	for _, option := range session.ConfigOptions {
		if option.Category != "mode" && !strings.EqualFold(option.ID, "mode") {
			continue
		}
		for _, value := range option.Options {
			if value.ID == preferred {
				return option.ID, value.ID
			}
		}
	}
	return "", ""
}

func currentSessionMode(session *middleware.NewSessionResponse) string {
	if session.Modes == nil {
		return ""
	}
	return session.Modes.CurrentModeID
}

func hasSessionMode(session *middleware.NewSessionResponse, preferred string) bool {
	if session.Modes == nil {
		return false
	}
	for _, mode := range session.Modes.AvailableModes {
		if mode.ID == preferred {
			return true
		}
	}
	return false
}

func wrapConfiguredModeError(mode string, err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("apply configured provider mode %q: %w", mode, err)
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
	resp, err := c.currentACPClient().ResumeSession(req.Ctx, acpResumeSessionRequest{
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
	if err := c.applySessionMode(req.Ctx, fromZedACPResumeSession(resp), req.RemoteSessionID, req.Log); err != nil {
		return false, err
	}
	return true, nil
}

func (c *acpConversationClient) loadACPRemoteSession(req acpLoadRemoteSessionRequest) error {
	obs := &simpleObserver{updates: make(chan struct{}, 1), notifier: req.Notifier}
	additionalDirectories, err := c.additionalDirectories(req.AdditionalDirectories)
	if err != nil {
		return err
	}
	resp, err := c.currentACPClient().LoadSession(req.Ctx, acpLoadSessionRequest{
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
	if err := c.applySessionMode(req.Ctx, fromZedACPLoadSession(resp), req.RemoteSessionID, req.Log); err != nil {
		return err
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

func (c *acpConversationClient) retryTurnWithFreshSession(ctx context.Context, turn middleware.ConversationTurn) (middleware.ConversationResult, error) {
	// executeTurnOnce, not ExecuteTurn: this retry is about a lost session, and
	// nesting the authentication retry inside it would give one caller-visible
	// operation more than the single authentication attempt it is allowed.
	return c.executeTurnOnce(ctx, middleware.ConversationTurn{
		AgentID:                  turn.AgentID,
		LogicalSessionID:         turn.LogicalSessionID,
		WorkspacePath:            turn.WorkspacePath,
		Message:                  turn.Message,
		ContentBlocks:            turn.ContentBlocks,
		ExtensionURIs:            turn.ExtensionURIs,
		ReferencedRemoteSessions: turn.ReferencedRemoteSessions,
		SidecarCapsules:          turn.SidecarCapsules,
		Tools:                    turn.Tools,
		McpServers:               turn.McpServers,
		AdditionalDirectories:    turn.AdditionalDirectories,
		ThoughtNotifier:          turn.ThoughtNotifier,
		LiveContextAttach:        turn.LiveContextAttach,
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

func (c *acpConversationClient) turnMCPServers(turnServers []middleware.McpServerConfig) ([]acpMcpServerConfig, error) {
	var servers []acpMcpServerConfig
	if len(turnServers) > 0 {
		servers = toZedACPMCPServers(turnServers)
	} else {
		servers = cloneACPMCPServers(c.mcpServers)
	}
	if err := c.validateMCPServers(servers); err != nil {
		return nil, err
	}
	return servers, nil
}

func (c *acpConversationClient) additionalDirectories(values []string) ([]string, error) {
	if len(values) == 0 {
		return nil, nil
	}
	if !c.sessionCapabilities.AdditionalDirectories {
		return nil, fmt.Errorf("ACP agent does not advertise additionalDirectories; requested directories cannot be used")
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

func (c *acpConversationClient) DeleteRemoteSession(ctx context.Context, remoteSessionID string) error {
	if !c.sessionCapabilities.Delete {
		return fmt.Errorf("ACP agent does not advertise session/delete")
	}
	if err := c.currentACPClient().DeleteSession(ctx, remoteSessionID); err != nil {
		return err
	}
	c.unmarkLoadedSession(remoteSessionID)
	return nil
}

func (c *acpConversationClient) CancelRemoteSession(ctx context.Context, remoteSessionID string) error {
	if strings.TrimSpace(remoteSessionID) == "" {
		return fmt.Errorf("ACP session id is required")
	}
	return c.currentACPClient().CancelSession(ctx, remoteSessionID)
}

func (c *acpConversationClient) CloseRemoteSession(ctx context.Context, remoteSessionID string) error {
	if !c.sessionCapabilities.Close {
		return fmt.Errorf("ACP agent does not advertise session/close")
	}
	if strings.TrimSpace(remoteSessionID) == "" {
		return fmt.Errorf("ACP session id is required")
	}
	if err := c.currentACPClient().CloseSession(ctx, remoteSessionID); err != nil {
		return err
	}
	c.unmarkLoadedSession(remoteSessionID)
	return nil
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
