package agents

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/jose/matrix-v2/internal/middleware"
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
		return nil, fmt.Errorf("ACP initialize failed: %w", err)
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

	return &acpConversationClient{
		client:               client,
		handler:              handler,
		cwd:                  deps.Cwd,
		listSessionSupport:   supportsSessionCapability(initResp, "list"),
		loadSessionSupport:   supportsLoadSession(initResp),
		closeSessionSupport:  supportsSessionCapability(initResp, "close"),
		deleteSessionSupport: supportsSessionCapability(initResp, "delete"),
		loadedSessions:       map[string]bool{},
	}, nil
}

type acpConversationClient struct {
	client               ACPClient
	handler              *defaultRequestHandler
	cwd                  string
	listSessionSupport   bool
	loadSessionSupport   bool
	closeSessionSupport  bool
	deleteSessionSupport bool
	loadedSessions       map[string]bool
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

	remoteSessionID := turn.RemoteSessionID
	cwd := c.turnCwd(turn)
	if remoteSessionID == "" {
		newSessResp, err := c.client.NewSession(ctx, acpNewSessionRequest{
			ClientTitle: turn.LogicalSessionID,
			Cwd:         cwd,
			McpServers:  []acpMcpServerConfig{},
			Tools:       toZedACPTools(turn.Tools),
		})
		if err != nil {
			return middleware.ConversationResult{}, fmt.Errorf("ACP new session failed: %w", err)
		}
		remoteSessionID = newSessResp.SessionID
		c.loadedSessions[remoteSessionID] = true
		if modeID := pickAutoApproveMode(fromZedACPSession(newSessResp)); modeID != "" {
			if err := c.client.SetMode(ctx, remoteSessionID, modeID); err != nil {
				log.Warn("failed to set ACP mode", "mode", modeID, "error", err)
			}
		}
	} else if c.loadSessionSupport && !c.loadedSessions[remoteSessionID] {
		obs := &simpleObserver{updates: make(chan struct{}, 1), notifier: turn.ThoughtNotifier}
		if err := c.client.LoadSession(ctx, acpLoadSessionRequest{
			SessionID:  remoteSessionID,
			Cwd:        cwd,
			McpServers: []acpMcpServerConfig{},
		}, obs); err != nil {
			log.Warn("ACP session load failed, will fall back to prompt/recreate flow", "agent_session", remoteSessionID, "error", err)
		} else {
			c.loadedSessions[remoteSessionID] = true
			obs.WaitIdle(ctx, 150*time.Millisecond)
		}
	}

	if turn.ThoughtNotifier != nil {
		turn.ThoughtNotifier.SetHeader(turn.AgentID, remoteSessionID)
	}
	if c.handler != nil {
		c.handler.WithNotifier(turn.ThoughtNotifier)
	}

	obs := &simpleObserver{updates: make(chan struct{}, 1), notifier: turn.ThoughtNotifier}
	resp, err := c.client.Prompt(ctx, acpPromptRequest{
		SessionID: remoteSessionID,
		Prompt: []acpContent{
			{Type: "text", Text: turn.Message},
		},
	}, obs)
	if err != nil && remoteSessionID != "" && isSessionNotFoundError(err) {
		log.Warn("ACP session lost, recreating", "agent_session", remoteSessionID)
		return c.ExecuteTurn(ctx, middleware.ConversationTurn{
			AgentID:          turn.AgentID,
			LogicalSessionID: turn.LogicalSessionID,
			WorkspacePath:    turn.WorkspacePath,
			Message:          turn.Message,
			Tools:            turn.Tools,
			ThoughtNotifier:  turn.ThoughtNotifier,
		})
	}
	if err != nil {
		return middleware.ConversationResult{
			Output:          obs.GetContent(),
			RemoteSessionID: remoteSessionID,
			Metadata:        obs.Metadata(),
		}, fmt.Errorf("ACP prompt failed: %w", err)
	}

	obs.WaitIdle(ctx, 150*time.Millisecond)
	return middleware.ConversationResult{
		Output:          obs.GetContent(),
		RemoteSessionID: remoteSessionID,
		ToolCalls:       fromZedACPToolCalls(resp.ToolCalls),
		Metadata:        obs.Metadata(),
	}, nil
}

func (c *acpConversationClient) turnCwd(turn middleware.ConversationTurn) string {
	if strings.TrimSpace(turn.WorkspacePath) != "" {
		return strings.TrimSpace(turn.WorkspacePath)
	}
	return c.cwd
}

func (c *acpConversationClient) SessionCapabilities() middleware.ConversationSessionCapabilities {
	return middleware.ConversationSessionCapabilities{List: c.listSessionSupport, Load: c.loadSessionSupport, Cancel: true, Close: c.closeSessionSupport, Delete: c.deleteSessionSupport}
}

func (c *acpConversationClient) ListRemoteSessions(ctx context.Context) ([]middleware.RemoteSessionInfo, error) {
	if !c.listSessionSupport {
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
			CanResume:       c.loadSessionSupport,
			CanDelete:       c.deleteSessionSupport,
		})
	}
	return out, nil
}

func (c *acpConversationClient) GetRemoteSession(ctx context.Context, remoteSessionID string) (middleware.RemoteSessionInfo, error) {
	sessions, err := c.ListRemoteSessions(ctx)
	if err != nil {
		return middleware.RemoteSessionInfo{}, err
	}
	for _, session := range sessions {
		if session.RemoteSessionID == remoteSessionID || session.DisplayID == remoteSessionID {
			return session, nil
		}
	}
	if c.loadSessionSupport {
		if err := c.client.LoadSession(ctx, acpLoadSessionRequest{
			SessionID:  remoteSessionID,
			Cwd:        c.cwd,
			McpServers: []acpMcpServerConfig{},
		}, nil); err == nil {
			c.loadedSessions[remoteSessionID] = true
			return middleware.RemoteSessionInfo{
				RemoteSessionID: remoteSessionID,
				DisplayID:       remoteSessionID,
				ProtocolKind:    middleware.ProtocolKindACP,
				CanResume:       c.loadSessionSupport,
				CanDelete:       c.deleteSessionSupport,
			}, nil
		}
	}
	return middleware.RemoteSessionInfo{}, fmt.Errorf("ACP session %s not found", remoteSessionID)
}

func (c *acpConversationClient) DeleteRemoteSession(ctx context.Context, remoteSessionID string) error {
	if !c.deleteSessionSupport {
		return fmt.Errorf("ACP agent does not advertise session/delete")
	}
	if err := c.client.DeleteSession(ctx, remoteSessionID); err != nil {
		return err
	}
	delete(c.loadedSessions, remoteSessionID)
	return nil
}

func (c *acpConversationClient) CancelRemoteSession(ctx context.Context, remoteSessionID string) error {
	if strings.TrimSpace(remoteSessionID) == "" {
		return fmt.Errorf("ACP session id is required")
	}
	return c.client.CancelSession(ctx, remoteSessionID)
}

func (c *acpConversationClient) CloseRemoteSession(ctx context.Context, remoteSessionID string) error {
	if !c.closeSessionSupport {
		return fmt.Errorf("ACP agent does not advertise session/close")
	}
	if strings.TrimSpace(remoteSessionID) == "" {
		return fmt.Errorf("ACP session id is required")
	}
	if err := c.client.CloseSession(ctx, remoteSessionID); err != nil {
		return err
	}
	delete(c.loadedSessions, remoteSessionID)
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

func fromZedACPSession(resp *acpNewSessionResponse) *middleware.NewSessionResponse {
	if resp == nil {
		return nil
	}
	out := &middleware.NewSessionResponse{SessionID: resp.SessionID}
	if resp.Modes != nil {
		out.Modes = &middleware.SessionModeState{
			CurrentModeID:  resp.Modes.CurrentModeID,
			AvailableModes: make([]middleware.SessionMode, 0, len(resp.Modes.AvailableModes)),
		}
		for _, mode := range resp.Modes.AvailableModes {
			out.Modes.AvailableModes = append(out.Modes.AvailableModes, middleware.SessionMode{
				ID:          mode.ID,
				Name:        mode.Name,
				Description: mode.Description,
			})
		}
	}
	if len(resp.ConfigOptions) > 0 {
		out.ConfigOptions = make([]middleware.ConfigOption, 0, len(resp.ConfigOptions))
		for _, opt := range resp.ConfigOptions {
			converted := middleware.ConfigOption{
				ID:       opt.ID,
				Name:     opt.Name,
				Category: opt.Category,
				Current:  opt.Current,
				Options:  make([]middleware.ConfigOptionValue, 0, len(opt.Options)),
			}
			for _, value := range opt.Options {
				converted.Options = append(converted.Options, middleware.ConfigOptionValue{
					ID:   value.ID,
					Name: value.Name,
				})
			}
			out.ConfigOptions = append(out.ConfigOptions, converted)
		}
	}
	return out
}
