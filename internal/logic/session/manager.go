package session

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/jose/matrix-v2/internal/logic/onboarding"
	"github.com/jose/matrix-v2/internal/logic/system_tools"
	"github.com/jose/matrix-v2/internal/logic/workspace"
	"github.com/jose/matrix-v2/internal/middleware"
)

// SessionMeta stores metadata for an active agent session in the SSOT vault.
type SessionMeta struct {
	ID               string                    `json:"id"`
	AgentSessionID   string                    `json:"agent_session_id"`
	CreatedAt        time.Time                 `json:"created_at"`
	AgentID          string                    `json:"agent_id"`
	Status           string                    `json:"status"`
	Alias            string                    `json:"alias,omitempty"`
	ProtocolKind     string                    `json:"protocol_kind,omitempty"`
	MirrorStatus     string                    `json:"mirror_status,omitempty"`
	RemoteTitle      string                    `json:"remote_title,omitempty"`
	RemoteStatus     string                    `json:"remote_status,omitempty"`
	RemoteMeta       map[string]interface{}    `json:"remote_meta,omitempty"`
	RemoteUpdatedAt  time.Time                 `json:"remote_updated_at,omitempty"`
	LastSyncedAt     time.Time                 `json:"last_synced_at,omitempty"`
	WorkspaceID      string                    `json:"workspace_id,omitempty"`
	WorkspacePath    string                    `json:"workspace_path,omitempty"`
	WorkspaceBranch  string                    `json:"workspace_branch,omitempty"`
	WorkspaceRole    string                    `json:"workspace_role,omitempty"`
	WorkspaceBoundAt time.Time                 `json:"workspace_bound_at,omitempty"`
	Mode             string                    `json:"mode,omitempty"`
	PendingHandoff   *middleware.HandoffPacket `json:"pending_handoff,omitempty"`
	LastHandoff      *middleware.HandoffPacket `json:"last_handoff,omitempty"`
}

// ChannelState tracks the active session and the history constraint to a channel.
type ChannelState struct {
	ActiveSessionID      string   `json:"active"`
	History              []string `json:"history"`
	PreferredWorkspaceID string   `json:"preferred_workspace_id,omitempty"`
	LastWorkspaceID      string   `json:"last_workspace_id,omitempty"`
}

// Manager handles routing between physical channels (e.g. telegram_123456789)
// and logical SessionIDs using the SSOT Vault.
type Manager struct {
	storage     middleware.Storage
	router      middleware.AgentRouter
	wizard      *onboarding.Wizard
	systemTools *system_tools.Handler
	resolver    middleware.AgentEndpointResolver

	// defaultAgent is the agent used for new sessions when none specified.
	defaultAgent string
	// actionAgent is the agent used for /action (meta-agent) routing.
	actionAgent string

	queues   map[string]*OrderedMerge
	queuesMu sync.Mutex
}

const defaultAgentID = "opencode"
const defaultActionAgentID = "gemini"

// NewManager creates a new Session Router logic instance.
// defaultAgent and actionAgent default to "opencode" and "gemini" respectively.
func NewManager(s middleware.Storage, router middleware.AgentRouter, w *onboarding.Wizard, st *system_tools.Handler) *Manager {
	return &Manager{
		storage:      s,
		router:       router,
		wizard:       w,
		systemTools:  st,
		defaultAgent: defaultAgentID,
		actionAgent:  defaultActionAgentID,
		queues:       make(map[string]*OrderedMerge),
	}
}

// SetDefaultAgent overrides the default agent for new sessions.
func (m *Manager) SetDefaultAgent(agentID string) {
	m.defaultAgent = agentID
}

// SetActionAgent overrides the agent used for /action routing.
func (m *Manager) SetActionAgent(agentID string) {
	m.actionAgent = agentID
}

// SetEndpointResolver configures the endpoint resolver used to mirror remote protocol metadata.
func (m *Manager) SetEndpointResolver(resolver middleware.AgentEndpointResolver) {
	m.resolver = resolver
}

// HandleMessage adapts the session manager to the neutral channel runtime.
func (m *Manager) HandleMessage(ctx context.Context, msg middleware.ChannelMessage) (middleware.ChannelResponse, error) {
	agentID := msg.DefaultAgentID
	if strings.TrimSpace(agentID) == "" {
		agentID = m.defaultAgent
	}
	output, err := m.RouteConversation(ctx, middleware.ConversationRequest{
		ChannelID:     msg.ChannelID,
		AgentID:       agentID,
		WorkspaceID:   msg.WorkspaceID,
		WorkspacePath: msg.WorkspacePath,
		Input:         msg.Input,
		Notifier:      msg.Notifier,
	})
	if err != nil {
		return middleware.ChannelResponse{}, err
	}
	return middleware.ChannelResponse{Output: output}, nil
}

// HandleSessionAction executes a typed session lifecycle action without requiring
// callers to synthesize slash-commands.
func (m *Manager) HandleSessionAction(ctx context.Context, channelID, action, target string) (string, error) {
	result, err := m.HandleSessionActionTyped(ctx, middleware.SessionActionRequest{
		ChannelID: channelID,
		Action:    action,
		Target:    target,
	})
	if err != nil {
		return "", err
	}
	return m.renderSessionAction(result, m.wizard.GetLanguage(channelID)), nil
}

// HandleWorkspaceAction executes a typed workspace action and renders it for chat-style channels.
func (m *Manager) HandleWorkspaceAction(ctx context.Context, channelID, action, target string) (string, error) {
	result, err := m.HandleWorkspaceActionTyped(ctx, middleware.WorkspaceActionRequest{
		ChannelID: channelID,
		Action:    action,
		Target:    target,
	})
	if err != nil {
		return "", err
	}
	return result.Message, nil
}

// HandleWorkspaceRead executes a typed workspace read and renders it for chat-style channels.
func (m *Manager) HandleWorkspaceRead(ctx context.Context, channelID, action, workspaceID string, limit int) (string, error) {
	result, err := m.HandleWorkspaceReadTyped(ctx, middleware.WorkspaceReadRequest{
		ChannelID:   channelID,
		Action:      action,
		WorkspaceID: workspaceID,
		Limit:       limit,
	})
	if err != nil {
		return "", err
	}
	return result.Message, nil
}

// HandleIntent executes a typed high-level operator intent and renders it for chat-style channels.
func (m *Manager) HandleIntent(ctx context.Context, channelID, intent, target string) (string, error) {
	result, err := m.HandleIntentTyped(ctx, middleware.IntentActionRequest{
		ChannelID: channelID,
		Intent:    intent,
		Target:    target,
	})
	if err != nil {
		return "", err
	}
	return result.Message, nil
}

// HandleSessionActionTyped executes a typed session lifecycle action using the shared core semantics.
func (m *Manager) HandleSessionActionTyped(ctx context.Context, req middleware.SessionActionRequest) (middleware.SessionActionResult, error) {
	lang := m.wizard.GetLanguage(req.ChannelID)
	switch strings.ToLower(strings.TrimSpace(req.Action)) {
	case "cancel":
		return m.handleSessionCancelTyped(ctx, req.ChannelID, lang, req.Target)
	case "delete":
		return m.handleSessionDeleteTyped(ctx, req.ChannelID, lang, req.Target)
	case "switch":
		return m.handleSessionSwitchTyped(ctx, req.ChannelID, lang, req.Target)
	case "list":
		return m.handleSessionListTyped(ctx, req.ChannelID, lang, req.WorkspaceID)
	case "status":
		return m.handleSessionStatusTyped(req.ChannelID, lang, req.WorkspaceID)
	case "new":
		return m.handleSessionNewTyped(req.ChannelID, lang, req.Target, req.WorkspaceID, "")
	case "name":
		return m.handleSessionNameTyped(req.ChannelID, lang, req.Target)
	default:
		return middleware.SessionActionResult{}, fmt.Errorf("unsupported session action: %s", req.Action)
	}
}

// HandleWorkspaceActionTyped executes a typed workspace control action using the shared core semantics.
func (m *Manager) HandleWorkspaceActionTyped(ctx context.Context, req middleware.WorkspaceActionRequest) (middleware.WorkspaceActionResult, error) {
	lang := m.wizard.GetLanguage(req.ChannelID)
	switch strings.ToLower(strings.TrimSpace(req.Action)) {
	case "list":
		return m.handleWorkspaceListTyped(req.ChannelID, lang)
	case "status":
		return m.handleWorkspaceStatusTyped(req.ChannelID, lang)
	case "snapshot":
		return m.handleWorkspaceSnapshotTyped(req.ChannelID, lang, req.Target)
	case "switch":
		return m.handleWorkspaceSwitchTyped(ctx, req.ChannelID, lang, req.Target)
	case "bind":
		return m.handleWorkspaceBindTyped(req.ChannelID, lang, req.Target)
	default:
		return middleware.WorkspaceActionResult{}, fmt.Errorf("unsupported workspace action: %s", req.Action)
	}
}

// HandleWorkspaceReadTyped executes a typed workspace read using the shared core semantics.
func (m *Manager) HandleWorkspaceReadTyped(ctx context.Context, req middleware.WorkspaceReadRequest) (middleware.WorkspaceReadResult, error) {
	lang := m.wizard.GetLanguage(req.ChannelID)
	switch strings.ToLower(strings.TrimSpace(req.Action)) {
	case "state":
		return m.handleWorkspaceStateReadTyped(ctx, req.ChannelID, lang, req.WorkspaceID)
	case "timeline":
		return m.handleWorkspaceTimelineReadTyped(ctx, req.ChannelID, lang, req.WorkspaceID, req.Limit)
	case "decisions":
		return m.handleWorkspaceDecisionsReadTyped(ctx, req.ChannelID, lang, req.WorkspaceID, req.Limit)
	case "memory":
		return m.handleWorkspaceMemoryReadTyped(ctx, req.ChannelID, lang, req.WorkspaceID, req.Limit)
	case "snapshots":
		return m.handleWorkspaceSnapshotsReadTyped(ctx, req.ChannelID, lang, req.WorkspaceID, req.Limit)
	default:
		return middleware.WorkspaceReadResult{}, fmt.Errorf("unsupported workspace read action: %s", req.Action)
	}
}

// HandleIntentTyped executes a typed high-level operator intent using the shared core semantics.
func (m *Manager) HandleIntentTyped(ctx context.Context, req middleware.IntentActionRequest) (middleware.IntentActionResult, error) {
	lang := m.wizard.GetLanguage(req.ChannelID)
	switch strings.ToLower(strings.TrimSpace(req.Intent)) {
	case "continue":
		return m.handleContinueIntentTyped(req.ChannelID, lang)
	case "resume":
		return m.handleResumeIntentTyped(ctx, req.ChannelID, lang, req.Target)
	case "review":
		return m.handleReviewIntentTyped(ctx, req.ChannelID, lang, req.Target)
	case "explain":
		return m.handleExplainModeTyped(ctx, req.ChannelID, lang, req.Target)
	case "triage":
		return m.handleTriageModeTyped(ctx, req.ChannelID, lang, req.Target)
	case "handoff":
		return m.handleHandoffIntentTyped(ctx, req.ChannelID, lang, req.WorkspaceID, req.AgentID, req.Target, req.Note)
	default:
		return middleware.IntentActionResult{}, fmt.Errorf("unsupported intent: %s", req.Intent)
	}
}

// getOrCreateQueue lazily creates an OrderedMerge per channelID.
// The onFlush callback persists the agent session mapping in order.
func (m *Manager) getOrCreateQueue(channelID string) *OrderedMerge {
	m.queuesMu.Lock()
	defer m.queuesMu.Unlock()

	if q, ok := m.queues[channelID]; ok {
		return q
	}

	q := NewOrderedMerge(func(_ int, result RouteResult) {
		if result.Err != nil {
			return
		}
		state, err := m.getChannelState(channelID)
		if err != nil {
			return
		}
		meta, found, err := m.loadSessionMeta(state.ActiveSessionID)
		if err != nil || !found {
			return
		}
		m.persistAgentSession(meta, result.AgentSessionID, result.Metadata, result.Err)
	})
	m.queues[channelID] = q
	return q
}

// getSessionKey returns the vault key for a SessionID metadata.
func getSessionKey(sessionID string) string {
	return "session.meta." + sessionID
}

// getChannelKey returns the vault key for a ChannelID mapping.
func getChannelKey(channelID string) string {
	return "session.channel." + channelID
}

// getChannelState returns the current ChannelState.
func (m *Manager) getChannelState(channelID string) (ChannelState, error) {
	channelKey := getChannelKey(channelID)
	data, err := m.storage.Get(channelKey)
	var state ChannelState
	if err != nil || len(data) == 0 {
		return state, err
	}
	return state, json.Unmarshal(data, &state)
}

func historyPush(history []string, newID string) []string {
	filtered := make([]string, 0, len(history))
	for _, id := range history {
		if id != newID {
			filtered = append(filtered, id)
		}
	}
	res := append([]string{newID}, filtered...)
	if len(res) > 10 {
		res = res[:10]
	}
	return res
}

// updateChannelState updates the channel's mapped active session and pushes it to history.
func (m *Manager) updateChannelState(channelID, activeSessionID string) error {
	state, err := m.getChannelState(channelID)
	if err != nil {
		return err
	}
	state.ActiveSessionID = activeSessionID
	state.History = historyPush(state.History, activeSessionID)
	newStateData, err := json.Marshal(state)
	if err != nil {
		return fmt.Errorf("failed to marshal channel state: %w", err)
	}
	return m.storage.Set(getChannelKey(channelID), newStateData)
}

func (m *Manager) updateChannelWorkspaceState(channelID, workspaceID string) error {
	if strings.TrimSpace(workspaceID) == "" {
		return nil
	}
	state, err := m.getChannelState(channelID)
	if err != nil {
		return err
	}
	state.PreferredWorkspaceID = workspaceID
	state.LastWorkspaceID = workspaceID
	newStateData, err := json.Marshal(state)
	if err != nil {
		return fmt.Errorf("failed to marshal channel state: %w", err)
	}
	if err := m.storage.Set(getChannelKey(channelID), newStateData); err != nil {
		return err
	}
	return workspace.UpdateChannelIndex(m.storage, workspaceID, channelID)
}

// GetOrCreateSession retrieves the active SessionID for a given channel,
// or creates a new one if it doesn't exist, assigning it to the targetAgent.
func (m *Manager) GetOrCreateSession(channelID, targetAgent string) (string, error) {
	state, err := m.getChannelState(channelID)
	if err == nil && state.ActiveSessionID != "" {
		return state.ActiveSessionID, nil
	}
	return m.forceNewSessionWithWorkspace(channelID, targetAgent, "", "")
}

func (m *Manager) forceNewSessionWithWorkspace(channelID, targetAgent, workspaceID, workspacePath string) (string, error) {
	sessionID := uuid.New().String()

	meta := SessionMeta{
		ID:           sessionID,
		CreatedAt:    time.Now().UTC(),
		AgentID:      targetAgent,
		Status:       "active",
		MirrorStatus: "pending",
	}
	if err := m.bindSessionWorkspace(&meta, workspaceID, workspacePath); err != nil {
		return "", fmt.Errorf("failed to bind workspace: %w", err)
	}
	if err := m.saveSessionMeta(meta); err != nil {
		return "", fmt.Errorf("failed to store session meta: %w", err)
	}

	if err := m.updateChannelState(channelID, sessionID); err != nil {
		return "", fmt.Errorf("failed to store channel mapping: %w", err)
	}
	if err := m.updateChannelWorkspaceState(channelID, meta.WorkspaceID); err != nil {
		return "", fmt.Errorf("failed to store channel workspace mapping: %w", err)
	}
	if err := m.indexSessionWorkspace(meta); err != nil {
		return "", fmt.Errorf("failed to index session workspace: %w", err)
	}
	m.recordWorkspaceEvent(meta, "session.created", channelID, "Created workspace session", "session-create", nil)

	return sessionID, nil
}

// AttachChannel forcefully maps an existing channel to a specific session ID.
func (m *Manager) AttachChannel(channelID, sessionID string) error {
	data, err := m.storage.Get(getSessionKey(sessionID))
	if err != nil {
		return fmt.Errorf("storage error reading session: %w", err)
	}
	if len(data) == 0 {
		return fmt.Errorf("session %s not found", sessionID)
	}

	if err := m.updateChannelState(channelID, sessionID); err != nil {
		return fmt.Errorf("failed to store channel mapping: %w", err)
	}
	return nil
}

// handleSessionCommand parses and executes chat-based session lifecycle commands.
func (m *Manager) handleSessionCommand(ctx context.Context, channelID, input string) (string, error) {
	lang := m.wizard.GetLanguage(channelID)

	parts := strings.Fields(input)
	if len(parts) < 2 {
		return m.wizard.GetString(lang, "usage_session"), nil
	}

	command := parts[1]
	args := strings.TrimSpace(strings.TrimPrefix(input, parts[0]+" "+parts[1]))

	switch command {
	case "status":
		return m.handleSessionStatus(channelID, lang)

	case "name":
		return m.handleSessionName(channelID, lang, args)

	case "new":
		return m.handleSessionNew(channelID, lang, parts)

	case "list":
		return m.handleSessionList(ctx, channelID, lang)

	case "switch":
		return m.handleSessionSwitch(ctx, channelID, lang, args)

	case "delete":
		return m.handleSessionDelete(ctx, channelID, lang, args)

	case "cancel":
		return m.handleSessionCancel(ctx, channelID, lang, args)

	default:
		return m.wizard.GetString(lang, "session_command_unknown"), nil
	}
}

// InspectSession returns the stored session metadata for operator surfaces such as the CLI.
func (m *Manager) InspectSession(sessionID string) (SessionMeta, bool, error) {
	return m.loadSessionMeta(sessionID)
}

// handleActionCommand delegates a system-level administrative command to the Matrix Meta-Agent
// attaching the OS tools so it can mutate configuration or perform APM installs.
func (m *Manager) handleActionCommand(ctx context.Context, channelID string, input string) (string, error) {
	lang := m.wizard.GetLanguage(channelID)
	instruction := strings.TrimSpace(strings.TrimPrefix(input, "/action"))
	if instruction == "" {
		return m.wizard.GetString(lang, "action_usage"), nil
	}

	systemActionSessionID := "system_action_session"

	tools := system_tools.GetSystemTools()

	prompt := fmt.Sprintf("SYSTEM: You are the autonomous Matrix OS Meta-Agent. Your objective is to manage the system using the tools provided. Evaluate the user request and call the appropriate tools. If no tool matches, provide a helpful explanation.\n\nUSER REQUEST: %s", instruction)

	responseTxt, _, toolCalls, _, err := m.router.Route(ctx, middleware.RouteRequest{
		AgentID:          m.actionAgent,
		LogicalSessionID: "MatrixOS",
		AgentSessionID:   systemActionSessionID,
		Message:          prompt,
		Tools:            tools,
	})
	if err != nil {
		return "", fmt.Errorf(m.wizard.GetString(lang, "action_meta_failed"), err)
	}

	resultMsg := responseTxt
	if len(toolCalls) > 0 {
		resultMsg += "\n\n" + m.wizard.GetString(lang, "action_executing_header") + "\n"
		for _, tc := range toolCalls {
			if m.systemTools != nil {
				execRes := m.systemTools.ExecuteTool(tc)
				resultMsg += fmt.Sprintf("- %s: %s\n", tc.Function.Name, execRes)
			}
		}
	}

	return resultMsg, nil
}

// handleHelpCommand returns a localized help message.
func (m *Manager) handleHelpCommand(channelID string) (string, error) {
	lang := m.wizard.GetLanguage(channelID)
	return m.wizard.GetString(lang, "help_text"), nil
}

// handleWizardCommand restarts the onboarding flow for the current channel.
func (m *Manager) handleWizardCommand(channelID string) (string, error) {
	lang := m.wizard.GetLanguage(channelID)
	prefix := m.wizard.GetString(lang, "wizard_start")
	prompt, err := m.wizard.ForceStart(channelID)
	if err != nil {
		return "", err
	}
	return prefix + "\n\n" + prompt, nil
}

// HandleAuthCallback processes an OAuth callback from an external auth provider.
func (m *Manager) HandleAuthCallback(channelID, provider, code string) (string, error) {
	return m.wizard.HandleAuthCallback(channelID, provider, code)
}
