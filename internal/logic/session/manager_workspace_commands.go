package session

import (
	"context"
	"fmt"
	"strings"

	"github.com/jose/matrix-v2/internal/logic/workspace"
	"github.com/jose/matrix-v2/internal/middleware"
)

func (m *Manager) handleWorkspaceCommand(ctx context.Context, channelID, input string) (string, error) {
	lang := m.wizard.GetLanguage(channelID)
	parts := strings.Fields(input)
	if len(parts) < 2 {
		return m.wizard.GetString(lang, "usage_workspace"), nil
	}

	command := parts[1]
	args := strings.TrimSpace(strings.TrimPrefix(input, parts[0]+" "+parts[1]))

	switch command {
	case "list":
		return m.handleWorkspaceList(channelID, lang)
	case "snapshot":
		return m.handleWorkspaceSnapshot(channelID, lang, args)
	case "status":
		return m.handleWorkspaceStatus(channelID, lang)
	case "switch":
		return m.handleWorkspaceSwitch(ctx, channelID, lang, args)
	case "bind":
		return m.handleWorkspaceBind(channelID, lang, args)
	default:
		return m.wizard.GetString(lang, "workspace_command_unknown"), nil
	}
}

func (m *Manager) handleWorkspaceList(channelID, lang string) (string, error) {
	result, err := m.handleWorkspaceListTyped(channelID, lang)
	if err != nil {
		return "", err
	}
	return result.Message, nil
}

func (m *Manager) handleWorkspaceStatus(channelID, lang string) (string, error) {
	result, err := m.handleWorkspaceStatusTyped(channelID, lang)
	if err != nil {
		return "", err
	}
	return result.Message, nil
}

func (m *Manager) handleWorkspaceSwitch(ctx context.Context, channelID, lang, args string) (string, error) {
	result, err := m.handleWorkspaceSwitchTyped(ctx, channelID, lang, args)
	if err != nil {
		return "", err
	}
	return result.Message, nil
}

func (m *Manager) handleWorkspaceBind(channelID, lang, args string) (string, error) {
	result, err := m.handleWorkspaceBindTyped(channelID, lang, args)
	if err != nil {
		return "", err
	}
	return result.Message, nil
}

func (m *Manager) handleWorkspaceSnapshot(channelID, lang, args string) (string, error) {
	result, err := m.handleWorkspaceSnapshotTyped(channelID, lang, args)
	if err != nil {
		return "", err
	}
	return result.Message, nil
}

func (m *Manager) handleWorkspaceListTyped(channelID, lang string) (middleware.WorkspaceActionResult, error) {
	metas, err := workspace.ListMeta(m.storage)
	if err != nil {
		return middleware.WorkspaceActionResult{}, err
	}
	if len(metas) == 0 {
		return middleware.WorkspaceActionResult{Action: "list", Message: m.wizard.GetString(lang, "workspace_list_empty")}, nil
	}
	state, _ := m.getChannelState(channelID)
	entries := make([]middleware.WorkspaceEntry, 0, len(metas))
	var sb strings.Builder
	sb.WriteString(m.wizard.GetString(lang, "workspace_list_header") + "\n")
	for i, meta := range metas {
		entry := m.toWorkspaceEntry(meta, meta.ID == state.PreferredWorkspaceID || meta.ID == state.LastWorkspaceID)
		entries = append(entries, entry)
		active := ""
		if entry.Active {
			active = " *"
		}
		line := fmt.Sprintf("[%d] %s%s", i+1, entry.ID, active)
		if entry.RootPath != "" {
			line += " - " + entry.RootPath
		}
		if entry.DefaultAgentID != "" {
			line += fmt.Sprintf(" (default: %s)", entry.DefaultAgentID)
		}
		sb.WriteString(line + "\n")
	}
	return middleware.WorkspaceActionResult{
		Action:     "list",
		Message:    strings.TrimSpace(sb.String()),
		Workspaces: entries,
	}, nil
}

func (m *Manager) handleWorkspaceStatusTyped(channelID, lang string) (middleware.WorkspaceActionResult, error) {
	state, _ := m.getChannelState(channelID)
	if strings.TrimSpace(state.ActiveSessionID) == "" && strings.TrimSpace(state.PreferredWorkspaceID) == "" {
		return middleware.WorkspaceActionResult{Action: "status", Message: m.wizard.GetString(lang, "workspace_status_empty")}, nil
	}
	sessionMeta, _, _ := m.loadSessionMeta(state.ActiveSessionID)
	workspaceID := sessionMeta.WorkspaceID
	if workspaceID == "" {
		workspaceID = state.PreferredWorkspaceID
	}
	if workspaceID == "" {
		return middleware.WorkspaceActionResult{Action: "status", Message: m.wizard.GetString(lang, "workspace_status_empty")}, nil
	}
	ws, _, err := workspace.LoadMeta(m.storage, workspaceID)
	if err != nil {
		return middleware.WorkspaceActionResult{}, err
	}
	path := ws.RootPath
	if path == "" {
		path = sessionMeta.WorkspacePath
	}
	agent := ws.DefaultAgentID
	if agent == "" {
		agent = sessionMeta.AgentID
	}
	return middleware.WorkspaceActionResult{
		Action:    "status",
		Message:   fmt.Sprintf(m.wizard.GetString(lang, "workspace_status"), workspaceID, path, agent),
		Workspace: workspaceEntryPtr(m.toWorkspaceEntry(ws, true)),
		Session:   m.toSessionEntry(sessionMeta, true),
	}, nil
}

func (m *Manager) handleWorkspaceSwitchTyped(ctx context.Context, channelID, lang, args string) (middleware.WorkspaceActionResult, error) {
	workspaceID := strings.TrimSpace(args)
	if workspaceID == "" {
		return middleware.WorkspaceActionResult{Action: "switch", Message: m.wizard.GetString(lang, "workspace_switch_usage")}, nil
	}
	ws, found, err := workspace.LoadMeta(m.storage, workspaceID)
	if err != nil {
		return middleware.WorkspaceActionResult{}, err
	}
	if !found {
		return middleware.WorkspaceActionResult{}, fmt.Errorf("workspace %s not found", workspaceID)
	}
	sessionID, decision, err := m.getOrCreateSessionForWorkspace(channelID, "", ws.ID, ws.RootPath)
	if err != nil {
		return middleware.WorkspaceActionResult{}, err
	}
	meta, _, _ := m.loadSessionMeta(sessionID)
	m.recordWorkspaceDecision(meta, channelID, decision)
	m.recordWorkspaceEvent(meta, "workspace.switched", channelID, "Switched workspace context", "workspace-switch", nil)
	return middleware.WorkspaceActionResult{
		Action:    "switch",
		Message:   fmt.Sprintf(m.wizard.GetString(lang, "workspace_switched"), ws.ID, meta.ID),
		Workspace: workspaceEntryPtr(m.toWorkspaceEntry(ws, true)),
		Session:   m.toSessionEntry(meta, true),
	}, nil
}

func (m *Manager) handleWorkspaceBindTyped(channelID, lang, args string) (middleware.WorkspaceActionResult, error) {
	workspaceID := strings.TrimSpace(args)
	if workspaceID == "" {
		return middleware.WorkspaceActionResult{Action: "bind", Message: m.wizard.GetString(lang, "workspace_bind_usage")}, nil
	}
	ws, found, err := workspace.LoadMeta(m.storage, workspaceID)
	if err != nil {
		return middleware.WorkspaceActionResult{}, err
	}
	if !found {
		return middleware.WorkspaceActionResult{}, fmt.Errorf("workspace %s not found", workspaceID)
	}
	state, err := m.getChannelState(channelID)
	if err != nil {
		return middleware.WorkspaceActionResult{}, err
	}
	meta, found, err := m.loadSessionMeta(state.ActiveSessionID)
	if err != nil {
		return middleware.WorkspaceActionResult{}, err
	}
	if !found {
		return middleware.WorkspaceActionResult{Action: "bind", Message: m.wizard.GetString(lang, "session_history_empty")}, nil
	}
	if err := m.bindSessionWorkspace(&meta, ws.ID, ws.RootPath); err != nil {
		return middleware.WorkspaceActionResult{}, err
	}
	if err := m.saveSessionMeta(meta); err != nil {
		return middleware.WorkspaceActionResult{}, err
	}
	if err := m.indexSessionWorkspace(meta); err != nil {
		return middleware.WorkspaceActionResult{}, err
	}
	if err := m.updateChannelWorkspaceState(channelID, ws.ID); err != nil {
		return middleware.WorkspaceActionResult{}, err
	}
	m.recordWorkspaceEvent(meta, "workspace.bound", channelID, "Bound session to workspace", "workspace-bind", nil)
	return middleware.WorkspaceActionResult{
		Action:    "bind",
		Message:   fmt.Sprintf(m.wizard.GetString(lang, "workspace_bound"), meta.ID, ws.ID),
		Workspace: workspaceEntryPtr(m.toWorkspaceEntry(ws, true)),
		Session:   m.toSessionEntry(meta, true),
	}, nil
}

func (m *Manager) handleWorkspaceSnapshotTyped(channelID, _lang, note string) (middleware.WorkspaceActionResult, error) {
	ws, meta, err := m.resolveWorkspaceReadContext(channelID, "")
	if err != nil {
		return middleware.WorkspaceActionResult{}, err
	}
	state, found, err := workspace.LoadState(m.storage, ws.ID)
	if err != nil {
		return middleware.WorkspaceActionResult{}, err
	}
	if !found {
		state = workspace.State{WorkspaceID: ws.ID}
	}
	turns, err := workspace.LoadTurns(m.storage, ws.ID, 8)
	if err != nil {
		return middleware.WorkspaceActionResult{}, err
	}
	events, err := workspace.LoadTimeline(m.storage, ws.ID, 8)
	if err != nil {
		return middleware.WorkspaceActionResult{}, err
	}
	turnIDs := make([]string, 0, len(turns))
	for _, turn := range turns {
		turnIDs = append(turnIDs, turn.ID)
	}
	eventIDs := make([]string, 0, len(events))
	for _, event := range events {
		eventIDs = append(eventIDs, event.ID)
	}
	snapshot, err := workspace.SaveSnapshot(m.storage, workspace.Snapshot{
		WorkspaceID:            ws.ID,
		Title:                  deriveSnapshotTitle(state, meta),
		Note:                   strings.TrimSpace(note),
		ActiveLogicalSessionID: state.ActiveLogicalSessionID,
		ActiveRemoteSessionID:  state.ActiveRemoteSessionID,
		ActiveAgentID:          state.ActiveAgentID,
		ActiveMode:             state.ActiveMode,
		RemoteStatus:           state.RemoteStatus,
		LastEventType:          state.LastEventType,
		LastEventAt:            state.LastEventAt,
		LastHandoff:            state.LastHandoff,
		LastDecision:           state.LastDecision,
		TurnIDs:                turnIDs,
		EventIDs:               eventIDs,
	})
	if err != nil {
		return middleware.WorkspaceActionResult{}, err
	}
	m.recordWorkspaceEvent(meta, "snapshot.created", channelID, "Created workspace snapshot", "snapshot-create", map[string]interface{}{"snapshot_id": snapshot.ID})
	message := "Created snapshot " + shortOrDash(snapshot.ID, 8)
	if snapshot.Title != "" {
		message += " - " + snapshot.Title
	}
	return middleware.WorkspaceActionResult{
		Action:    "snapshot",
		Message:   message,
		Workspace: workspaceEntryPtr(m.toWorkspaceEntry(ws, true)),
		Session:   m.toSessionEntry(meta, meta.ID != ""),
	}, nil
}

func deriveSnapshotTitle(state workspace.State, meta SessionMeta) string {
	if strings.TrimSpace(meta.RemoteTitle) != "" {
		return meta.RemoteTitle
	}
	if strings.TrimSpace(state.ActiveAgentID) != "" || strings.TrimSpace(state.ActiveMode) != "" {
		return strings.TrimSpace(state.ActiveAgentID + " " + state.ActiveMode)
	}
	return "workspace snapshot"
}

// SwitchWorkspaceForChannel is used by operator-facing CLI surfaces.
func (m *Manager) SwitchWorkspaceForChannel(_ context.Context, channelID, workspaceID string) (SessionMeta, bool, error) {
	ws, found, err := workspace.LoadMeta(m.storage, workspaceID)
	if err != nil {
		return SessionMeta{}, false, err
	}
	if !found {
		return SessionMeta{}, false, nil
	}
	sessionID, decision, err := m.getOrCreateSessionForWorkspace(channelID, "", ws.ID, ws.RootPath)
	if err != nil {
		return SessionMeta{}, false, err
	}
	meta, found, err := m.loadSessionMeta(sessionID)
	if err == nil && found {
		m.recordWorkspaceDecision(meta, channelID, decision)
	}
	return meta, found, err
}

func (m *Manager) toWorkspaceEntry(meta workspace.Meta, active bool) middleware.WorkspaceEntry {
	return middleware.WorkspaceEntry{
		ID:              meta.ID,
		Name:            meta.Name,
		Kind:            meta.Kind,
		RootPath:        meta.RootPath,
		DefaultAgentID:  meta.DefaultAgentID,
		ReviewerAgentID: meta.ReviewerAgentID,
		DefaultMode:     normalizeMode(meta.DefaultMode),
		PolicyProfile:   meta.PolicyProfile,
		Active:          active,
	}
}

func workspaceEntryPtr(entry middleware.WorkspaceEntry) *middleware.WorkspaceEntry {
	return &entry
}
