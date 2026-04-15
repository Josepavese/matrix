package session

import (
	"context"
	"fmt"
	"strings"

	"github.com/jose/matrix-v2/internal/logic/workspace"
	"github.com/jose/matrix-v2/internal/middleware"
)

func (m *Manager) handleContinueIntentTyped(channelID, lang string) (middleware.IntentActionResult, error) {
	state, _ := m.getChannelState(channelID)
	if strings.TrimSpace(state.ActiveSessionID) == "" {
		return middleware.IntentActionResult{Intent: "continue", Message: m.wizard.GetString(lang, "status_empty")}, nil
	}
	meta, found, err := m.loadSessionMeta(state.ActiveSessionID)
	if err != nil {
		return middleware.IntentActionResult{}, err
	}
	if !found {
		return middleware.IntentActionResult{Intent: "continue", Message: m.wizard.GetString(lang, "status_empty")}, nil
	}
	meta.Mode = modeImplementation
	if err := m.saveSessionMeta(meta); err != nil {
		return middleware.IntentActionResult{}, err
	}
	m.recordWorkspaceEvent(meta, "mode.changed", channelID, "Changed mode to implementation", "mode-change", nil)
	m.recordWorkspaceEvent(meta, "intent.continue", channelID, "Continue current work context", "intent-continue", nil)
	wsEntry, _ := m.currentWorkspaceEntry(state, meta)
	return middleware.IntentActionResult{
		Intent:    "continue",
		Message:   fmt.Sprintf(m.wizard.GetString(lang, "intent_continue"), valueOrDash(meta.WorkspaceID), meta.AgentID),
		Workspace: wsEntry,
		Session:   m.toSessionEntry(meta, true),
	}, nil
}

func (m *Manager) handleResumeIntentTyped(ctx context.Context, channelID, lang, target string) (middleware.IntentActionResult, error) {
	target = strings.TrimSpace(target)
	if target != "" {
		wsResult, err := m.handleWorkspaceSwitchTyped(ctx, channelID, lang, target)
		if err != nil {
			return middleware.IntentActionResult{}, err
		}
		if wsResult.Session != nil {
			if meta, found, err := m.loadSessionMeta(wsResult.Session.LogicalSessionID); err == nil && found {
				m.recordWorkspaceEvent(meta, "intent.resume", channelID, "Resume selected workspace context", "intent-resume", nil)
			}
		}
		return middleware.IntentActionResult{
			Intent:    "resume",
			Message:   fmt.Sprintf(m.wizard.GetString(lang, "intent_resume"), target, sessionLabel(wsResult.Session)),
			Workspace: wsResult.Workspace,
			Session:   wsResult.Session,
		}, nil
	}
	state, _ := m.getChannelState(channelID)
	if strings.TrimSpace(state.PreferredWorkspaceID) != "" {
		return m.handleResumeIntentTyped(ctx, channelID, lang, state.PreferredWorkspaceID)
	}
	if strings.TrimSpace(state.ActiveSessionID) != "" {
		meta, found, err := m.loadSessionMeta(state.ActiveSessionID)
		if err != nil {
			return middleware.IntentActionResult{}, err
		}
		if found {
			wsEntry, _ := m.currentWorkspaceEntry(state, meta)
			m.recordWorkspaceEvent(meta, "session.resumed", channelID, "Resumed current session", "intent-resume", nil)
			m.recordWorkspaceEvent(meta, "intent.resume", channelID, "Resume current work context", "intent-resume", nil)
			return middleware.IntentActionResult{
				Intent:    "resume",
				Message:   fmt.Sprintf(m.wizard.GetString(lang, "intent_resume_current"), sessionLabel(m.toSessionEntry(meta, true))),
				Workspace: wsEntry,
				Session:   m.toSessionEntry(meta, true),
			}, nil
		}
	}
	return middleware.IntentActionResult{Intent: "resume", Message: m.wizard.GetString(lang, "status_empty")}, nil
}

func (m *Manager) handleReviewIntentTyped(ctx context.Context, channelID, lang, target string) (middleware.IntentActionResult, error) {
	wsMeta, err := m.resolveIntentWorkspace(channelID, target)
	if err != nil {
		return middleware.IntentActionResult{}, err
	}
	reviewer := strings.TrimSpace(wsMeta.ReviewerAgentID)
	if reviewer == "" {
		reviewer = strings.TrimSpace(m.actionAgent)
	}
	if reviewer == "" {
		reviewer = strings.TrimSpace(wsMeta.DefaultAgentID)
	}
	if reviewer == "" {
		return middleware.IntentActionResult{}, fmt.Errorf("no reviewer agent configured for workspace %s", wsMeta.ID)
	}
	sourceMeta, _ := m.currentSessionForWorkspace(channelID, wsMeta.ID)
	sessionID, decision, err := m.getOrCreateSessionForWorkspace(channelID, reviewer, wsMeta.ID, wsMeta.RootPath)
	if err != nil {
		return middleware.IntentActionResult{}, err
	}
	meta, found, err := m.loadSessionMeta(sessionID)
	if err != nil {
		return middleware.IntentActionResult{}, err
	}
	if !found {
		return middleware.IntentActionResult{}, fmt.Errorf("review session %s not found", sessionID)
	}
	meta.Mode = modeReview
	m.recordWorkspaceDecision(meta, channelID, decision)
	packet := m.buildHandoffPacket(sourceMeta, meta, wsMeta, "Switching to review specialist.")
	if packet != nil && (sourceMeta.ID == "" || sourceMeta.ID != meta.ID) {
		meta.PendingHandoff = packet
	}
	meta.LastHandoff = packet
	if err := m.saveSessionMeta(meta); err != nil {
		return middleware.IntentActionResult{}, err
	}
	m.recordWorkspaceEvent(meta, "mode.changed", channelID, "Changed mode to review", "mode-change", nil)
	m.recordWorkspaceEvent(meta, "intent.review", channelID, "Entered review mode", "intent-review", nil)
	if packet != nil {
		m.recordWorkspaceEvent(meta, "handoff.created", channelID, "Created specialist handoff", "specialist-handoff", handoffMetadata(meta))
	}
	return middleware.IntentActionResult{
		Intent:    "review",
		Message:   fmt.Sprintf(m.wizard.GetString(lang, "intent_review"), wsMeta.ID, reviewer, sessionLabel(m.toSessionEntry(meta, true))),
		Workspace: workspaceEntryPtr(m.toWorkspaceEntry(wsMeta, true)),
		Session:   m.toSessionEntry(meta, true),
		Handoff:   packet,
	}, nil
}

func (m *Manager) resolveIntentWorkspace(channelID, target string) (workspace.Meta, error) {
	target = strings.TrimSpace(target)
	if target != "" {
		ws, found, err := workspace.LoadMeta(m.storage, target)
		if err != nil {
			return workspace.Meta{}, err
		}
		if !found {
			return workspace.Meta{}, fmt.Errorf("workspace %s not found", target)
		}
		return ws, nil
	}
	state, _ := m.getChannelState(channelID)
	if strings.TrimSpace(state.PreferredWorkspaceID) != "" {
		ws, found, err := workspace.LoadMeta(m.storage, state.PreferredWorkspaceID)
		if err != nil {
			return workspace.Meta{}, err
		}
		if found {
			return ws, nil
		}
	}
	if strings.TrimSpace(state.ActiveSessionID) != "" {
		meta, found, err := m.loadSessionMeta(state.ActiveSessionID)
		if err != nil {
			return workspace.Meta{}, err
		}
		if found && strings.TrimSpace(meta.WorkspaceID) != "" {
			ws, found, err := workspace.LoadMeta(m.storage, meta.WorkspaceID)
			if err != nil {
				return workspace.Meta{}, err
			}
			if found {
				return ws, nil
			}
		}
	}
	return workspace.Meta{}, fmt.Errorf("no workspace context available")
}

func (m *Manager) currentWorkspaceEntry(state ChannelState, meta SessionMeta) (*middleware.WorkspaceEntry, error) {
	workspaceID := meta.WorkspaceID
	if workspaceID == "" {
		workspaceID = state.PreferredWorkspaceID
	}
	if workspaceID == "" {
		return nil, nil
	}
	ws, found, err := workspace.LoadMeta(m.storage, workspaceID)
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, nil
	}
	return workspaceEntryPtr(m.toWorkspaceEntry(ws, true)), nil
}

func (m *Manager) handleModeAction(ctx context.Context, channelID, mode, target string) (string, error) {
	result, err := m.handleModeActionTyped(ctx, channelID, m.wizard.GetLanguage(channelID), mode, target)
	if err != nil {
		return "", err
	}
	return result.Message, nil
}

func (m *Manager) handleModeActionTyped(ctx context.Context, channelID, lang, mode, target string) (middleware.IntentActionResult, error) {
	switch normalizeMode(mode) {
	case modeReview:
		return m.handleReviewIntentTyped(ctx, channelID, lang, target)
	case modeExplain:
		return m.handleExplainModeTyped(ctx, channelID, lang, target)
	case modeTriage:
		return m.handleTriageModeTyped(ctx, channelID, lang, target)
	default:
		return m.handleResumeIntentTyped(ctx, channelID, lang, target)
	}
}

func (m *Manager) handleExplainModeTyped(ctx context.Context, channelID, lang, target string) (middleware.IntentActionResult, error) {
	state, _ := m.getChannelState(channelID)
	if strings.TrimSpace(target) != "" {
		if _, err := m.handleResumeIntentTyped(ctx, channelID, lang, target); err != nil {
			return middleware.IntentActionResult{}, err
		}
		state, _ = m.getChannelState(channelID)
	}
	if strings.TrimSpace(state.ActiveSessionID) == "" {
		return middleware.IntentActionResult{Intent: "explain", Message: m.wizard.GetString(lang, "status_empty")}, nil
	}
	meta, found, err := m.loadSessionMeta(state.ActiveSessionID)
	if err != nil {
		return middleware.IntentActionResult{}, err
	}
	if !found {
		return middleware.IntentActionResult{Intent: "explain", Message: m.wizard.GetString(lang, "status_empty")}, nil
	}
	meta.Mode = modeExplain
	if err := m.saveSessionMeta(meta); err != nil {
		return middleware.IntentActionResult{}, err
	}
	m.recordWorkspaceEvent(meta, "mode.changed", channelID, "Changed mode to explain", "mode-change", nil)
	m.recordWorkspaceEvent(meta, "intent.explain", channelID, "Entered explain mode", "intent-explain", nil)
	wsEntry, _ := m.currentWorkspaceEntry(state, meta)
	return middleware.IntentActionResult{
		Intent:    "explain",
		Message:   fmt.Sprintf(m.wizard.GetString(lang, "intent_explain"), valueOrDash(meta.WorkspaceID), meta.AgentID),
		Workspace: wsEntry,
		Session:   m.toSessionEntry(meta, true),
	}, nil
}

func (m *Manager) handleTriageModeTyped(ctx context.Context, channelID, lang, target string) (middleware.IntentActionResult, error) {
	state, _ := m.getChannelState(channelID)
	if strings.TrimSpace(target) != "" {
		if _, err := m.handleResumeIntentTyped(ctx, channelID, lang, target); err != nil {
			return middleware.IntentActionResult{}, err
		}
		state, _ = m.getChannelState(channelID)
	}
	if strings.TrimSpace(state.ActiveSessionID) == "" {
		return middleware.IntentActionResult{Intent: "triage", Message: m.wizard.GetString(lang, "status_empty")}, nil
	}
	meta, found, err := m.loadSessionMeta(state.ActiveSessionID)
	if err != nil {
		return middleware.IntentActionResult{}, err
	}
	if !found {
		return middleware.IntentActionResult{Intent: "triage", Message: m.wizard.GetString(lang, "status_empty")}, nil
	}
	meta.Mode = modeTriage
	if err := m.saveSessionMeta(meta); err != nil {
		return middleware.IntentActionResult{}, err
	}
	m.recordWorkspaceEvent(meta, "mode.changed", channelID, "Changed mode to triage", "mode-change", nil)
	m.recordWorkspaceEvent(meta, "intent.triage", channelID, "Entered triage mode", "intent-triage", nil)
	wsEntry, _ := m.currentWorkspaceEntry(state, meta)
	return middleware.IntentActionResult{
		Intent:    "triage",
		Message:   fmt.Sprintf(m.wizard.GetString(lang, "intent_triage"), valueOrDash(meta.WorkspaceID), meta.AgentID),
		Workspace: wsEntry,
		Session:   m.toSessionEntry(meta, true),
	}, nil
}

func sessionLabel(entry *middleware.SessionEntry) string {
	if entry == nil {
		return "-"
	}
	if strings.TrimSpace(entry.LogicalSessionID) == "" {
		return "-"
	}
	if len(entry.LogicalSessionID) <= 8 {
		return entry.LogicalSessionID
	}
	return entry.LogicalSessionID[:8]
}

func valueOrDash(v string) string {
	if strings.TrimSpace(v) == "" {
		return "-"
	}
	return v
}
