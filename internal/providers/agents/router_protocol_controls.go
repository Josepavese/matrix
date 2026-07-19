package agents

import (
	"context"
	"fmt"

	"github.com/Josepavese/matrix/internal/middleware"
)

func (r *Router) AgentAuthenticationMethods(ctx context.Context, agentID string) ([]middleware.AuthenticationMethod, error) {
	control, err := r.authenticationControl(ctx, agentID)
	if err != nil {
		return nil, err
	}
	return control.AuthenticationMethods(), nil
}

func (r *Router) AuthenticateAgent(ctx context.Context, agentID, methodID string) error {
	control, err := r.authenticationControl(ctx, agentID)
	if err != nil {
		return err
	}
	return control.Authenticate(ctx, methodID)
}

func (r *Router) LogoutAgent(ctx context.Context, agentID string) error {
	control, err := r.authenticationControl(ctx, agentID)
	if err != nil {
		return err
	}
	return control.Logout(ctx)
}

func (r *Router) authenticationControl(ctx context.Context, agentID string) (middleware.ConversationAuthenticationControl, error) {
	client, err := r.getOrCreateSessionControlClient(ctx, agentID)
	if err != nil {
		return nil, err
	}
	control, ok := client.(middleware.ConversationAuthenticationControl)
	if !ok {
		return nil, fmt.Errorf("agent %s does not expose protocol authentication control", agentID)
	}
	return control, nil
}

func (r *Router) SubscribeAgentTask(ctx context.Context, agentID, remoteSessionID string, observer middleware.RemoteTaskObserver) error {
	client, err := r.getOrCreateSessionControlClient(ctx, agentID)
	if err != nil {
		return err
	}
	subscriber, ok := client.(middleware.ConversationTaskSubscriber)
	if !ok {
		return fmt.Errorf("agent %s does not expose task subscription", agentID)
	}
	return subscriber.SubscribeRemoteTask(ctx, remoteSessionID, observer)
}

func (r *Router) CreateAgentTaskPushConfig(ctx context.Context, agentID string, config middleware.TaskPushConfig) (middleware.TaskPushConfig, error) {
	control, err := r.taskPushControl(ctx, agentID)
	if err != nil {
		return middleware.TaskPushConfig{}, err
	}
	return control.CreateTaskPushConfig(ctx, config)
}

func (r *Router) GetAgentTaskPushConfig(ctx context.Context, agentID, remoteSessionID, configID string) (middleware.TaskPushConfig, error) {
	control, err := r.taskPushControl(ctx, agentID)
	if err != nil {
		return middleware.TaskPushConfig{}, err
	}
	return control.GetTaskPushConfig(ctx, remoteSessionID, configID)
}

func (r *Router) ListAgentTaskPushConfigs(ctx context.Context, agentID, remoteSessionID string) ([]middleware.TaskPushConfig, error) {
	control, err := r.taskPushControl(ctx, agentID)
	if err != nil {
		return nil, err
	}
	return control.ListTaskPushConfigs(ctx, remoteSessionID)
}

func (r *Router) DeleteAgentTaskPushConfig(ctx context.Context, agentID, remoteSessionID, configID string) error {
	control, err := r.taskPushControl(ctx, agentID)
	if err != nil {
		return err
	}
	return control.DeleteTaskPushConfig(ctx, remoteSessionID, configID)
}

func (r *Router) taskPushControl(ctx context.Context, agentID string) (middleware.ConversationTaskPushControl, error) {
	client, err := r.getOrCreateSessionControlClient(ctx, agentID)
	if err != nil {
		return nil, err
	}
	control, ok := client.(middleware.ConversationTaskPushControl)
	if !ok {
		return nil, fmt.Errorf("agent %s does not expose task push configuration", agentID)
	}
	return control, nil
}

func (r *Router) GetAgentExtendedProfile(ctx context.Context, agentID string) (middleware.ProtocolDocument, error) {
	client, err := r.getOrCreateSessionControlClient(ctx, agentID)
	if err != nil {
		return middleware.ProtocolDocument{}, err
	}
	reader, ok := client.(middleware.ConversationExtendedProfileReader)
	if !ok {
		return middleware.ProtocolDocument{}, fmt.Errorf("agent %s does not expose an extended profile", agentID)
	}
	return reader.GetExtendedProfile(ctx)
}

var (
	_ middleware.AgentAuthenticationController = (*Router)(nil)
	_ middleware.AgentTaskSubscriber           = (*Router)(nil)
	_ middleware.AgentTaskPushController       = (*Router)(nil)
	_ middleware.AgentExtendedProfileReader    = (*Router)(nil)
)
