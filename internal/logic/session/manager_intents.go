package session

import (
	"context"
	"fmt"
	"strings"

	"github.com/Josepavese/matrix/internal/logic/workspace"
	"github.com/Josepavese/matrix/internal/middleware"
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

func (m *Manager) handleReviewIntentTyped(_ context.Context, channelID, lang, target string) (middleware.IntentActionResult, error) {
	wsMeta, err := m.resolveIntentWorkspace(channelID, target)
	if err != nil {
		return middleware.IntentActionResult{}, err
	}
	reviewer := firstNonEmpty(wsMeta.ReviewerAgentID, m.actionAgent, wsMeta.DefaultAgentID)
	if reviewer == "" {
		return middleware.IntentActionResult{}, fmt.Errorf("no reviewer agent configured for workspace %s", wsMeta.ID)
	}
	sourceMeta, _ := m.currentSessionForWorkspace(channelID, wsMeta.ID)
	sessionID, decision, err := m.getOrCreateSessionForWorkspace(channelID, reviewer, wsMeta.ID, wsMeta.RootPath)
	if err != nil {
		return middleware.IntentActionResult{}, err
	}
	meta, err := m.loadRequiredSessionMeta(sessionID, "review session")
	if err != nil {
		return middleware.IntentActionResult{}, err
	}
	meta.Mode = modeReview
	m.recordWorkspaceDecision(meta, channelID, decision)
	packet := applyHandoffPacket(&meta, sourceMeta, m.buildHandoffPacket(sourceMeta, meta, wsMeta, "Switching to review specialist."))
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
		return m.loadRequiredWorkspace(target)
	}
	state, _ := m.getChannelState(channelID)
	if ws, found, err := m.workspaceFromPreferredState(state); err != nil || found {
		return ws, err
	}
	if ws, _, found, err := m.workspaceFromActiveSessionState(state); err != nil || found {
		return ws, err
	}
	return workspace.Meta{}, fmt.Errorf("no workspace context available")
}

func (m *Manager) loadRequiredSessionMeta(sessionID, label string) (SessionMeta, error) {
	meta, found, err := m.loadSessionMeta(sessionID)
	if err != nil {
		return SessionMeta{}, err
	}
	if !found {
		return SessionMeta{}, fmt.Errorf("%s %s not found", label, sessionID)
	}
	return meta, nil
}

func (m *Manager) loadRequiredWorkspace(workspaceID string) (workspace.Meta, error) {
	ws, found, err := workspace.LoadMeta(m.storage, workspaceID)
	if err != nil {
		return workspace.Meta{}, err
	}
	if !found {
		return workspace.Meta{}, fmt.Errorf("workspace %s not found", workspaceID)
	}
	return ws, nil
}

func (m *Manager) workspaceFromPreferredState(state ChannelState) (workspace.Meta, bool, error) {
	workspaceID := strings.TrimSpace(state.PreferredWorkspaceID)
	if workspaceID == "" {
		return workspace.Meta{}, false, nil
	}
	ws, found, err := workspace.LoadMeta(m.storage, workspaceID)
	return ws, found, err
}

func (m *Manager) workspaceFromActiveSessionState(state ChannelState) (workspace.Meta, SessionMeta, bool, error) {
	sessionID := strings.TrimSpace(state.ActiveSessionID)
	if sessionID == "" {
		return workspace.Meta{}, SessionMeta{}, false, nil
	}
	meta, found, err := m.loadSessionMeta(sessionID)
	if err != nil || !found || strings.TrimSpace(meta.WorkspaceID) == "" {
		return workspace.Meta{}, meta, false, err
	}
	ws, found, err := workspace.LoadMeta(m.storage, meta.WorkspaceID)
	return ws, meta, found, err
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
	return m.handleSimpleModeIntent(simpleModeIntentRequest{Ctx: ctx, ChannelID: channelID, Lang: lang, Target: target, Spec: simpleModeIntentSpec{
		Intent:       "explain",
		Mode:         modeExplain,
		WizardKey:    "intent_explain",
		ModeMessage:  "Changed mode to explain",
		IntentEvent:  "intent.explain",
		IntentReason: "intent-explain",
		IntentDetail: "Entered explain mode",
	}})
}

func (m *Manager) handleTriageModeTyped(ctx context.Context, channelID, lang, target string) (middleware.IntentActionResult, error) {
	return m.handleSimpleModeIntent(simpleModeIntentRequest{Ctx: ctx, ChannelID: channelID, Lang: lang, Target: target, Spec: simpleModeIntentSpec{
		Intent:       "triage",
		Mode:         modeTriage,
		WizardKey:    "intent_triage",
		ModeMessage:  "Changed mode to triage",
		IntentEvent:  "intent.triage",
		IntentReason: "intent-triage",
		IntentDetail: "Entered triage mode",
	}})
}

type simpleModeIntentRequest struct {
	Ctx       context.Context
	ChannelID string
	Lang      string
	Target    string
	Spec      simpleModeIntentSpec
}

type simpleModeIntentSpec struct {
	Intent       string
	Mode         string
	WizardKey    string
	ModeMessage  string
	IntentEvent  string
	IntentReason string
	IntentDetail string
}

func (m *Manager) handleSimpleModeIntent(req simpleModeIntentRequest) (middleware.IntentActionResult, error) {
	state, _ := m.getChannelState(req.ChannelID)
	if strings.TrimSpace(req.Target) != "" {
		if _, err := m.handleResumeIntentTyped(req.Ctx, req.ChannelID, req.Lang, req.Target); err != nil {
			return middleware.IntentActionResult{}, err
		}
		state, _ = m.getChannelState(req.ChannelID)
	}
	if strings.TrimSpace(state.ActiveSessionID) == "" {
		return middleware.IntentActionResult{Intent: req.Spec.Intent, Message: m.wizard.GetString(req.Lang, "status_empty")}, nil
	}
	meta, found, err := m.loadSessionMeta(state.ActiveSessionID)
	if err != nil {
		return middleware.IntentActionResult{}, err
	}
	if !found {
		return middleware.IntentActionResult{Intent: req.Spec.Intent, Message: m.wizard.GetString(req.Lang, "status_empty")}, nil
	}
	meta.Mode = req.Spec.Mode
	if err := m.saveSessionMeta(meta); err != nil {
		return middleware.IntentActionResult{}, err
	}
	m.recordWorkspaceEvent(meta, "mode.changed", req.ChannelID, req.Spec.ModeMessage, "mode-change", nil)
	m.recordWorkspaceEvent(meta, req.Spec.IntentEvent, req.ChannelID, req.Spec.IntentDetail, req.Spec.IntentReason, nil)
	wsEntry, _ := m.currentWorkspaceEntry(state, meta)
	return middleware.IntentActionResult{
		Intent:    req.Spec.Intent,
		Message:   fmt.Sprintf(m.wizard.GetString(req.Lang, req.Spec.WizardKey), valueOrDash(meta.WorkspaceID), meta.AgentID),
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
