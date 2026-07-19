package middleware

import (
	"context"
	"encoding/json"
)

// RemoteTaskEvent is the protocol-neutral projection of an asynchronous task
// event. A2A resubscribe maps here without leaking A2A SDK types into callers.
type RemoteTaskEvent struct {
	RemoteSessionID string                 `json:"remote_session_id,omitempty"`
	Kind            string                 `json:"kind,omitempty"`
	State           string                 `json:"state,omitempty"`
	Final           bool                   `json:"final,omitempty"`
	ContentBlocks   []Content              `json:"content_blocks,omitempty"`
	Metadata        map[string]interface{} `json:"metadata,omitempty"`
}

type RemoteTaskObserver interface {
	OnRemoteTaskEvent(event RemoteTaskEvent)
}

type ConversationTaskSubscriber interface {
	SubscribeRemoteTask(ctx context.Context, remoteSessionID string, observer RemoteTaskObserver) error
}

type TaskPushAuth struct {
	Scheme      string `json:"scheme,omitempty"`
	Credentials string `json:"credentials,omitempty"`
}

type TaskPushConfig struct {
	RemoteSessionID string        `json:"remote_session_id,omitempty"`
	ID              string        `json:"id,omitempty"`
	URL             string        `json:"url"`
	Token           string        `json:"token,omitempty"`
	Auth            *TaskPushAuth `json:"authentication,omitempty"`
}

type ConversationTaskPushControl interface {
	CreateTaskPushConfig(ctx context.Context, config TaskPushConfig) (TaskPushConfig, error)
	GetTaskPushConfig(ctx context.Context, remoteSessionID, configID string) (TaskPushConfig, error)
	ListTaskPushConfigs(ctx context.Context, remoteSessionID string) ([]TaskPushConfig, error)
	DeleteTaskPushConfig(ctx context.Context, remoteSessionID, configID string) error
}

type ProtocolDocument struct {
	MediaType string          `json:"media_type"`
	Data      json.RawMessage `json:"data"`
}

type ConversationExtendedProfileReader interface {
	GetExtendedProfile(ctx context.Context) (ProtocolDocument, error)
}

type AuthenticationMethod struct {
	ID          string                 `json:"id"`
	Type        string                 `json:"type,omitempty"`
	Name        string                 `json:"name"`
	Description string                 `json:"description,omitempty"`
	Metadata    map[string]interface{} `json:"metadata,omitempty"`
}

type ConversationAuthenticationControl interface {
	AuthenticationMethods() []AuthenticationMethod
	Authenticate(ctx context.Context, methodID string) error
	Logout(ctx context.Context) error
}

type AgentAuthenticationController interface {
	AgentAuthenticationMethods(ctx context.Context, agentID string) ([]AuthenticationMethod, error)
	AuthenticateAgent(ctx context.Context, agentID, methodID string) error
	LogoutAgent(ctx context.Context, agentID string) error
}

type AgentTaskSubscriber interface {
	SubscribeAgentTask(ctx context.Context, agentID, remoteSessionID string, observer RemoteTaskObserver) error
}

type AgentTaskPushController interface {
	CreateAgentTaskPushConfig(ctx context.Context, agentID string, config TaskPushConfig) (TaskPushConfig, error)
	GetAgentTaskPushConfig(ctx context.Context, agentID, remoteSessionID, configID string) (TaskPushConfig, error)
	ListAgentTaskPushConfigs(ctx context.Context, agentID, remoteSessionID string) ([]TaskPushConfig, error)
	DeleteAgentTaskPushConfig(ctx context.Context, agentID, remoteSessionID, configID string) error
}

type AgentExtendedProfileReader interface {
	GetAgentExtendedProfile(ctx context.Context, agentID string) (ProtocolDocument, error)
}
