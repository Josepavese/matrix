package session

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/jose/matrix-v2/internal/logic/workspace"
	"github.com/jose/matrix-v2/internal/middleware"
)

func (m *Manager) handleHandoffIntentTyped(_ context.Context, channelID, lang, workspaceID, agentID, target, note string) (middleware.IntentActionResult, error) {
	toAgentID := strings.TrimSpace(agentID)
	if toAgentID == "" {
		toAgentID = strings.TrimSpace(target)
	}
	if toAgentID == "" {
		return middleware.IntentActionResult{}, fmt.Errorf("handoff requires a target agent")
	}

	wsMeta, err := m.resolveIntentWorkspaceWithHint(channelID, workspaceID)
	if err != nil {
		return middleware.IntentActionResult{}, err
	}

	sourceMeta, _ := m.currentSessionForWorkspace(channelID, wsMeta.ID)
	targetSessionID, _, err := m.getOrCreateSessionForWorkspace(channelID, toAgentID, wsMeta.ID, wsMeta.RootPath)
	if err != nil {
		return middleware.IntentActionResult{}, err
	}
	targetMeta, found, err := m.loadSessionMeta(targetSessionID)
	if err != nil {
		return middleware.IntentActionResult{}, err
	}
	if !found {
		return middleware.IntentActionResult{}, fmt.Errorf("handoff session %s not found", targetSessionID)
	}

	targetMeta.Mode = normalizeMode(targetMeta.Mode)
	packet := m.buildHandoffPacket(sourceMeta, targetMeta, wsMeta, note)
	if packet != nil && (sourceMeta.ID == "" || sourceMeta.ID != targetMeta.ID) {
		targetMeta.PendingHandoff = packet
	}
	targetMeta.LastHandoff = packet
	if err := m.saveSessionMeta(targetMeta); err != nil {
		return middleware.IntentActionResult{}, err
	}
	m.recordWorkspaceEvent(targetMeta, "intent.handoff", channelID, "Handed off work to another specialist", "intent-handoff", nil)
	if packet != nil {
		m.recordWorkspaceEvent(targetMeta, "handoff.created", channelID, "Created specialist handoff", "specialist-handoff", handoffMetadata(targetMeta))
	}

	return middleware.IntentActionResult{
		Intent:    "handoff",
		Message:   fmt.Sprintf(m.wizard.GetString(lang, "intent_handoff"), wsMeta.ID, targetMeta.AgentID, sessionLabel(m.toSessionEntry(targetMeta, true))),
		Workspace: workspaceEntryPtr(m.toWorkspaceEntry(wsMeta, true)),
		Session:   m.toSessionEntry(targetMeta, true),
		Handoff:   packet,
	}, nil
}

func (m *Manager) currentSessionForWorkspace(channelID, workspaceID string) (SessionMeta, bool) {
	state, err := m.getChannelState(channelID)
	if err != nil || strings.TrimSpace(state.ActiveSessionID) == "" {
		return SessionMeta{}, false
	}
	meta, found, err := m.loadSessionMeta(state.ActiveSessionID)
	if err != nil || !found {
		return SessionMeta{}, false
	}
	if workspaceID != "" && meta.WorkspaceID != workspaceID {
		return SessionMeta{}, false
	}
	return meta, true
}

func (m *Manager) resolveIntentWorkspaceWithHint(channelID, workspaceID string) (workspace.Meta, error) {
	if strings.TrimSpace(workspaceID) != "" {
		ws, found, err := workspace.LoadMeta(m.storage, workspaceID)
		if err != nil {
			return workspace.Meta{}, err
		}
		if !found {
			return workspace.Meta{}, fmt.Errorf("workspace %s not found", workspaceID)
		}
		return ws, nil
	}
	return m.resolveIntentWorkspace(channelID, "")
}

func (m *Manager) buildHandoffPacket(sourceMeta, targetMeta SessionMeta, wsMeta workspace.Meta, note string) *middleware.HandoffPacket {
	if sourceMeta.ID == "" {
		return &middleware.HandoffPacket{
			ToAgentID:   targetMeta.AgentID,
			WorkspaceID: wsMeta.ID,
			Mode:        normalizeMode(targetMeta.Mode),
			Reason:      "specialist-handoff",
			Summary:     fmt.Sprintf("Open or continue work in workspace %s as %s mode.", wsMeta.ID, normalizeMode(targetMeta.Mode)),
			CreatedAt:   time.Now().UTC().Format(time.RFC3339),
		}
	}

	mode := normalizeMode(sourceMeta.Mode)
	if mode == "" {
		mode = modeImplementation
	}
	summary := fmt.Sprintf(
		"Continue workspace %s. Previous specialist: %s. Previous mode: %s. Previous remote status: %s.",
		valueOrDash(sourceMeta.WorkspaceID),
		valueOrDash(sourceMeta.AgentID),
		valueOrDash(mode),
		valueOrDash(sourceMeta.RemoteStatus),
	)
	if sourceMeta.RemoteTitle != "" {
		summary += fmt.Sprintf(" Current thread title: %s.", sourceMeta.RemoteTitle)
	}
	if trimmed := strings.TrimSpace(note); trimmed != "" {
		summary += " Operator note: " + trimmed
	}
	return &middleware.HandoffPacket{
		FromLogicalSessionID: sourceMeta.ID,
		FromRemoteSessionID:  sourceMeta.AgentSessionID,
		FromAgentID:          sourceMeta.AgentID,
		ToAgentID:            targetMeta.AgentID,
		WorkspaceID:          wsMeta.ID,
		Mode:                 normalizeMode(targetMeta.Mode),
		Reason:               "specialist-handoff",
		Summary:              summary,
		CreatedAt:            time.Now().UTC().Format(time.RFC3339),
	}
}

func renderHandoffPrompt(packet *middleware.HandoffPacket) string {
	if packet == nil {
		return ""
	}
	var b strings.Builder
	b.WriteString("[Matrix handoff context]\n")
	if packet.WorkspaceID != "" {
		b.WriteString("Workspace: " + packet.WorkspaceID + "\n")
	}
	if packet.FromAgentID != "" {
		b.WriteString("Previous specialist: " + packet.FromAgentID + "\n")
	}
	if packet.ToAgentID != "" {
		b.WriteString("Target specialist: " + packet.ToAgentID + "\n")
	}
	if packet.Mode != "" {
		b.WriteString("Requested mode: " + packet.Mode + "\n")
	}
	if packet.Summary != "" {
		b.WriteString("Summary: " + packet.Summary + "\n")
	}
	b.WriteString("[End handoff context]")
	return b.String()
}
