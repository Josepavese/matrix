package a2aclient

import (
	"context"
	"fmt"
	"net/url"
	"strings"

	"github.com/Josepavese/matrix/internal/middleware"
	"github.com/Josepavese/matrix/internal/providers/a2astate"
	"github.com/Josepavese/matrix/internal/providers/sidecarprojection"
	"github.com/a2aproject/a2a-go/v2/a2a"
	a2ago "github.com/a2aproject/a2a-go/v2/a2aclient"
	"github.com/a2aproject/a2a-go/v2/a2aclient/agentcard"
)

type Factory struct{}

func (f Factory) NewClient(ctx context.Context, endpoint middleware.ProtocolEndpoint, _ middleware.ConversationFactoryDeps) (middleware.ConversationClient, error) {
	var transport a2a.TransportProtocol
	switch strings.ToUpper(strings.TrimSpace(endpoint.Transport)) {
	case "", "JSONRPC":
		transport = a2a.TransportProtocolJSONRPC
	case "HTTP+JSON":
		transport = a2a.TransportProtocolHTTPJSON
	default:
		return nil, fmt.Errorf("unsupported A2A transport: %s", endpoint.Transport)
	}

	if endpoint.Address == "" && endpoint.CardURL == "" {
		return nil, fmt.Errorf("A2A endpoint address or agent card URL is required")
	}

	options := []a2ago.FactoryOption{a2ago.WithConfig(a2ago.Config{
		PreferredTransports: []a2a.TransportProtocol{transport},
		AcceptedOutputModes: []string{"text/plain", "application/json", "application/octet-stream", "image/*", "audio/*"},
	})}
	if len(endpoint.Headers) > 0 {
		options = append(options, a2ago.WithCallInterceptors(&headerInterceptor{headers: endpoint.Headers}))
	}
	client, advertised, err := newSDKClient(ctx, endpoint, transport, options)
	if err != nil {
		return nil, fmt.Errorf("failed to create A2A client: %w", err)
	}

	return &a2aConversationClient{client: client, advertised: advertised}, nil
}

func newSDKClient(ctx context.Context, endpoint middleware.ProtocolEndpoint, transport a2a.TransportProtocol, options []a2ago.FactoryOption) (*a2ago.Client, *a2a.AgentCapabilities, error) {
	if endpoint.CardURL != "" {
		card, err := resolveAgentCard(ctx, endpoint.CardURL, endpoint.Headers)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to resolve A2A agent card: %w", err)
		}
		client, err := a2ago.NewFromCard(ctx, card, options...)
		if err != nil {
			return nil, nil, err
		}
		capabilities := card.Capabilities
		return client, &capabilities, nil
	}
	iface := a2a.NewAgentInterface(endpoint.Address, transport)
	iface.Tenant = strings.TrimSpace(endpoint.Tenant)
	if endpoint.ProtocolVersion != "" {
		iface.ProtocolVersion = a2a.ProtocolVersion(endpoint.ProtocolVersion)
	}
	client, err := a2ago.NewFromEndpoints(ctx, []*a2a.AgentInterface{iface}, options...)
	return client, nil, err
}

func resolveAgentCard(ctx context.Context, cardURL string, headers map[string]string) (*a2a.AgentCard, error) {
	parsed, err := url.Parse(cardURL)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return nil, fmt.Errorf("invalid A2A agent card URL %q", cardURL)
	}
	baseURL := parsed.Scheme + "://" + parsed.Host
	var options []agentcard.ResolveOption
	if cardPath := parsed.EscapedPath(); cardPath != "" && cardPath != "/" {
		options = append(options, agentcard.WithPath(cardPath))
	}
	for key, value := range headers {
		options = append(options, agentcard.WithRequestHeader(key, value))
	}
	return agentcard.DefaultResolver.Resolve(ctx, baseURL, options...)
}

type headerInterceptor struct {
	a2ago.PassthroughInterceptor
	headers map[string]string
}

func (i *headerInterceptor) Before(ctx context.Context, req *a2ago.Request) (context.Context, any, error) {
	if req.ServiceParams == nil {
		req.ServiceParams = make(a2ago.ServiceParams)
	}
	for key, value := range i.headers {
		if strings.TrimSpace(key) != "" {
			req.ServiceParams.Append(key, value)
		}
	}
	return ctx, nil, nil
}

type a2aConversationClient struct {
	client     *a2ago.Client
	advertised *a2a.AgentCapabilities
}

func (c *a2aConversationClient) Alive() bool {
	return c.client != nil
}

func (c *a2aConversationClient) Close() error {
	if c.client == nil {
		return nil
	}
	return c.client.Destroy()
}

func (c *a2aConversationClient) ExecuteTurn(ctx context.Context, turn middleware.ConversationTurn) (middleware.ConversationResult, error) {
	req := a2aSendMessageRequest(turn)
	result, err := c.executeA2ATurn(ctx, req, turn)
	if err != nil && turn.RemoteSessionID != "" && isA2ASessionNotFound(err) {
		return c.ExecuteTurn(ctx, a2aTurnWithoutRemoteSession(turn))
	}
	return result, err
}

func a2aSendMessageRequest(turn middleware.ConversationTurn) *a2a.SendMessageRequest {
	state := a2astate.Decode(turn.RemoteSessionID)
	msg := a2a.NewMessage(a2a.MessageRoleUser, sidecarprojection.A2AMessageParts(turn)...)
	msg.ContextID = state.ContextID
	msg.TaskID = a2a.TaskID(state.TaskID)
	msg.Extensions = append([]string(nil), turn.ExtensionURIs...)
	for _, remoteSessionID := range turn.ReferencedRemoteSessions {
		if taskID := a2astate.TaskID(remoteSessionID); taskID != "" {
			msg.ReferenceTasks = append(msg.ReferenceTasks, a2a.TaskID(taskID))
		}
	}
	sidecarprojection.ApplyA2AMetadata(msg, turn.SidecarCapsules)
	return &a2a.SendMessageRequest{Message: msg, Metadata: sidecarprojection.A2ARequestMetadata(turn.SidecarCapsules)}
}

func (c *a2aConversationClient) executeA2ATurn(ctx context.Context, req *a2a.SendMessageRequest, turn middleware.ConversationTurn) (middleware.ConversationResult, error) {
	if turn.ThoughtNotifier == nil || c.advertised == nil || !c.advertised.Streaming {
		return c.sendA2ANonStreaming(ctx, req, turn)
	}
	return c.streamA2AResult(ctx, req, turn)
}

func (c *a2aConversationClient) streamA2AResult(ctx context.Context, req *a2a.SendMessageRequest, turn middleware.ConversationTurn) (middleware.ConversationResult, error) {
	output, blocks, metadata, nextState, err := c.streamA2A(ctx, req, turn)
	if err != nil {
		return middleware.ConversationResult{}, err
	}
	return middleware.ConversationResult{
		Output:          output,
		ContentBlocks:   blocks,
		RemoteSessionID: a2astate.Encode(nextState),
		Metadata:        metadata,
	}, nil
}

func (c *a2aConversationClient) sendA2ANonStreaming(ctx context.Context, req *a2a.SendMessageRequest, turn middleware.ConversationTurn) (middleware.ConversationResult, error) {
	resp, err := c.client.SendMessage(ctx, req)
	if err != nil {
		return middleware.ConversationResult{}, fmt.Errorf("A2A send message failed: %w", err)
	}
	result := a2aResultFromSendMessage(resp)
	if turn.ThoughtNotifier != nil && result.RemoteSessionID != "" {
		turn.ThoughtNotifier.SetHeader(turn.AgentID, result.RemoteSessionID)
	}
	return result, nil
}

func (c *a2aConversationClient) ListRemoteSessions(ctx context.Context) ([]middleware.RemoteSessionInfo, error) {
	var out []middleware.RemoteSessionInfo
	pageToken := ""
	for page := 0; page < 1000; page++ {
		resp, err := c.client.ListTasks(ctx, &a2a.ListTasksRequest{PageSize: 100, PageToken: pageToken})
		if err != nil {
			return nil, err
		}
		for _, task := range resp.Tasks {
			if task == nil {
				continue
			}
			state := a2astate.State{TaskID: string(task.ID), ContextID: task.ContextID}
			out = append(out, middleware.RemoteSessionInfo{
				RemoteSessionID: a2astate.Encode(state),
				DisplayID:       string(task.ID),
				Title:           a2aTaskTitle(task),
				Status:          string(task.Status.State),
				UpdatedAt:       a2aTaskUpdatedAt(task),
				ProtocolKind:    middleware.ProtocolKindA2A,
				CanResume:       true,
				CanDelete:       false,
			})
		}
		next := strings.TrimSpace(resp.NextPageToken)
		if next == "" {
			return out, nil
		}
		if next == pageToken {
			return nil, fmt.Errorf("A2A tasks/list returned a repeated page token")
		}
		pageToken = next
	}
	return nil, fmt.Errorf("A2A tasks/list pagination exceeded safety limit")
}

func (c *a2aConversationClient) GetRemoteSession(ctx context.Context, remoteSessionID string) (middleware.RemoteSessionInfo, error) {
	taskID := a2astate.TaskID(remoteSessionID)
	if taskID == "" {
		return middleware.RemoteSessionInfo{}, fmt.Errorf("A2A task id is required")
	}
	task, err := c.client.GetTask(ctx, &a2a.GetTaskRequest{ID: a2a.TaskID(taskID)})
	if err != nil {
		return middleware.RemoteSessionInfo{}, err
	}
	info := a2astate.State{TaskID: string(task.ID), ContextID: task.ContextID}
	return middleware.RemoteSessionInfo{
		RemoteSessionID: a2astate.Encode(info),
		DisplayID:       string(task.ID),
		Title:           a2aTaskTitle(task),
		Status:          string(task.Status.State),
		UpdatedAt:       a2aTaskUpdatedAt(task),
		ProtocolKind:    middleware.ProtocolKindA2A,
		CanResume:       true,
		CanDelete:       false,
	}, nil
}

func (c *a2aConversationClient) DeleteRemoteSession(_ context.Context, _ string) error {
	return fmt.Errorf("A2A delete is unsupported; use cancel for task lifecycle cleanup")
}

func (c *a2aConversationClient) CancelRemoteSession(ctx context.Context, remoteSessionID string) error {
	taskID := a2astate.TaskID(remoteSessionID)
	if taskID == "" {
		return fmt.Errorf("A2A task id is required")
	}
	_, err := c.client.CancelTask(ctx, &a2a.CancelTaskRequest{ID: a2a.TaskID(taskID)})
	if err != nil && a2aTaskGoneError(err) {
		return nil
	}
	return err
}

func (c *a2aConversationClient) CloseRemoteSession(_ context.Context, _ string) error {
	return fmt.Errorf("A2A remote session close is not supported")
}
