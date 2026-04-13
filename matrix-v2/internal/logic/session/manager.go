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
	"github.com/jose/matrix-v2/internal/middleware"
)

// Session metadata stored in the SSOT Vault
type SessionMeta struct {
	ID             string    `json:"id"`
	AgentSessionID string    `json:"agent_session_id"`
	CreatedAt      time.Time `json:"created_at"`
	AgentID        string    `json:"agent_id"`
	Status         string    `json:"status"`
	Alias          string    `json:"alias,omitempty"`
}

// ChannelState tracks the active session and the history constraint to a channel.
type ChannelState struct {
	ActiveSessionID string   `json:"active"`
	History         []string `json:"history"`
}

// Manager handles routing between physical channels (e.g. telegram_123456789)
// and logical SessionIDs using the SSOT Vault.
type Manager struct {
	storage     middleware.Storage
	router      middleware.AgentRouter
	wizard      *onboarding.Wizard
	systemTools *system_tools.Handler

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

// getOrCreateQueue lazily creates an OrderedMerge per channelID.
// The onFlush callback persists the agent session mapping in order.
func (m *Manager) getOrCreateQueue(channelID string) *OrderedMerge {
	m.queuesMu.Lock()
	defer m.queuesMu.Unlock()

	if q, ok := m.queues[channelID]; ok {
		return q
	}

	q := NewOrderedMerge(func(seq int, result RouteResult) {
		if result.Err != nil || result.AgentSessionID == "" {
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
		m.persistAgentSession(meta, result.AgentSessionID, result.Err)
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

// GetOrCreateSession retrieves the active SessionID for a given channel,
// or creates a new one if it doesn't exist, assigning it to the targetAgent.
func (m *Manager) GetOrCreateSession(channelID, targetAgent string) (string, error) {
	state, err := m.getChannelState(channelID)
	if err == nil && state.ActiveSessionID != "" {
		return state.ActiveSessionID, nil
	}
	return m.forceNewSession(channelID, targetAgent)
}

// forceNewSession creates a new session and maps the channel to it.
func (m *Manager) forceNewSession(channelID, targetAgent string) (string, error) {
	sessionID := uuid.New().String()

	meta := SessionMeta{
		ID:        sessionID,
		CreatedAt: time.Now().UTC(),
		AgentID:   targetAgent,
		Status:    "active",
	}
	metaData, err := json.Marshal(meta)
	if err != nil {
		return "", fmt.Errorf("failed to marshal session meta: %w", err)
	}
	if err := m.storage.Set(getSessionKey(sessionID), metaData); err != nil {
		return "", fmt.Errorf("failed to store session meta: %w", err)
	}

	if err := m.updateChannelState(channelID, sessionID); err != nil {
		return "", fmt.Errorf("failed to store channel mapping: %w", err)
	}

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
func (m *Manager) handleSessionCommand(channelID, input string) (string, error) {
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
		return m.handleSessionList(channelID, lang)

	case "switch":
		return m.handleSessionSwitch(channelID, lang, args)

	default:
		return m.wizard.GetString(lang, "session_command_unknown"), nil
	}
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

	responseTxt, _, toolCalls, err := m.router.Route(ctx, middleware.RouteRequest{
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

func (m *Manager) HandleAuthCallback(channelID, provider, code string) (string, error) {
	return m.wizard.HandleAuthCallback(channelID, provider, code)
}
