package session

import (
	"context"
	"fmt"
	"strings"

	"github.com/Josepavese/matrix/internal/logic/system_tools"
	"github.com/Josepavese/matrix/internal/middleware"
)

func (m *Manager) handleSessionCommand(ctx context.Context, channelID, input string) (string, error) {
	lang := m.wizard.GetLanguage(channelID)
	parts := strings.Fields(input)
	if len(parts) < 2 {
		return m.wizard.GetString(lang, "usage_session"), nil
	}

	command := parts[1]
	args := strings.TrimSpace(strings.TrimPrefix(input, parts[0]+" "+parts[1]))
	var (
		result middleware.SessionActionResult
		err    error
	)
	switch command {
	case "status":
		result, err = m.handleSessionStatusTyped(channelID, lang, "")
	case "name":
		result, err = m.handleSessionNameTyped(channelID, lang, args)
	case "new":
		result, err = m.handleSessionNewTyped(newSessionRequest{ChannelID: channelID, Lang: lang, AgentID: agentFromSessionParts(parts, m.defaultAgent)})
	case "list":
		result, err = m.handleSessionListTyped(ctx, channelID, lang, "")
	case "switch":
		result, err = m.handleSessionSwitchTyped(ctx, channelID, lang, args)
	case "delete":
		result, err = m.handleSessionDeleteTyped(ctx, sessionCleanupRequest{ChannelID: channelID, Lang: lang, Target: args})
	case "cancel":
		result, err = m.handleSessionCancelTyped(ctx, channelID, lang, args)
	default:
		return m.wizard.GetString(lang, "session_command_unknown"), nil
	}
	if err != nil {
		return "", err
	}
	return m.renderSessionAction(result, lang), nil
}

func agentFromSessionParts(parts []string, fallback string) string {
	if len(parts) >= 3 {
		return parts[2]
	}
	return fallback
}

func (m *Manager) handleActionCommand(ctx context.Context, channelID string, input string) (string, error) {
	lang := m.wizard.GetLanguage(channelID)
	instruction := strings.TrimSpace(strings.TrimPrefix(input, "/action"))
	if instruction == "" {
		return m.wizard.GetString(lang, "action_usage"), nil
	}

	tools := system_tools.GetSystemTools()
	prompt := fmt.Sprintf("SYSTEM: You are the autonomous Matrix OS Meta-Agent. Your objective is to manage the system using the tools provided. Evaluate the user request and call the appropriate tools. If no tool matches, provide a helpful explanation.\n\nUSER REQUEST: %s", instruction)
	responseTxt, _, toolCalls, _, err := m.router.Route(ctx, middleware.RouteRequest{
		AgentID:          m.actionAgent,
		LogicalSessionID: "MatrixOS",
		AgentSessionID:   "system_action_session",
		Message:          prompt,
		Tools:            tools,
	})
	if err != nil {
		return "", fmt.Errorf(m.wizard.GetString(lang, "action_meta_failed"), err)
	}
	return m.renderActionResult(lang, responseTxt, toolCalls), nil
}

func (m *Manager) renderActionResult(lang, response string, toolCalls []middleware.ToolCall) string {
	if len(toolCalls) == 0 {
		return response
	}
	result := response + "\n\n" + m.wizard.GetString(lang, "action_executing_header") + "\n"
	for _, toolCall := range toolCalls {
		if m.systemTools != nil {
			result += fmt.Sprintf("- %s: %s\n", toolCall.Function.Name, m.systemTools.ExecuteTool(toolCall))
		}
	}
	return result
}

func (m *Manager) handleHelpCommand(channelID string) (string, error) {
	lang := m.wizard.GetLanguage(channelID)
	return m.wizard.GetString(lang, "help_text"), nil
}

func (m *Manager) handleWizardCommand(channelID string) (string, error) {
	lang := m.wizard.GetLanguage(channelID)
	prefix := m.wizard.GetString(lang, "wizard_start")
	prompt, err := m.wizard.ForceStart(channelID)
	if err != nil {
		return "", err
	}
	return prefix + "\n\n" + prompt, nil
}
