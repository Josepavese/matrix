package a2aclient

import "github.com/Josepavese/matrix/internal/middleware"

func a2aTurnWithoutRemoteSession(turn middleware.ConversationTurn) middleware.ConversationTurn {
	return middleware.ConversationTurn{
		AgentID:               turn.AgentID,
		LogicalSessionID:      turn.LogicalSessionID,
		WorkspacePath:         turn.WorkspacePath,
		Message:               turn.Message,
		ContentBlocks:         turn.ContentBlocks,
		SidecarCapsules:       turn.SidecarCapsules,
		Tools:                 turn.Tools,
		McpServers:            turn.McpServers,
		AdditionalDirectories: turn.AdditionalDirectories,
		ThoughtNotifier:       turn.ThoughtNotifier,
		LiveContextAttach:     turn.LiveContextAttach,
	}
}
