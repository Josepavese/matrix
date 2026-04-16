package agents

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/a2aproject/a2a-go/v2/a2a"
	"github.com/a2aproject/a2a-go/v2/a2aclient"
	"github.com/jose/matrix-v2/internal/middleware"
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
	state := decodeA2ARemoteSession(turn.RemoteSessionID)
	msg := a2a.NewMessage(a2a.MessageRoleUser, a2a.NewTextPart(turn.Message))
	msg.ContextID = state.ContextID
	msg.TaskID = a2a.TaskID(state.TaskID)

	req := &a2a.SendMessageRequest{Message: msg}

	if turn.ThoughtNotifier == nil {
		resp, err := c.client.SendMessage(ctx, req)
		if err != nil && turn.RemoteSessionID != "" && isA2ASessionNotFound(err) {
			return c.ExecuteTurn(ctx, middleware.ConversationTurn{
				AgentID:          turn.AgentID,
				LogicalSessionID: turn.LogicalSessionID,
				Message:          turn.Message,
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
			Message:          turn.Message,
			Tools:            turn.Tools,
			ThoughtNotifier:  turn.ThoughtNotifier,
		})
	}
	if err != nil {
		return middleware.ConversationResult{}, err
	}
	return middleware.ConversationResult{
		Output:          output,
		RemoteSessionID: encodeA2ARemoteSession(nextState),
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
	}
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
		state := a2aRemoteSession{TaskID: string(task.ID), ContextID: task.ContextID}
		out = append(out, middleware.RemoteSessionInfo{
			RemoteSessionID: encodeA2ARemoteSession(state),
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
	state := decodeA2ARemoteSession(remoteSessionID)
	taskID := strings.TrimSpace(state.TaskID)
	if taskID == "" {
		taskID = strings.TrimSpace(remoteSessionID)
	}
	if taskID == "" {
		return middleware.RemoteSessionInfo{}, fmt.Errorf("A2A task id is required")
	}
	task, err := c.client.GetTask(ctx, &a2a.GetTaskRequest{ID: a2a.TaskID(taskID)})
	if err != nil {
		return middleware.RemoteSessionInfo{}, err
	}
	info := a2aRemoteSession{TaskID: string(task.ID), ContextID: task.ContextID}
	return middleware.RemoteSessionInfo{
		RemoteSessionID: encodeA2ARemoteSession(info),
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
	state := decodeA2ARemoteSession(remoteSessionID)
	taskID := strings.TrimSpace(state.TaskID)
	if taskID == "" {
		taskID = strings.TrimSpace(remoteSessionID)
	}
	if taskID == "" {
		return fmt.Errorf("A2A task id is required")
	}
	_, err := c.client.CancelTask(ctx, &a2a.CancelTaskRequest{ID: a2a.TaskID(taskID)})
	if err != nil && a2aTaskGoneError(err) {
		return nil
	}
	return err
}

func (c *a2aConversationClient) streamA2A(ctx context.Context, req *a2a.SendMessageRequest, turn middleware.ConversationTurn) (string, a2aRemoteSession, error) {
	var builder strings.Builder
	state := decodeA2ARemoteSession(turn.RemoteSessionID)

	for event, err := range c.client.SendStreamingMessage(ctx, req) {
		if err != nil {
			return "", a2aRemoteSession{}, fmt.Errorf("A2A streaming failed: %w", err)
		}
		chunk, nextState := a2aTextFromEvent(event)
		if nextState.TaskID != "" {
			state = nextState
			if turn.ThoughtNotifier != nil {
				turn.ThoughtNotifier.SetHeader(turn.AgentID, encodeA2ARemoteSession(state))
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

type a2aRemoteSession struct {
	TaskID    string `json:"task_id,omitempty"`
	ContextID string `json:"context_id,omitempty"`
}

func encodeA2ARemoteSession(state a2aRemoteSession) string {
	if state.TaskID == "" && state.ContextID == "" {
		return ""
	}
	data, _ := json.Marshal(state)
	return string(data)
}

func decodeA2ARemoteSession(raw string) a2aRemoteSession {
	if strings.TrimSpace(raw) == "" {
		return a2aRemoteSession{}
	}
	var state a2aRemoteSession
	if err := json.Unmarshal([]byte(raw), &state); err != nil {
		return a2aRemoteSession{}
	}
	return state
}

func a2aResultFromSendMessage(resp a2a.SendMessageResult) middleware.ConversationResult {
	output, state := a2aTextFromEvent(resp)
	return middleware.ConversationResult{
		Output:          strings.TrimSpace(output),
		RemoteSessionID: encodeA2ARemoteSession(state),
		Metadata:        middleware.ConversationMetadata{Status: "active"},
	}
}

func a2aTextFromEvent(event a2a.Event) (string, a2aRemoteSession) {
	switch v := event.(type) {
	case *a2a.Message:
		return a2aPartsText(v.Parts), a2aRemoteSession{TaskID: string(v.TaskID), ContextID: v.ContextID}
	case *a2a.Task:
		var parts []string
		if v.Status.Message != nil {
			parts = append(parts, a2aPartsText(v.Status.Message.Parts))
		}
		for _, artifact := range v.Artifacts {
			parts = append(parts, a2aPartsText(artifact.Parts))
		}
		return strings.TrimSpace(strings.Join(parts, "\n")), a2aRemoteSession{TaskID: string(v.ID), ContextID: v.ContextID}
	case *a2a.TaskArtifactUpdateEvent:
		return a2aPartsText(v.Artifact.Parts), a2aRemoteSession{TaskID: string(v.TaskID), ContextID: v.ContextID}
	case *a2a.TaskStatusUpdateEvent:
		if v.Status.Message != nil {
			return a2aPartsText(v.Status.Message.Parts), a2aRemoteSession{TaskID: string(v.TaskID), ContextID: v.ContextID}
		}
		return "", a2aRemoteSession{TaskID: string(v.TaskID), ContextID: v.ContextID}
	default:
		return "", a2aRemoteSession{}
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
