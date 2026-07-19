package a2aclient

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/Josepavese/matrix/internal/middleware"
	"github.com/Josepavese/matrix/internal/providers/a2astate"
	"github.com/a2aproject/a2a-go/v2/a2a"
)

func (c *a2aConversationClient) SubscribeRemoteTask(ctx context.Context, remoteSessionID string, observer middleware.RemoteTaskObserver) error {
	if c.advertised == nil || !c.advertised.Streaming {
		return fmt.Errorf("a2a agent does not advertise streaming task subscription")
	}
	if observer == nil {
		return fmt.Errorf("A2A task observer is required")
	}
	taskID := a2astate.TaskID(remoteSessionID)
	if taskID == "" {
		return fmt.Errorf("A2A task id is required")
	}
	for event, err := range c.client.SubscribeToTask(ctx, &a2a.SubscribeToTaskRequest{ID: a2a.TaskID(taskID)}) {
		if err != nil {
			return fmt.Errorf("A2A task subscription failed: %w", err)
		}
		projection := projectA2AEvent(event)
		observer.OnRemoteTaskEvent(remoteTaskEvent(projection))
	}
	return nil
}

func remoteTaskEvent(projection a2aEventProjection) middleware.RemoteTaskEvent {
	metadata := cloneA2AMetadata(projection.Metadata)
	if projection.State.TaskID != "" {
		metadata["task_id"] = projection.State.TaskID
	}
	if projection.State.ContextID != "" {
		metadata["context_id"] = projection.State.ContextID
	}
	return middleware.RemoteTaskEvent{
		RemoteSessionID: a2astate.Encode(projection.State),
		Kind:            projection.Kind,
		State:           string(projection.TaskState),
		Final:           projection.Final,
		ContentBlocks:   projection.Blocks,
		Metadata:        metadata,
	}
}

func (c *a2aConversationClient) CreateTaskPushConfig(ctx context.Context, config middleware.TaskPushConfig) (middleware.TaskPushConfig, error) {
	if c.advertised == nil || !c.advertised.PushNotifications {
		return middleware.TaskPushConfig{}, fmt.Errorf("a2a agent does not advertise push notifications")
	}
	request, err := toA2APushConfig(config)
	if err != nil {
		return middleware.TaskPushConfig{}, err
	}
	created, err := c.client.CreateTaskPushConfig(ctx, request)
	if err != nil {
		return middleware.TaskPushConfig{}, err
	}
	return fromA2APushConfig(created), nil
}

func (c *a2aConversationClient) GetTaskPushConfig(ctx context.Context, remoteSessionID, configID string) (middleware.TaskPushConfig, error) {
	if c.advertised == nil || !c.advertised.PushNotifications {
		return middleware.TaskPushConfig{}, fmt.Errorf("a2a agent does not advertise push notifications")
	}
	taskID := a2astate.TaskID(remoteSessionID)
	if taskID == "" || configID == "" {
		return middleware.TaskPushConfig{}, fmt.Errorf("A2A task id and push config id are required")
	}
	config, err := c.client.GetTaskPushConfig(ctx, &a2a.GetTaskPushConfigRequest{TaskID: a2a.TaskID(taskID), ID: configID})
	if err != nil {
		return middleware.TaskPushConfig{}, err
	}
	return fromA2APushConfig(config), nil
}

func (c *a2aConversationClient) ListTaskPushConfigs(ctx context.Context, remoteSessionID string) ([]middleware.TaskPushConfig, error) {
	if c.advertised == nil || !c.advertised.PushNotifications {
		return nil, fmt.Errorf("a2a agent does not advertise push notifications")
	}
	taskID := a2astate.TaskID(remoteSessionID)
	if taskID == "" {
		return nil, fmt.Errorf("A2A task id is required")
	}
	configs, err := c.client.ListTaskPushConfigs(ctx, &a2a.ListTaskPushConfigRequest{TaskID: a2a.TaskID(taskID)})
	if err != nil {
		return nil, err
	}
	out := make([]middleware.TaskPushConfig, 0, len(configs))
	for _, config := range configs {
		out = append(out, fromA2APushConfig(config))
	}
	return out, nil
}

func (c *a2aConversationClient) DeleteTaskPushConfig(ctx context.Context, remoteSessionID, configID string) error {
	if c.advertised == nil || !c.advertised.PushNotifications {
		return fmt.Errorf("a2a agent does not advertise push notifications")
	}
	taskID := a2astate.TaskID(remoteSessionID)
	if taskID == "" || configID == "" {
		return fmt.Errorf("A2A task id and push config id are required")
	}
	return c.client.DeleteTaskPushConfig(ctx, &a2a.DeleteTaskPushConfigRequest{TaskID: a2a.TaskID(taskID), ID: configID})
}

func toA2APushConfig(config middleware.TaskPushConfig) (*a2a.PushConfig, error) {
	taskID := a2astate.TaskID(config.RemoteSessionID)
	if taskID == "" || config.URL == "" {
		return nil, fmt.Errorf("A2A task id and callback URL are required")
	}
	out := &a2a.PushConfig{TaskID: a2a.TaskID(taskID), ID: config.ID, URL: config.URL, Token: config.Token}
	if config.Auth != nil {
		out.Auth = &a2a.PushAuthInfo{Scheme: config.Auth.Scheme, Credentials: config.Auth.Credentials}
	}
	return out, nil
}

func fromA2APushConfig(config *a2a.PushConfig) middleware.TaskPushConfig {
	if config == nil {
		return middleware.TaskPushConfig{}
	}
	out := middleware.TaskPushConfig{
		RemoteSessionID: a2astate.Encode(a2astate.State{TaskID: string(config.TaskID)}),
		ID:              config.ID,
		URL:             config.URL,
		Token:           config.Token,
	}
	if config.Auth != nil {
		out.Auth = &middleware.TaskPushAuth{Scheme: config.Auth.Scheme, Credentials: config.Auth.Credentials}
	}
	return out
}

func (c *a2aConversationClient) GetExtendedProfile(ctx context.Context) (middleware.ProtocolDocument, error) {
	if c.advertised == nil || !c.advertised.ExtendedAgentCard {
		return middleware.ProtocolDocument{}, fmt.Errorf("a2a agent does not advertise an extended agent card")
	}
	card, err := c.client.GetExtendedAgentCard(ctx, &a2a.GetExtendedAgentCardRequest{})
	if err != nil {
		return middleware.ProtocolDocument{}, err
	}
	data, err := json.Marshal(card)
	if err != nil {
		return middleware.ProtocolDocument{}, fmt.Errorf("encode A2A extended agent card: %w", err)
	}
	return middleware.ProtocolDocument{MediaType: "application/a2a-agent-card+json", Data: data}, nil
}

var (
	_ middleware.ConversationTaskSubscriber        = (*a2aConversationClient)(nil)
	_ middleware.ConversationTaskPushControl       = (*a2aConversationClient)(nil)
	_ middleware.ConversationExtendedProfileReader = (*a2aConversationClient)(nil)
)
