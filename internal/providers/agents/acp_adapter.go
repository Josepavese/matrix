package agents

import (
	"context"
	"fmt"
	"log/slog"
	"os"
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
		ProtocolVersion: 1,
		ClientInfo:      map[string]interface{}{"name": "matrix", "version": "1.0"},
		ClientCapabilities: &acpClientCapabilities{
			Fs: &acpFsCapability{
				ReadTextFile:  deps.FS != nil,
				WriteTextFile: deps.FS != nil,
			},
			Terminal: deps.Process != nil,
		},
	}
	initResp, err := client.Initialize(ctx, initReq)
	if err != nil {
		_ = transport.Close()
		return nil, classifyProviderFailure("", endpoint, "initialize", fmt.Errorf("ACP initialize failed: %w", err))
	}
	for _, m := range initResp.AuthMethods {
		if m.Type != "env_var" || m.EnvVar == "" {
			continue
		}
		if val, ok := os.LookupEnv(m.EnvVar); ok {
			if err := client.Authenticate(ctx, m.ID, map[string]string{"api_key": val}); err != nil {
				slog.Warn("ACP authentication failed", "methodId", m.ID, "error", err)
			}
		}
	}

	caps := acpSessionCapabilities(initResp)
	return &acpConversationClient{
		client:              client,
		handler:             handler,
		cwd:                 deps.Cwd,
		endpoint:            endpoint,
		sessionCapabilities: caps,
		loadedSessions:      map[string]bool{},
	}, nil
}

type acpConversationClient struct {
	client              ACPClient
	handler             *defaultRequestHandler
	cwd                 string
	endpoint            middleware.ProtocolEndpoint
	sessionCapabilities middleware.ConversationSessionCapabilities
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
		if c.resumeACPRemoteSession(acpLoadRemoteSessionRequest{
			Ctx:             ctx,
			RemoteSessionID: turn.RemoteSessionID,
			Cwd:             cwd,
			Notifier:        turn.ThoughtNotifier,
			Log:             log,
		}) {
			return turn.RemoteSessionID, nil
		}
	}
	if !c.isLoadedSession(turn.RemoteSessionID) && c.sessionCapabilities.Load {
		c.loadACPRemoteSession(acpLoadRemoteSessionRequest{
			Ctx:             ctx,
			RemoteSessionID: turn.RemoteSessionID,
			Cwd:             cwd,
			Notifier:        turn.ThoughtNotifier,
			Log:             log,
		})
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
		LogicalSessionID: turn.LogicalSessionID,
		WorkspacePath:    cwd,
		Tools:            turn.Tools,
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
	Ctx             context.Context
	RemoteSessionID string
	Cwd             string
	Notifier        middleware.ThoughtNotifier
	Log             *slog.Logger
}

func (c *acpConversationClient) resumeACPRemoteSession(req acpLoadRemoteSessionRequest) bool {
	resp, err := c.client.ResumeSession(req.Ctx, acpResumeSessionRequest{
		SessionID:  req.RemoteSessionID,
		Cwd:        req.Cwd,
		McpServers: []acpMcpServerConfig{},
	})
	if err != nil {
		req.Log.Warn("ACP session resume failed, will fall back to load/prompt flow", "agent_session", req.RemoteSessionID, "error", err)
		return false
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
	return true
}

func (c *acpConversationClient) loadACPRemoteSession(req acpLoadRemoteSessionRequest) {
	obs := &simpleObserver{updates: make(chan struct{}, 1), notifier: req.Notifier}
	resp, err := c.client.LoadSession(req.Ctx, acpLoadSessionRequest{SessionID: req.RemoteSessionID, Cwd: req.Cwd, McpServers: []acpMcpServerConfig{}}, obs)
	if err != nil {
		req.Log.Warn("ACP session load failed, will fall back to prompt/recreate flow", "agent_session", req.RemoteSessionID, "error", err)
		return
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
		Prompt:    []acpContent{{Type: "text", Text: promptText}},
		Meta:      sidecarprojection.ACPMeta(turn.SidecarCapsules),
	}, obs)
}

func (c *acpConversationClient) retryTurnWithFreshSession(ctx context.Context, turn middleware.ConversationTurn) (middleware.ConversationResult, error) {
	return c.ExecuteTurn(ctx, middleware.ConversationTurn{
		AgentID:           turn.AgentID,
		LogicalSessionID:  turn.LogicalSessionID,
		WorkspacePath:     turn.WorkspacePath,
		Message:           turn.Message,
		SidecarCapsules:   turn.SidecarCapsules,
		Tools:             turn.Tools,
		ThoughtNotifier:   turn.ThoughtNotifier,
		LiveContextAttach: turn.LiveContextAttach,
	})
}

func (c *acpConversationClient) turnCwd(turn middleware.ConversationTurn) string {
	if strings.TrimSpace(turn.WorkspacePath) != "" {
		return strings.TrimSpace(turn.WorkspacePath)
	}
	return c.cwd
}

func (c *acpConversationClient) SessionCapabilities() middleware.ConversationSessionCapabilities {
	return c.sessionCapabilities
}

func (c *acpConversationClient) ListRemoteSessions(ctx context.Context) ([]middleware.RemoteSessionInfo, error) {
	if !c.sessionCapabilities.List {
		return nil, fmt.Errorf("ACP agent does not advertise session/list")
	}
	resp, err := c.client.ListSessions(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]middleware.RemoteSessionInfo, 0, len(resp.Sessions))
	for _, session := range resp.Sessions {
		out = append(out, middleware.RemoteSessionInfo{
			RemoteSessionID: session.SessionID,
			DisplayID:       session.SessionID,
			Title:           session.Title,
			UpdatedAt:       session.UpdatedAt,
			ProtocolKind:    middleware.ProtocolKindACP,
			CanResume:       c.sessionCapabilities.Load || c.sessionCapabilities.Resume,
			CanDelete:       c.sessionCapabilities.Delete,
		})
	}
	return out, nil
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
			McpServers: []acpMcpServerConfig{},
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
			McpServers: []acpMcpServerConfig{},
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
