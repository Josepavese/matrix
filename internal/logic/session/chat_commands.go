package session

import (
	"context"
	"fmt"
	"strings"

	"github.com/Josepavese/matrix/internal/logic/sessioncmd"
	"github.com/Josepavese/matrix/internal/logic/system_tools"
	"github.com/Josepavese/matrix/internal/middleware"
)

type sessionCommandRequest struct {
	ctx       context.Context
	channelID string
	lang      string
	command   string
	args      string
	parts     []string
}

func (m *Manager) handleSessionCommand(ctx context.Context, channelID, input string) (string, error) {
	lang := m.wizard.GetLanguage(channelID)
	parts := strings.Fields(input)
	if len(parts) < 2 {
		return m.wizard.GetString(lang, "usage_session"), nil
	}

	command := strings.ToLower(parts[1])
	args := strings.TrimSpace(strings.TrimPrefix(input, parts[0]+" "+parts[1]))
	req := sessionCommandRequest{
		ctx:       ctx,
		channelID: channelID,
		lang:      lang,
		command:   command,
		args:      args,
		parts:     parts,
	}
	result, handled, err := m.handleCoreSessionCommand(req)
	if !handled && err == nil {
		result, handled, err = m.handleProviderSessionCommand(req)
	}
	if !handled {
		return m.wizard.GetString(lang, "session_command_unknown"), nil
	}
	if err != nil {
		return "", err
	}
	return m.renderSessionAction(result, lang), nil
}

func (m *Manager) handleCoreSessionCommand(req sessionCommandRequest) (middleware.SessionActionResult, bool, error) {
	switch req.command {
	case "status":
		result, err := m.handleSessionStatusTyped(req.channelID, req.lang, "")
		return result, true, err
	case "name":
		result, err := m.handleSessionNameTyped(req.channelID, req.lang, req.args)
		return result, true, err
	case "new":
		result, err := m.handleSessionNewTyped(newSessionRequest{ChannelID: req.channelID, Lang: req.lang, AgentID: agentFromSessionParts(req.parts, m.defaultAgent)})
		return result, true, err
	case "list":
		result, err := m.handleSessionListTyped(req.ctx, middleware.SessionActionRequest{ChannelID: req.channelID, Action: "list"}, req.lang)
		return result, true, err
	case "switch":
		result, err := m.handleSessionSwitchTyped(req.ctx, req.channelID, req.lang, req.args)
		return result, true, err
	case "delete":
		result, err := m.handleSessionDeleteTyped(req.ctx, sessionCleanupRequest{ChannelID: req.channelID, Lang: req.lang, Target: req.args})
		return result, true, err
	case "cleanup":
		result, err := m.handleSessionCleanupTyped(req.ctx, sessionCleanupRequest{ChannelID: req.channelID, Lang: req.lang, Target: req.args})
		return result, true, err
	case "cancel":
		result, err := m.handleSessionCancelTyped(req.ctx, req.channelID, req.lang, req.args)
		return result, true, err
	default:
		return middleware.SessionActionResult{}, false, nil
	}
}

func (m *Manager) handleProviderSessionCommand(req sessionCommandRequest) (middleware.SessionActionResult, bool, error) {
	switch req.command {
	case "capabilities", "capability", "providers":
		result, err := m.handleSessionCapabilitiesTyped(req.ctx, middleware.SessionActionRequest{
			ChannelID: req.channelID,
			Action:    "capabilities",
			Target:    req.args,
		})
		return result, true, err
	case "reconcile":
		result, err := m.handleSessionReconcileTyped(req.ctx, middleware.SessionActionRequest{
			ChannelID: req.channelID,
			Action:    "reconcile",
		})
		return result, true, err
	case "fork":
		result, err := m.handleSessionForkTyped(req.ctx, sessionForkRequest(req.channelID, req.args))
		return result, true, err
	case "fork-async", "fork_async":
		forkReq := sessionForkRequest(req.channelID, req.args)
		forkReq.Async = true
		result, err := m.handleSessionForkTyped(req.ctx, forkReq)
		return result, true, err
	case "fork-status", "fork_status", "forkstatus":
		result, err := m.handleSessionForkStatusTyped(middleware.SessionActionRequest{
			ChannelID: req.channelID,
			Action:    "fork_status",
			Target:    req.args,
		})
		return result, true, err
	default:
		return middleware.SessionActionResult{}, false, nil
	}
}

func sessionForkRequest(channelID, args string) middleware.SessionActionRequest {
	fork := sessioncmd.ParseFork(args)
	return middleware.SessionActionRequest{
		ChannelID:     channelID,
		Action:        "fork",
		Target:        fork.Target,
		Ephemeral:     fork.Ephemeral,
		CleanupPolicy: fork.CleanupPolicy,
		MakeActive:    fork.MakeActive,
		RestoreParent: fork.RestoreParent,
		Async:         fork.Async,
		Input:         fork.Input,
	}
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
