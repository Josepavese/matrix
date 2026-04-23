package session

import (
	"context"

	"github.com/Josepavese/matrix/internal/logic/sessioncmd"
)

type commandHandler func(context.Context, string, sessioncmd.Invocation) (string, error)

type commandInterpreter struct {
	handlers map[string]commandHandler
}

func newCommandInterpreter(manager *Manager) *commandInterpreter {
	interpreter := &commandInterpreter{handlers: map[string]commandHandler{}}
	interpreter.registerWorkspaceCommands(manager)
	interpreter.registerModeCommands(manager)
	interpreter.registerSessionCommands(manager)
	interpreter.registerSystemCommands(manager)
	return interpreter
}

func (c *commandInterpreter) registerWorkspaceCommands(manager *Manager) {
	c.handlers["/workspaces"] = func(ctx context.Context, channelID string, _ sessioncmd.Invocation) (string, error) {
		return manager.HandleWorkspaceAction(ctx, channelID, "list", "")
	}
	c.handlers["/now"] = func(ctx context.Context, channelID string, _ sessioncmd.Invocation) (string, error) {
		return manager.HandleWorkspaceRead(ctx, channelID, "state", "", 0)
	}
	c.handlers["/timeline"] = func(ctx context.Context, channelID string, invocation sessioncmd.Invocation) (string, error) {
		return manager.HandleWorkspaceRead(ctx, channelID, "timeline", invocation.Args, 10)
	}
	c.handlers["/decisions"] = func(ctx context.Context, channelID string, invocation sessioncmd.Invocation) (string, error) {
		return manager.HandleWorkspaceRead(ctx, channelID, "decisions", invocation.Args, 10)
	}
	c.handlers["/why"] = func(ctx context.Context, channelID string, _ sessioncmd.Invocation) (string, error) {
		return manager.HandleWorkspaceRead(ctx, channelID, "decisions", "", 1)
	}
	c.handlers["/memory"] = func(ctx context.Context, channelID string, _ sessioncmd.Invocation) (string, error) {
		return manager.HandleWorkspaceRead(ctx, channelID, "memory", "", 12)
	}
	c.handlers["/snapshots"] = func(ctx context.Context, channelID string, _ sessioncmd.Invocation) (string, error) {
		return manager.HandleWorkspaceRead(ctx, channelID, "snapshots", "", 10)
	}
	c.handlers["/snapshot"] = func(ctx context.Context, channelID string, invocation sessioncmd.Invocation) (string, error) {
		return manager.HandleWorkspaceAction(ctx, channelID, "snapshot", invocation.Args)
	}
	c.handlers["/workspace"] = func(ctx context.Context, channelID string, invocation sessioncmd.Invocation) (string, error) {
		return manager.handleWorkspaceCommand(ctx, channelID, invocation.Input)
	}
	c.handlers["/use"] = func(ctx context.Context, channelID string, invocation sessioncmd.Invocation) (string, error) {
		return manager.HandleWorkspaceAction(ctx, channelID, "switch", invocation.Args)
	}
}

func (c *commandInterpreter) registerModeCommands(manager *Manager) {
	c.handlers["/review"] = func(ctx context.Context, channelID string, invocation sessioncmd.Invocation) (string, error) {
		return manager.handleModeAction(ctx, channelID, modeReview, invocation.Args)
	}
	c.handlers["/continue"] = func(ctx context.Context, channelID string, _ sessioncmd.Invocation) (string, error) {
		return manager.HandleIntent(ctx, channelID, "continue", "")
	}
	c.handlers["/resume"] = func(ctx context.Context, channelID string, invocation sessioncmd.Invocation) (string, error) {
		return manager.HandleIntent(ctx, channelID, "resume", invocation.Args)
	}
	c.handlers["/explain"] = func(ctx context.Context, channelID string, invocation sessioncmd.Invocation) (string, error) {
		return manager.handleModeAction(ctx, channelID, modeExplain, invocation.Args)
	}
	c.handlers["/triage"] = func(ctx context.Context, channelID string, invocation sessioncmd.Invocation) (string, error) {
		return manager.handleModeAction(ctx, channelID, modeTriage, invocation.Args)
	}
	c.handlers["/handoff"] = func(ctx context.Context, channelID string, invocation sessioncmd.Invocation) (string, error) {
		return manager.HandleIntent(ctx, channelID, "handoff", invocation.Args)
	}
}

func (c *commandInterpreter) registerSessionCommands(manager *Manager) {
	cancel := func(ctx context.Context, channelID string, invocation sessioncmd.Invocation) (string, error) {
		return manager.handleSessionCancel(ctx, channelID, manager.wizard.GetLanguage(channelID), invocation.Args)
	}
	c.handlers["/status"] = func(_ context.Context, channelID string, _ sessioncmd.Invocation) (string, error) {
		return manager.handleStatusCommand(channelID)
	}
	c.handlers["/session"] = func(ctx context.Context, channelID string, invocation sessioncmd.Invocation) (string, error) {
		return manager.handleSessionCommand(ctx, channelID, invocation.Input)
	}
	c.handlers["/cancel"] = cancel
	c.handlers["/stop"] = cancel
}

func (c *commandInterpreter) registerSystemCommands(manager *Manager) {
	c.handlers["/action"] = func(ctx context.Context, channelID string, invocation sessioncmd.Invocation) (string, error) {
		return manager.handleActionCommand(ctx, channelID, invocation.Input)
	}
	c.handlers["/help"] = func(_ context.Context, channelID string, _ sessioncmd.Invocation) (string, error) {
		return manager.handleHelpCommand(channelID)
	}
	c.handlers["/wizard"] = func(_ context.Context, channelID string, _ sessioncmd.Invocation) (string, error) {
		return manager.handleWizardCommand(channelID)
	}
}

func (m *Manager) tryHandleCommand(ctx context.Context, channelID, input string) (bool, string, error) {
	if m.commands == nil {
		m.commands = newCommandInterpreter(m)
	}
	return m.commands.TryHandle(ctx, channelID, input)
}

func (c *commandInterpreter) TryHandle(ctx context.Context, channelID, input string) (bool, string, error) {
	invocation, ok := sessioncmd.Parse(input)
	if !ok {
		return false, "", nil
	}
	handler, ok := c.handlers[invocation.Command]
	if !ok {
		return false, "", nil
	}
	response, err := handler(ctx, channelID, invocation)
	return true, response, err
}
