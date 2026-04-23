package agents

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/a2aproject/a2a-go/v2/a2a"
	"github.com/a2aproject/a2a-go/v2/a2aclient"
	"github.com/jose/matrix-v2/internal/middleware"
	"github.com/jose/matrix-v2/internal/providers/a2astate"
	"github.com/jose/matrix-v2/internal/providers/sidecarprojection"
)

type a2aConversationFactory struct{}

func (f *a2aConversationFactory) NewClient(ctx context.Context, endpoint middleware.ProtocolEndpoint, _ middleware.ConversationFactoryDeps) (middleware.ConversationClient, error) {
	var transport a2a.TransportProtocol
	switch strings.ToUpper(strings.TrimSpace(endpoint.Transport)) {
	case "", "JSONRPC":
		transport = a2a.TransportProtocolJSONRPC
	case "HTTP+JSON":
		transport = a2a.TransportProtocolHTTPJSON
	default:
		return nil, fmt.Errorf("unsupported A2A transport: %s", endpoint.Transport)
	}

	if endpoint.Address == "" {
		return nil, fmt.Errorf("A2A endpoint address is required")
	}

	iface := a2a.NewAgentInterface(endpoint.Address, transport)
	if endpoint.ProtocolVersion != "" {
		iface.ProtocolVersion = a2a.ProtocolVersion(endpoint.ProtocolVersion)
	}

	client, err := a2aclient.NewFromEndpoints(ctx, []*a2a.AgentInterface{iface})
	if err != nil {
		return nil, fmt.Errorf("failed to create A2A client: %w", err)
	}

	return &a2aConversationClient{client: client}, nil
}

type a2aConversationClient struct {
	client *a2aclient.Client
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
	state := a2astate.Decode(turn.RemoteSessionID)
	msg := a2a.NewMessage(a2a.MessageRoleUser, sidecarprojection.A2AMessageParts(turn)...)
	msg.ContextID = state.ContextID
	msg.TaskID = a2a.TaskID(state.TaskID)
	sidecarprojection.ApplyA2AMetadata(msg, turn.SidecarCapsules)

	req := &a2a.SendMessageRequest{Message: msg, Metadata: sidecarprojection.A2ARequestMetadata(turn.SidecarCapsules)}

	if turn.ThoughtNotifier == nil {
		resp, err := c.client.SendMessage(ctx, req)
		if err != nil && turn.RemoteSessionID != "" && isA2ASessionNotFound(err) {
			return c.ExecuteTurn(ctx, middleware.ConversationTurn{
				AgentID:          turn.AgentID,
				LogicalSessionID: turn.LogicalSessionID,
				WorkspacePath:    turn.WorkspacePath,
				Message:          turn.Message,
				SidecarCapsules:  turn.SidecarCapsules,
				Tools:            turn.Tools,
				ThoughtNotifier:  turn.ThoughtNotifier,
			})
		}
		if err != nil {
			return middleware.ConversationResult{}, fmt.Errorf("A2A send message failed: %w", err)
		}
		return a2aResultFromSendMessage(resp), nil
	}

	output, nextState, err := c.streamA2A(ctx, req, turn)
	if err != nil && turn.RemoteSessionID != "" && isA2ASessionNotFound(err) {
		return c.ExecuteTurn(ctx, middleware.ConversationTurn{
			AgentID:          turn.AgentID,
			LogicalSessionID: turn.LogicalSessionID,
			WorkspacePath:    turn.WorkspacePath,
			Message:          turn.Message,
			SidecarCapsules:  turn.SidecarCapsules,
			Tools:            turn.Tools,
			ThoughtNotifier:  turn.ThoughtNotifier,
		})
	}
	if err != nil {
		return middleware.ConversationResult{}, err
	}
	return middleware.ConversationResult{
		Output:          output,
		RemoteSessionID: a2astate.Encode(nextState),
		Metadata: middleware.ConversationMetadata{
			Status: "active",
		},
	}, nil
}

func (c *a2aConversationClient) SessionCapabilities() middleware.ConversationSessionCapabilities {
	return middleware.ConversationSessionCapabilities{
		List:   true,
		Load:   true,
		Cancel: true,
		Delete: true,
		Details: map[string]middleware.CapabilityDescriptor{
			"list":   a2aCapability("list", true, "stable", "a2a_tasks/list"),
			"load":   a2aCapability("load", true, "stable", "a2a_task_get"),
			"cancel": a2aCapability("cancel", true, "stable", "a2a_tasks/cancel"),
			"delete": a2aCapability("delete", true, "stable", "a2a_task_delete"),
			"close":  a2aCapability("close", false, "unsupported", "a2a_no_close_mapping"),
			"resume": a2aCapability("resume", false, "unsupported", "a2a_task_state_mapping"),
			"fork":   a2aCapability("fork", false, "unsupported", "a2a_no_fork_mapping"),
		},
	}
}

func a2aCapability(name string, supported bool, stability, source string) middleware.CapabilityDescriptor {
	status := "unsupported"
	if supported {
		status = "supported"
	}
	desc := middleware.CapabilityDescriptor{Name: name, Supported: supported, Status: status, Stability: stability, Source: source}
	if name == "fork" {
		desc.ActiveParentSafe = boolPtr(false)
		desc.RequiresIdleParent = boolPtr(false)
		desc.ArtifactTurn = boolPtr(false)
	}
	return desc
}

func (c *a2aConversationClient) ListRemoteSessions(ctx context.Context) ([]middleware.RemoteSessionInfo, error) {
	resp, err := c.client.ListTasks(ctx, &a2a.ListTasksRequest{PageSize: 100})
	if err != nil {
		return nil, err
	}
	out := make([]middleware.RemoteSessionInfo, 0, len(resp.Tasks))
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
			CanDelete:       true,
		})
	}
	return out, nil
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
		CanDelete:       true,
	}, nil
}

func (c *a2aConversationClient) DeleteRemoteSession(ctx context.Context, remoteSessionID string) error {
	return c.CancelRemoteSession(ctx, remoteSessionID)
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

func (c *a2aConversationClient) streamA2A(ctx context.Context, req *a2a.SendMessageRequest, turn middleware.ConversationTurn) (string, a2astate.State, error) {
	var builder strings.Builder
	state := a2astate.Decode(turn.RemoteSessionID)

	for event, err := range c.client.SendStreamingMessage(ctx, req) {
		if err != nil {
			return "", a2astate.State{}, fmt.Errorf("A2A streaming failed: %w", err)
		}
		chunk, nextState := a2aTextFromEvent(event)
		if nextState.TaskID != "" {
			state = nextState
			if turn.ThoughtNotifier != nil {
				turn.ThoughtNotifier.SetHeader(turn.AgentID, a2astate.Encode(state))
			}
		}
		if chunk == "" {
			continue
		}
		builder.WriteString(chunk)
		turn.ThoughtNotifier.OnThought(middleware.ThoughtUpdate{
			Type:    middleware.ThoughtTypeThinking,
			Content: chunk,
		})
	}

	return strings.TrimSpace(builder.String()), state, nil
}

func a2aResultFromSendMessage(resp a2a.SendMessageResult) middleware.ConversationResult {
	output, state := a2aTextFromEvent(resp)
	return middleware.ConversationResult{
		Output:          strings.TrimSpace(output),
		RemoteSessionID: a2astate.Encode(state),
		Metadata:        middleware.ConversationMetadata{Status: "active"},
	}
}

func a2aTextFromEvent(event a2a.Event) (string, a2astate.State) {
	switch v := event.(type) {
	case *a2a.Message:
		return a2aPartsText(v.Parts), a2astate.State{TaskID: string(v.TaskID), ContextID: v.ContextID}
	case *a2a.Task:
		var parts []string
		if v.Status.Message != nil {
			parts = append(parts, a2aPartsText(v.Status.Message.Parts))
		}
		for _, artifact := range v.Artifacts {
			parts = append(parts, a2aPartsText(artifact.Parts))
		}
		return strings.TrimSpace(strings.Join(parts, "\n")), a2astate.State{TaskID: string(v.ID), ContextID: v.ContextID}
	case *a2a.TaskArtifactUpdateEvent:
		return a2aPartsText(v.Artifact.Parts), a2astate.State{TaskID: string(v.TaskID), ContextID: v.ContextID}
	case *a2a.TaskStatusUpdateEvent:
		if v.Status.Message != nil {
			return a2aPartsText(v.Status.Message.Parts), a2astate.State{TaskID: string(v.TaskID), ContextID: v.ContextID}
		}
		return "", a2astate.State{TaskID: string(v.TaskID), ContextID: v.ContextID}
	default:
		return "", a2astate.State{}
	}
}

func a2aPartsText(parts a2a.ContentParts) string {
	lines := make([]string, 0, len(parts))
	for _, part := range parts {
		if text := strings.TrimSpace(part.Text()); text != "" {
			lines = append(lines, text)
		}
	}
	return strings.TrimSpace(strings.Join(lines, "\n"))
}

func isA2ASessionNotFound(err error) bool {
	return strings.Contains(strings.ToLower(err.Error()), "task not found")
}

func a2aTaskTitle(task *a2a.Task) string {
	if task == nil {
		return ""
	}
	if task.Metadata != nil {
		if raw, ok := task.Metadata["title"].(string); ok {
			return strings.TrimSpace(raw)
		}
	}
	if task.Status.Message != nil {
		return strings.TrimSpace(a2aPartsText(task.Status.Message.Parts))
	}
	return ""
}

func a2aTaskUpdatedAt(task *a2a.Task) string {
	if task == nil || task.Status.Timestamp == nil {
		return ""
	}
	return task.Status.Timestamp.UTC().Format(time.RFC3339)
}

func a2aTaskGoneError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "task not found") || strings.Contains(msg, "failed to load a task")
}
