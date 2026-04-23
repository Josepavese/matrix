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
